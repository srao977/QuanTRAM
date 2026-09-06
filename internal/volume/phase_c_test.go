package volume

import (
	"math"
	"path/filepath"
	"testing"
	"time"
)

// derivativeTol is the Phase C comparison for exact low-degree synthetics.
//
// Rationale: a 3×3 full-rank Vandermonde at minute-scale τ is well-conditioned.
// gonum SVD on these cases recovers coefficients near double precision.
// 1e-12 fails if V1/V2 are swapped, if a positional [-2,-1,0] grid is used
// instead of actual elapsed minutes, or if a lower-order fit is substituted.
// It is not a bitwise-NumPy claim; Phase F owns corpus equivalence.
const derivativeTol = 1e-12

func clockAt(hour, min, sec int) time.Time {
	return time.Date(2026, 9, 5, hour, min, sec, 0, time.UTC)
}

func requireAlmost(t *testing.T, name string, got, want float64) {
	t.Helper()
	if !finite(got) || math.Abs(got-want) > derivativeTol {
		t.Fatalf("%s %g want %g (tol %g)", name, got, want, derivativeTol)
	}
}

func knownQuadratic(tau float64) float64 {
	return 2*tau*tau + 3*tau + 5
}

func TestC01InsufficientPositionalObservations(t *testing.T) {
	t.Log("invariant: 0/1/2 V_N positions → V1/V2 FeatureInsufficient")
	empty := derivativesFromState(nil, nil)
	requireStatus(t, "v1-0", empty.V1, FeatureInsufficient)
	requireStatus(t, "v2-0", empty.V2, FeatureInsufficient)

	oneT := []time.Time{clockAt(10, 0, 0)}
	one := derivativesFromState([]float64{1.0}, oneT)
	requireStatus(t, "v1-1", one.V1, FeatureInsufficient)
	requireStatus(t, "v2-1", one.V2, FeatureInsufficient)

	twoT := []time.Time{clockAt(10, 0, 0), clockAt(10, 1, 0)}
	two := derivativesFromState([]float64{1.0, 1.1}, twoT)
	requireStatus(t, "v1-2", two.V1, FeatureInsufficient)
	requireStatus(t, "v2-2", two.V2, FeatureInsufficient)
}

func TestC02ThreeFiniteObservations(t *testing.T) {
	t.Log("invariant: three finite V_N with distinct times → V1/V2 available")
	vn := []float64{knownQuadratic(-2), knownQuadratic(-1), knownQuadratic(0)}
	times := []time.Time{clockAt(10, 0, 0), clockAt(10, 1, 0), clockAt(10, 2, 0)}
	got := derivativesFromState(vn, times)
	if !got.V1.Available() || !got.V2.Available() {
		t.Fatalf("expected available V1/V2: %+v", got)
	}

	_, results := advanceSeq(t, uintRange(1, 17))
	last := results[16]
	if !last.V1.Available() || !last.V2.Available() {
		t.Fatalf("AdvanceFeatures after 17 regular obs should yield V1/V2: v1=%+v v2=%+v", last.V1, last.V2)
	}
}

func TestC03KnownConstantQuadratic(t *testing.T) {
	t.Log("invariant: y=2τ²+3τ+5 → a=2, b=3, V1=3, V2=4")
	vn := []float64{knownQuadratic(-2), knownQuadratic(-1), knownQuadratic(0)}
	times := []time.Time{clockAt(10, 0, 0), clockAt(10, 1, 0), clockAt(10, 2, 0)}
	got := derivativesFromState(vn, times)
	requireAlmost(t, "V1", got.V1.Value, 3)
	requireAlmost(t, "V2", got.V2.Value, 4)
}

func TestC04KnownLinearCase(t *testing.T) {
	t.Log("invariant: y=4τ+7 → a≈0, V1≈4, V2≈0")
	linear := func(tau float64) float64 { return 4*tau + 7 }
	vn := []float64{linear(-2), linear(-1), linear(0)}
	times := []time.Time{clockAt(10, 0, 0), clockAt(10, 1, 0), clockAt(10, 2, 0)}
	got := derivativesFromState(vn, times)
	requireAlmost(t, "V1", got.V1.Value, 4)
	requireAlmost(t, "V2", got.V2.Value, 0)
}

func TestC05KnownConstantCase(t *testing.T) {
	t.Log("invariant: y=2.5 → V1≈0, V2≈0")
	vn := []float64{2.5, 2.5, 2.5}
	times := []time.Time{clockAt(10, 0, 0), clockAt(10, 1, 0), clockAt(10, 2, 0)}
	got := derivativesFromState(vn, times)
	requireAlmost(t, "V1", got.V1.Value, 0)
	requireAlmost(t, "V2", got.V2.Value, 0)
}

func TestC06CurrentTimeIsZero(t *testing.T) {
	t.Log("invariant: latest positional observation is assigned τ=0")
	times := []time.Time{clockAt(10, 0, 0), clockAt(10, 1, 0), clockAt(10, 4, 0)}
	tau, ok := elapsedMinutes(times)
	if !ok {
		t.Fatal("elapsed minutes must be available")
	}
	if tau[2] != 0 {
		t.Fatalf("τ_current %g want 0", tau[2])
	}
	if tau[0] >= 0 || tau[1] >= 0 {
		t.Fatalf("prior τ must be negative: %v", tau)
	}
}

func TestC07ActualIrregularElapsedTime(t *testing.T) {
	t.Log("invariant: 10:00,10:01,10:04 → τ=[-4,-3,0], not positional [-2,-1,0]")
	times := []time.Time{clockAt(10, 0, 0), clockAt(10, 1, 0), clockAt(10, 4, 0)}
	tau, ok := elapsedMinutes(times)
	if !ok {
		t.Fatal("elapsed minutes must be available")
	}
	if tau[0] != -4 || tau[1] != -3 || tau[2] != 0 {
		t.Fatalf("τ %v want [-4,-3,0]", tau)
	}
	vn := []float64{knownQuadratic(-4), knownQuadratic(-3), knownQuadratic(0)}
	got := derivativesFromState(vn, times)
	requireAlmost(t, "V1", got.V1.Value, 3)
	requireAlmost(t, "V2", got.V2.Value, 4)
	// Positional [-2,-1,0] on these y values cannot recover V1=3, V2=4.
}

func TestC08StrongIrregularElapsedTime(t *testing.T) {
	t.Log("invariant: 10:00,10:03,10:10 → τ=[-10,-7,0], not [-2,-1,0]")
	times := []time.Time{clockAt(10, 0, 0), clockAt(10, 3, 0), clockAt(10, 10, 0)}
	tau, ok := elapsedMinutes(times)
	if !ok {
		t.Fatal("elapsed minutes must be available")
	}
	if tau[0] != -10 || tau[1] != -7 || tau[2] != 0 {
		t.Fatalf("τ %v want [-10,-7,0]", tau)
	}
	vn := []float64{knownQuadratic(-10), knownQuadratic(-7), knownQuadratic(0)}
	got := derivativesFromState(vn, times)
	requireAlmost(t, "V1", got.V1.Value, 3)
	requireAlmost(t, "V2", got.V2.Value, 4)
}

func TestC09FractionalMinuteSpacing(t *testing.T) {
	t.Log("invariant: non-integer minute gaps are not truncated")
	times := []time.Time{clockAt(10, 0, 0), clockAt(10, 1, 30), clockAt(10, 4, 0)}
	tau, ok := elapsedMinutes(times)
	if !ok {
		t.Fatal("elapsed minutes must be available")
	}
	if tau[0] != -4 || tau[1] != -2.5 || tau[2] != 0 {
		t.Fatalf("τ %v want [-4,-2.5,0]", tau)
	}
	vn := []float64{knownQuadratic(-4), knownQuadratic(-2.5), knownQuadratic(0)}
	got := derivativesFromState(vn, times)
	requireAlmost(t, "V1", got.V1.Value, 3)
	requireAlmost(t, "V2", got.V2.Value, 4)
}

func TestC10PositionalNaN(t *testing.T) {
	t.Log("invariant: last three [finite,NaN,finite] → FeatureUndefined; no skip-back")
	times := []time.Time{clockAt(10, 0, 0), clockAt(10, 1, 0), clockAt(10, 2, 0)}
	got := derivativesFromState([]float64{1.1, math.NaN(), 1.2}, times)
	requireStatus(t, "v1", got.V1, FeatureUndefined)
	requireStatus(t, "v2", got.V2, FeatureUndefined)
}

func TestC11PositionalPosInf(t *testing.T) {
	t.Log("invariant: +Inf in last three V_N → FeatureUndefined")
	times := []time.Time{clockAt(10, 0, 0), clockAt(10, 1, 0), clockAt(10, 2, 0)}
	got := derivativesFromState([]float64{1.1, math.Inf(1), 1.2}, times)
	requireStatus(t, "v1", got.V1, FeatureUndefined)
	requireStatus(t, "v2", got.V2, FeatureUndefined)
}

func TestC12PositionalNegInf(t *testing.T) {
	t.Log("invariant: -Inf in last three V_N → FeatureUndefined")
	times := []time.Time{clockAt(10, 0, 0), clockAt(10, 1, 0), clockAt(10, 2, 0)}
	got := derivativesFromState([]float64{math.Inf(-1), 0.9, 1.2}, times)
	requireStatus(t, "v1", got.V1, FeatureUndefined)
	requireStatus(t, "v2", got.V2, FeatureUndefined)
}

func TestC13DuplicateTimestamps(t *testing.T) {
	t.Log("invariant: duplicate IntervalStart in the 3-window → FeatureUndefined")
	dup := clockAt(10, 1, 0)
	times := []time.Time{clockAt(10, 0, 0), dup, dup}
	got := derivativesFromState([]float64{1.0, 1.1, 1.2}, times)
	requireStatus(t, "v1", got.V1, FeatureUndefined)
	requireStatus(t, "v2", got.V2, FeatureUndefined)
}

func TestC14ReversedNonCausalTime(t *testing.T) {
	t.Log("invariant: non-increasing positional timestamps → FeatureUndefined; no reorder")
	times := []time.Time{clockAt(10, 4, 0), clockAt(10, 1, 0), clockAt(10, 0, 0)}
	got := derivativesFromState([]float64{1.0, 1.1, 1.2}, times)
	requireStatus(t, "v1", got.V1, FeatureUndefined)
	requireStatus(t, "v2", got.V2, FeatureUndefined)
	if !times[0].Equal(clockAt(10, 4, 0)) || !times[2].Equal(clockAt(10, 0, 0)) {
		t.Fatal("caller Times were reordered")
	}
}

func TestC15NoTimeMutation(t *testing.T) {
	t.Log("invariant: derivative calculation must not mutate Times")
	times := []time.Time{clockAt(10, 0, 0), clockAt(10, 1, 0), clockAt(10, 2, 0)}
	before := append([]time.Time(nil), times...)
	_ = derivativesFromState([]float64{1.0, 1.1, 1.2}, times)
	for i := range before {
		if !times[i].Equal(before[i]) {
			t.Fatalf("Times mutated at %d", i)
		}
	}
}

func TestC16NoVNMutation(t *testing.T) {
	t.Log("invariant: derivative calculation must not mutate VN")
	vn := []float64{1.0, 1.1, 1.2}
	before := append([]float64(nil), vn...)
	times := []time.Time{clockAt(10, 0, 0), clockAt(10, 1, 0), clockAt(10, 2, 0)}
	_ = derivativesFromState(vn, times)
	for i := range before {
		if vn[i] != before[i] {
			t.Fatalf("VN mutated at %d: %v", i, vn)
		}
	}
}

func TestC17StateBoundedness(t *testing.T) {
	t.Log("invariant: repeated Phase B+C advancement keeps rings <= 15")
	vols := make([]uint64, 80)
	for i := range vols {
		vols[i] = uint64(i + 1)
	}
	s, _ := advanceSeq(t, vols)
	if s.RawLen() > RawWindow || s.VNLen() > IntervalMeanWindow || len(s.Feature.Times) > RawWindow {
		t.Fatalf("bounds raw=%d vn=%d times=%d", s.RawLen(), s.VNLen(), len(s.Feature.Times))
	}
}

func TestC18CurrentOnlyCausality(t *testing.T) {
	t.Log("invariant: an observation older than the last 3 positional V_N cannot change V1/V2")
	times := []time.Time{clockAt(9, 59, 0), clockAt(10, 0, 0), clockAt(10, 1, 0), clockAt(10, 2, 0)}
	vn := []float64{99, knownQuadratic(-2), knownQuadratic(-1), knownQuadratic(0)}
	a := derivativesFromState(vn, times)
	vn[0] = -99
	b := derivativesFromState(vn, times)
	if a.V1.Value != b.V1.Value || a.V2.Value != b.V2.Value {
		t.Fatalf("older observation leaked into derivatives: %+v vs %+v", a, b)
	}
	requireAlmost(t, "V1", a.V1.Value, 3)
	requireAlmost(t, "V2", a.V2.Value, 4)
}

func TestC19PositionalReplacementForbidden(t *testing.T) {
	t.Log("invariant: older finite V_N is not substituted for a NaN in the last 3")
	times := []time.Time{
		clockAt(9, 59, 0),
		clockAt(10, 0, 0),
		clockAt(10, 1, 0),
		clockAt(10, 2, 0),
	}
	vn := []float64{1.1, 0.9, math.NaN(), 1.2}
	got := derivativesFromState(vn, times)
	requireStatus(t, "v1", got.V1, FeatureUndefined)
	requireStatus(t, "v2", got.V2, FeatureUndefined)
}

func TestC20NumericalResultFinite(t *testing.T) {
	t.Log("invariant: an available result guarantees finite V1 and V2")
	vn := []float64{knownQuadratic(-2), knownQuadratic(-1), knownQuadratic(0)}
	times := []time.Time{clockAt(10, 0, 0), clockAt(10, 1, 0), clockAt(10, 2, 0)}
	got := derivativesFromState(vn, times)
	if !got.V1.Available() || !got.V2.Available() {
		t.Fatal("expected available")
	}
	if !finite(got.V1.Value) || !finite(got.V2.Value) {
		t.Fatal("available result must be finite")
	}
}

func TestC21PhaseBPreservation(t *testing.T) {
	t.Log("invariant: Phase B identities still hold after Phase C integration")
	_, results := advanceSeq(t, uintRange(1, 15))
	last := results[14]
	if last.Median.Value != 8 || last.VN.Value != 15.0/8.0 {
		t.Fatalf("Phase B 1..15 identity broken median=%g vn=%g", last.Median.Value, last.VN.Value)
	}
	if last.Predicted.Value != last.VN.Value {
		t.Fatal("VOLUME_POINT identity broken")
	}
}

func TestC22NoPricingScientificDependency(t *testing.T) {
	t.Log("invariant: internal/volume must not import internal/pricing")
	assertNoProductionImport(t, packageDir(t), `"quantram/internal/pricing"`)
}

func TestC23NoAdaptiveScientificDependency(t *testing.T) {
	t.Log("invariant: internal/volume must not import Adaptive implementation")
	assertNoProductionImport(t, packageDir(t), `"quantram/internal/adaptive"`)
}

func TestC24NoRuntimeIntegration(t *testing.T) {
	t.Log("invariant: ingestion must not import Phase C Volume code")
	root := repoRoot(t)
	assertNoImport(t, filepath.Join(root, "internal", "ingestion"), `"quantram/internal/volume"`, false)
}
