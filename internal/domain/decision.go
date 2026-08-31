package domain

import "time"

type Side string

const (
	SideUnspecified Side = ""
	SideBuy         Side = "BUY"
	SideSell        Side = "SELL"
	SideHold        Side = "HOLD"
)

type PathDirection string

const (
	PathUnspecified PathDirection = ""
	PathUpward      PathDirection = "UPWARD"
	PathDownward    PathDirection = "DOWNWARD"
	PathFlat        PathDirection = "FLAT"
)

type EmitterPosition string

const (
	EmitterUnspecified EmitterPosition = ""
	EmitterFlat        EmitterPosition = "FLAT"
	EmitterLong        EmitterPosition = "LONG"
	EmitterShort       EmitterPosition = "SHORT"
)

type ModelStatus string

const (
	StatusUnspecified  ModelStatus = ""
	StatusInitializing ModelStatus = "INITIALIZING"
	StatusActionable   ModelStatus = "ACTIONABLE"
)

type SkipReason string

const (
	SkipUnspecified           SkipReason = ""
	SkipInferOff              SkipReason = "INFER_OFF"
	SkipNotModelEligible      SkipReason = "NOT_MODEL_ELIGIBLE"
	SkipInitializing          SkipReason = "INITIALIZING"
	SkipDuplicateOrRegression SkipReason = "DUPLICATE_OR_REGRESSION"
	SkipInputGap              SkipReason = "INPUT_GAP"
	SkipQueueOverflow         SkipReason = "QUEUE_OVERFLOW"
	SkipTimeout               SkipReason = "TIMEOUT"
	SkipInvalidInput          SkipReason = "INVALID_INPUT"
	SkipEngineError           SkipReason = "ENGINE_ERROR"
	SkipEnginePanic           SkipReason = "ENGINE_PANIC"
	SkipStateDiscontinuous    SkipReason = "STATE_DISCONTINUOUS"
)

// Decision is a terminal BUY/SELL/HOLD. HOLD is a decision, not a skip.
type Decision struct {
	Side                 Side
	Confidence           float64
	H                    int
	QG                   float64
	QS                   float64
	QR                   float64
	PathDirection        PathDirection
	ModelStatus          ModelStatus
	EmitterPosition      EmitterPosition
	RulePath             string
	Strength             float64
	Coherence            float64
	Persistence          float64
	Uncertainty          float64
	Reversal             float64
	TerminalDisplacement float64
}

// Skip is a typed non-decision. It must not carry a Side.
type Skip struct {
	Reason      SkipReason
	Detail      string
	ModelStatus ModelStatus
}

// DecisionEvent is one terminal outcome per considered bar: a decision or a skip.
type DecisionEvent struct {
	EventID          string
	SignalID         string
	DecisionID       string
	Symbol           string
	IntervalStart    time.Time
	MarketSnapshotID string
	SourceTimestamp  string
	AcceptedSequence int
	ReceivedAt       time.Time
	CompletedAt      time.Time
	Latency          time.Duration
	ModelVersion     string
	SchemaVersion    string
	PreStateHash     string
	PostStateHash    string
	Decision         *Decision
	Skip             *Skip
}

func (e DecisionEvent) IsDecision() bool {
	return e.Decision != nil && e.Skip == nil
}

func (e DecisionEvent) IsSkip() bool {
	return e.Skip != nil && e.Decision == nil
}
