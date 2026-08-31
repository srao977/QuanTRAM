package ingestion

import (
	"sync"
	"time"

	"quantram/internal/config"
	"quantram/internal/domain"
)

type CircuitBreaker struct {
	mu     sync.Mutex
	state  domain.FeedState
	misses uint32
}

func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{state: domain.FeedFailed}
}

func (b *CircuitBreaker) State() domain.FeedState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

func (b *CircuitBreaker) Observe(health domain.FeedHealth, now time.Time) domain.FeedState {
	b.mu.Lock()
	defer b.mu.Unlock()

	if health.State == domain.FeedRecovering {
		b.state = domain.FeedRecovering
		return b.state
	}
	if health.LastPongRTT > config.HeartbeatMaxRTT && health.LastPongRTT > 0 {
		b.misses++
		b.state = domain.FeedFailed
		return b.state
	}
	if health.ConsecutiveHeartbeatFailures >= uint32(config.HeartbeatMaxMisses) {
		b.misses = health.ConsecutiveHeartbeatFailures
		b.state = domain.FeedFailed
		return b.state
	}
	if health.State == domain.FeedFailed {
		b.state = domain.FeedFailed
		return b.state
	}
	b.misses = 0
	b.state = domain.FeedHealthy
	return b.state
}

func (b *CircuitBreaker) MarkRecovering() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = domain.FeedRecovering
}

func (b *CircuitBreaker) MarkHealthy() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.misses = 0
	b.state = domain.FeedHealthy
}
