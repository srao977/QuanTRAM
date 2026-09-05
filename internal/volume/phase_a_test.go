package volume

import (
	"math"
	"os"
	"testing"
	"time"

	"quantram/internal/domain"
)

func testBar(symbol string, start time.Time, volume uint64, snapshot string) domain.Bar {
	return domain.Bar{
		Symbol:           symbol,
		InstrumentType:   domain.InstrumentStock,
		Tradable:         true,
		Interval:         domain.Interval1Min,
		IntervalStart:    start,
		IntervalEnd:      start.Add(time.Minute),
		Volume:           volume,
		SourceTimestamp:  "2026-09-05T14:00:00Z",
		QualityStatus:    domain.QualityComplete,
		IsFinal:          true,
		MarketSnapshotID: snapshot,
	}
}

func TestA01FrozenScientificConstants(t *testing.T) {
	t.Log("invariant: frozen Volume Feature and Interpretation Science values")
	if NormalizationID != "ROLLING_MEDIAN_RATIO_15" {
		t.Fatalf("normalization %q", NormalizationID)
	}
	if RawWindow != 15 || DerivativeWindow != 3 || IntervalMeanWindow != 15 {
		t.Fatalf("windows raw=%d deriv=%d mean=%d", RawWindow, DerivativeWindow, IntervalMeanWindow)
	}
	if ProjectionID != "VOLUME_POINT" {
		t.Fatalf("projection %q", ProjectionID)
	}
	if InterpretationID != "V_INTERVAL_B10_C2" || StateSource != "INTERVAL_MEAN_V_N" {
		t.Fatalf("interpretation %q source %q", InterpretationID, StateSource)
	}
	if LowerThreshold != 0.9 || UpperThreshold != 1.1 || ConfirmationCount != 2 || Epsilon != 1e-12 {
		t.Fatalf("thresholds %g %g confirm=%d eps=%g", LowerThreshold, UpperThreshold, ConfirmationCount, Epsilon)
	}
	cfg := FrozenConfig("SYM1")
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestA02CanonicalBarMapping(t *testing.T) {
	t.Log("invariant: mapper preserves initiating-Bar identity and exact V_RAW")
	start := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)
	bar := testBar("sym1", start, 4060, "snap-001")
	obs := ObservationFromBar(bar)
	if obs.Entity != "SYM1" || obs.Lineage.Symbol != "SYM1" {
		t.Fatalf("entity %q lineage %q", obs.Entity, obs.Lineage.Symbol)
	}
	if !obs.Lineage.IntervalStart.Equal(start) || !obs.Lineage.IntervalEnd.Equal(start.Add(time.Minute)) {
		t.Fatalf("times start=%s end=%s", obs.Lineage.IntervalStart, obs.Lineage.IntervalEnd)
	}
	if obs.Lineage.SourceTimestamp != bar.SourceTimestamp {
		t.Fatalf("source timestamp %q", obs.Lineage.SourceTimestamp)
	}
	if obs.Lineage.MarketSnapshotID != "snap-001" {
		t.Fatalf("snapshot %q", obs.Lineage.MarketSnapshotID)
	}
	if obs.VRaw != 4060 {
		t.Fatalf("V_RAW %g", obs.VRaw)
	}
}

func TestA03RawZeroPreservation(t *testing.T) {
	t.Log("invariant: Bar.Volume==0 maps to V_RAW==0")
	bar := testBar("SYM1", time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC), 0, "snap-zero")
	obs := ObservationFromBar(bar)
	if obs.VRaw != 0 {
		t.Fatalf("V_RAW %g want 0", obs.VRaw)
	}
}

func TestA04BarImmutability(t *testing.T) {
	t.Log("invariant: mapping does not mutate the incoming Bar")
	start := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)
	bar := testBar("SYM1", start, 10, "snap-imm")
	before := bar
	_ = ObservationFromBar(bar)
	if bar != before {
		t.Fatal("mapper mutated domain.Bar")
	}
}

func TestA05InitiatingBarLineage(t *testing.T) {
	t.Log("invariant: Volume input retains exact causal identity for future Price/Volume compare")
	start := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)
	bar := testBar("SYM1", start, 99, "shared-snapshot")
	obs := ObservationFromBar(bar)
	if !obs.Lineage.SameInitiatingBar(bar.Symbol, bar.MarketSnapshotID, bar.IntervalStart) {
		t.Fatal("lineage must match initiating Bar Symbol, MarketSnapshotID, IntervalStart")
	}
	// Future P-04 PriceEvent uses the same three fields. EffectiveTime is not required.
	priceLikeSymbol := bar.Symbol
	priceLikeSnapshot := bar.MarketSnapshotID
	priceLikeStart := bar.IntervalStart
	if !obs.Lineage.SameInitiatingBar(priceLikeSymbol, priceLikeSnapshot, priceLikeStart) {
		t.Fatal("same-Bar lineage contract failed")
	}
}

func TestA06RawStateBoundedness(t *testing.T) {
	t.Log("invariant: raw ring never exceeds 15")
	s := NewState("SYM1")
	start := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)
	for i := 0; i < 40; i++ {
		s.AppendPositional(float64(i+1), math.NaN(), start.Add(time.Duration(i)*time.Minute))
	}
	if s.RawLen() != RawWindow {
		t.Fatalf("raw len %d", s.RawLen())
	}
	if s.Feature.Raw[0] != 26 || s.Feature.Raw[14] != 40 {
		t.Fatalf("raw window %+v", s.Feature.Raw)
	}
}

func TestA07NormalizedStateBoundedness(t *testing.T) {
	t.Log("invariant: normalized positional ring never exceeds 15")
	s := NewState("SYM1")
	start := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)
	for i := 0; i < 40; i++ {
		s.AppendPositional(float64(i+1), float64(i+1)/10, start.Add(time.Duration(i)*time.Minute))
	}
	if s.VNLen() != IntervalMeanWindow {
		t.Fatalf("vn len %d", s.VNLen())
	}
}

func TestA08PositionalSemantics(t *testing.T) {
	t.Log("invariant: unavailable derived V_N keeps its causal position")
	s := NewState("SYM1")
	start := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)
	s.AppendPositional(10, 1.0, start)
	s.AppendPositional(20, math.NaN(), start.Add(time.Minute))
	s.AppendPositional(30, 1.5, start.Add(2*time.Minute))
	if s.RawLen() != 3 || s.VNLen() != 3 || len(s.Feature.Times) != 3 {
		t.Fatal("rings must stay aligned")
	}
	if !math.IsNaN(s.Feature.VN[1]) {
		t.Fatal("NaN V_N must remain at position 1; must not be skipped")
	}
	if s.Feature.Raw[1] != 20 || !s.Feature.Times[1].Equal(start.Add(time.Minute)) {
		t.Fatal("raw/time at the unavailable position drifted")
	}
}

func TestA09PerEntityIsolation(t *testing.T) {
	t.Log("invariant: two VolumeState instances do not share scientific storage")
	a := NewState("SYM1")
	b := NewState("SYM2")
	start := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)
	a.AppendPositional(1, 1, start)
	b.AppendPositional(9, 9, start)
	if a.aliasesFeature(b) {
		t.Fatal("distinct states aliased")
	}
	if a.Feature.Raw[0] == b.Feature.Raw[0] {
		t.Fatal("values leaked across entities")
	}
	a.Feature.Raw[0] = 42
	if b.Feature.Raw[0] != 9 {
		t.Fatal("mutating SYM1 changed SYM2")
	}
}

func TestA10FullReset(t *testing.T) {
	t.Log("invariant: Reset clears Feature and Interpretation; no independent interpretation reset")
	s := NewState("SYM1")
	s.AppendPositional(1, 1, time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC))
	s.Interpretation = InterpretationState{Color: domain.VolumeColorGreen, PendingColor: domain.VolumeColorRed, PendingCount: 1}
	s.Reset()
	if s.Entity != "SYM1" {
		t.Fatalf("entity cleared: %q", s.Entity)
	}
	if s.RawLen() != 0 || s.VNLen() != 0 || len(s.Feature.Times) != 0 {
		t.Fatal("feature state not cleared")
	}
	if !s.InterpretationIsEmpty() {
		t.Fatalf("interpretation not cleared: %+v", s.Interpretation)
	}
}

func TestA11CandidateCopyIsolation(t *testing.T) {
	t.Log("invariant: Clone must not alias committed ring backing arrays")
	committed := NewState("SYM1")
	start := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)
	committed.RecordObservation(ObservationFromBar(testBar("SYM1", start, 5, "snap-c")))
	candidate := committed.Clone()
	if committed.aliasesFeature(candidate) {
		t.Fatal("candidate aliased committed feature storage")
	}
	candidate.AppendPositional(99, 99, start.Add(time.Minute))
	if committed.RawLen() != 1 || committed.Feature.Raw[0] != 5 {
		t.Fatal("candidate append mutated committed state")
	}
}

func TestA15ConstantsAreNotEnvKnobs(t *testing.T) {
	t.Log("invariant: frozen constants are not sourced from environment")
	t.Setenv("QUANTRAM_VOLUME_RAW_WINDOW", "99")
	t.Setenv("QUANTRAM_VOLUME_LOWER", "0.1")
	cfg := FrozenConfig("SYM1")
	if cfg.RawWindow != 15 || cfg.LowerThreshold != 0.9 {
		t.Fatal("environment overrode frozen science")
	}
	if os.Getenv("QUANTRAM_VOLUME_RAW_WINDOW") != "99" {
		t.Fatal("test env setup failed")
	}
}
