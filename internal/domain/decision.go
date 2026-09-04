package domain

import "time"

// Side is the terminal trading action carried by a decision.
type Side string

// Sides are the complete terminal decision action set.
const (
	SideUnspecified Side = ""
	SideBuy         Side = "BUY"
	SideSell        Side = "SELL"
	SideHold        Side = "HOLD"
)

// PathDirection summarizes the model's projected path orientation.
type PathDirection string

// Path directions classify the projected return path.
const (
	PathUnspecified PathDirection = ""
	PathUpward      PathDirection = "UPWARD"
	PathDownward    PathDirection = "DOWNWARD"
	PathFlat        PathDirection = "FLAT"
)

// EmitterPosition is the position state maintained by the decision emitter.
type EmitterPosition string

// Emitter positions describe committed directional exposure.
const (
	EmitterUnspecified EmitterPosition = ""
	EmitterFlat        EmitterPosition = "FLAT"
	EmitterLong        EmitterPosition = "LONG"
	EmitterShort       EmitterPosition = "SHORT"
)

// ModelStatus identifies whether model output is still warming or actionable.
type ModelStatus string

// Model statuses separate warm-up output from actionable output.
const (
	StatusUnspecified  ModelStatus = ""
	StatusInitializing ModelStatus = "INITIALIZING"
	StatusActionable   ModelStatus = "ACTIONABLE"
)

// SkipReason classifies why a considered bar produced no decision.
type SkipReason string

// Skip reasons preserve the terminal cause of every non-decision outcome.
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
	Side                 Side            `bson:"side"`
	Confidence           float64         `bson:"confidence"`
	H                    int             `bson:"h"`
	QG                   float64         `bson:"qg"`
	QS                   float64         `bson:"qs"`
	QR                   float64         `bson:"qr"`
	PathDirection        PathDirection   `bson:"path_direction"`
	ModelStatus          ModelStatus     `bson:"model_status"`
	EmitterPosition      EmitterPosition `bson:"emitter_position"`
	RulePath             string          `bson:"rule_path"`
	Strength             float64         `bson:"strength"`
	Coherence            float64         `bson:"coherence"`
	Persistence          float64         `bson:"persistence"`
	Uncertainty          float64         `bson:"uncertainty"`
	Reversal             float64         `bson:"reversal"`
	TerminalDisplacement float64         `bson:"terminal_displacement"`
}

// Skip is a typed non-decision. It must not carry a Side.
type Skip struct {
	Reason      SkipReason  `bson:"reason"`
	Detail      string      `bson:"detail"`
	ModelStatus ModelStatus `bson:"model_status"`
}

// DecisionEvent is one terminal outcome per considered bar: a decision or a skip.
type DecisionEvent struct {
	EventID          string        `bson:"event_id"`
	SignalID         string        `bson:"signal_id"`
	DecisionID       string        `bson:"decision_id"`
	Symbol           string        `bson:"symbol"`
	IntervalStart    time.Time     `bson:"interval_start_unix_ms"`
	MarketSnapshotID string        `bson:"market_snapshot_id"`
	SourceTimestamp  string        `bson:"source_timestamp"`
	AcceptedSequence int           `bson:"accepted_sequence"`
	ReceivedAt       time.Time     `bson:"received_at_unix_ms"`
	CompletedAt      time.Time     `bson:"completed_at_unix_ms"`
	Latency          time.Duration `bson:"latency_ns"`
	ModelVersion     string        `bson:"model_version"`
	SchemaVersion    string        `bson:"schema_version"`
	PreStateHash     string        `bson:"pre_state_hash"`
	PostStateHash    string        `bson:"post_state_hash"`
	Decision         *Decision     `bson:"decision,omitempty"`
	Skip             *Skip         `bson:"skip,omitempty"`
}

// IsDecision reports whether the event contains only a decision outcome.
func (e DecisionEvent) IsDecision() bool {
	return e.Decision != nil && e.Skip == nil
}

// IsSkip reports whether the event contains only a skip outcome.
func (e DecisionEvent) IsSkip() bool {
	return e.Skip != nil && e.Decision == nil
}
