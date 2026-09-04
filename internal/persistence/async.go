// Package persistence provides asynchronous capture and MongoDB ledger
// adapters. This file owns the bounded queue worker and its close boundary; it
// does not define scientific ordering or MongoDB document semantics.
package persistence

import (
	"context"
	"errors"
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

type disconnecter interface {
	Disconnect(context.Context) error
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
	cancel   context.CancelFunc
	drainMu  sync.Mutex
	drainErr error
	drained  bool
	closeMu  sync.Mutex
	closeErr error
	closedOK bool
	mu       sync.Mutex
	closed   bool
	dropped  atomic.Uint64
	written  atomic.Uint64
	failures atomic.Uint64
	lastErr  atomic.Value
}

// NewAsyncStore starts one context-cancelable FIFO persistence worker with a
// bounded queue. Producers never wait for queue capacity.
func NewAsyncStore(w writer, capacity int) *AsyncStore {
	if capacity <= 0 {
		capacity = 1024
	}
	workerCtx, cancel := context.WithCancel(context.Background())
	store := &AsyncStore{writer: w, queue: make(chan capture, capacity), done: make(chan struct{}), cancel: cancel}
	go store.run(workerCtx)
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

// run owns all writes accepted by the queue. It preserves dequeue order,
// applies bounded attempts, and accounts items canceled during forced close.
func (s *AsyncStore) run(workerCtx context.Context) {
	defer close(s.done)
	for item := range s.queue {
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			ctx, cancel := context.WithTimeout(workerCtx, 5*time.Second)
			err = s.write(ctx, item)
			cancel()
			if err == nil {
				break
			}
			if workerCtx.Err() != nil {
				break
			}
			if attempt < 2 {
				delay := time.NewTimer(time.Duration(attempt+1) * 25 * time.Millisecond)
				select {
				case <-workerCtx.Done():
					delay.Stop()
				case <-delay.C:
				}
			}
		}
		if err != nil {
			s.failures.Add(1)
			s.lastErr.Store(err.Error())
			log.Printf("persistence write failed after retries: %v", err)
		} else {
			s.written.Add(1)
		}
		if workerCtx.Err() != nil {
			s.accountAbandoned()
			return
		}
	}
}

// accountAbandoned marks captures that cannot be attempted after forced worker
// cancellation. The queue is closed before cancellation, so this drain has a
// stable finite end and does not race new submissions.
func (s *AsyncStore) accountAbandoned() {
	const reason = "persistence capture abandoned during shutdown timeout"
	for range s.queue {
		s.failures.Add(1)
	}
	s.lastErr.Store(reason)
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

// Drain seals capture submission and waits for every accepted item to finish.
// It does not SHUT the Aperture or disconnect, allowing the Snapshot service to
// evaluate the resulting durable ledger before final persistence closure.
func (s *AsyncStore) Drain(ctx context.Context) error {
	s.drainMu.Lock()
	defer s.drainMu.Unlock()
	if s.drained {
		return s.drainErr
	}
	defer func() { s.drained = true }()
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.queue)
	}
	s.mu.Unlock()
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		s.cancel()
		<-s.done
		s.drainErr = fmt.Errorf("flush persistence queue: %w", ctx.Err())
		return s.drainErr
	}
}

// Close is idempotent. A successful drain commits Aperture SHUT before MongoDB
// disconnect; a failed drain preserves OPEN and performs disconnect only.
func (s *AsyncStore) Close(ctx context.Context) error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closedOK {
		return s.closeErr
	}
	defer func() { s.closedOK = true }()

	if err := s.Drain(ctx); err != nil {
		disconnector, ok := s.writer.(disconnecter)
		if !ok {
			s.closeErr = err
			return s.closeErr
		}
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.closeErr = errors.Join(err, disconnector.Disconnect(disconnectCtx))
		return s.closeErr
	}
	s.closeErr = s.writer.Close(ctx)
	return s.closeErr
}
