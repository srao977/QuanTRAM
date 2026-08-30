package ingestion

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"quantram/internal/config"
	"quantram/internal/domain"
	"quantram/internal/marketfeed"
)

type Pipeline struct {
	live       marketfeed.LiveBarSource
	historical marketfeed.HistoricalBarSource
	breaker    *CircuitBreaker
	window     *WindowStore
	sourceID   string
	symbols    []string

	mu          sync.Mutex
	subscribers map[uint64]chan domain.Bar
	nextSub     uint64
	inferReady  atomic.Bool
	filling     atomic.Bool
}

func NewPipeline(live marketfeed.LiveBarSource, historical marketfeed.HistoricalBarSource, sourceID string, symbols []string) *Pipeline {
	return &Pipeline{
		live:        live,
		historical:  historical,
		breaker:     NewCircuitBreaker(),
		window:      NewWindowStore(config.WindowLimit),
		sourceID:    sourceID,
		symbols:     symbols,
		subscribers: make(map[uint64]chan domain.Bar),
	}
}

func (p *Pipeline) Run(ctx context.Context) error {
	incoming := make(chan domain.Bar, config.SubscriberQueue)
	errCh := make(chan error, 1)
	go func() {
		errCh <- p.live.Run(ctx, p.symbols, incoming)
	}()

	ticker := time.NewTicker(config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			p.drain(incoming)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			p.breaker.Observe(p.live.Health(), time.Now())
			p.inferReady.Store(false)
			if err != nil && ctx.Err() == nil {
				log.Printf("live source ended: %v", err)
			}
			return err
		case bar := <-incoming:
			p.accept(bar)
			p.breaker.Observe(p.live.Health(), time.Now())
		case <-ticker.C:
			state := p.breaker.Observe(p.live.Health(), time.Now())
			if state == domain.FeedFailed {
				p.inferReady.Store(false)
			}
			if state == domain.FeedRecovering {
				p.startRecovery(ctx)
			}
		}
	}
}

func (p *Pipeline) drain(incoming <-chan domain.Bar) {
	for {
		select {
		case bar := <-incoming:
			p.accept(bar)
		default:
			return
		}
	}
}

func (p *Pipeline) accept(bar domain.Bar) {
	if !p.window.Add(bar) {
		return
	}
	if p.breaker.State() == domain.FeedRecovering && !bar.IsBackfilled {
		p.breaker.MarkHealthy()
		p.inferReady.Store(true)
	}
	if p.breaker.State() == domain.FeedHealthy {
		p.inferReady.Store(true)
	}
	p.fanout(bar)
}

func (p *Pipeline) fanout(bar domain.Bar) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, ch := range p.subscribers {
		select {
		case ch <- bar:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- bar:
			default:
				log.Printf("drop bar for slow subscriber %d symbol=%s", id, bar.Symbol)
			}
		}
	}
}

func (p *Pipeline) Subscribe(buffer int) (uint64, <-chan domain.Bar) {
	if buffer <= 0 {
		buffer = config.SubscriberQueue
	}
	id := atomic.AddUint64(&p.nextSub, 1)
	ch := make(chan domain.Bar, buffer)
	p.mu.Lock()
	p.subscribers[id] = ch
	p.mu.Unlock()
	return id, ch
}

func (p *Pipeline) Unsubscribe(id uint64) {
	p.mu.Lock()
	ch, ok := p.subscribers[id]
	delete(p.subscribers, id)
	p.mu.Unlock()
	if ok {
		close(ch)
	}
}

func (p *Pipeline) Symbols() []string {
	return append([]string(nil), p.symbols...)
}

func (p *Pipeline) Window(symbol string, limit int) []domain.Bar {
	return p.window.Window(symbol, limit)
}

func (p *Pipeline) GapFill(ctx context.Context, symbol string, from, to time.Time) (fetched, injected, deduped int, err error) {
	if p.historical == nil {
		return 0, 0, 0, fmt.Errorf("historical source is not configured")
	}
	if from.IsZero() {
		if last, ok := p.window.Last(symbol); ok {
			from = last.IntervalStart
		} else {
			from = time.Now().UTC().Add(-15 * time.Minute)
		}
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	p.breaker.MarkRecovering()
	p.inferReady.Store(false)
	bars, err := p.historical.Bars(ctx, marketfeed.BarRangeRequest{Symbol: symbol, From: from, To: to})
	if err != nil {
		return 0, 0, 0, err
	}
	fetched = len(bars)
	for _, bar := range bars {
		if p.window.Add(bar) {
			injected++
			p.fanout(bar)
		} else {
			deduped++
		}
	}
	p.breaker.MarkHealthy()
	p.inferReady.Store(true)
	return fetched, injected, deduped, nil
}

func (p *Pipeline) startRecovery(ctx context.Context) {
	if !p.filling.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer p.filling.Store(false)
		p.RecoverAfterReconnect(ctx)
	}()
}

func (p *Pipeline) RecoverAfterReconnect(ctx context.Context) {
	if p.historical == nil {
		p.breaker.MarkHealthy()
		return
	}
	now := time.Now().UTC()
	for _, symbol := range p.symbols {
		from := now.Add(-15 * time.Minute)
		if last, ok := p.window.Last(symbol); ok {
			from = last.IntervalStart
		}
		if _, _, _, err := p.GapFill(ctx, symbol, from, now); err != nil {
			log.Printf("gap fill %s: %v", symbol, err)
		}
	}
}

func (p *Pipeline) FeedHealth() domain.FeedHealth {
	health := p.live.Health()
	health.State = p.breaker.State()
	if health.SourceID == "" {
		health.SourceID = p.sourceID
	}
	if len(health.SubscribedSymbols) == 0 {
		health.SubscribedSymbols = append([]string(nil), p.symbols...)
	}
	return health
}

func (p *Pipeline) Health() domain.HealthReport {
	feed := p.FeedHealth()
	feedState := domain.ComponentHealthy
	switch feed.State {
	case domain.FeedDegraded:
		feedState = domain.ComponentDegraded
	case domain.FeedFailed:
		feedState = domain.ComponentUnavailable
	case domain.FeedRecovering:
		feedState = domain.ComponentDegraded
	}
	report := domain.HealthReport{
		State: feedState,
		Components: []domain.ComponentHealth{
			{Name: "marketfeed", State: feedState, Detail: feed.SourceID},
			{Name: "ingestion", State: feedState, Detail: string(feed.State)},
		},
	}
	return report
}

func (p *Pipeline) Readiness() domain.Readiness {
	state := p.breaker.State()
	observe := state == domain.FeedHealthy || state == domain.FeedDegraded || state == domain.FeedRecovering
	infer := p.inferReady.Load() && state == domain.FeedHealthy
	ready := observe
	message := string(state)
	if !ready {
		message = "feed not ready"
	}
	return domain.Readiness{Ready: ready, Observe: observe, Infer: infer, Message: message}
}
