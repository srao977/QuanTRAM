package volume

import "quantram/internal/domain"

// P-04V Volume Engine lifecycle (Phase E).
//
// Purpose: compose Phase A–D science into one per-entity engine with
// explicit PrepareStep / Commit and a first-class Volume Output.
//
// Inputs: an immutable domain.Bar and the committed VolumeState.
//
// Outputs: domain.VolumeEvent plus a candidate Engine. Commit adopts the
// candidate. No proto, bus, or host publication.
//
// Scientific parameters: FrozenConfig only. Not environment knobs.
//
// State ownership: one Engine per entity owns one VolumeState (Feature +
// Interpretation). No cross-entity storage.
//
// Lifecycle:
//
//	PrepareStep clones committed state, advances the candidate once
//	(AdvanceFeatures + Interpret), and never writes the authoritative rings.
//	Commit adopts a candidate prepared from the current generation.
//	Reset clears Feature and Interpretation; the next observation matures
//	from empty. There is no session/date/provider/gap reset.
//
// Candidate: *Engine carrying cloned State and the generation at prepare
// time. A stale or already-committed candidate is rejected.
//
// Status:
//
//	MATURING  — required interpretation inputs are not yet positionally due
//	AVAILABLE — VN, V1, V2, interval_mean, predicted, and color are valid
//	INVALID   — a due calculation is scientifically undefined
//	ENGINE_ERROR — implementation cannot prepare (entity mismatch); no commit
//
// Scientific invalidity still records the causal position (NaN VN).
// InterpretationState advances only when Interpret produces a valid color.
//
// Concurrency: not safe for concurrent mutation. Future keyed workers
// serialize per entity. No internal goroutine or mutex.
//
// Non-responsibilities: modelhost, ingestion, Adaptive/Price science,
// proto, StageTransition, P/V fusion, color-age, session reset.

// Engine is the per-entity P-04V Volume Engine.
type Engine struct {
	cfg   Config
	state State
	gen   uint64
}

// NewEngine constructs a cold per-entity Volume Engine.
func NewEngine(entity string) (*Engine, error) {
	cfg := FrozenConfig(entity)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Engine{cfg: cfg, state: NewState(cfg.Entity)}, nil
}

// Entity is the owning symbol.
func (e *Engine) Entity() string {
	if e == nil {
		return ""
	}
	return e.cfg.Entity
}

// Snapshot returns a deep copy of committed VolumeState.
func (e *Engine) Snapshot() State {
	if e == nil {
		return State{}
	}
	return e.state.Clone()
}

// PrepareStep evaluates one Bar on a clone. It does not mutate committed state.
//
// commit is false only for engine/implementation failure (entity mismatch).
// MATURING and INVALID scientific results are committable candidates.
func (e *Engine) PrepareStep(bar domain.Bar) (domain.VolumeEvent, *Engine, bool) {
	if e == nil {
		return domain.VolumeEvent{Status: domain.VolumeStatusError, Reason: "NIL_ENGINE"}, nil, false
	}
	obs := ObservationFromBar(bar)
	if obs.Entity != e.cfg.Entity {
		ev := domain.VolumeEvent{
			Lineage: obs.Lineage,
			Status:  domain.VolumeStatusError,
			Reason:  "ENTITY_MISMATCH",
			VRaw:    domain.VolumeQuantity{Value: obs.VRaw, Status: domain.VolumeQtyAvailable},
		}
		return ev, nil, false
	}

	working := e.clone()
	features := working.state.AdvanceFeatures(obs)
	interp, nextInterp := Interpret(features, working.state.Interpretation)
	if interp.ColorAvailable() {
		working.state.Interpretation = nextInterp
	}
	event := assembleEvent(obs, features, interp, working.state)
	return event, working, true
}

// Commit adopts a candidate prepared from the current generation.
// A nil, stale, or already-consumed candidate is ignored.
func (e *Engine) Commit(working *Engine) {
	if e == nil || working == nil || working.gen != e.gen {
		return
	}
	e.state = working.state.Clone()
	e.gen++
}

// Reset clears Feature State and Interpretation State. Entity is kept.
func (e *Engine) Reset() {
	if e == nil {
		return
	}
	e.state.Reset()
	e.gen++
}

func (e *Engine) clone() *Engine {
	return &Engine{
		cfg:   e.cfg,
		state: e.state.Clone(),
		gen:   e.gen,
	}
}

func assembleEvent(obs Observation, features FeatureResult, interp InterpretationResult, candidate State) domain.VolumeEvent {
	status, reason := classifyOutput(features, candidate)
	ev := domain.VolumeEvent{
		Lineage:         obs.Lineage,
		Status:          status,
		Reason:          reason,
		VRaw:            domain.VolumeQuantity{Value: obs.VRaw, Status: domain.VolumeQtyAvailable},
		VN:              exportQty(features.VN),
		V1:              exportQty(features.V1),
		V2:              exportQty(features.V2),
		IntervalMeanVN:  exportQty(features.IntervalMean),
		PredictedNextVN: exportQty(features.Predicted),
	}
	if interp.ColorAvailable() {
		ev.RawColor = interp.RawColor
		ev.Indicator = interp.CockpitColor
		ev.Confidence = interp.Confidence
		ev.DomainState = interp.DomainState
		ev.Transition = interp.Transition
	}
	if interp.PhaseStatus == FeatureAvailable {
		ev.Phase = interp.Phase
	}
	return ev
}

func exportQty(q Quantity) domain.VolumeQuantity {
	switch q.Status {
	case FeatureAvailable:
		return domain.VolumeQuantity{Value: q.Value, Status: domain.VolumeQtyAvailable}
	case FeatureUndefined:
		return domain.VolumeQuantity{Value: q.Value, Status: domain.VolumeQtyUndefined}
	default:
		return domain.VolumeQuantity{Value: q.Value, Status: domain.VolumeQtyInsufficient}
	}
}

// classifyOutput maps feature readiness to engine status without a row counter.
func classifyOutput(features FeatureResult, candidate State) (status, reason string) {
	if features.VN.Available() &&
		features.V1.Available() &&
		features.V2.Available() &&
		features.IntervalMean.Available() &&
		features.Predicted.Available() {
		return domain.VolumeStatusAvailable, "AVAILABLE"
	}
	if features.VN.Status == FeatureUndefined {
		return domain.VolumeStatusInvalid, "UNDEFINED_VN"
	}
	if (features.V1.Status == FeatureUndefined || features.V2.Status == FeatureUndefined) &&
		lastNFinite(candidate.Feature.VN, DerivativeWindow) {
		return domain.VolumeStatusInvalid, "UNDEFINED_DERIVATIVE"
	}
	if features.IntervalMean.Status == FeatureUndefined &&
		lastNFinite(candidate.Feature.VN, IntervalMeanWindow) {
		return domain.VolumeStatusInvalid, "UNDEFINED_INTERVAL_MEAN"
	}
	return domain.VolumeStatusMaturing, "MATURING_FEATURES"
}

func lastNFinite(values []float64, n int) bool {
	if len(values) < n {
		return false
	}
	for _, v := range values[len(values)-n:] {
		if !finite(v) {
			return false
		}
	}
	return true
}
