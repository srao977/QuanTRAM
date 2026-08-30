package domain

import "time"

type FeedState string

const (
	FeedUnspecified FeedState = ""
	FeedHealthy     FeedState = "HEALTHY"
	FeedDegraded    FeedState = "DEGRADED"
	FeedFailed      FeedState = "FAILED"
	FeedRecovering  FeedState = "RECOVERING"
)

type ComponentState string

const (
	ComponentUnspecified ComponentState = ""
	ComponentHealthy     ComponentState = "HEALTHY"
	ComponentDegraded    ComponentState = "DEGRADED"
	ComponentUnavailable ComponentState = "UNAVAILABLE"
)

type FeedHealth struct {
	SourceID                     string
	State                        FeedState
	LastMessage                  time.Time
	LastPongRTT                  time.Duration
	ConsecutiveHeartbeatFailures uint32
	LastError                    string
	SubscribedSymbols            []string
}

type ComponentHealth struct {
	Name   string
	State  ComponentState
	Detail string
}

type HealthReport struct {
	State      ComponentState
	Components []ComponentHealth
}

type Readiness struct {
	Ready   bool
	Observe bool
	Infer   bool
	Message string
}
