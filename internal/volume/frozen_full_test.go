package volume

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"quantram/internal/domain"
)

const (
	art009VObs = "APTF_TEST_009V_PRICE_VOLUME_OBSERVATIONS_V0_1.csv"
	art009VSel = "APTF_TEST_009V_VOLUME_SELECTION_V0_1.json"
	art010     = "APTF_TEST_010_VOLUME_ENGINE_EMISSIONS_V0_1.csv"
	art014CPol = "APTF_TEST_014C_SPY_V_EMISSION_POLICY_V0_1.json"
	art014CEmi = "APTF_TEST_014C_SPY_V_ENGINE_EMISSIONS_V0_1.csv"
	art014CSum = "APTF_TEST_014C_SUMMARY_V0_1.json"
)

func discoverFrozenDir(t *testing.T) string {
	t.Helper()
	if d := os.Getenv("QUANTRAM_P04V_FROZEN_DIR"); d != "" {
		return d
	}
	candidates := []string{
		filepath.Join(repoRoot(t), "..", "APTF", "pre08242026_docs"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, art010)); err == nil {
			return c
		}
	}
	return ""
}

func requireFrozenDir(t *testing.T) string {
	t.Helper()
	dir := discoverFrozenDir(t)
	if dir == "" {
		t.Skip("full frozen corpus not present; set QUANTRAM_P04V_FROZEN_DIR")
	}
	return dir
}

func TestFFullFrozenHashes(t *testing.T) {
	dir := requireFrozenDir(t)
	requireFileSHA256(t, filepath.Join(dir, art009VObs), src009VObsSHA)
	requireFileSHA256(t, filepath.Join(dir, art009VSel), src009VSelSHA)
	requireFileSHA256(t, filepath.Join(dir, art010), src010SHA)
	requireFileSHA256(t, filepath.Join(dir, art014CPol), src014CPolSHA)
	requireFileSHA256(t, filepath.Join(dir, art014CEmi), src014CEmiSHA)
	requireFileSHA256(t, filepath.Join(dir, art014CSum), src014CSumSHA)
	info010, err := os.Stat(filepath.Join(dir, art010))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("010 path=%s size=%d", filepath.Join(dir, art010), info010.Size())
}

func TestFFullEngineFeatureReplay(t *testing.T) {
	dir := requireFrozenDir(t)
	requireFileSHA256(t, filepath.Join(dir, art009VObs), src009VObsSHA)
	requireFileSHA256(t, filepath.Join(dir, art010), src010SHA)

	src := loadSourceCSV(t, filepath.Join(dir, art009VObs), "source_observation_index", "timestamp", "V_RAW")
	if len(src) != 101221 {
		t.Fatalf("source records %d want 101221", len(src))
	}
	zeroRaw := 0
	for _, obs := range src {
		if obs.VRaw == 0 {
			zeroRaw++
		}
	}
	t.Logf("source zero-volume rows=%d", zeroRaw)

	feats := loadFeatCSV(t, filepath.Join(dir, art010))
	if len(feats) != 101205 {
		t.Fatalf("010 records %d want 101205", len(feats))
	}

	evs := replayEngine(t, src)
	if evs[14].Status == domain.VolumeStatusError {
		t.Fatal("ENGINE_ERROR at first V_N observation")
	}
	stats, mismatches := compareFeatures(t, src, evs, feats)
	t.Logf("010 aligned rows=%d numeric mismatches=%d", len(feats), len(mismatches))
	for i, m := range mismatches {
		if i >= 20 {
			t.Logf("... %d further numeric/alignment mismatches omitted from log", len(mismatches)-20)
			break
		}
		t.Errorf("%s %s want %s got %s class=%s %s",
			m.When.Format(time.RFC3339), m.Field, m.Want, m.Got, m.Class, m.Detail)
	}
	for _, name := range []string{"V_RAW", "V_N", "V1", "V2", "interval_mean_vn", "predicted_next_V_N"} {
		st := stats[name]
		if st.Outside != 0 {
			t.Errorf("%s outside_tol=%d max_abs=%g", name, st.Outside, st.MaxAbs)
		}
	}

	invalid := 0
	for _, ev := range evs {
		if ev.Status == domain.VolumeStatusInvalid {
			invalid++
		}
		if ev.Status == domain.VolumeStatusError {
			t.Fatalf("ENGINE_ERROR during full replay at %s", ev.Lineage.IntervalStart)
		}
	}
	t.Logf("Engine scientific-INVALID events=%d", invalid)
}

func TestFFull014CSessionSlicedInterpretation(t *testing.T) {
	dir := requireFrozenDir(t)
	requireFileSHA256(t, filepath.Join(dir, art014CEmi), src014CEmiSHA)
	rows := loadInterpCSV(t, filepath.Join(dir, art014CEmi))
	if len(rows) != 55199 {
		t.Fatalf("014C records %d want 55199", len(rows))
	}

	prod := replay014C(rows, nil)
	exact, mismatches, near := compare014CScience(t, rows, prod)
	t.Logf("014C production Interpret categorical exact counts:")
	for _, field := range []string{"raw_color", "phase", "confidence", "domain_state", "Indicator/cockpit_color", "transition_state"} {
		c := exact[field]
		t.Logf("  %-28s %d / %d", field, c[0], c[1])
		if c[0] != c[1] {
			t.Errorf("%s mismatches %d", field, c[1]-c[0])
		}
	}
	t.Logf("threshold/phase-band rows logged=%d", len(near))
	for i, line := range near {
		if i >= 24 {
			break
		}
		t.Logf("  %s", line)
	}
	var firstInd *catMismatch
	nInd, nTrans := 0, 0
	for i := range mismatches {
		m := mismatches[i]
		if m.Field == "Indicator/cockpit_color" {
			nInd++
			if firstInd == nil {
				firstInd = &mismatches[i]
			}
		}
		if m.Field == "transition_state" {
			nTrans++
		}
		if i < 20 {
			t.Errorf("%s %s want %s got %s %s", m.When.Format(time.RFC3339), m.Field, m.Want, m.Got, m.Detail)
		}
	}
	t.Logf("production Indicator mismatches=%d transition mismatches=%d", nInd, nTrans)
	if firstInd != nil {
		t.Errorf("FIRST Indicator mismatch: %s want=%s got=%s %s",
			firstInd.When.Format(time.RFC3339), firstInd.Want, firstInd.Got, firstInd.Detail)
	}

	oracle := replay014C(rows, aptfConfirm)
	oExact, oMis, _ := compare014CScience(t, rows, oracle)
	t.Logf("014C APTF-oracle categorical exact counts:")
	for _, field := range []string{"raw_color", "phase", "confidence", "domain_state", "Indicator/cockpit_color", "transition_state"} {
		c := oExact[field]
		t.Logf("  %-28s %d / %d", field, c[0], c[1])
		if c[0] != c[1] {
			t.Errorf("APTF-oracle %s mismatches %d", field, c[1]-c[0])
		}
	}
	for i, m := range oMis {
		if i >= 20 {
			break
		}
		t.Errorf("APTF-oracle %s %s want %s got %s %s", m.When.Format(time.RFC3339), m.Field, m.Want, m.Got, m.Detail)
	}
}

func TestFFull014CEngineFeaturesAtAlignedTimestamps(t *testing.T) {
	dir := requireFrozenDir(t)
	requireFileSHA256(t, filepath.Join(dir, art009VObs), src009VObsSHA)
	requireFileSHA256(t, filepath.Join(dir, art014CEmi), src014CEmiSHA)

	src := loadSourceCSV(t, filepath.Join(dir, art009VObs), "source_observation_index", "timestamp", "V_RAW")
	evs := replayEngine(t, src)
	byTime := make(map[time.Time]domain.VolumeEvent, len(evs))
	for _, ev := range evs {
		byTime[ev.Lineage.IntervalStart] = ev
	}
	rows := loadInterpCSV(t, filepath.Join(dir, art014CEmi))

	stats := map[string]*numFieldStat{
		"V_RAW":              {Name: "V_RAW"},
		"V_N":                {Name: "V_N"},
		"V1":                 {Name: "V1"},
		"V2":                 {Name: "V2"},
		"interval_mean_vn":   {Name: "interval_mean_vn"},
		"predicted_next_V_N": {Name: "predicted_next_V_N"},
	}
	missing := 0
	indMismatch := 0
	var firstInd *catMismatch
	for _, row := range rows {
		ev, ok := byTime[row.Time]
		if !ok {
			missing++
			continue
		}
		_ = addNum(stats["V_RAW"], optionalFloat{Value: row.VRaw, Valid: true}, ev.VRaw, tolVRaw)
		_ = addNum(stats["V_N"], optionalFloat{Value: row.VN, Valid: true}, ev.VN, tolVN)
		_ = addNum(stats["V1"], optionalFloat{Value: row.V1, Valid: true}, ev.V1, tolDeriv)
		_ = addNum(stats["V2"], optionalFloat{Value: row.V2, Valid: true}, ev.V2, tolDeriv)
		_ = addNum(stats["interval_mean_vn"], optionalFloat{Value: row.Activity, Valid: true}, ev.IntervalMeanVN, tolMean)
		_ = addNum(stats["predicted_next_V_N"], optionalFloat{Value: row.Predicted, Valid: true}, ev.PredictedNextVN, tolVN)
		if ev.Indicator != row.Cockpit {
			indMismatch++
			if firstInd == nil {
				firstInd = &catMismatch{
					When:   row.Time,
					Field:  "Engine.Indicator vs 014C cockpit (no production session reset)",
					Want:   row.Cockpit,
					Got:    ev.Indicator,
					Class:  "E. SESSION / HISTORICAL HARNESS LIFECYCLE and/or confirmation bookkeeping",
					Detail: "production Engine does not reset InterpretationState on date:session",
				}
			}
		}
	}
	t.Logf("014C timestamps missing from Engine replay=%d / %d", missing, len(rows))
	for _, name := range []string{"V_RAW", "V_N", "V1", "V2", "interval_mean_vn", "predicted_next_V_N"} {
		finalizeStat(stats[name])
		logNumStat(t, *stats[name])
		if stats[name].Outside != 0 {
			t.Errorf("%s outside_tol=%d max_abs=%g", name, stats[name].Outside, stats[name].MaxAbs)
		}
	}
	t.Logf("Engine.Indicator vs 014C cockpit mismatches=%d (expected lifecycle: no production session reset)", indMismatch)
	if firstInd != nil {
		t.Logf("FIRST Engine.Indicator mismatch: %s want=%s got=%s class=%s",
			firstInd.When.Format(time.RFC3339), firstInd.Want, firstInd.Got, firstInd.Class)
	}
	if missing != 0 {
		t.Errorf("A. INPUT ALIGNMENT: %d 014C timestamps not in source replay", missing)
	}
}

func TestFFullEnginePhaseEpsilonAudit(t *testing.T) {
	dir := requireFrozenDir(t)
	requireFileSHA256(t, filepath.Join(dir, art009VObs), src009VObsSHA)
	requireFileSHA256(t, filepath.Join(dir, art010), src010SHA)
	src := loadSourceCSV(t, filepath.Join(dir, art009VObs), "source_observation_index", "timestamp", "V_RAW")
	evs := replayEngine(t, src)
	byIdx := eventByIndex(src, evs)
	feats := loadFeatCSV(t, filepath.Join(dir, art010))

	near := 0
	flips := 0
	var firstFlip string
	maxAbsV1, maxAbsV2 := 0.0, 0.0
	var maxV1At, maxV2At string
	for _, row := range feats {
		ev := byIdx[row.Index]
		if row.V1.Valid && ev.V1.Available() {
			abs := absDiff(row.V1.Value, ev.V1.Value)
			if abs > maxAbsV1 {
				maxAbsV1 = abs
				maxV1At = fmt.Sprintf("%s frozen=%.17g go=%.17g", row.Time.Format(time.RFC3339), row.V1.Value, ev.V1.Value)
			}
		}
		if row.V2.Valid && ev.V2.Available() {
			abs := absDiff(row.V2.Value, ev.V2.Value)
			if abs > maxAbsV2 {
				maxAbsV2 = abs
				maxV2At = fmt.Sprintf("%s frozen=%.17g go=%.17g", row.Time.Format(time.RFC3339), row.V2.Value, ev.V2.Value)
			}
		}
		if !row.V1.Valid || !row.V2.Valid || !ev.V1.Available() || !ev.V2.Available() {
			continue
		}
		if math.Abs(row.V1.Value) <= phaseEpsBand || math.Abs(row.V2.Value) <= phaseEpsBand ||
			math.Abs(ev.V1.Value) <= phaseEpsBand || math.Abs(ev.V2.Value) <= phaseEpsBand {
			near++
		}
		want := classifyPhase(row.V1.Value, row.V2.Value)
		got := classifyPhase(ev.V1.Value, ev.V2.Value)
		if want != got {
			flips++
			if firstFlip == "" {
				firstFlip = fmt.Sprintf("%s frozen_phase=%s go_phase=%s V1_f=%.17g V1_g=%.17g V2_f=%.17g V2_g=%.17g",
					row.Time.Format(time.RFC3339), want, got, row.V1.Value, ev.V1.Value, row.V2.Value, ev.V2.Value)
			}
		}
	}
	t.Logf("phase-epsilon neighborhood rows=%d Engine-vs-frozen phase flips=%d", near, flips)
	t.Logf("max |ΔV1|=%g at %s", maxAbsV1, maxV1At)
	t.Logf("max |ΔV2|=%g at %s", maxAbsV2, maxV2At)
	if firstFlip != "" {
		t.Errorf("D. THRESHOLD/CATEGORICAL DIFFERENCE first phase flip: %s", firstFlip)
	}
}

func absDiff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}
