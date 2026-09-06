package volume

import (
	"math"
	"path/filepath"
	"testing"

	"quantram/internal/domain"
)

func qty(value float64, status FeatureStatus) Quantity {
	if status != FeatureAvailable {
		return unavailable(status)
	}
	return Quantity{Value: value, Status: FeatureAvailable}
}

func colorFeatures(activity float64) FeatureResult {
	return FeatureResult{
		IntervalMean: qty(activity, FeatureAvailable),
		Predicted:    qty(activity, FeatureAvailable),
		V1:           qty(0, FeatureAvailable),
		V2:           qty(0, FeatureAvailable),
	}
}

func interpretColor(t *testing.T, activity float64, prior InterpretationState) (InterpretationResult, InterpretationState) {
	t.Helper()
	return Interpret(colorFeatures(activity), prior)
}

func requireColor(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s %q want %q", name, got, want)
	}
}

func TestD01RawGreenAboveThreshold(t *testing.T) {
	t.Log("invariant: activity > 1.1 → raw GREEN")
	if rawColor(1.1000001) != domain.VolumeColorGreen {
		t.Fatal(rawColor(1.1000001))
	}
}

func TestD02RawGreenExactUpperBoundary(t *testing.T) {
	t.Log("invariant: activity == 1.1 → GREEN (inclusive)")
	if rawColor(1.1) != domain.VolumeColorGreen {
		t.Fatal(rawColor(1.1))
	}
	if rawColor(UpperThreshold) != domain.VolumeColorGreen {
		t.Fatal("UpperThreshold is not inclusive GREEN")
	}
}

func TestD03RawRedBelowThreshold(t *testing.T) {
	t.Log("invariant: activity < 0.9 → raw RED")
	if rawColor(0.8999999) != domain.VolumeColorRed {
		t.Fatal(rawColor(0.8999999))
	}
}

func TestD04RawRedExactLowerBoundary(t *testing.T) {
	t.Log("invariant: activity == 0.9 → RED (inclusive)")
	if rawColor(0.9) != domain.VolumeColorRed {
		t.Fatal(rawColor(0.9))
	}
	if rawColor(LowerThreshold) != domain.VolumeColorRed {
		t.Fatal("LowerThreshold is not inclusive RED")
	}
}

func TestD05RawAmberInsideBand(t *testing.T) {
	t.Log("invariant: 0.9 < activity < 1.1 → AMBER")
	justBelowGreen := math.Nextafter(1.1, 0)
	justAboveRed := math.Nextafter(0.9, 2)
	if rawColor(justBelowGreen) != domain.VolumeColorAmber {
		t.Fatalf("1.1-ulp %g → %s", justBelowGreen, rawColor(justBelowGreen))
	}
	if rawColor(justAboveRed) != domain.VolumeColorAmber {
		t.Fatalf("0.9+ulp %g → %s", justAboveRed, rawColor(justAboveRed))
	}
	if rawColor(1.0) != domain.VolumeColorAmber {
		t.Fatal(rawColor(1.0))
	}
}

func TestD06InsufficientActivity(t *testing.T) {
	t.Log("invariant: FeatureInsufficient interval_mean_vn produces no raw color")
	res, next := Interpret(FeatureResult{IntervalMean: qty(0, FeatureInsufficient)}, InterpretationState{})
	requireStatus(t, "color", Quantity{Status: res.ColorStatus, Value: math.NaN()}, FeatureInsufficient)
	if res.RawColor != domain.VolumeColorUnset || res.ColorAvailable() {
		t.Fatalf("raw %q", res.RawColor)
	}
	if next != (InterpretationState{}) {
		t.Fatalf("state advanced: %+v", next)
	}
}

func TestD07UndefinedActivity(t *testing.T) {
	t.Log("invariant: FeatureUndefined interval_mean_vn produces no raw color")
	res, next := Interpret(FeatureResult{IntervalMean: qty(0, FeatureUndefined)}, InterpretationState{
		Color: domain.VolumeColorGreen,
	})
	if res.ColorStatus != FeatureUndefined || res.RawColor != domain.VolumeColorUnset {
		t.Fatalf("result %+v", res)
	}
	if next.Color != domain.VolumeColorGreen {
		t.Fatal("undefined activity must not rewrite confirmed color")
	}
}

func TestD08FirstValidGreenImmediate(t *testing.T) {
	t.Log("invariant: first valid GREEN confirms immediately")
	res, next := interpretColor(t, 1.2, InterpretationState{})
	requireColor(t, "raw", res.RawColor, domain.VolumeColorGreen)
	requireColor(t, "confirmed", next.Color, domain.VolumeColorGreen)
	requireColor(t, "cockpit", res.CockpitColor, domain.VolumeColorGreen)
	if next.PendingCount != 0 || next.PendingColor != domain.VolumeColorUnset {
		t.Fatalf("pending %+v", next)
	}
	if res.Transition != "STABLE" {
		t.Fatalf("transition %q", res.Transition)
	}
}

func TestD09FirstValidRedImmediate(t *testing.T) {
	t.Log("invariant: first valid RED confirms immediately")
	res, next := interpretColor(t, 0.5, InterpretationState{})
	requireColor(t, "confirmed", next.Color, domain.VolumeColorRed)
	requireColor(t, "cockpit", res.CockpitColor, domain.VolumeColorRed)
}

func TestD10FirstValidAmberImmediate(t *testing.T) {
	t.Log("invariant: first valid AMBER confirms immediately")
	res, next := interpretColor(t, 1.0, InterpretationState{})
	requireColor(t, "confirmed", next.Color, domain.VolumeColorAmber)
	requireColor(t, "cockpit", res.CockpitColor, domain.VolumeColorAmber)
	if next.PendingCount != 0 {
		t.Fatal("first AMBER must not be pending")
	}
}

func TestD11SameRawClearsPending(t *testing.T) {
	t.Log("invariant: raw matching carried state.Color clears pending")
	// Constructed prior (Color still GREEN with a leftover pending) is the
	// superseded Phase-D representation. Frozen compare is raw == Color.
	prior := InterpretationState{Color: domain.VolumeColorGreen, PendingColor: domain.VolumeColorRed, PendingCount: 1}
	res, next := interpretColor(t, 1.2, prior)
	requireColor(t, "carried", next.Color, domain.VolumeColorGreen)
	if next.PendingColor != domain.VolumeColorUnset || next.PendingCount != 0 {
		t.Fatalf("pending not cleared: %+v", next)
	}
	requireColor(t, "cockpit", res.CockpitColor, domain.VolumeColorGreen)
}

func TestD12FirstDifferingRawBeginsPending(t *testing.T) {
	t.Log("invariant: first differing raw starts pending count 1 and writes Indicator AMBER into Color")
	res, next := interpretColor(t, 0.5, InterpretationState{Color: domain.VolumeColorGreen})
	requireColor(t, "carried", next.Color, domain.VolumeColorAmber)
	requireColor(t, "pending", next.PendingColor, domain.VolumeColorRed)
	if next.PendingCount != 1 {
		t.Fatalf("count %d", next.PendingCount)
	}
	requireColor(t, "cockpit", res.CockpitColor, domain.VolumeColorAmber)
	if res.Transition != "PENDING_RED" {
		t.Fatalf("transition %q", res.Transition)
	}
}

func TestD13SecondSameDifferingRawConfirms(t *testing.T) {
	t.Log("invariant: second consecutive differing raw confirms the transition")
	_, mid := interpretColor(t, 0.5, InterpretationState{Color: domain.VolumeColorGreen})
	res, next := interpretColor(t, 0.4, mid)
	requireColor(t, "confirmed", next.Color, domain.VolumeColorRed)
	if next.PendingColor != domain.VolumeColorUnset || next.PendingCount != 2 {
		t.Fatalf("frozen confirm leaves pending_count=2 and pending_color empty: %+v", next)
	}
	requireColor(t, "cockpit", res.CockpitColor, domain.VolumeColorRed)
	if res.Transition != "CONFIRMED_RED" {
		t.Fatalf("transition %q", res.Transition)
	}
}

func TestD14PendingCandidateChangeResetsCount(t *testing.T) {
	t.Log("invariant: after pending writes AMBER into Color, a new raw ≠ AMBER starts a new pending candidate")
	// Superseded Phase-D expectation: GREEN+RED then AMBER kept Color=GREEN
	// and switched pending to AMBER. Frozen: GREEN+RED writes Color=AMBER,
	// so the candidate-change case is GREEN → RED → GREEN.
	_, mid := interpretColor(t, 0.5, InterpretationState{Color: domain.VolumeColorGreen})
	res, next := interpretColor(t, 1.2, mid)
	requireColor(t, "carried", next.Color, domain.VolumeColorAmber)
	requireColor(t, "pending", next.PendingColor, domain.VolumeColorGreen)
	if next.PendingCount != 1 {
		t.Fatalf("count %d want 1 (not a majority vote)", next.PendingCount)
	}
	requireColor(t, "cockpit", res.CockpitColor, domain.VolumeColorAmber)
	if res.Transition != "PENDING_GREEN" {
		t.Fatalf("transition %q", res.Transition)
	}
}

func TestD15ReturnToPriorRawAfterPendingAmber(t *testing.T) {
	t.Log("invariant: GREEN, RED, GREEN does not immediately restore GREEN; it is PENDING_GREEN")
	// Superseded Phase-D: return to confirmed GREEN / STABLE because Color
	// still held GREEN. Frozen APTF wrote AMBER into Color on the first
	// pending step, so the return GREEN is a new pending candidate.
	_, mid := interpretColor(t, 0.5, InterpretationState{Color: domain.VolumeColorGreen})
	res, next := interpretColor(t, 1.2, mid)
	requireColor(t, "carried", next.Color, domain.VolumeColorAmber)
	requireColor(t, "pending", next.PendingColor, domain.VolumeColorGreen)
	if next.PendingCount != 1 {
		t.Fatalf("pending %+v", next)
	}
	requireColor(t, "cockpit", res.CockpitColor, domain.VolumeColorAmber)
	if res.Transition != "PENDING_GREEN" {
		t.Fatalf("transition %q", res.Transition)
	}
}

func TestD16PendingCockpitForcedAmber(t *testing.T) {
	t.Log("invariant: while a GREEN→RED transition is pending, cockpit is AMBER")
	res, _ := interpretColor(t, 0.5, InterpretationState{Color: domain.VolumeColorGreen})
	requireColor(t, "raw", res.RawColor, domain.VolumeColorRed)
	requireColor(t, "cockpit", res.CockpitColor, domain.VolumeColorAmber)
}

func TestD17ConfirmedAmberVsPendingAmber(t *testing.T) {
	t.Log("invariant: pending AMBER writes Color=AMBER; next same AMBER is STABLE, not CONFIRMED_AMBER")
	// Superseded Phase-D: pending Color stayed GREEN; second AMBER was
	// CONFIRMED_AMBER. Frozen: first pending writes Color=AMBER, so the
	// next AMBER matches Color and is STABLE.
	pendingRes, pending := interpretColor(t, 1.0, InterpretationState{Color: domain.VolumeColorGreen})
	requireColor(t, "pending-cockpit", pendingRes.CockpitColor, domain.VolumeColorAmber)
	requireColor(t, "pending-carried", pending.Color, domain.VolumeColorAmber)
	requireColor(t, "pending-candidate", pending.PendingColor, domain.VolumeColorAmber)
	if pending.PendingCount != 1 {
		t.Fatal("expected pending AMBER count 1")
	}
	if pendingRes.Transition != "PENDING_AMBER" {
		t.Fatalf("transition %q", pendingRes.Transition)
	}

	stableRes, stable := interpretColor(t, 1.0, pending)
	requireColor(t, "stable-cockpit", stableRes.CockpitColor, domain.VolumeColorAmber)
	requireColor(t, "stable-color", stable.Color, domain.VolumeColorAmber)
	if stable.PendingCount != 0 || stable.PendingColor != domain.VolumeColorUnset {
		t.Fatalf("second AMBER should clear pending: %+v", stable)
	}
	if stableRes.Transition != "STABLE" {
		t.Fatalf("second AMBER transition %q want STABLE", stableRes.Transition)
	}
	if pending.PendingCount == stable.PendingCount {
		t.Fatal("pending AMBER and subsequent STABLE AMBER must differ in pending fields")
	}
}

func TestD18UnavailableDoesNotAdvancePending(t *testing.T) {
	t.Log("invariant: unavailable activity does not increment or promote pending")
	prior := InterpretationState{Color: domain.VolumeColorGreen, PendingColor: domain.VolumeColorRed, PendingCount: 1}
	_, nextUndef := Interpret(FeatureResult{IntervalMean: qty(0, FeatureUndefined)}, prior)
	_, nextInsuff := Interpret(FeatureResult{IntervalMean: qty(0, FeatureInsufficient)}, prior)
	if nextUndef != prior || nextInsuff != prior {
		t.Fatalf("pending advanced undef=%+v insuff=%+v", nextUndef, nextInsuff)
	}
}

func TestD19StationaryAtZero(t *testing.T) {
	t.Log("invariant: V1==0 → ACTIVITY_STATIONARY")
	if classifyPhase(0, 9) != domain.VolumePhaseStationary {
		t.Fatal(classifyPhase(0, 9))
	}
}

func TestD20StationaryAtPlusEpsilon(t *testing.T) {
	t.Log("invariant: V1==+ε → STATIONARY")
	if classifyPhase(Epsilon, 1) != domain.VolumePhaseStationary {
		t.Fatal(classifyPhase(Epsilon, 1))
	}
}

func TestD21StationaryAtMinusEpsilon(t *testing.T) {
	t.Log("invariant: V1==-ε → STATIONARY")
	if classifyPhase(-Epsilon, -1) != domain.VolumePhaseStationary {
		t.Fatal(classifyPhase(-Epsilon, -1))
	}
}

func TestD22IncreasingAccelerating(t *testing.T) {
	t.Log("invariant: V1>ε and V2>ε → INCREASING_ACCELERATING")
	outside := math.Nextafter(Epsilon, 2)
	if classifyPhase(outside, outside) != domain.VolumePhaseIncreasingAccelerating {
		t.Fatal(classifyPhase(outside, outside))
	}
}

func TestD23IncreasingDecelerating(t *testing.T) {
	t.Log("invariant: V1>ε and V2<=ε → INCREASING_DECELERATING")
	v1 := math.Nextafter(Epsilon, 2)
	if classifyPhase(v1, Epsilon) != domain.VolumePhaseIncreasingDecelerating {
		t.Fatal(classifyPhase(v1, Epsilon))
	}
	if classifyPhase(v1, 0) != domain.VolumePhaseIncreasingDecelerating {
		t.Fatal(classifyPhase(v1, 0))
	}
}

func TestD24DecreasingAccelerating(t *testing.T) {
	t.Log("invariant: V1<-ε and V2<-ε → DECREASING_ACCELERATING")
	v1 := math.Nextafter(-Epsilon, -2)
	v2 := math.Nextafter(-Epsilon, -2)
	if classifyPhase(v1, v2) != domain.VolumePhaseDecreasingAccelerating {
		t.Fatal(classifyPhase(v1, v2))
	}
}

func TestD25DecreasingDecelerating(t *testing.T) {
	t.Log("invariant: V1<-ε and V2>=-ε → DECREASING_DECELERATING")
	v1 := math.Nextafter(-Epsilon, -2)
	if classifyPhase(v1, -Epsilon) != domain.VolumePhaseDecreasingDecelerating {
		t.Fatal(classifyPhase(v1, -Epsilon))
	}
	if classifyPhase(v1, 0) != domain.VolumePhaseDecreasingDecelerating {
		t.Fatal(classifyPhase(v1, 0))
	}
}

func TestD26PhaseUnavailableWhenV1Insufficient(t *testing.T) {
	t.Log("invariant: insufficient V1 → phase FeatureInsufficient, not STATIONARY")
	label, status := classifyPhaseQuantity(qty(0, FeatureInsufficient), qty(1, FeatureAvailable))
	if status != FeatureInsufficient || label != domain.VolumePhaseUnset {
		t.Fatalf("phase %q status %d", label, status)
	}
}

func TestD27PhaseUnavailableWhenV2Insufficient(t *testing.T) {
	t.Log("invariant: insufficient V2 → phase FeatureInsufficient")
	_, status := classifyPhaseQuantity(qty(1, FeatureAvailable), qty(0, FeatureInsufficient))
	if status != FeatureInsufficient {
		t.Fatalf("status %d", status)
	}
}

func TestD28PhaseUndefinedWhenV1Undefined(t *testing.T) {
	t.Log("invariant: undefined V1 → phase FeatureUndefined, not STATIONARY")
	label, status := classifyPhaseQuantity(qty(0, FeatureUndefined), qty(1, FeatureAvailable))
	if status != FeatureUndefined || label != domain.VolumePhaseUnset {
		t.Fatalf("phase %q status %d", label, status)
	}
}

func TestD29PhaseUndefinedWhenV2Undefined(t *testing.T) {
	t.Log("invariant: undefined V2 → phase FeatureUndefined")
	_, status := classifyPhaseQuantity(qty(1, FeatureAvailable), qty(0, FeatureUndefined))
	if status != FeatureUndefined {
		t.Fatalf("status %d", status)
	}
}

func TestD30ConfidenceHighForValidIntervalMean(t *testing.T) {
	t.Log("invariant: valid INTERVAL_MEAN_V_N interpretation emits confidence HIGH")
	res, _ := interpretColor(t, 1.2, InterpretationState{})
	if res.Confidence != domain.VolumeConfidenceHigh {
		t.Fatalf("confidence %q", res.Confidence)
	}
	bad, _ := Interpret(FeatureResult{IntervalMean: qty(0, FeatureInsufficient)}, InterpretationState{})
	if bad.Confidence != domain.VolumeConfidenceUnset {
		t.Fatalf("immature interpretation must not emit HIGH: %q", bad.Confidence)
	}
}

func TestD31DomainCausalLocalVolume(t *testing.T) {
	t.Log("invariant: valid interpretation domain is CAUSAL_LOCAL_VOLUME")
	res, _ := interpretColor(t, 1.2, InterpretationState{})
	if res.DomainState != domain.VolumeDomainCausalLocal {
		t.Fatalf("domain %q", res.DomainState)
	}
	if res.DomainState == "control" {
		t.Fatal("domain must not drive control flow")
	}
}

func TestD32PredictedDoesNotDriveRawColor(t *testing.T) {
	t.Log("invariant: predicted_next_V_N is not the color activity source")
	features := FeatureResult{
		IntervalMean: qty(1.2, FeatureAvailable),
		Predicted:    qty(0.5, FeatureAvailable),
		V1:           qty(0, FeatureAvailable),
		V2:           qty(0, FeatureAvailable),
	}
	res, _ := Interpret(features, InterpretationState{})
	requireColor(t, "raw", res.RawColor, domain.VolumeColorGreen)
	if res.Predicted.Value != 0.5 {
		t.Fatal("predicted should be carried, not used as activity")
	}
}

func TestD33CandidateDoesNotMutateCommitted(t *testing.T) {
	t.Log("invariant: Interpret returns a candidate and does not alias committed InterpretationState")
	committed := NewState("SYM1")
	committed.Interpretation = InterpretationState{Color: domain.VolumeColorGreen}
	prior := committed.Interpretation
	_, candidate := interpretColor(t, 0.5, prior)
	if committed.Interpretation.PendingCount != 0 || committed.Interpretation.Color != domain.VolumeColorGreen {
		t.Fatal("committed interpretation mutated")
	}
	if candidate.PendingCount != 1 {
		t.Fatal("candidate was not produced")
	}
	committed.Interpretation.PendingCount = 99
	if candidate.PendingCount != 1 {
		t.Fatal("candidate aliased committed pending count")
	}
}

func TestD34PhaseABCStillPass(t *testing.T) {
	t.Log("invariant: Phase B 1..15 identity remains after Phase D")
	_, results := advanceSeq(t, uintRange(1, 15))
	last := results[14]
	if last.Median.Value != 8 || last.VN.Value != 15.0/8.0 {
		t.Fatalf("Phase B identity broken median=%g vn=%g", last.Median.Value, last.VN.Value)
	}
}

func TestD35NoPricingImport(t *testing.T) {
	t.Log("invariant: internal/volume must not import internal/pricing")
	assertNoProductionImport(t, packageDir(t), `"quantram/internal/pricing"`)
}

func TestD36NoAdaptiveImport(t *testing.T) {
	t.Log("invariant: internal/volume must not import Adaptive implementation")
	assertNoProductionImport(t, packageDir(t), `"quantram/internal/adaptive"`)
}

func TestD37NoRuntimeIntegration(t *testing.T) {
	t.Log("invariant: ingestion must not import Phase D Volume code")
	root := repoRoot(t)
	assertNoImport(t, filepath.Join(root, "internal", "ingestion"), `"quantram/internal/volume"`, false)
}

// Frozen APTF confirmation sequences. Expected next.Color is the emitted
// Indicator (historical cockpit_color), not a retained prior raw.
func TestFRConfirmationSequences(t *testing.T) {
	t.Log("Phase F-R: table reconstructed from HEAD spy_volume_engine.observe")
	type step struct {
		activity   float64
		raw        string
		indicator  string
		transition string
		color      string
		pending    string
		count      int
	}
	cases := []struct {
		id    string
		steps []step
	}{
		{"R01 first valid GREEN", []step{
			{1.2, domain.VolumeColorGreen, domain.VolumeColorGreen, "STABLE", domain.VolumeColorGreen, "", 0},
		}},
		{"R02 first valid AMBER", []step{
			{1.0, domain.VolumeColorAmber, domain.VolumeColorAmber, "STABLE", domain.VolumeColorAmber, "", 0},
		}},
		{"R03 first valid RED", []step{
			{0.5, domain.VolumeColorRed, domain.VolumeColorRed, "STABLE", domain.VolumeColorRed, "", 0},
		}},
		{"R04 stable GREEN", []step{
			{1.2, domain.VolumeColorGreen, domain.VolumeColorGreen, "STABLE", domain.VolumeColorGreen, "", 0},
			{1.3, domain.VolumeColorGreen, domain.VolumeColorGreen, "STABLE", domain.VolumeColorGreen, "", 0},
		}},
		{"R05 stable RED", []step{
			{0.5, domain.VolumeColorRed, domain.VolumeColorRed, "STABLE", domain.VolumeColorRed, "", 0},
			{0.4, domain.VolumeColorRed, domain.VolumeColorRed, "STABLE", domain.VolumeColorRed, "", 0},
		}},
		{"R06 stable AMBER", []step{
			{1.0, domain.VolumeColorAmber, domain.VolumeColorAmber, "STABLE", domain.VolumeColorAmber, "", 0},
			{1.05, domain.VolumeColorAmber, domain.VolumeColorAmber, "STABLE", domain.VolumeColorAmber, "", 0},
		}},
		{"R07 first GREEN transition candidate", []step{
			{0.5, domain.VolumeColorRed, domain.VolumeColorRed, "STABLE", domain.VolumeColorRed, "", 0},
			{1.2, domain.VolumeColorGreen, domain.VolumeColorAmber, "PENDING_GREEN", domain.VolumeColorAmber, domain.VolumeColorGreen, 1},
		}},
		{"R08 second GREEN confirmation", []step{
			{0.5, domain.VolumeColorRed, domain.VolumeColorRed, "STABLE", domain.VolumeColorRed, "", 0},
			{1.2, domain.VolumeColorGreen, domain.VolumeColorAmber, "PENDING_GREEN", domain.VolumeColorAmber, domain.VolumeColorGreen, 1},
			{1.3, domain.VolumeColorGreen, domain.VolumeColorGreen, "CONFIRMED_GREEN", domain.VolumeColorGreen, "", 2},
		}},
		{"R09 first RED transition candidate", []step{
			{1.2, domain.VolumeColorGreen, domain.VolumeColorGreen, "STABLE", domain.VolumeColorGreen, "", 0},
			{0.5, domain.VolumeColorRed, domain.VolumeColorAmber, "PENDING_RED", domain.VolumeColorAmber, domain.VolumeColorRed, 1},
		}},
		{"R10 second RED confirmation", []step{
			{1.2, domain.VolumeColorGreen, domain.VolumeColorGreen, "STABLE", domain.VolumeColorGreen, "", 0},
			{0.5, domain.VolumeColorRed, domain.VolumeColorAmber, "PENDING_RED", domain.VolumeColorAmber, domain.VolumeColorRed, 1},
			{0.4, domain.VolumeColorRed, domain.VolumeColorRed, "CONFIRMED_RED", domain.VolumeColorRed, "", 2},
		}},
		{"R11 candidate changes before confirmation", []step{
			{1.2, domain.VolumeColorGreen, domain.VolumeColorGreen, "STABLE", domain.VolumeColorGreen, "", 0},
			{0.5, domain.VolumeColorRed, domain.VolumeColorAmber, "PENDING_RED", domain.VolumeColorAmber, domain.VolumeColorRed, 1},
			{1.3, domain.VolumeColorGreen, domain.VolumeColorAmber, "PENDING_GREEN", domain.VolumeColorAmber, domain.VolumeColorGreen, 1},
		}},
		{"R12 pending AMBER writes AMBER into Color", []step{
			{1.2, domain.VolumeColorGreen, domain.VolumeColorGreen, "STABLE", domain.VolumeColorGreen, "", 0},
			{1.0, domain.VolumeColorAmber, domain.VolumeColorAmber, "PENDING_AMBER", domain.VolumeColorAmber, domain.VolumeColorAmber, 1},
		}},
		{"R13 return to prior raw after pending AMBER", []step{
			{1.2, domain.VolumeColorGreen, domain.VolumeColorGreen, "STABLE", domain.VolumeColorGreen, "", 0},
			{1.0, domain.VolumeColorAmber, domain.VolumeColorAmber, "PENDING_AMBER", domain.VolumeColorAmber, domain.VolumeColorAmber, 1},
			{1.2, domain.VolumeColorGreen, domain.VolumeColorAmber, "PENDING_GREEN", domain.VolumeColorAmber, domain.VolumeColorGreen, 1},
		}},
		{"R14 confirmed AMBER vs pending AMBER", []step{
			{1.0, domain.VolumeColorAmber, domain.VolumeColorAmber, "STABLE", domain.VolumeColorAmber, "", 0},
			{1.2, domain.VolumeColorGreen, domain.VolumeColorAmber, "PENDING_GREEN", domain.VolumeColorAmber, domain.VolumeColorGreen, 1},
			{1.0, domain.VolumeColorAmber, domain.VolumeColorAmber, "STABLE", domain.VolumeColorAmber, "", 0},
		}},
		{"R15 first Phase-F mismatch 2023-03-30T11:16:00Z", []step{
			{1.2, domain.VolumeColorGreen, domain.VolumeColorGreen, "STABLE", domain.VolumeColorGreen, "", 0},
			{1.063455419405814, domain.VolumeColorAmber, domain.VolumeColorAmber, "PENDING_AMBER", domain.VolumeColorAmber, domain.VolumeColorAmber, 1},
			{1.1057924296925286, domain.VolumeColorGreen, domain.VolumeColorAmber, "PENDING_GREEN", domain.VolumeColorAmber, domain.VolumeColorGreen, 1},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			var prior InterpretationState
			for i, st := range tc.steps {
				res, next := interpretColor(t, st.activity, prior)
				if res.RawColor != st.raw || res.CockpitColor != st.indicator || res.Transition != st.transition {
					t.Fatalf("step %d emit raw=%s Indicator=%s trans=%s want raw=%s Indicator=%s trans=%s",
						i, res.RawColor, res.CockpitColor, res.Transition, st.raw, st.indicator, st.transition)
				}
				if next.Color != st.color || next.PendingColor != st.pending || next.PendingCount != st.count {
					t.Fatalf("step %d state %+v want Color=%s Pending=%s count=%d",
						i, next, st.color, st.pending, st.count)
				}
				prior = next
			}
		})
	}
}

func TestFR15ExactFirstPhaseFMismatch(t *testing.T) {
	t.Log("regression: 2023-03-30T11:16:00Z frozen Indicator AMBER / PENDING_GREEN")
	_, afterGreen := interpretColor(t, 1.2, InterpretationState{})
	_, afterAmber := interpretColor(t, 1.063455419405814, afterGreen)
	res, next := interpretColor(t, 1.1057924296925286, afterAmber)
	requireColor(t, "raw", res.RawColor, domain.VolumeColorGreen)
	requireColor(t, "Indicator", res.CockpitColor, domain.VolumeColorAmber)
	if res.Transition != "PENDING_GREEN" {
		t.Fatalf("transition %q", res.Transition)
	}
	requireColor(t, "carried Color", next.Color, domain.VolumeColorAmber)
	requireColor(t, "pending", next.PendingColor, domain.VolumeColorGreen)
	if next.PendingCount != 1 {
		t.Fatalf("count %d", next.PendingCount)
	}
}
