package modelhost

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"quantram/internal/adaptive"
	"quantram/internal/config"
	"quantram/internal/domain"
	"quantram/internal/ingestion"
)

const (
	modelSubscribeBuffer = config.WindowLimit
	workerInboxSize      = config.WindowLimit
	eventBuffer          = 128
	pathPollInterval     = 50 * time.Millisecond
)

var ErrUnavailable = errors.New("adaptive model unavailable")

// Unavailable is a Health-only stand-in when adaptive was requested but could not start.
// Do not call Run on it; ingestion stays up and GetHealth reports unavailable (not off).
type Unavailable struct{}

func (Unavailable) Health() domain.ComponentHealth {
	return domain.ComponentHealth{Name: "model", State: domain.ComponentUnavailable, Detail: "unavailable"}
}

type SymbolStatus string

const (
	StatusOff           SymbolStatus = "off"
	StatusCold          SymbolStatus = "cold"
	StatusInitializing  SymbolStatus = "initializing"
	StatusReady         SymbolStatus = "ready"
	StatusPaused        SymbolStatus = "paused"
	StatusDiscontinuous SymbolStatus = "discontinuous"
	StatusError         SymbolStatus = "error"
)

type BarSource interface {
	SubscribeModelBars(buffer int) (uint64, <-chan domain.Bar)
	Unsubscribe(id uint64)
	Readiness() domain.Readiness
	ReadinessFor(symbol string) domain.Readiness
	ModelPathStatus(symbol string) ingestion.ModelPathStatus
}

type Options struct {
	Mode      config.ModelMode
	Deadline  time.Duration
	Delay     time.Duration
	PanicOn   string
	InboxSize int
}

type Host struct {
	src     BarSource
	symbols []string
	opts    Options
	workers map[string]*worker
	seq     atomic.Uint64
	enabled atomic.Bool
	unavail atomic.Bool
	started atomic.Bool
	nextSub atomic.Uint64

	mu         sync.Mutex
	closed     bool
	cancel     context.CancelFunc
	subs       map[uint64]chan domain.DecisionEvent
	lastBySym  map[string]domain.DecisionEvent
	eventsOnce sync.Once
	events     <-chan domain.DecisionEvent
}

type worker struct {
	symbol       string
	engine       *adaptive.Engine
	inbox        chan domain.Bar
	lastAccepted time.Time
	hasAccepted  bool
	disc         atomic.Bool
	discReason   atomic.Value
	lastSkip     atomic.Value
	lastEventAt  atomic.Value
	timeouts     atomic.Uint32
	errors       atomic.Uint32
	lastOutcome  atomic.Value
}

type SymbolHealth struct {
	Symbol    string
	Status    SymbolStatus
	Accepted  int
	Warmup    string
	LastSkip  domain.SkipReason
	LastEvent time.Time
}

func New(src BarSource, symbols []string, opts Options) (*Host, error) {
	if opts.Mode == "" {
		opts.Mode = config.ModelOff
	}
	if opts.Mode == config.ModelOff {
		return nil, nil
	}
	if opts.Mode != config.ModelAdaptive {
		return nil, fmt.Errorf("unknown model mode %q", opts.Mode)
	}
	if opts.Deadline <= 0 {
		opts.Deadline = config.DefaultModelDeadline
	}
	if opts.Deadline > config.MaxModelDeadline {
		return nil, fmt.Errorf("model deadline %s exceeds max %s", opts.Deadline, config.MaxModelDeadline)
	}
	if src == nil {
		return nil, fmt.Errorf("%w: bar source is nil", ErrUnavailable)
	}
	if adaptive.DefaultConfig().SHA256() != adaptive.DefaultConfigSHA256 ||
		adaptive.BaselineRuleFingerprint == "" ||
		adaptive.BaselineImplementationFingerprint == "" {
		return nil, fmt.Errorf("%w: baseline fingerprints/config hash invalid", ErrUnavailable)
	}

	inbox := opts.InboxSize
	if inbox <= 0 {
		inbox = workerInboxSize
	}
	h := &Host{
		src:       src,
		symbols:   append([]string(nil), symbols...),
		opts:      opts,
		workers:   make(map[string]*worker, len(symbols)),
		subs:      make(map[uint64]chan domain.DecisionEvent),
		lastBySym: make(map[string]domain.DecisionEvent, len(symbols)),
	}
	for _, symbol := range h.symbols {
		h.workers[symbol] = &worker{
			symbol: symbol,
			engine: adaptive.NewEngine(symbol),
			inbox:  make(chan domain.Bar, inbox),
		}
	}
	h.enabled.Store(true)
	return h, nil
}

func (h *Host) Started() bool {
	return h != nil && h.started.Load()
}

func (h *Host) Events() <-chan domain.DecisionEvent {
	if h == nil {
		return nil
	}
	h.eventsOnce.Do(func() {
		_, h.events = h.SubscribeEvents(eventBuffer)
	})
	return h.events
}

func (h *Host) SubscribeEvents(buffer int) (uint64, <-chan domain.DecisionEvent) {
	if h == nil {
		ch := make(chan domain.DecisionEvent)
		close(ch)
		return 0, ch
	}
	if buffer <= 0 {
		buffer = eventBuffer
	}
	ch := make(chan domain.DecisionEvent, buffer)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		close(ch)
		return 0, ch
	}
	id := h.nextSub.Add(1)
	h.subs[id] = ch
	return id, ch
}

func (h *Host) UnsubscribeEvents(id uint64) {
	if h == nil || id == 0 {
		return
	}
	h.mu.Lock()
	ch, ok := h.subs[id]
	if ok {
		delete(h.subs, id)
	}
	h.mu.Unlock()
	if ok {
		close(ch)
	}
}

func (h *Host) LastEvents() []domain.DecisionEvent {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]domain.DecisionEvent, 0, len(h.lastBySym))
	for _, ev := range h.lastBySym {
		out = append(out, ev)
	}
	return out
}

func (h *Host) Run(ctx context.Context) error {
	if h == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	h.mu.Lock()
	h.cancel = cancel
	h.mu.Unlock()
	defer cancel()

	id, bars := h.src.SubscribeModelBars(modelSubscribeBuffer)
	defer h.src.Unsubscribe(id)
	h.started.Store(true)

	var wg sync.WaitGroup
	for _, w := range h.workers {
		wg.Add(1)
		go func(w *worker) {
			defer wg.Done()
			h.runWorker(ctx, w)
		}(w)
	}

	ticker := time.NewTicker(pathPollInterval)
	defer ticker.Stop()

	defer func() {
		for _, w := range h.workers {
			close(w.inbox)
		}
		wg.Wait()
		h.mu.Lock()
		h.closed = true
		subs := h.subs
		h.subs = make(map[uint64]chan domain.DecisionEvent)
		h.mu.Unlock()
		for _, ch := range subs {
			close(ch)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			h.refreshPathStatus()
		case bar, ok := <-bars:
			if !ok {
				return nil
			}
			h.dispatch(bar)
			h.refreshPathStatus()
		}
	}
}

func (h *Host) runWorker(ctx context.Context, w *worker) {
	for {
		select {
		case <-ctx.Done():
			return
		case bar, ok := <-w.inbox:
			if !ok {
				return
			}
			h.handle(w, bar)
		}
	}
}

func (h *Host) dispatch(bar domain.Bar) {
	w := h.workers[bar.Symbol]
	if w == nil {
		return
	}
	if w.disc.Load() {
		h.emit(h.discontinuousSkip(w, bar, w.engine.StateHash()))
		return
	}
	select {
	case w.inbox <- bar:
	default:
		w.markDisc(domain.SkipQueueOverflow)
		h.emit(h.gateSkip(bar, domain.SkipQueueOverflow, w.engine.StateHash()))
	}
}

func (h *Host) refreshPathStatus() {
	for symbol, w := range h.workers {
		st := h.src.ModelPathStatus(symbol)
		if st.Discontinuous && w.markDisc(st.Reason) {
			bar := domain.Bar{Symbol: symbol, IntervalStart: st.LastInterval, IsFinal: true}
			h.emit(h.gateSkip(bar, st.Reason, w.engine.StateHash()))
		}
	}
}

func (h *Host) handle(w *worker, bar domain.Bar) {
	defer func() {
		if rec := recover(); rec != nil {
			w.errors.Add(1)
			w.markDisc(domain.SkipEnginePanic)
			h.emit(h.gateSkip(bar, domain.SkipEnginePanic, w.engine.StateHash()))
			log.Printf("model panic isolated symbol=%s err=%v", w.symbol, rec)
		}
	}()
	if h.opts.PanicOn == bar.Symbol {
		panic("injected model panic")
	}

	pre := w.engine.StateHash()
	w.lastEventAt.Store(time.Now())

	if w.disc.Load() {
		h.emit(h.discontinuousSkip(w, bar, pre))
		return
	}
	if !h.src.Readiness().Infer {
		ev := h.gateSkip(bar, domain.SkipInferOff, pre)
		w.lastSkip.Store(domain.SkipInferOff)
		h.emit(ev)
		return
	}
	if !bar.IsFinal || !bar.ModelEligible() {
		ev := h.gateSkip(bar, domain.SkipNotModelEligible, pre)
		w.lastSkip.Store(domain.SkipNotModelEligible)
		h.emit(ev)
		return
	}
	if w.hasAccepted {
		if !bar.IntervalStart.After(w.lastAccepted) {
			ev := h.gateSkip(bar, domain.SkipDuplicateOrRegression, pre)
			w.lastSkip.Store(domain.SkipDuplicateOrRegression)
			h.emit(ev)
			return
		}
		if bar.IntervalStart.Sub(w.lastAccepted) != time.Minute {
			w.markDisc(domain.SkipInputGap)
			ev := h.gateSkip(bar, domain.SkipInputGap, pre)
			w.lastSkip.Store(domain.SkipInputGap)
			h.emit(ev)
			return
		}
	}

	started := time.Now()
	if h.opts.Delay > 0 {
		time.Sleep(h.opts.Delay)
	}
	if time.Since(started) > h.opts.Deadline {
		w.timeouts.Add(1)
		ev := h.gateSkip(bar, domain.SkipTimeout, pre)
		ev.Skip.Detail = "step exceeded deadline"
		w.lastSkip.Store(domain.SkipTimeout)
		h.emit(ev)
		return
	}
	event, working, commit := w.engine.PrepareStep(bar)
	if time.Since(started) > h.opts.Deadline {
		w.timeouts.Add(1)
		event.Decision = nil
		event.Skip = &domain.Skip{Reason: domain.SkipTimeout, Detail: "step exceeded deadline"}
		event.PostStateHash = pre
		event.SignalID = ""
		event.DecisionID = ""
		w.lastSkip.Store(domain.SkipTimeout)
		h.emit(event)
		return
	}
	if commit {
		w.engine.Commit(working)
		w.lastAccepted = bar.IntervalStart
		w.hasAccepted = true
	}
	if event.Skip != nil {
		w.lastSkip.Store(event.Skip.Reason)
	}
	if event.Decision != nil {
		w.lastOutcome.Store(event.Decision.Side)
	}
	h.emit(event)
}

func (h *Host) gateSkip(bar domain.Bar, reason domain.SkipReason, hash string) domain.DecisionEvent {
	id := h.seq.Add(1)
	ev := domain.DecisionEvent{
		EventID:          fmt.Sprintf("%s:host:%d", bar.Symbol, id),
		Symbol:           bar.Symbol,
		IntervalStart:    bar.IntervalStart,
		MarketSnapshotID: bar.MarketSnapshotID,
		SourceTimestamp:  bar.SourceTimestamp,
		ReceivedAt:       time.Now(),
		CompletedAt:      time.Now(),
		ModelVersion:     adaptive.ModelVersionLabel,
		SchemaVersion:    adaptive.SchemaVersion,
		PreStateHash:     hash,
		PostStateHash:    hash,
		Skip:             &domain.Skip{Reason: reason},
	}
	if reason == domain.SkipInitializing {
		ev.Skip.ModelStatus = domain.StatusInitializing
	}
	return ev
}

func (h *Host) discontinuousSkip(w *worker, bar domain.Bar, hash string) domain.DecisionEvent {
	ev := h.gateSkip(bar, domain.SkipStateDiscontinuous, hash)
	if reason, ok := w.discReason.Load().(domain.SkipReason); ok && reason != "" && reason != domain.SkipStateDiscontinuous {
		ev.Skip.Detail = string(reason)
	}
	return ev
}

func (h *Host) emit(ev domain.DecisionEvent) {
	h.mu.Lock()
	if !h.closed {
		h.lastBySym[ev.Symbol] = ev
		for id, ch := range h.subs {
			select {
			case ch <- ev:
			default:
				log.Printf("model event buffer full subscriber=%d symbol=%s", id, ev.Symbol)
			}
		}
	}
	h.mu.Unlock()
	if ev.IsDecision() {
		log.Printf("model decision symbol=%s side=%s interval=%s", ev.Symbol, ev.Decision.Side, ev.IntervalStart.UTC().Format(time.RFC3339))
	} else if ev.Skip != nil {
		log.Printf("model skip symbol=%s reason=%s interval=%s", ev.Symbol, ev.Skip.Reason, ev.IntervalStart.UTC().Format(time.RFC3339))
	}
}

func (w *worker) markDisc(reason domain.SkipReason) bool {
	if w.disc.Swap(true) {
		return false
	}
	w.discReason.Store(reason)
	w.lastSkip.Store(reason)
	return true
}

func (h *Host) SymbolHealth(symbol string) SymbolHealth {
	if h == nil {
		return SymbolHealth{Symbol: symbol, Status: StatusOff}
	}
	w := h.workers[symbol]
	if w == nil {
		return SymbolHealth{Symbol: symbol, Status: StatusOff}
	}
	out := SymbolHealth{
		Symbol:   symbol,
		Accepted: w.engine.CompletedCount(),
		Warmup:   fmt.Sprintf("%d/%d", w.engine.CompletedCount(), adaptive.ActionableAfter),
	}
	if t, ok := w.lastEventAt.Load().(time.Time); ok {
		out.LastEvent = t
	}
	if skip, ok := w.lastSkip.Load().(domain.SkipReason); ok {
		out.LastSkip = skip
	}
	switch {
	case w.disc.Load():
		out.Status = StatusDiscontinuous
	case !h.src.Readiness().Infer && w.hasAccepted:
		out.Status = StatusPaused
	case w.errors.Load() > 0:
		out.Status = StatusError
	case out.Accepted == 0:
		out.Status = StatusCold
	case out.Accepted < adaptive.ActionableAfter:
		out.Status = StatusInitializing
	default:
		out.Status = StatusReady
	}
	return out
}

func (h *Host) Health() domain.ComponentHealth {
	if h == nil {
		return domain.ComponentHealth{Name: "model", State: domain.ComponentHealthy, Detail: "off"}
	}
	if h.unavail.Load() {
		return domain.ComponentHealth{Name: "model", State: domain.ComponentUnavailable, Detail: "unavailable"}
	}
	if !h.enabled.Load() {
		return domain.ComponentHealth{Name: "model", State: domain.ComponentHealthy, Detail: "off"}
	}
	anyDisc, anyInit, anyPause, anyErr := false, false, false, false
	for _, symbol := range h.symbols {
		st := h.SymbolHealth(symbol)
		switch st.Status {
		case StatusDiscontinuous:
			anyDisc = true
		case StatusInitializing, StatusCold:
			anyInit = true
		case StatusPaused:
			anyPause = true
		case StatusError:
			anyErr = true
		}
	}
	switch {
	case anyErr && allFailed(h):
		return domain.ComponentHealth{Name: "model", State: domain.ComponentUnavailable, Detail: "error"}
	case anyDisc || anyErr:
		return domain.ComponentHealth{Name: "model", State: domain.ComponentDegraded, Detail: "discontinuous"}
	case anyInit || anyPause:
		return domain.ComponentHealth{Name: "model", State: domain.ComponentDegraded, Detail: "initializing"}
	default:
		return domain.ComponentHealth{Name: "model", State: domain.ComponentHealthy, Detail: "adaptive"}
	}
}

func allFailed(h *Host) bool {
	if len(h.symbols) == 0 {
		return false
	}
	for _, symbol := range h.symbols {
		st := h.SymbolHealth(symbol)
		if st.Status != StatusError && st.Status != StatusDiscontinuous {
			return false
		}
	}
	return true
}

func (h *Host) WorkerCompleted(symbol string) int {
	if h == nil || h.workers[symbol] == nil {
		return 0
	}
	return h.workers[symbol].engine.CompletedCount()
}

func (h *Host) WorkerHash(symbol string) string {
	if h == nil || h.workers[symbol] == nil {
		return ""
	}
	return h.workers[symbol].engine.StateHash()
}
