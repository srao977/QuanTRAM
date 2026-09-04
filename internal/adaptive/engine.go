package adaptive

// This file orchestrates the adaptive stages and decision-context state machine.

import (
	"fmt"
	"slices"
	"time"

	"quantram/internal/domain"
)

const (
	decisionBUY  = "BUY"
	decisionSELL = "SELL"
	decisionHOLD = "HOLD"
)

var directionSign = map[domain.PathDirection]int{
	domain.PathUpward:   1,
	domain.PathDownward: -1,
	domain.PathFlat:     0,
}

type contextRecord struct {
	C               float64
	PathDirection   domain.PathDirection
	ObservationID   string
	SourceTimestamp string
}

// AdaptiveProperties summarizes the prior fixed-length capturability and path
// context used by the baseline decision predicate.
type AdaptiveProperties struct {
	Prior15MedianC   float64
	Prior15MinC      float64
	Prior15MaxC      float64
	Prior15RangeC    float64
	PriorC           float64
	DeltaC           float64
	UpCount          int
	DownCount        int
	FlatCount        int
	DirectionBalance int
}

// Engine owns committed D01 state, the rolling decision context, and the last
// canonical outputs for one entity.
type Engine struct {
	entityID         string
	ruleFingerprint  string
	codeFingerprint  string
	d01              *Model
	context          []contextRecord
	positionState    domain.EmitterPosition
	previousDecision *domain.Side
	completedCount   int
	lastSourceTime   *float64
	eventSeq         int
	last             EvalSnapshot
	lastPipeline     PipelineOutputs
}

// PipelineOutputs retains the canonical stage outputs already produced by one
// successful adaptive evaluation. It does not add or recompute science.
type PipelineOutputs struct {
	DMO           DMOOutput           `bson:"dmo"`
	FMO           FMOOutput           `bson:"fmo"`
	ReturnShape   ReturnShape         `bson:"return_shape"`
	Capturability CapturabilityResult `bson:"capturability"`
}

// EvalSnapshot is the last committed scientific evaluation (INITIALIZING included).
type EvalSnapshot struct {
	Path                 domain.PathDirection
	H                    int
	QG                   float64
	QS                   float64
	QR                   float64
	C                    float64
	Strength             float64
	Coherence            float64
	Persistence          float64
	Uncertainty          float64
	Reversal             float64
	TerminalDisplacement float64
	PositionAfter        domain.EmitterPosition
}

// NewEngine creates an adaptive engine at flat position with empty context.
func NewEngine(entityID string) *Engine {
	cfg := DefaultConfig()
	return &Engine{
		entityID:        entityID,
		ruleFingerprint: BaselineRuleFingerprint,
		codeFingerprint: BaselineImplementationFingerprint,
		d01:             NewModel(entityID, cfg),
		context:         make([]contextRecord, 0, ContextLength),
		positionState:   domain.EmitterFlat,
	}
}

// CompletedCount returns the number of successfully committed observations.
func (e *Engine) CompletedCount() int { return e.completedCount }

// PositionState returns the emitter position after the last committed decision.
func (e *Engine) PositionState() domain.EmitterPosition { return e.positionState }

// StateHash returns the canonical hash of the committed D01 state.
func (e *Engine) StateHash() string { return e.d01.StateHash() }

// LastEval returns the last committed scientific evaluation summary.
func (e *Engine) LastEval() EvalSnapshot { return e.last }

// LastPipelineOutputs returns defensive copies of the last committed D01, D02,
// and D04 outputs.
func (e *Engine) LastPipelineOutputs() PipelineOutputs {
	out := e.lastPipeline
	out.DMO.ParameterState = cloneFloatMap(out.DMO.ParameterState)
	out.DMO.ParameterUpdateMagnitude = cloneFloatMap(out.DMO.ParameterUpdateMagnitude)
	out.FMO.Samples = slices.Clone(out.FMO.Samples)
	out.ReturnShape.ForwardSamples = slices.Clone(out.ReturnShape.ForwardSamples)
	out.Capturability.ReasonCodes = slices.Clone(out.Capturability.ReasonCodes)
	return out
}

func cloneFloatMap(source map[string]float64) map[string]float64 {
	if source == nil {
		return nil
	}
	out := make(map[string]float64, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func (e *Engine) clone() *Engine {
	out := *e
	out.d01 = e.d01.Clone()
	out.context = slices.Clone(e.context)
	if e.previousDecision != nil {
		side := *e.previousDecision
		out.previousDecision = &side
	}
	if e.lastSourceTime != nil {
		t := *e.lastSourceTime
		out.lastSourceTime = &t
	}
	return &out
}

func (e *Engine) adopt(src *Engine) {
	e.d01 = src.d01
	e.context = src.context
	e.positionState = src.positionState
	e.previousDecision = src.previousDecision
	e.completedCount = src.completedCount
	e.lastSourceTime = src.lastSourceTime
	e.eventSeq = src.eventSeq
	e.last = src.last
	e.lastPipeline = src.lastPipeline
}

// Step runs D01→D02→D04→decide on a copy and commits only if the full path succeeds.
func (e *Engine) Step(bar domain.Bar) domain.DecisionEvent {
	event, working, commit := e.PrepareStep(bar)
	if commit {
		e.Commit(working)
	}
	return event
}

// PrepareStep evaluates on a clone. The caller must Commit only if the deadline still holds.
func (e *Engine) PrepareStep(bar domain.Bar) (domain.DecisionEvent, *Engine, bool) {
	started := time.Now()
	e.eventSeq++
	working := e.clone()
	event, commit := working.stepLocked(bar, started)
	event.CompletedAt = time.Now()
	event.Latency = event.CompletedAt.Sub(started)
	if !commit {
		return event, nil, false
	}
	return event, working, true
}

// Commit atomically adopts a successfully prepared engine state.
func (e *Engine) Commit(working *Engine) {
	if working != nil {
		e.adopt(working)
	}
}

func (e *Engine) stepLocked(bar domain.Bar, started time.Time) (domain.DecisionEvent, bool) {
	eventID := fmt.Sprintf("%s:evt:%d", e.entityID, e.eventSeq)
	base := domain.DecisionEvent{
		EventID:          eventID,
		Symbol:           bar.Symbol,
		IntervalStart:    bar.IntervalStart,
		MarketSnapshotID: bar.MarketSnapshotID,
		SourceTimestamp:  bar.SourceTimestamp,
		ReceivedAt:       started,
		ModelVersion:     ModelVersionLabel,
		SchemaVersion:    SchemaVersion,
		PreStateHash:     e.d01.StateHash(),
		AcceptedSequence: e.completedCount,
	}

	obs := ObservationFromBar(bar, e.completedCount)
	if !finite(obs.Price) || !finite(obs.Volume) || !finite(obs.EventTime) {
		base.Skip = &domain.Skip{Reason: domain.SkipInvalidInput, Detail: "non-finite close, volume, or interval"}
		return base, false
	}
	if e.lastSourceTime != nil && obs.EventTime-*e.lastSourceTime <= 0 {
		base.Skip = &domain.Skip{Reason: domain.SkipDuplicateOrRegression, Detail: "source time must increase"}
		return base, false
	}

	dmo, fmo, err := e.d01.Step(obs)
	if err != nil {
		base.Skip = &domain.Skip{Reason: domain.SkipEngineError, Detail: err.Error()}
		return base, false
	}
	shape, err := BuildReturnShape(dmo, fmo)
	if err != nil {
		base.Skip = &domain.Skip{Reason: domain.SkipEngineError, Detail: err.Error()}
		return base, false
	}
	capture, err := EvaluateCapturability(shape, ProductionContext(obs.EventTime))
	if err != nil {
		base.Skip = &domain.Skip{Reason: domain.SkipEngineError, Detail: err.Error()}
		return base, false
	}
	if err := validateScores(capture, shape); err != nil {
		base.Skip = &domain.Skip{Reason: domain.SkipInvalidInput, Detail: err.Error()}
		return base, false
	}
	e.lastPipeline = PipelineOutputs{
		DMO:           dmo,
		FMO:           fmo,
		ReturnShape:   shape,
		Capturability: capture,
	}

	signalID := fmt.Sprintf("%s:sig:%d", e.entityID, e.completedCount+1)
	base.SignalID = signalID
	base.PostStateHash = e.d01.StateHash()

	record := contextRecord{
		C:               capture.CapturabilityScore,
		PathDirection:   shape.PathDirection,
		SourceTimestamp: bar.SourceTimestamp,
	}
	snap := EvalSnapshot{
		Path:                 shape.PathDirection,
		H:                    capture.HardEligibility,
		QG:                   capture.GeometryQuality,
		QS:                   capture.StructuralQuality,
		QR:                   capture.RiskQuality,
		C:                    capture.CapturabilityScore,
		Strength:             shape.Strength,
		Coherence:            shape.Coherence,
		Persistence:          shape.Persistence,
		Uncertainty:          shape.Uncertainty,
		Reversal:             shape.ReversalPropensity,
		TerminalDisplacement: shape.TerminalDisplacement,
		PositionAfter:        e.positionState,
	}

	if e.completedCount < ContextLength {
		base.Skip = &domain.Skip{
			Reason:      domain.SkipInitializing,
			Detail:      fmt.Sprintf("%d/%d", e.completedCount+1, ActionableAfter),
			ModelStatus: domain.StatusInitializing,
		}
		e.appendContext(record)
		e.completedCount++
		t := obs.EventTime
		e.lastSourceTime = &t
		e.last = snap
		return base, true
	}

	if len(e.context) != ContextLength {
		base.Skip = &domain.Skip{Reason: domain.SkipEngineError, Detail: "actionable context length is not 15"}
		return base, false
	}

	adaptive := adaptiveProperties(e.context, capture.CapturabilityScore)
	side, rulePath := decide(shape.PathDirection, capture.HardEligibility, capture.CapturabilityScore, adaptive)
	nextPosition := e.positionState
	switch side {
	case domain.SideBuy:
		nextPosition = domain.EmitterLong
	case domain.SideSell:
		nextPosition = domain.EmitterShort
	}

	decisionID := fmt.Sprintf("%s:dec:%d", e.entityID, e.completedCount+1)
	base.DecisionID = decisionID
	base.Decision = &domain.Decision{
		Side:                 side,
		Confidence:           capture.CapturabilityScore,
		H:                    capture.HardEligibility,
		QG:                   capture.GeometryQuality,
		QS:                   capture.StructuralQuality,
		QR:                   capture.RiskQuality,
		PathDirection:        shape.PathDirection,
		ModelStatus:          domain.StatusActionable,
		EmitterPosition:      nextPosition,
		RulePath:             rulePath,
		Strength:             shape.Strength,
		Coherence:            shape.Coherence,
		Persistence:          shape.Persistence,
		Uncertainty:          shape.Uncertainty,
		Reversal:             shape.ReversalPropensity,
		TerminalDisplacement: shape.TerminalDisplacement,
	}
	e.appendContext(record)
	e.positionState = nextPosition
	e.previousDecision = &side
	e.completedCount++
	t := obs.EventTime
	e.lastSourceTime = &t
	snap.PositionAfter = nextPosition
	e.last = snap
	return base, true
}

func (e *Engine) appendContext(record contextRecord) {
	e.context = append(e.context, record)
	if len(e.context) > ContextLength {
		e.context = e.context[len(e.context)-ContextLength:]
	}
}

func adaptiveProperties(context []contextRecord, currentC float64) AdaptiveProperties {
	cValues := make([]float64, len(context))
	up, down, flat, balance := 0, 0, 0, 0
	for i, item := range context {
		cValues[i] = item.C
		sign := directionSign[item.PathDirection]
		balance += sign
		switch {
		case sign > 0:
			up++
		case sign < 0:
			down++
		default:
			flat++
		}
	}
	sorted := slices.Clone(cValues)
	slices.Sort(sorted)
	return AdaptiveProperties{
		Prior15MedianC:   medianSorted(sorted),
		Prior15MinC:      sorted[0],
		Prior15MaxC:      sorted[len(sorted)-1],
		Prior15RangeC:    sorted[len(sorted)-1] - sorted[0],
		PriorC:           cValues[len(cValues)-1],
		DeltaC:           currentC - cValues[len(cValues)-1],
		UpCount:          up,
		DownCount:        down,
		FlatCount:        flat,
		DirectionBalance: balance,
	}
}

func medianSorted(sorted []float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

func decide(path domain.PathDirection, h int, cValue float64, adaptive AdaptiveProperties) (domain.Side, string) {
	qualityEligible := h == 1 && cValue >= adaptive.Prior15MedianC
	if path == domain.PathUpward && qualityEligible && adaptive.UpCount >= adaptive.DownCount {
		return domain.SideBuy, "UPWARD_AND_PRIOR_DIRECTION_AGREEMENT_AND_C_GE_PRIOR_MEDIAN"
	}
	if path == domain.PathDownward && qualityEligible && adaptive.DownCount >= adaptive.UpCount {
		return domain.SideSell, "DOWNWARD_AND_PRIOR_DIRECTION_AGREEMENT_AND_C_GE_PRIOR_MEDIAN"
	}
	return domain.SideHold, "AFFIRM_POSITION_STATE_TRANSITION_PREDICATE_NOT_SATISFIED"
}

func validateScores(capture CapturabilityResult, shape ReturnShape) error {
	for _, item := range []struct {
		name  string
		value float64
	}{
		{"C", capture.CapturabilityScore},
		{"Q_G", capture.GeometryQuality},
		{"Q_S", capture.StructuralQuality},
		{"Q_R", capture.RiskQuality},
		{"H", float64(capture.HardEligibility)},
		{"strength", shape.Strength},
		{"coherence", shape.Coherence},
		{"persistence", shape.Persistence},
		{"uncertainty", shape.Uncertainty},
		{"reversal", shape.ReversalPropensity},
	} {
		if !finite(item.value) {
			return fmt.Errorf("non-finite %s", item.name)
		}
	}
	if capture.HardEligibility != 0 && capture.HardEligibility != 1 {
		return fmt.Errorf("H out of domain")
	}
	if capture.CapturabilityScore < 0 || capture.CapturabilityScore > 1 {
		return fmt.Errorf("C out of [0,1]")
	}
	return nil
}
