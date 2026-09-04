// Package ingestion coordinates feed health, normalized bar windows, recovery,
// persistence capture, and downstream observation and model delivery.
package ingestion

import (
	"sync"
	"time"

	"quantram/internal/config"
	"quantram/internal/domain"
)

// CircuitBreaker serializes the feed state derived from heartbeat observations.
type CircuitBreaker struct {
	mu     sync.Mutex
	state  domain.FeedState
	misses uint32
}

// NewCircuitBreaker returns a breaker that remains failed until health is observed.
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{state: domain.FeedFailed}
}

// State returns the breaker's current feed state.
func (b *CircuitBreaker) State() domain.FeedState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// Observe atomically derives and stores feed state from a health snapshot.
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

// MarkRecovering gates inference while historical recovery is in progress.
func (b *CircuitBreaker) MarkRecovering() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = domain.FeedRecovering
}

// MarkHealthy clears heartbeat misses and marks the feed healthy.
func (b *CircuitBreaker) MarkHealthy() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.misses = 0
	b.state = domain.FeedHealthy
}
