package ingestion

import (
	"testing"
	"time"

	"quantram/internal/domain"
)

func TestCircuitBreakerHealthyAndFailed(t *testing.T) {
	breaker := NewCircuitBreaker()
	now := time.Now()
	state := breaker.Observe(domain.FeedHealth{
		State:       domain.FeedHealthy,
		LastMessage: now,
	}, now)
	if state != domain.FeedHealthy {
		t.Fatalf("state %s", state)
	}

	state = breaker.Observe(domain.FeedHealth{
		State:                        domain.FeedHealthy,
		LastMessage:                  now.Add(-5 * time.Second),
		ConsecutiveHeartbeatFailures: 3,
	}, now)
	if state != domain.FeedFailed {
		t.Fatalf("expected failed, got %s", state)
	}

	state = breaker.Observe(domain.FeedHealth{
		State:       domain.FeedHealthy,
		LastMessage: now.Add(-5 * time.Second),
	}, now)
	if state != domain.FeedHealthy {
		t.Fatalf("quiet data interval must not fail the feed, got %s", state)
	}
}

func TestCircuitBreakerRTT(t *testing.T) {
	breaker := NewCircuitBreaker()
	now := time.Now()
	state := breaker.Observe(domain.FeedHealth{
		State:       domain.FeedHealthy,
		LastMessage: now,
		LastPongRTT: 2 * time.Second,
	}, now)
	if state != domain.FeedFailed {
		t.Fatalf("expected failed on high rtt, got %s", state)
	}
}
