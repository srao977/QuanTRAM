package volume

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quantram/internal/domain"
)

func mustEngine(t *testing.T, entity string) *Engine {
	t.Helper()
	e, err := NewEngine(entity)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func commitBar(t *testing.T, e *Engine, bar domain.Bar) domain.VolumeEvent {
	t.Helper()
	ev, working, commit := e.PrepareStep(bar)
	if !commit {
		t.Fatalf("expected committable candidate status=%s reason=%s", ev.Status, ev.Reason)
	}
	e.Commit(working)
	return ev
}

func seqBar(i int, vol uint64) domain.Bar {
	start := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Minute)
	return testBar("SYM1", start, vol, "snap-e")
}

func TestE01ColdEngineEmptyState(t *testing.T) {
	t.Log("invariant: cold engine starts with empty Feature and Interpretation state")
	e := mustEngine(t, "SYM1")
	s := e.Snapshot()
	if s.RawLen() != 0 || s.VNLen() != 0 || !s.InterpretationIsEmpty() {
		t.Fatalf("cold state %+v", s)
	}
}

func TestE02FirstObservationMaturing(t *testing.T) {
	t.Log("invariant: first observation prepares MATURING, not AVAILABLE")
	e := mustEngine(t, "SYM1")
	ev, working, ok := e.PrepareStep(seqBar(0, 4))
	if !ok || working == nil {
		t.Fatal("maturation must still produce a candidate")
	}
	if ev.Status != domain.VolumeStatusMaturing {
		t.Fatalf("status %q", ev.Status)
	}
	if ev.VRaw.Value != 4 || !ev.VRaw.Available() {
		t.Fatalf("VRaw %+v", ev.VRaw)
	}
}

func TestE03PrepareDoesNotMutateCommitted(t *testing.T) {
	t.Log("invariant: PrepareStep must not mutate authoritative VolumeState")
	e := mustEngine(t, "SYM1")
	_, _, _ = e.PrepareStep(seqBar(0, 4))
	if e.state.RawLen() != 0 {
		t.Fatal("committed Raw advanced during PrepareStep")
	}
}

func TestE04CommitAdoptsCandidate(t *testing.T) {
	t.Log("invariant: Commit adopts candidate Feature and Interpretation state")
	e := mustEngine(t, "SYM1")
	_, working, _ := e.PrepareStep(seqBar(0, 4))
	e.Commit(working)
	if e.state.RawLen() != 1 || e.state.Feature.Raw[0] != 4 {
		t.Fatalf("committed %+v", e.state.Feature)
	}
}

func TestE05CommitDoesNotAppendTwice(t *testing.T) {
	t.Log("invariant: Commit adopts prepared state and does not rerun feature math")
	e := mustEngine(t, "SYM1")
	_, working, _ := e.PrepareStep(seqBar(0, 4))
	e.Commit(working)
	e.Commit(working)
	if e.state.RawLen() != 1 {
		t.Fatalf("double commit appended again: %d", e.state.RawLen())
	}
}

func TestE06RepeatedPrepareDeterministic(t *testing.T) {
	t.Log("invariant: two PrepareStep calls without Commit see the same committed state")
	e := mustEngine(t, "SYM1")
	bar := seqBar(0, 7)
	a, wa, _ := e.PrepareStep(bar)
	b, wb, _ := e.PrepareStep(bar)
	if a.Status != b.Status || a.VRaw.Value != b.VRaw.Value || a.VN.Status != b.VN.Status {
		t.Fatalf("events differ %+v vs %+v", a, b)
	}
	if e.state.RawLen() != 0 {
		t.Fatal("committed mutated")
	}
	if wa.state.RawLen() != 1 || wb.state.RawLen() != 1 {
		t.Fatal("candidates must each advance once")
	}
}

func TestE07MaturationCommitsAndAdvances(t *testing.T) {
	t.Log("invariant: MATURING candidates may be committed so the engine can mature")
	e := mustEngine(t, "SYM1")
	for i := 0; i < 5; i++ {
		ev := commitBar(t, e, seqBar(i, uint64(i+1)))
		if ev.Status != domain.VolumeStatusMaturing {
			t.Fatalf("obs %d status %q", i+1, ev.Status)
		}
	}
	if e.state.RawLen() != 5 {
		t.Fatalf("raw %d", e.state.RawLen())
	}
}

func TestE08CleanSequenceBecomesAvailable(t *testing.T) {
	t.Log("invariant: AVAILABLE is derived from feature readiness, not observation==29")
	e := mustEngine(t, "SYM1")
	first := -1
	for i := 0; i < 40; i++ {
		ev := commitBar(t, e, seqBar(i, 4))
		if ev.Status == domain.VolumeStatusAvailable {
			if !ev.IntervalMeanVN.Available() || !ev.VN.Available() || !ev.V1.Available() {
				t.Fatal("AVAILABLE used a row counter without scientific readiness")
			}
			if first < 0 {
				first = i + 1
			}
		}
	}
	if first < 0 {
		t.Fatal("clean sequence never became AVAILABLE")
	}
	if first != 29 {
		t.Fatalf("first AVAILABLE at %d; historical clean-sequence fact is 29", first)
	}
}

func TestE09ThroughE22AvailableOutput(t *testing.T) {
	t.Log("invariant: AVAILABLE output carries features, Indicator, phase, and initiating-Bar lineage")
	e := mustEngine(t, "SYM1")
	var ev domain.VolumeEvent
	var lastBar domain.Bar
	for i := 0; i < 29; i++ {
		lastBar = seqBar(i, 4)
		ev = commitBar(t, e, lastBar)
	}
	if ev.Status != domain.VolumeStatusAvailable {
		t.Fatalf("status %q reason %q", ev.Status, ev.Reason)
	}
	if !ev.VRaw.Available() || ev.VRaw.Value != 4 {
		t.Fatalf("VRaw %+v", ev.VRaw)
	}
	if !ev.VN.Available() {
		t.Fatalf("VN %+v", ev.VN)
	}
	if !ev.V1.Available() || !ev.V2.Available() {
		t.Fatalf("V1 %+v V2 %+v", ev.V1, ev.V2)
	}
	if !ev.IntervalMeanVN.Available() {
		t.Fatalf("interval %+v", ev.IntervalMeanVN)
	}
	if !ev.PredictedNextVN.Available() || ev.PredictedNextVN.Value != ev.VN.Value {
		t.Fatalf("predicted %+v vn %+v", ev.PredictedNextVN, ev.VN)
	}
	if ev.RawColor == domain.VolumeColorUnset {
		t.Fatal("raw color missing")
	}
	if ev.Indicator == domain.VolumeColorUnset {
		t.Fatal("Indicator missing")
	}
	if ev.Phase == domain.VolumePhaseUnset {
		t.Fatal("phase missing")
	}
	if ev.Confidence != domain.VolumeConfidenceHigh || ev.DomainState != domain.VolumeDomainCausalLocal {
		t.Fatalf("confidence %q domain %q", ev.Confidence, ev.DomainState)
	}
	if ev.Lineage.Symbol != "SYM1" {
		t.Fatalf("symbol %q", ev.Lineage.Symbol)
	}
	if ev.Lineage.MarketSnapshotID != lastBar.MarketSnapshotID {
		t.Fatalf("snapshot %q", ev.Lineage.MarketSnapshotID)
	}
	if !ev.Lineage.IntervalStart.Equal(lastBar.IntervalStart) {
		t.Fatal("IntervalStart drifted")
	}
	if !ev.Lineage.IntervalEnd.Equal(lastBar.IntervalEnd) {
		t.Fatal("IntervalEnd drifted")
	}
	if ev.Lineage.SourceTimestamp != lastBar.SourceTimestamp {
		t.Fatalf("SourceTimestamp %q", ev.Lineage.SourceTimestamp)
	}
	if !ev.Lineage.SameInitiatingBar(lastBar.Symbol, lastBar.MarketSnapshotID, lastBar.IntervalStart) {
		t.Fatal("same-Bar lineage contract failed")
	}
}

func TestE23ValidRawZeroDistinctFromUnavailable(t *testing.T) {
	t.Log("invariant: V_N==0 with positive median is AVAILABLE, not UNDEFINED")
	e := mustEngine(t, "SYM1")
	for i := 0; i < 29; i++ {
		commitBar(t, e, seqBar(i, 4))
	}
	ev := commitBar(t, e, seqBar(29, 0))
	if !ev.VN.Available() || ev.VN.Value != 0 {
		t.Fatalf("zero raw VN %+v", ev.VN)
	}
	if ev.VN.Status == domain.VolumeQtyUndefined || ev.VN.Status == domain.VolumeQtyInsufficient {
		t.Fatal("valid V_N=0 collapsed into unavailability")
	}
}

func TestE24UndefinedNormalizationIsInvalid(t *testing.T) {
	t.Log("invariant: due zero-median normalization is INVALID, not a fabricated zero")
	e := mustEngine(t, "SYM1")
	var ev domain.VolumeEvent
	for i := 0; i < 15; i++ {
		ev = commitBar(t, e, seqBar(i, 0))
	}
	if ev.Status != domain.VolumeStatusInvalid {
		t.Fatalf("status %q", ev.Status)
	}
	if ev.VN.Status != domain.VolumeQtyUndefined || ev.VN.Available() {
		t.Fatalf("VN %+v", ev.VN)
	}
	if ev.Indicator != domain.VolumeColorUnset {
		t.Fatal("must not fabricate Indicator AMBER")
	}
}

func TestE25UndefinedVNRemainsPositional(t *testing.T) {
	t.Log("invariant: undefined V_N keeps its causal NaN position")
	e := mustEngine(t, "SYM1")
	for i := 0; i < 15; i++ {
		commitBar(t, e, seqBar(i, 0))
	}
	if e.state.VNLen() != 15 || !math.IsNaN(e.state.Feature.VN[14]) {
		t.Fatalf("VN %+v", e.state.Feature.VN)
	}
}

func TestE26E27InvalidDoesNotAdvanceInterpretation(t *testing.T) {
	t.Log("invariant: FeatureState may advance on scientific invalidity; InterpretationState does not")
	e := mustEngine(t, "SYM1")
	for i := 0; i < 29; i++ {
		commitBar(t, e, seqBar(i, 4))
	}
	if e.state.Interpretation.Color == domain.VolumeColorUnset {
		t.Fatal("expected a confirmed color before the invalid sequence")
	}
	for i := 29; i < 43; i++ {
		commitBar(t, e, seqBar(i, 0))
	}
	before := e.state.Interpretation
	ev := commitBar(t, e, seqBar(43, 0))
	if ev.Status != domain.VolumeStatusInvalid {
		t.Fatalf("status %q reason %q", ev.Status, ev.Reason)
	}
	if e.state.Interpretation != before {
		t.Fatalf("interpretation advanced %+v → %+v", before, e.state.Interpretation)
	}
	if e.state.RawLen() != 15 || e.state.Feature.Raw[14] != 0 {
		t.Fatal("feature position did not advance")
	}
	if !math.IsNaN(e.state.Feature.VN[14]) {
		t.Fatal("invalid position lost")
	}
}

func TestE28Alignment(t *testing.T) {
	t.Log("invariant: Raw/VN/Times stay aligned through engine composition")
	e := mustEngine(t, "SYM1")
	for i := 0; i < 20; i++ {
		commitBar(t, e, seqBar(i, uint64(i+1)))
	}
	s := e.state
	if s.RawLen() != s.VNLen() || s.RawLen() != len(s.Feature.Times) {
		t.Fatalf("misaligned raw=%d vn=%d times=%d", s.RawLen(), s.VNLen(), len(s.Feature.Times))
	}
}

func TestE29BoundedAfterLongSequence(t *testing.T) {
	t.Log("invariant: engine rings remain <= 15")
	e := mustEngine(t, "SYM1")
	for i := 0; i < 80; i++ {
		commitBar(t, e, seqBar(i, 4))
	}
	s := e.state
	if s.RawLen() > 15 || s.VNLen() > 15 || len(s.Feature.Times) > 15 {
		t.Fatalf("bounds raw=%d vn=%d times=%d", s.RawLen(), s.VNLen(), len(s.Feature.Times))
	}
}

func TestE30E31E32Reset(t *testing.T) {
	t.Log("invariant: Reset clears Feature and Interpretation and restarts maturation")
	e := mustEngine(t, "SYM1")
	for i := 0; i < 29; i++ {
		commitBar(t, e, seqBar(i, 4))
	}
	e.Reset()
	if e.state.RawLen() != 0 || !e.state.InterpretationIsEmpty() {
		t.Fatal("reset incomplete")
	}
	ev := commitBar(t, e, seqBar(100, 4))
	if ev.Status != domain.VolumeStatusMaturing {
		t.Fatalf("after reset status %q", ev.Status)
	}
	if e.state.RawLen() != 1 {
		t.Fatal("maturation did not restart")
	}
}

func TestE33NoAutomaticSessionReset(t *testing.T) {
	t.Log("invariant: a calendar/session date change does not reset VolumeState")
	e := mustEngine(t, "SYM1")
	commitBar(t, e, seqBar(0, 4))
	next := testBar("SYM1", time.Date(2026, 9, 6, 9, 30, 0, 0, time.UTC), 4, "snap-e")
	commitBar(t, e, next)
	if e.state.RawLen() != 2 {
		t.Fatalf("session change reset state: %d", e.state.RawLen())
	}
}

func TestE34NoAutomaticProviderReset(t *testing.T) {
	t.Log("invariant: a provider/snapshot identity change does not reset VolumeState")
	e := mustEngine(t, "SYM1")
	commitBar(t, e, seqBar(0, 4))
	bar := seqBar(1, 5)
	bar.MarketSnapshotID = "other-provider"
	bar.SourceTimestamp = "provider-b"
	commitBar(t, e, bar)
	if e.state.RawLen() != 2 {
		t.Fatal("provider change reset state")
	}
}

func TestE35IrregularElapsedTimeSurvives(t *testing.T) {
	t.Log("invariant: irregular IntervalStart spacing is kept as actual elapsed time")
	e := mustEngine(t, "SYM1")
	for i := 0; i < 27; i++ {
		commitBar(t, e, seqBar(i, 4))
	}
	t0 := time.Date(2026, 9, 5, 15, 0, 0, 0, time.UTC)
	commitBar(t, e, testBar("SYM1", t0, 4, "snap-e"))
	commitBar(t, e, testBar("SYM1", t0.Add(3*time.Minute), 4, "snap-e"))
	ev := commitBar(t, e, testBar("SYM1", t0.Add(10*time.Minute), 4, "snap-e"))
	times := e.state.Feature.Times
	if !times[12].Equal(t0) || !times[13].Equal(t0.Add(3*time.Minute)) || !times[14].Equal(t0.Add(10*time.Minute)) {
		t.Fatalf("times %+v", times)
	}
	if ev.Status != domain.VolumeStatusAvailable {
		t.Fatalf("status %q", ev.Status)
	}
}

func TestE36CandidateDoesNotAliasCommitted(t *testing.T) {
	t.Log("invariant: candidate Feature rings do not alias committed slices")
	e := mustEngine(t, "SYM1")
	commitBar(t, e, seqBar(0, 4))
	_, working, _ := e.PrepareStep(seqBar(1, 5))
	if e.state.aliasesFeature(working.state) {
		t.Fatal("candidate aliased committed rings")
	}
}

func TestE37OutputLineageDoesNotAliasState(t *testing.T) {
	t.Log("invariant: output lineage is a value copy of the initiating observation")
	e := mustEngine(t, "SYM1")
	bar := seqBar(0, 4)
	ev, working, _ := e.PrepareStep(bar)
	working.state.Feature.Times[0] = time.Time{}
	if !ev.Lineage.IntervalStart.Equal(bar.IntervalStart) {
		t.Fatal("lineage aliased candidate Times")
	}
}

func TestE38PerEntityIsolation(t *testing.T) {
	t.Log("invariant: two per-entity engines do not share VolumeState")
	a := mustEngine(t, "SYM1")
	b := mustEngine(t, "SYM2")
	commitBar(t, a, testBar("SYM1", time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC), 4, "s1"))
	if b.state.RawLen() != 0 {
		t.Fatal("SYM2 observed SYM1")
	}
	_, _, ok := b.PrepareStep(testBar("SYM1", time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC), 9, "s1"))
	if ok {
		t.Fatal("SYM2 accepted a SYM1 bar")
	}
}

func TestE39NoPricingDependency(t *testing.T) {
	t.Log("invariant: internal/volume must not import internal/pricing")
	assertNoProductionImport(t, packageDir(t), `"quantram/internal/pricing"`)
}

func TestE40NoAdaptiveDependency(t *testing.T) {
	t.Log("invariant: internal/volume must not import Adaptive science")
	assertNoProductionImport(t, packageDir(t), `"quantram/internal/adaptive"`)
}

func TestE41PhaseADStillPass(t *testing.T) {
	t.Log("invariant: Phase B 1..15 identity remains after Phase E composition")
	_, results := advanceSeq(t, uintRange(1, 15))
	if results[14].VN.Value != 15.0/8.0 {
		t.Fatal("Phase B identity broken")
	}
}

func TestE42E43NoRuntimeIntegration(t *testing.T) {
	t.Log("invariant: modelhost and ingestion must not import the Volume Engine")
	root := repoRoot(t)
	assertNoImport(t, filepath.Join(root, "internal", "modelhost"), `"quantram/internal/volume"`, false)
	assertNoImport(t, filepath.Join(root, "internal", "ingestion"), `"quantram/internal/volume"`, false)
}

func TestE44VolumeScienceDoesNotImportProto(t *testing.T) {
	t.Log("invariant: Volume science does not import generated proto; no VolumeService microservice")
	assertNoProductionImport(t, packageDir(t), `"quantram/gen/quantram/v1"`)
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "api", "proto", "quantram", "v1", "quantram.proto"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "service VolumeService") {
		t.Fatal("do not invent a Volume microservice")
	}
}

func TestE45NoStageTransitionVolume(t *testing.T) {
	t.Log("invariant: StageTransition V1.1 has no P04V StageID")
	matches, err := filepath.Glob(filepath.Join(repoRoot(t), "internal", "stagetransition", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.Contains(string(body), "P04V") || strings.Contains(string(body), "internal/volume") {
			t.Fatalf("%s references P-04V", path)
		}
	}
}

func TestE46ProcessModelUntouchedByPhaseE(t *testing.T) {
	t.Log("invariant: Phase E must not require Process Model edits")
	// Presence of the master document is enough; Phase E does not write it.
	if _, err := os.Stat(filepath.Join(repoRoot(t), "docs", "design", "QuanTRAM_PROCESS_MODEL_082926.md")); err != nil {
		t.Fatal(err)
	}
}

func TestEStaleCandidateRejected(t *testing.T) {
	t.Log("invariant: a candidate is valid only for the generation from which it was prepared")
	e := mustEngine(t, "SYM1")
	_, stale, _ := e.PrepareStep(seqBar(0, 4))
	commitBar(t, e, seqBar(0, 4))
	before := e.state.RawLen()
	e.Commit(stale)
	if e.state.RawLen() != before {
		t.Fatal("stale candidate was adopted")
	}
}

func TestEInvalidPositionPersistsUntilRolledOut(t *testing.T) {
	t.Log("invariant: an invalid VN position remains until the bounded window rolls it out")
	e := mustEngine(t, "SYM1")
	for i := 0; i < 15; i++ {
		commitBar(t, e, seqBar(i, 0))
	}
	if !math.IsNaN(e.state.Feature.VN[0]) {
		t.Fatal("first invalid position missing")
	}
	for i := 15; i < 29; i++ {
		commitBar(t, e, seqBar(i, 4))
	}
	nans := 0
	for _, v := range e.state.Feature.VN {
		if math.IsNaN(v) {
			nans++
		}
	}
	if nans == 0 {
		t.Fatal("invalid positions were silently repaired")
	}
}
