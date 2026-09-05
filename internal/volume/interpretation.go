package volume

import "quantram/internal/domain"

// Frozen Volume Interpretation Mathematics (P-04V Phase D).
//
// Purpose: classify raw color from interval_mean_vn, apply two-observation
// confirmation/hysteresis, and attach phase/confidence/domain metadata.
//
// Scientific inputs: FeatureResult quantities already computed by Phase B/C
// (interval_mean_vn, V1, V2, predicted_next_V_N) plus prior InterpretationState.
//
// Scientific outputs: InterpretationResult and a candidate next
// InterpretationState. Features are not recalculated.
//
// Frozen identity: V_INTERVAL_B10_C2
//
//	state_source = INTERVAL_MEAN_V_N
//	lower = 0.9  (inclusive RED)
//	upper = 1.1  (inclusive GREEN)
//	confirmation = 2
//	epsilon = 1e-12 (phase only; not used on color thresholds)
//
// Confirmation (frozen APTF VolumeEngine.observe):
//
//	first valid raw color is accepted immediately (STABLE);
//	same raw as carried state.Color clears pending (STABLE);
//	a differing raw with pending_count < 2 emits Indicator AMBER,
//	writes next Color = AMBER (the emitted Indicator), and PENDING_{raw};
//	a second consecutive same pending raw that is still ≠ Color confirms
//	(CONFIRMED_{raw}, Color = raw);
//	a changed pending candidate resets count to 1.
//
// InterpretationState.Color is the carried Indicator, not an independent
// memory of the last raw that completed confirmation.
//
// Ownership: InterpretationState lives on VolumeState. Interpret copies prior
// and returns a candidate; it does not write committed state.
//
// Failure: FeatureInsufficient / FeatureUndefined activity produces no raw
// color and does not advance pending. Phase readiness is independent.
// Final INVALID cockpit interpretation is not emitted here.
//
// Causal invariants: activity source is interval_mean_vn, never
// predicted_next_V_N. No Price. No session reset. No color-age.
//
// Lifecycle: pure transformation for later Phase E prepare/commit.
//
// Concurrency: caller serializes per-entity State.
//
// Non-responsibilities: Engine, PrepareStep/Commit, proto, modelhost,
// P/V fusion, BUY/SELL/HOLD.

// InterpretationResult is one observation's interpretation without emission.
type InterpretationResult struct {
	Activity       Quantity
	Predicted      Quantity
	V1             Quantity
	V2             Quantity
	RawColor       string
	CockpitColor   string
	ConfirmedColor string
	PendingColor   string
	PendingCount   int
	Transition     string
	ColorStatus    FeatureStatus
	Phase          string
	PhaseStatus    FeatureStatus
	Confidence     string
	DomainState    string
}

// ColorAvailable reports a scientifically valid raw/cockpit interpretation.
func (r InterpretationResult) ColorAvailable() bool {
	return r.ColorStatus == FeatureAvailable && r.RawColor != domain.VolumeColorUnset
}

// Interpret applies frozen interpretation to prior state and current features.
//
// prior is copied by value. The returned next state is a candidate only.
func Interpret(features FeatureResult, prior InterpretationState) (InterpretationResult, InterpretationState) {
	out := InterpretationResult{
		Activity:    features.IntervalMean,
		Predicted:   features.Predicted,
		V1:          features.V1,
		V2:          features.V2,
		ColorStatus: features.IntervalMean.Status,
	}
	out.Phase, out.PhaseStatus = classifyPhaseQuantity(features.V1, features.V2)

	if !features.IntervalMean.Available() {
		return out, prior
	}

	raw := rawColor(features.IntervalMean.Value)
	cockpit, next, transition := confirmColor(raw, prior)
	out.RawColor = raw
	out.CockpitColor = cockpit
	out.ConfirmedColor = next.Color
	out.PendingColor = next.PendingColor
	out.PendingCount = next.PendingCount
	out.Transition = transition
	out.ColorStatus = FeatureAvailable
	out.Confidence = domain.VolumeConfidenceHigh
	out.DomainState = domain.VolumeDomainCausalLocal
	return out, next
}

// rawColor is the inclusive 0.9 / 1.1 band classifier.
func rawColor(activity float64) string {
	if activity >= UpperThreshold {
		return domain.VolumeColorGreen
	}
	if activity <= LowerThreshold {
		return domain.VolumeColorRed
	}
	return domain.VolumeColorAmber
}

// confirmColor is the frozen two-observation hysteresis machine.
//
// next.Color is the emitted Indicator (historical cockpit_color), matching
// HEAD spy_volume_engine.observe: VolumePolicyState.color = cockpit_color.
// While a transition is pending, that value is AMBER.
func confirmColor(raw string, prior InterpretationState) (cockpit string, next InterpretationState, transition string) {
	color := raw
	pendingColor := ""
	pendingCount := 0
	transition = "STABLE"
	if prior.Color != domain.VolumeColorUnset && raw != prior.Color {
		pendingCount = 1
		if prior.PendingColor == raw {
			pendingCount = prior.PendingCount + 1
		}
		if pendingCount < ConfirmationCount {
			color = domain.VolumeColorAmber
			pendingColor = raw
			transition = "PENDING_" + raw
		} else {
			transition = "CONFIRMED_" + raw
			// Frozen observe leaves pending_count at the confirming
			// increment and pending_color unset. The next observation
			// that differs starts count at 1 because pending_color is empty.
		}
	}
	next = InterpretationState{Color: color, PendingColor: pendingColor, PendingCount: pendingCount}
	return color, next, transition
}
