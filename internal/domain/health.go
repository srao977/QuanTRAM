package domain

import "time"

// FeedState describes the operational lifecycle of a market-data source.
type FeedState string

// Feed states describe connectivity and recovery progression.
const (
	FeedUnspecified FeedState = ""
	FeedHealthy     FeedState = "HEALTHY"
	FeedDegraded    FeedState = "DEGRADED"
	FeedFailed      FeedState = "FAILED"
	FeedRecovering  FeedState = "RECOVERING"
)

// ComponentState is the service-level health classification of a component.
type ComponentState string

// Component states classify service availability.
const (
	ComponentUnspecified ComponentState = ""
	ComponentHealthy     ComponentState = "HEALTHY"
	ComponentDegraded    ComponentState = "DEGRADED"
	ComponentUnavailable ComponentState = "UNAVAILABLE"
)

// FeedHealth is a point-in-time snapshot of source connectivity and heartbeat state.
type FeedHealth struct {
	SourceID                     string
	State                        FeedState
	LastMessage                  time.Time
	LastPongRTT                  time.Duration
	ConsecutiveHeartbeatFailures uint32
	LastError                    string
	SubscribedSymbols            []string
}

// ComponentHealth reports one named service component.
type ComponentHealth struct {
	Name   string
	State  ComponentState
	Detail string
}

// HealthReport aggregates component health into an overall state.
type HealthReport struct {
	State      ComponentState
	Components []ComponentHealth
}

// Readiness separates observation availability from inference eligibility.
type Readiness struct {
	Ready   bool
	Observe bool
	Infer   bool
	Message string
}
