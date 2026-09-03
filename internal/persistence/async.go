package persistence

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"quantram/internal/adaptive"
	"quantram/internal/domain"
)

type writer interface {
	WriteBar(context.Context, domain.Bar) error
	WriteDecision(context.Context, domain.DecisionEvent, *adaptive.PipelineOutputs) error
	WritePrice(context.Context, domain.PriceEvent) error
	Close(context.Context) error
}

type captureKind uint8

const (
	captureBar captureKind = iota
	captureDecision
	capturePrice
)

type capture struct {
	kind     captureKind
	bar      domain.Bar
	decision domain.DecisionEvent
	outputs  *adaptive.PipelineOutputs
	price    domain.PriceEvent
}

type AsyncStore struct {
	writer   writer
	queue    chan capture
	done     chan struct{}
	mu       sync.Mutex
	closed   bool
	dropped  atomic.Uint64
	written  atomic.Uint64
	failures atomic.Uint64
	lastErr  atomic.Value
}

func NewAsyncStore(w writer, capacity int) *AsyncStore {
	if capacity <= 0 {
		capacity = 1024
	}
	store := &AsyncStore{writer: w, queue: make(chan capture, capacity), done: make(chan struct{})}
	go store.run()
	return store
}

func (s *AsyncStore) CaptureBar(bar domain.Bar) bool {
	if bar.MarketSnapshotID == "" {
		s.dropped.Add(1)
		return false
	}
	return s.enqueue(capture{kind: captureBar, bar: bar})
}

func (s *AsyncStore) CaptureDecision(event domain.DecisionEvent, outputs *adaptive.PipelineOutputs) bool {
	if event.MarketSnapshotID == "" {
		s.dropped.Add(1)
		return false
	}
	return s.enqueue(capture{kind: captureDecision, decision: event, outputs: outputs})
}

func (s *AsyncStore) CapturePrice(event domain.PriceEvent) bool {
	if event.MarketSnapshotID == "" {
		s.dropped.Add(1)
		return false
	}
	return s.enqueue(capture{kind: capturePrice, price: event})
}

func (s *AsyncStore) enqueue(item capture) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		s.dropped.Add(1)
		return false
	}
	select {
	case s.queue <- item:
		return true
	default:
		s.dropped.Add(1)
		return false
	}
}

func (s *AsyncStore) run() {
	defer close(s.done)
	for item := range s.queue {
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err = s.write(ctx, item)
			cancel()
			if err == nil {
				break
			}
			if attempt < 2 {
				time.Sleep(time.Duration(attempt+1) * 25 * time.Millisecond)
			}
		}
		if err != nil {
			s.failures.Add(1)
			s.lastErr.Store(err.Error())
			log.Printf("persistence write failed after retries: %v", err)
		} else {
			s.written.Add(1)
		}
	}
}

func (s *AsyncStore) write(ctx context.Context, item capture) error {
	switch item.kind {
	case captureBar:
		return s.writer.WriteBar(ctx, item.bar)
	case captureDecision:
		return s.writer.WriteDecision(ctx, item.decision, item.outputs)
	case capturePrice:
		return s.writer.WritePrice(ctx, item.price)
	default:
		return fmt.Errorf("unknown capture kind %d", item.kind)
	}
}

func (s *AsyncStore) Health() Health {
	health := Health{QueueDepth: len(s.queue), Dropped: s.dropped.Load(), Written: s.written.Load(), Failures: s.failures.Load()}
	if value := s.lastErr.Load(); value != nil {
		health.LastError, _ = value.(string)
	}
	return health
}

func (s *AsyncStore) Close(ctx context.Context) error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.queue)
	}
	s.mu.Unlock()
	select {
	case <-s.done:
		return s.writer.Close(ctx)
	case <-ctx.Done():
		return fmt.Errorf("flush persistence queue: %w", ctx.Err())
	}
}
