// Package modelhost runs isolated per-symbol adaptive and pricing workers over
// the ingestion model path and publishes terminal outcomes.
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
	"quantram/internal/pricing"
)

const (
	modelSubscribeBuffer = config.WindowLimit
	workerInboxSize      = config.WindowLimit
	eventBuffer          = 128
	pathPollInterval     = 50 * time.Millisecond
)

// ErrUnavailable identifies failures that prevent the adaptive host from starting.
var ErrUnavailable = errors.New("adaptive model unavailable")

// Unavailable is a Health-only stand-in when adaptive was requested but could not start.
// Do not call Run on it; ingestion stays up and GetHealth reports unavailable (not off).
type Unavailable struct{}

// Health reports the requested model as unavailable rather than disabled.
func (Unavailable) Health() domain.ComponentHealth {
	return domain.ComponentHealth{Name: "model", State: domain.ComponentUnavailable, Detail: "unavailable"}
}

// PricingHealth reports pricing as disabled for an unavailable model stand-in.
func (Unavailable) PricingHealth() domain.ComponentHealth {
	return domain.ComponentHealth{Name: "pricing", State: domain.ComponentHealthy, Detail: "off"}
}

// SymbolStatus describes one symbol worker's lifecycle and continuity state.
type SymbolStatus string

// Symbol statuses describe worker readiness, continuity, and failure states.
const (
	StatusOff           SymbolStatus = "off"
	StatusCold          SymbolStatus = "cold"
	StatusInitializing  SymbolStatus = "initializing"
	StatusReady         SymbolStatus = "ready"
	StatusPaused        SymbolStatus = "paused"
	StatusDiscontinuous SymbolStatus = "discontinuous"
	StatusError         SymbolStatus = "error"
)

// BarSource is the ordered, loss-intolerant ingestion boundary consumed by Host.
type BarSource interface {
	SubscribeModelBars(buffer int) (uint64, <-chan domain.Bar)
	Unsubscribe(id uint64)
	Readiness() domain.Readiness
	ReadinessFor(symbol string) domain.Readiness
	ModelPathStatus(symbol string) ingestion.ModelPathStatus
}

// provenMissingInspector is a test/harness hook. Live Pipeline does not implement
// it: a skipped IEX minute is not evidence that a required observation was lost.
type provenMissingInspector interface {
	ProvenMissingEligible(symbol string, from, to time.Time) bool
}

type modelPathResetter interface {
	ResetModelPath(symbol string)
}

func continuityDetail(class domain.ContinuityClass, last, current time.Time, elapsed time.Duration) string {
	return fmt.Sprintf("%s last=%s current=%s elapsed=%s", class, last.UTC().Format(time.RFC3339), current.UTC().Format(time.RFC3339), elapsed)
}

// Options configures model execution, deadlines, fault injection, and capture.
type Options struct {
	Mode      config.ModelMode
	Pricing   config.PricingMode
	Deadline  time.Duration
	Delay     time.Duration
	PanicOn   string
	InboxSize int
	Capture   EventCapture
}

// EventCapture persists terminal model and pricing outcomes.
type EventCapture interface {
	CaptureDecision(domain.DecisionEvent, *adaptive.PipelineOutputs) bool
	CapturePrice(domain.PriceEvent) bool
}

// Host owns one serial worker per symbol and fans out their terminal outcomes.
// The host mutex protects lifecycle state, subscriptions, and last-event indexes.
type Host struct {
	src            BarSource
	symbols        []string
	opts           Options
	workers        map[string]*worker
	seq            atomic.Uint64
	enabled        atomic.Bool
	unavail        atomic.Bool
	pricingUnavail atomic.Bool
	failPricing    atomic.Bool
	started        atomic.Bool
	nextSub        atomic.Uint64
	nextPriceSub   atomic.Uint64

	mu           sync.Mutex
	closed       bool
	cancel       context.CancelFunc
	subs         map[uint64]chan domain.DecisionEvent
	priceSubs    map[uint64]chan domain.PriceEvent
	lastBySym    map[string]domain.DecisionEvent
	lastPriceSym map[string]domain.PriceEvent
	eventsOnce   sync.Once
	events       <-chan domain.DecisionEvent
}

type worker struct {
	// engine, pricing, inbox, and acceptance history are owned by runWorker.
	symbol       string
	engine       *adaptive.Engine
	pricing      *pricing.Engine
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
	lastPrice    atomic.Value
}

// SymbolHealth reports progress and the latest gate state for one symbol.
type SymbolHealth struct {
	Symbol    string
	Status    SymbolStatus
	Accepted  int
	Warmup    string
	LastSkip  domain.SkipReason
	LastEvent time.Time
}

// New validates configuration and constructs an adaptive host.
// It returns a nil host without error when model execution is disabled.
func New(src BarSource, symbols []string, opts Options) (*Host, error) {
	if opts.Mode == "" {
		opts.Mode = config.ModelOff
	}
	if opts.Pricing == "" {
		opts.Pricing = config.PricingOff
	}
	if opts.Pricing != config.PricingOff && opts.Pricing != config.PricingExpm {
		return nil, fmt.Errorf("unknown pricing mode %q", opts.Pricing)
	}
	if err := config.ValidatePricingRequiresAdaptive(opts.Pricing, opts.Mode); err != nil {
		return nil, err
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
		src:          src,
		symbols:      append([]string(nil), symbols...),
		opts:         opts,
		workers:      make(map[string]*worker, len(symbols)),
		subs:         make(map[uint64]chan domain.DecisionEvent),
		priceSubs:    make(map[uint64]chan domain.PriceEvent),
		lastBySym:    make(map[string]domain.DecisionEvent, len(symbols)),
		lastPriceSym: make(map[string]domain.PriceEvent, len(symbols)),
	}
	for _, symbol := range h.symbols {
		h.workers[symbol] = &worker{
			symbol: symbol,
			engine: adaptive.NewEngine(symbol),
			inbox:  make(chan domain.Bar, inbox),
		}
	}
	if opts.Pricing == config.PricingExpm {
		for _, symbol := range h.symbols {
			eng, err := pricing.NewEngine(symbol)
			if err != nil {
				h.pricingUnavail.Store(true)
				break
			}
			h.workers[symbol].pricing = eng
		}
		if h.pricingUnavail.Load() {
			for _, w := range h.workers {
				w.pricing = nil
			}
		}
	}
	h.enabled.Store(true)
	return h, nil
}

// ResetSymbol reinitializes one symbol's adaptive and pricing engines.
// It is an explicit, auditable reinitialization (warm-up restarts). It is not
// an operator RPC and must not be used to hide a live irregular interval.
func (h *Host) ResetSymbol(symbol string) error {
	if h == nil {
		return fmt.Errorf("host is nil")
	}
	w := h.workers[symbol]
	if w == nil {
		return fmt.Errorf("unknown symbol %s", symbol)
	}
	w.engine = adaptive.NewEngine(symbol)
	if h.opts.Pricing == config.PricingExpm && !h.pricingUnavail.Load() {
		eng, err := pricing.NewEngine(symbol)
		if err != nil {
			return err
		}
		w.pricing = eng
	} else {
		w.pricing = nil
	}
	w.hasAccepted = false
	w.lastAccepted = time.Time{}
	w.disc.Store(false)
	w.discReason.Store(domain.SkipReason(""))
	w.lastSkip.Store(domain.SkipReason(""))
	w.lastOutcome.Store(domain.Side(""))
	w.lastPrice.Store(domain.PriceEvent{})
	if r, ok := h.src.(modelPathResetter); ok {
		r.ResetModelPath(symbol)
	}
	log.Printf("model reset symbol=%s (explicit per-symbol reinitialization)", symbol)
	return nil
}

// FailNextPricing injects one pricing prepare failure for verification.
func (h *Host) FailNextPricing() {
	if h != nil {
		h.failPricing.Store(true)
	}
}

// Started reports whether Run has subscribed to its bar source.
func (h *Host) Started() bool {
	return h != nil && h.started.Load()
}

// Events returns the lazily created default decision-event subscription.
func (h *Host) Events() <-chan domain.DecisionEvent {
	if h == nil {
		return nil
	}
	h.eventsOnce.Do(func() {
		_, h.events = h.SubscribeEvents(eventBuffer)
	})
	return h.events
}

// SubscribeEvents registers a non-blocking decision-event subscriber.
// A full subscriber buffer drops the new event without blocking model workers.
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

// UnsubscribeEvents removes and closes a decision-event subscription.
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

// LastEvents returns the latest decision event retained for each symbol.
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

// SubscribePriceEvents registers a non-blocking price-event subscriber.
func (h *Host) SubscribePriceEvents(buffer int) (uint64, <-chan domain.PriceEvent) {
	if h == nil {
		ch := make(chan domain.PriceEvent)
		close(ch)
		return 0, ch
	}
	if buffer <= 0 {
		buffer = eventBuffer
	}
	ch := make(chan domain.PriceEvent, buffer)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		close(ch)
		return 0, ch
	}
	id := h.nextPriceSub.Add(1)
	h.priceSubs[id] = ch
	return id, ch
}

// UnsubscribePriceEvents removes and closes a price-event subscription.
func (h *Host) UnsubscribePriceEvents(id uint64) {
	if h == nil || id == 0 {
		return
	}
	h.mu.Lock()
	ch, ok := h.priceSubs[id]
	if ok {
		delete(h.priceSubs, id)
	}
	h.mu.Unlock()
	if ok {
		close(ch)
	}
}

// LastPriceEvents returns the latest price event retained for each symbol.
func (h *Host) LastPriceEvents() []domain.PriceEvent {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]domain.PriceEvent, 0, len(h.lastPriceSym))
	for _, ev := range h.lastPriceSym {
		out = append(out, ev)
	}
	return out
}

// PricingEnabled reports whether EXPM pricing is configured and available.
func (h *Host) PricingEnabled() bool {
	return h != nil && h.opts.Pricing == config.PricingExpm && !h.pricingUnavail.Load()
}

// Run dispatches ordered model bars and manages the per-symbol worker lifecycle.
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
		priceSubs := h.priceSubs
		h.priceSubs = make(map[uint64]chan domain.PriceEvent)
		h.mu.Unlock()
		for _, ch := range subs {
			close(ch)
		}
		for _, ch := range priceSubs {
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
	// Worker inbox overflow is a continuity failure, not a lossy queue policy.
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
	if class, elapsed := domain.ClassifyBarContinuity(w.lastAccepted, w.hasAccepted, bar.IntervalStart); class != domain.ContinuityFirst && class != domain.ContinuityNormal && class != domain.ContinuityIrregular {
		reason := domain.SkipDuplicateOrRegression
		if class == domain.ContinuityUnaligned {
			reason = domain.SkipInvalidInput
		}
		ev := h.gateSkip(bar, reason, pre)
		ev.Skip.Detail = continuityDetail(class, w.lastAccepted, bar.IntervalStart, elapsed)
		w.lastSkip.Store(reason)
		h.emit(ev)
		return
	} else if class == domain.ContinuityIrregular {
		if inspector, ok := h.src.(provenMissingInspector); ok && inspector.ProvenMissingEligible(w.symbol, w.lastAccepted, bar.IntervalStart) {
			w.markDisc(domain.SkipInputGap)
			ev := h.gateSkip(bar, domain.SkipInputGap, pre)
			ev.Skip.Detail = continuityDetail(class, w.lastAccepted, bar.IntervalStart, elapsed) + " proven_missing_eligible"
			w.lastSkip.Store(domain.SkipInputGap)
			h.emit(ev)
			return
		}
		log.Printf("model accept irregular symbol=%s last=%s current=%s elapsed=%s", w.symbol, w.lastAccepted.UTC().Format(time.RFC3339), bar.IntervalStart.UTC().Format(time.RFC3339), elapsed)
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
	// Both engines prepare from their committed states. Neither state advances
	// unless both preparations permit the coordinated commit below.
	event, working, commitA := w.engine.PrepareStep(bar)
	var priceEv domain.PriceEvent
	var priceWork *pricing.Engine
	commitP := true
	if w.pricing != nil {
		if h.failPricing.CompareAndSwap(true, false) {
			commitP = false
			priceEv = domain.PriceEvent{
				Symbol:           bar.Symbol,
				IntervalStart:    bar.IntervalStart,
				MarketSnapshotID: bar.MarketSnapshotID,
				SourceTimestamp:  bar.SourceTimestamp,
				Status:           domain.PricingStatusUnspecified,
				Skip:             &domain.PricingSkip{Reason: domain.PricingSkipEngineError, Detail: "injected pricing failure"},
			}
		} else {
			priceEv, priceWork, commitP = w.pricing.PrepareStep(bar)
		}
	}
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
	var committedOutputs *adaptive.PipelineOutputs
	if commitA && commitP {
		w.engine.Commit(working)
		if h.opts.Capture != nil {
			outputs := w.engine.LastPipelineOutputs()
			committedOutputs = &outputs
		}
		if w.pricing != nil {
			w.pricing.Commit(priceWork)
			if priceEv.EventID == "" {
				priceEv.EventID = fmt.Sprintf("%s:price:%d", w.symbol, h.seq.Add(1))
			}
			// Price capture intentionally precedes adaptive capture for one bar.
			w.lastPrice.Store(priceEv)
			h.emitPrice(priceEv)
		}
		w.lastAccepted = bar.IntervalStart
		w.hasAccepted = true
	} else if w.pricing != nil && commitA && !commitP {
		event.Decision = nil
		detail := "pricing prepare failed"
		if priceEv.Skip != nil && priceEv.Skip.Detail != "" {
			detail = priceEv.Skip.Detail
		}
		event.Skip = &domain.Skip{Reason: domain.SkipEngineError, Detail: detail}
		event.PostStateHash = pre
		event.SignalID = ""
		event.DecisionID = ""
	}
	if event.Skip != nil {
		w.lastSkip.Store(event.Skip.Reason)
	}
	if event.Decision != nil {
		w.lastOutcome.Store(event.Decision.Side)
	}
	if w.pricing != nil && commitA && commitP && priceEv.Emission != nil {
		log.Printf("pricing emit symbol=%s color=%s interval=%s", bar.Symbol, priceEv.Emission.Color, bar.IntervalStart.UTC().Format(time.RFC3339))
	}
	h.emitWithOutputs(event, committedOutputs)
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
	h.emitWithOutputs(ev, nil)
}

func (h *Host) emitWithOutputs(ev domain.DecisionEvent, outputs *adaptive.PipelineOutputs) {
	if h.opts.Capture != nil {
		h.opts.Capture.CaptureDecision(ev, outputs)
	}
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

func (h *Host) emitPrice(ev domain.PriceEvent) {
	if h.opts.Capture != nil {
		h.opts.Capture.CapturePrice(ev)
	}
	h.mu.Lock()
	if !h.closed {
		h.lastPriceSym[ev.Symbol] = ev
		for id, ch := range h.priceSubs {
			select {
			case ch <- ev:
			default:
				log.Printf("pricing event buffer full subscriber=%d symbol=%s", id, ev.Symbol)
			}
		}
	}
	h.mu.Unlock()
}

func (w *worker) markDisc(reason domain.SkipReason) bool {
	if w.disc.Swap(true) {
		return false
	}
	w.discReason.Store(reason)
	w.lastSkip.Store(reason)
	return true
}

// SymbolHealth returns the current lifecycle status for one symbol worker.
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

// Health aggregates adaptive worker states into model component health.
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

// WorkerCompleted returns the committed adaptive step count for a symbol.
func (h *Host) WorkerCompleted(symbol string) int {
	if h == nil || h.workers[symbol] == nil {
		return 0
	}
	return h.workers[symbol].engine.CompletedCount()
}

// WorkerHash returns the committed adaptive state hash for a symbol.
func (h *Host) WorkerHash(symbol string) string {
	if h == nil || h.workers[symbol] == nil {
		return ""
	}
	return h.workers[symbol].engine.StateHash()
}

// WorkerPricingHash returns the committed pricing state hash for a symbol.
func (h *Host) WorkerPricingHash(symbol string) string {
	if h == nil || h.workers[symbol] == nil || h.workers[symbol].pricing == nil {
		return ""
	}
	return h.workers[symbol].pricing.StateHash()
}

// WorkerPricingReceived returns the pricing engine's accepted input count.
func (h *Host) WorkerPricingReceived(symbol string) int {
	if h == nil || h.workers[symbol] == nil || h.workers[symbol].pricing == nil {
		return 0
	}
	return h.workers[symbol].pricing.Received()
}

// PricingHealth aggregates pricing worker states into component health.
func (h *Host) PricingHealth() domain.ComponentHealth {
	if h == nil {
		return domain.ComponentHealth{Name: "pricing", State: domain.ComponentHealthy, Detail: "off"}
	}
	if h.pricingUnavail.Load() {
		return domain.ComponentHealth{Name: "pricing", State: domain.ComponentUnavailable, Detail: "unavailable"}
	}
	if h.opts.Pricing != config.PricingExpm {
		return domain.ComponentHealth{Name: "pricing", State: domain.ComponentHealthy, Detail: "off"}
	}
	anyDisc, anyInit, anyPause, anyErr, anyReady := false, false, false, false, false
	allCold := len(h.symbols) > 0
	for _, symbol := range h.symbols {
		st := h.pricingSymbolStatus(symbol)
		if st != StatusCold && st != StatusOff {
			allCold = false
		}
		switch st {
		case StatusDiscontinuous:
			anyDisc = true
		case StatusInitializing:
			anyInit = true
		case StatusPaused:
			anyPause = true
		case StatusError:
			anyErr = true
		case StatusReady:
			anyReady = true
		case StatusCold:
			anyInit = true
		}
	}
	switch {
	case anyErr && allPricingFailed(h):
		return domain.ComponentHealth{Name: "pricing", State: domain.ComponentUnavailable, Detail: "error"}
	case anyDisc || anyErr:
		return domain.ComponentHealth{Name: "pricing", State: domain.ComponentDegraded, Detail: "discontinuous"}
	case allCold:
		return domain.ComponentHealth{Name: "pricing", State: domain.ComponentDegraded, Detail: "cold"}
	case anyPause && !anyInit:
		return domain.ComponentHealth{Name: "pricing", State: domain.ComponentDegraded, Detail: "paused"}
	case anyInit || anyPause:
		return domain.ComponentHealth{Name: "pricing", State: domain.ComponentDegraded, Detail: "initializing"}
	case anyReady:
		return domain.ComponentHealth{Name: "pricing", State: domain.ComponentHealthy, Detail: "expm"}
	default:
		return domain.ComponentHealth{Name: "pricing", State: domain.ComponentHealthy, Detail: "expm"}
	}
}

func (h *Host) pricingSymbolStatus(symbol string) SymbolStatus {
	w := h.workers[symbol]
	if w == nil || w.pricing == nil {
		return StatusOff
	}
	received := w.pricing.Received()
	warmup := w.pricing.WarmupBars()
	switch {
	case w.disc.Load():
		return StatusDiscontinuous
	case !h.src.Readiness().Infer && w.hasAccepted:
		return StatusPaused
	case w.errors.Load() > 0:
		return StatusError
	case received == 0:
		return StatusCold
	case received <= warmup:
		return StatusInitializing
	default:
		return StatusReady
	}
}

func allPricingFailed(h *Host) bool {
	if len(h.symbols) == 0 {
		return false
	}
	for _, symbol := range h.symbols {
		st := h.pricingSymbolStatus(symbol)
		if st != StatusError && st != StatusDiscontinuous {
			return false
		}
	}
	return true
}
