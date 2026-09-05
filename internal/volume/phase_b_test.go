package volume

import (
	"math"
	"path/filepath"
	"testing"
	"time"
)

func advanceSeq(t *testing.T, vols []uint64) (State, []FeatureResult) {
	t.Helper()
	s := NewState("SYM1")
	start := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)
	results := make([]FeatureResult, 0, len(vols))
	for i, v := range vols {
		obs := ObservationFromBar(testBar("SYM1", start.Add(time.Duration(i)*time.Minute), v, "snap-b"))
		results = append(results, s.AdvanceFeatures(obs))
	}
	return s, results
}

func uintRange(from, to uint64) []uint64 {
	out := make([]uint64, 0, to-from+1)
	for v := from; v <= to; v++ {
		out = append(out, v)
	}
	return out
}

func requireStatus(t *testing.T, name string, q Quantity, want FeatureStatus) {
	t.Helper()
	if q.Status != want {
		t.Fatalf("%s status %d want %d value=%g", name, q.Status, want, q.Value)
	}
	if want != FeatureAvailable && !math.IsNaN(q.Value) {
		t.Fatalf("%s unavailable value must be NaN, got %g", name, q.Value)
	}
}

func TestB01MedianReadiness(t *testing.T) {
	t.Log("invariant: fewer than 15 raw positions → median unavailable (maturation)")
	_, results := advanceSeq(t, uintRange(1, 14))
	last := results[len(results)-1]
	requireStatus(t, "median", last.Median, FeatureInsufficient)
	requireStatus(t, "vn", last.VN, FeatureInsufficient)
}

func TestB02MedianExactOddN(t *testing.T) {
	t.Log("invariant: N=15 median is sorted[7], not an even-N average")
	window := []float64{15, 1, 14, 2, 13, 3, 12, 4, 11, 5, 10, 6, 9, 7, 8}
	med, ok := rollingMedian15(window)
	if !ok {
		t.Fatal("median must be available for 15 finite values")
	}
	if med != 8 {
		t.Fatalf("median %g want sorted[7]=8", med)
	}
}

func TestB03MedianDoesNotReorderState(t *testing.T) {
	t.Log("invariant: median calculation must not mutate the causal Raw window")
	s, _ := advanceSeq(t, uintRange(1, 15))
	before := append([]float64(nil), s.Feature.Raw...)
	med, ok := rollingMedian15(s.Feature.Raw)
	if !ok || med != 8 {
		t.Fatalf("median %g ok=%v", med, ok)
	}
	for i := range before {
		if s.Feature.Raw[i] != before[i] {
			t.Fatalf("Raw reordered at %d: %+v", i, s.Feature.Raw)
		}
	}
	if s.Feature.Raw[0] != 1 || s.Feature.Raw[14] != 15 {
		t.Fatalf("causal order lost: %+v", s.Feature.Raw)
	}
}

func TestB04VNFormula(t *testing.T) {
	t.Log("invariant: V_N == V_RAW_current / median_15 when denominator > 0")
	_, results := advanceSeq(t, uintRange(1, 15))
	last := results[14]
	if last.Median.Value != 8 || !last.Median.Available() {
		t.Fatalf("median %+v", last.Median)
	}
	want := 15.0 / 8.0
	if last.VN.Value != want || !last.VN.Available() {
		t.Fatalf("V_N %g want %g", last.VN.Value, want)
	}
}

func TestB05CurrentObservationIncluded(t *testing.T) {
	t.Log("invariant: current V_RAW is a member of the 15-value median window")
	vols := append(uintRange(1, 15), 1000)
	_, results := advanceSeq(t, vols)
	last := results[15]
	// Correct window: 2..15,1000 → sorted[7]=9.
	// Current-excluded window would still be 1..15 → median=8.
	if last.Median.Value != 9 {
		t.Fatalf("median %g want 9 (current-inclusive); 8 would mean current was excluded", last.Median.Value)
	}
	if last.VN.Value != 1000.0/9.0 {
		t.Fatalf("V_N %g want 1000/9", last.VN.Value)
	}
}

func TestB06ZeroRaw(t *testing.T) {
	t.Log("invariant: V_RAW==0 with positive median → V_N==0, not unavailable")
	vols := append(uintRange(1, 14), 0)
	_, results := advanceSeq(t, vols)
	last := results[14]
	if last.Median.Value != 7 {
		t.Fatalf("median %g want 7 (window 0..14)", last.Median.Value)
	}
	if !last.VN.Available() || last.VN.Value != 0 {
		t.Fatalf("V_N %+v want 0", last.VN)
	}
}

func TestB07ZeroDenominator(t *testing.T) {
	t.Log("invariant: median==0 → V_N undefined, not imputed")
	_, results := advanceSeq(t, make([]uint64, 15))
	last := results[14]
	if last.Median.Value != 0 || last.Median.Status != FeatureAvailable {
		t.Fatalf("median %+v want available 0", last.Median)
	}
	requireStatus(t, "vn", last.VN, FeatureUndefined)
}

func TestB08NonPositiveDenominator(t *testing.T) {
	t.Log("invariant: median<0 → V_N undefined (mathematical guard)")
	neg := make([]float64, 15)
	for i := range neg {
		neg[i] = -float64(i + 1)
	}
	median, vn := normalizeFromRaw(neg, -1)
	if median.Value != -8 || !median.Available() {
		t.Fatalf("median %+v want available -8", median)
	}
	requireStatus(t, "vn", vn, FeatureUndefined)
}

func TestB09ExactlyOneVNPositionalAppend(t *testing.T) {
	t.Log("invariant: every consumed observation advances VN by exactly one position")
	s := NewState("SYM1")
	start := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)
	for i := 0; i < 20; i++ {
		obs := ObservationFromBar(testBar("SYM1", start.Add(time.Duration(i)*time.Minute), uint64(i+1), "snap-b"))
		s.AdvanceFeatures(obs)
		if s.VNLen() != s.RawLen() || s.VNLen() != len(s.Feature.Times) {
			t.Fatalf("after %d: raw=%d vn=%d times=%d", i+1, s.RawLen(), s.VNLen(), len(s.Feature.Times))
		}
		if i < RawWindow && s.VNLen() != i+1 {
			t.Fatalf("vn len %d want %d", s.VNLen(), i+1)
		}
	}
}

func TestB10UnavailablePositionNotSkipped(t *testing.T) {
	t.Log("invariant: NaN/unavailable V_N remains at its causal position")
	s, _ := advanceSeq(t, make([]uint64, 15))
	if s.VNLen() != 15 {
		t.Fatalf("vn len %d", s.VNLen())
	}
	for i, v := range s.Feature.VN {
		if !math.IsNaN(v) {
			t.Fatalf("position %d was compacted away; vn=%g", i, v)
		}
		if s.Feature.Raw[i] != 0 {
			t.Fatalf("raw at unavailable position %d drifted", i)
		}
	}
}

func TestB11IntervalMeanReadiness(t *testing.T) {
	t.Log("invariant: fewer than 15 V_N positions → interval_mean_vn unavailable")
	_, results := advanceSeq(t, uintRange(1, 14))
	requireStatus(t, "interval_mean", results[13].IntervalMean, FeatureInsufficient)
}

func TestB12IntervalMeanExact(t *testing.T) {
	t.Log("invariant: 15 finite positional V_N → arithmetic mean, not nanmean")
	known := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	q := intervalMeanVN(known)
	if !q.Available() || q.Value != 8 {
		t.Fatalf("mean %+v want 8", q)
	}
}

func TestB13IntervalMeanPositionalNonfinite(t *testing.T) {
	t.Log("invariant: one NaN/Inf in the last 15 V_N positions poisons the mean")
	poisoned := []float64{1, 2, 3, 4, 5, 6, 7, math.NaN(), 9, 10, 11, 12, 13, 14, 15}
	q := intervalMeanVN(poisoned)
	requireStatus(t, "interval_mean", q, FeatureUndefined)

	// Searching backward for 16 would wrongly succeed if NaN were skipped.
	withExtra := append([]float64{8}, poisoned...)
	q2 := intervalMeanVN(withExtra)
	requireStatus(t, "interval_mean-no-skipback", q2, FeatureUndefined)

	infWin := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, math.Inf(1), 15}
	requireStatus(t, "interval_mean-inf", intervalMeanVN(infWin), FeatureUndefined)
}

func TestB14VolumePoint(t *testing.T) {
	t.Log("invariant: VOLUME_POINT is predicted_next_V_N = V_N when V_N is finite")
	_, results := advanceSeq(t, uintRange(1, 15))
	last := results[14]
	if !last.Predicted.Available() || last.Predicted.Value != last.VN.Value {
		t.Fatalf("predicted %+v vn %+v", last.Predicted, last.VN)
	}
	if last.Predicted.Value != 15.0/8.0 {
		t.Fatalf("predicted %g", last.Predicted.Value)
	}
}

func TestB15VolumePointUnavailable(t *testing.T) {
	t.Log("invariant: unavailable V_N → predicted_next_V_N unavailable with same class")
	_, immature := advanceSeq(t, uintRange(1, 5))
	requireStatus(t, "predicted-maturation", immature[4].Predicted, FeatureInsufficient)

	_, zeros := advanceSeq(t, make([]uint64, 15))
	requireStatus(t, "predicted-undefined", zeros[14].Predicted, FeatureUndefined)
	requireStatus(t, "vn-undefined", zeros[14].VN, FeatureUndefined)
}

func TestB16RingBounds(t *testing.T) {
	t.Log("invariant: feature advancement keeps Raw/VN/Times bounded at 15")
	vols := make([]uint64, 80)
	for i := range vols {
		vols[i] = uint64(i + 1)
	}
	s, _ := advanceSeq(t, vols)
	if s.RawLen() > RawWindow || s.VNLen() > IntervalMeanWindow || len(s.Feature.Times) > RawWindow {
		t.Fatalf("bounds raw=%d vn=%d times=%d", s.RawLen(), s.VNLen(), len(s.Feature.Times))
	}
	if s.RawLen() != 15 || s.VNLen() != 15 || len(s.Feature.Times) != 15 {
		t.Fatalf("expected full rings raw=%d vn=%d times=%d", s.RawLen(), s.VNLen(), len(s.Feature.Times))
	}
}

func TestB17Alignment(t *testing.T) {
	t.Log("invariant: Raw, VN, Times stay positionally aligned after warm-up and unavailable V_N")
	start := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)
	s := NewState("SYM1")
	for i := 0; i < 15; i++ {
		obs := ObservationFromBar(testBar("SYM1", start.Add(time.Duration(i)*time.Minute), uint64(i+1), "snap-b"))
		s.AdvanceFeatures(obs)
	}
	assertAligned(t, s)
	if !math.IsNaN(s.Feature.VN[0]) {
		t.Fatal("first warm-up V_N must remain NaN at position 0")
	}
	if !finite(s.Feature.VN[14]) {
		t.Fatal("15th V_N must be finite at position 14")
	}

	// 15 zeros: every V_N undefined, still one slot per raw/time.
	z, _ := advanceSeq(t, make([]uint64, 15))
	assertAligned(t, z)
	for i := range z.Feature.VN {
		if !math.IsNaN(z.Feature.VN[i]) {
			t.Fatalf("zero-median V_N at %d was skipped", i)
		}
	}
}

func assertAligned(t *testing.T, s State) {
	t.Helper()
	if s.RawLen() != s.VNLen() || s.RawLen() != len(s.Feature.Times) {
		t.Fatalf("misaligned raw=%d vn=%d times=%d", s.RawLen(), s.VNLen(), len(s.Feature.Times))
	}
}

func TestB18CandidateStateSafety(t *testing.T) {
	t.Log("invariant: AdvanceFeatures on a clone must not mutate committed State")
	committed := NewState("SYM1")
	start := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)
	committed.RecordObservation(ObservationFromBar(testBar("SYM1", start, 5, "snap-c")))
	candidate := committed.Clone()
	obs := ObservationFromBar(testBar("SYM1", start.Add(time.Minute), 6, "snap-c2"))
	_ = candidate.AdvanceFeatures(obs)
	if committed.RawLen() != 1 || committed.Feature.Raw[0] != 5 {
		t.Fatal("candidate feature prep mutated committed Raw")
	}
	if committed.VNLen() != 1 || !math.IsNaN(committed.Feature.VN[0]) {
		t.Fatal("candidate feature prep mutated committed VN")
	}
	if committed.aliasesFeature(candidate) {
		t.Fatal("candidate aliased committed rings")
	}
}

func TestB19NoPriceDependency(t *testing.T) {
	t.Log("invariant: Phase B production sources must not import internal/pricing")
	assertNoProductionImport(t, packageDir(t), `"quantram/internal/pricing"`)
}

func TestB20NoAdaptiveDependency(t *testing.T) {
	t.Log("invariant: Phase B production sources must not import Adaptive science")
	assertNoProductionImport(t, packageDir(t), `"quantram/internal/adaptive"`)
}

func TestB21NoRuntimeIntegration(t *testing.T) {
	t.Log("invariant: modelhost and ingestion must not import Phase B Volume code")
	root := repoRoot(t)
	assertNoImport(t, filepath.Join(root, "internal", "modelhost"), `"quantram/internal/volume"`, false)
	assertNoImport(t, filepath.Join(root, "internal", "ingestion"), `"quantram/internal/volume"`, false)
}

func TestB22NoNamedProductionEntityAssumptions(t *testing.T) {
	t.Log("invariant: no entity-specific production Volume logic")
	TestA14NoProductionEntityHardcoding(t)
}

func TestBReferenceVectorCurrentInclusiveMedian(t *testing.T) {
	t.Log("reference: raw 1..15 → median 8, V_N=15/8; next 1000 → median 9 not 8")
	s, results := advanceSeq(t, append(uintRange(1, 15), 1000))
	firstReady := results[14]
	if firstReady.Median.Value != 8 || firstReady.VN.Value != 15.0/8.0 {
		t.Fatalf("1..15 median=%g vn=%g", firstReady.Median.Value, firstReady.VN.Value)
	}
	if results[15].Median.Value != 9 {
		t.Fatalf("current-inclusive median %g want 9", results[15].Median.Value)
	}
	if s.Feature.Raw[14] != 1000 {
		t.Fatalf("current raw not at last position: %+v", s.Feature.Raw)
	}
}

func TestBReferenceVectorIntervalMeanPoison(t *testing.T) {
	t.Log("reference: after 1..15, 14 NaN V_N + one finite V_N poison interval mean")
	_, results := advanceSeq(t, uintRange(1, 15))
	last := results[14]
	if !last.VN.Available() {
		t.Fatal("15th V_N should be finite")
	}
	requireStatus(t, "interval_mean", last.IntervalMean, FeatureUndefined)

	// 15 identical positives after warm-up produce 15 finite V_N=1 and mean=1.
	ones := make([]uint64, 29)
	for i := range ones {
		ones[i] = 4
	}
	_, ready := advanceSeq(t, ones)
	mean := ready[28].IntervalMean
	if !mean.Available() || mean.Value != 1 {
		t.Fatalf("mature identical-raw mean %+v want 1", mean)
	}
}
