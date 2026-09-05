package volume

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"quantram/internal/domain"
)

// Phase F frozen-equivalence helpers and Level-1 fixtures.
// Production Engine does not read these files.

const (
	frozenTestEntity = "VOL1"

	src009VObsSHA = "71432eb6e19aa65df5a8fadbef120c519f89e88a16dbbb2f6d6a6f9237122fa9"
	src009VSelSHA = "4dbc78a1e213715577a9b68eca8cae186137fcaa2959afeb9d6bac308f430dd9"
	src010SHA     = "0d9134f3a1996d83dd43257264ddd6a43b5e02215b61c94ec034c3e1ee152d3c"
	src014CPolSHA = "f719134f241b00888099e237c02f237a2db4b59f02b25ea5498c51006991bcd8"
	src014CEmiSHA = "ecd946532e32a8c5167aab72e8c56d3d3389ab00705a75d4cb91cf3031fd451e"
	src014CSumSHA = "100f0b4807831f6eebd2e44fe8ab7b2c9597113916243b635b75d819fe80044b"

	fixPolicySHA = "f719134f241b00888099e237c02f237a2db4b59f02b25ea5498c51006991bcd8"
	fixSrcSHA    = "a041cd823e3371c76c3be9d21f039f60a883a796f0790c90faa7259ed89b14a3"
	fix010SHA    = "5537bd3923ab40305d469fa9b3f2d0f8e629dc998ff08857f05f76f488f60319"
	fix014CSHA   = "7cd38ab82d3116dd1a6c73e248c2cd1a294feae23c56fafbb7f1afc930c29e58"

	tolVRaw  = 0.0
	tolVN    = 1e-12
	tolMean  = 1e-12
	tolDeriv = 1e-9

	thresholdBand = 0.02
	phaseEpsBand  = 1e-9
)

var meanVNRe = regexp.MustCompile(`"mean_vn"\s*:\s*([^,}]+)`)

type srcObs struct {
	Index int
	Time  time.Time
	VRaw  float64
}

type featRow struct {
	Index     int
	Time      time.Time
	VRaw      float64
	VN        optionalFloat
	V1        optionalFloat
	V2        optionalFloat
	Predicted optionalFloat
	MeanVN    optionalFloat
}

type interpRow struct {
	Time       time.Time
	Session    string
	SessionKey string
	VRaw       float64
	VN         float64
	V1         float64
	V2         float64
	Predicted  float64
	Activity   float64
	Phase      string
	Transition string
	Confidence string
	Domain     string
	RawColor   string
	Cockpit    string
}

type optionalFloat struct {
	Value float64
	Valid bool
}

type numFieldStat struct {
	Name     string
	Compared int
	Exact    int
	Outside  int
	MaxAbs   float64
	MaxRel   float64
	MeanAbs  float64
	sumAbs   float64
}

type catMismatch struct {
	When   time.Time
	Field  string
	Want   string
	Got    string
	Class  string
	Detail string
}

func testdataPath(name string) string {
	return filepath.Join("testdata", name)
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func requireFileSHA256(t *testing.T, path, want string) {
	t.Helper()
	got := fileSHA256(t, path)
	if got != want {
		t.Fatalf("SHA-256 mismatch %s\n want %s\n got  %s", path, want, got)
	}
}

func parseFrozenFloat(s string) optionalFloat {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "nan") || strings.EqualFold(s, "null") {
		return optionalFloat{}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || !finite(v) {
		return optionalFloat{}
	}
	return optionalFloat{Value: v, Valid: true}
}

func extractMeanVN(blob string) optionalFloat {
	m := meanVNRe.FindStringSubmatch(blob)
	if m == nil {
		return optionalFloat{}
	}
	return parseFrozenFloat(strings.TrimSpace(m[1]))
}

func parseRFC3339(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("timestamp %q: %v", s, err)
	}
	return ts.UTC()
}

func loadLevel1Source(t *testing.T) []srcObs {
	t.Helper()
	return loadSourceCSV(t, testdataPath("level1_source_prefix.csv"), "source_observation_index", "timestamp", "V_RAW")
}

func loadSourceCSV(t *testing.T, path, indexCol, timeCol, rawCol string) []srcObs {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.ReuseRecord = false
	head, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	idx := csvIndex(t, path, head, indexCol, timeCol, rawCol)
	var out []srcObs
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		n, err := strconv.Atoi(rec[idx[0]])
		if err != nil {
			t.Fatalf("%s index %q: %v", path, rec[idx[0]], err)
		}
		raw := parseFrozenFloat(rec[idx[2]])
		if !raw.Valid {
			t.Fatalf("%s row %d: non-finite V_RAW %q", path, n, rec[idx[2]])
		}
		out = append(out, srcObs{Index: n, Time: parseRFC3339(t, rec[idx[1]]), VRaw: raw.Value})
	}
	return out
}

func loadFeatCSV(t *testing.T, path string) []featRow {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	head, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	need := []string{"observation_index", "timestamp", "V_RAW", "V_N", "V1", "V2", "predicted_next_V_N"}
	idx := csvIndex(t, path, head, need...)
	meanIdx := -1
	for i, h := range head {
		if h == "interval_state_json" {
			meanIdx = i
		}
	}
	var out []featRow
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		n, err := strconv.Atoi(rec[idx[0]])
		if err != nil {
			t.Fatal(err)
		}
		row := featRow{
			Index:     n,
			Time:      parseRFC3339(t, rec[idx[1]]),
			VRaw:      mustFinite(t, rec[idx[2]], "V_RAW"),
			VN:        parseFrozenFloat(rec[idx[3]]),
			V1:        parseFrozenFloat(rec[idx[4]]),
			V2:        parseFrozenFloat(rec[idx[5]]),
			Predicted: parseFrozenFloat(rec[idx[6]]),
		}
		if meanIdx >= 0 {
			row.MeanVN = extractMeanVN(rec[meanIdx])
		}
		out = append(out, row)
	}
	return out
}

func loadInterpCSV(t *testing.T, path string) []interpRow {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	head, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	idx := csvIndex(t, path, head,
		"timestamp", "session", "V_RAW", "V", "V1", "V2", "projected_V",
		"activity_state_value", "phase", "transition_state", "confidence",
		"domain_state", "raw_color", "cockpit_color")
	var out []interpRow
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		ts := parseRFC3339(t, rec[idx[0]])
		session := rec[idx[1]]
		out = append(out, interpRow{
			Time:       ts,
			Session:    session,
			SessionKey: ts.Format("2006-01-02") + ":" + session,
			VRaw:       mustFinite(t, rec[idx[2]], "V_RAW"),
			VN:         mustFinite(t, rec[idx[3]], "V"),
			V1:         mustFinite(t, rec[idx[4]], "V1"),
			V2:         mustFinite(t, rec[idx[5]], "V2"),
			Predicted:  mustFinite(t, rec[idx[6]], "projected_V"),
			Activity:   mustFinite(t, rec[idx[7]], "activity"),
			Phase:      rec[idx[8]],
			Transition: rec[idx[9]],
			Confidence: rec[idx[10]],
			Domain:     rec[idx[11]],
			RawColor:   rec[idx[12]],
			Cockpit:    rec[idx[13]],
		})
	}
	return out
}

func csvIndex(t *testing.T, path string, head []string, names ...string) []int {
	t.Helper()
	pos := make(map[string]int, len(head))
	for i, h := range head {
		pos[h] = i
	}
	out := make([]int, len(names))
	for i, name := range names {
		p, ok := pos[name]
		if !ok {
			t.Fatalf("%s missing column %s", path, name)
		}
		out[i] = p
	}
	return out
}

func mustFinite(t *testing.T, s, name string) float64 {
	t.Helper()
	v := parseFrozenFloat(s)
	if !v.Valid {
		t.Fatalf("expected finite %s, got %q", name, s)
	}
	return v.Value
}

func frozenBar(obs srcObs) domain.Bar {
	vol := uint64(obs.VRaw)
	return domain.Bar{
		Symbol:           frozenTestEntity,
		InstrumentType:   domain.InstrumentStock,
		Tradable:         true,
		Interval:         domain.Interval1Min,
		IntervalStart:    obs.Time,
		IntervalEnd:      obs.Time.Add(time.Minute),
		Volume:           vol,
		SourceTimestamp:  obs.Time.Format(time.RFC3339),
		QualityStatus:    domain.QualityComplete,
		IsFinal:          true,
		MarketSnapshotID: fmt.Sprintf("frozen-synthetic-%d", obs.Index),
	}
}

func replayEngine(t *testing.T, src []srcObs) []domain.VolumeEvent {
	t.Helper()
	e := mustEngine(t, frozenTestEntity)
	out := make([]domain.VolumeEvent, 0, len(src))
	for _, obs := range src {
		if float64(uint64(obs.VRaw)) != obs.VRaw {
			t.Fatalf("non-integer V_RAW at %d %s value=%g — cannot map through uint64", obs.Index, obs.Time.Format(time.RFC3339), obs.VRaw)
		}
		out = append(out, commitBar(t, e, frozenBar(obs)))
	}
	return out
}

func eventByIndex(src []srcObs, evs []domain.VolumeEvent) map[int]domain.VolumeEvent {
	m := make(map[int]domain.VolumeEvent, len(src))
	for i, obs := range src {
		m[obs.Index] = evs[i]
	}
	return m
}

func addNum(st *numFieldStat, want optionalFloat, got domain.VolumeQuantity, tol float64) *catMismatch {
	st.Compared++
	gotValid := got.Available() && finite(got.Value)
	if !want.Valid && !gotValid {
		st.Exact++
		return nil
	}
	if want.Valid != gotValid {
		return &catMismatch{
			Field:  st.Name + "_readiness",
			Want:   fmt.Sprintf("valid=%v %g", want.Valid, want.Value),
			Got:    fmt.Sprintf("status=%s %g", got.Status, got.Value),
			Class:  "B. WARM-UP / READINESS ALIGNMENT",
			Detail: "finite/unavailable pairing differs",
		}
	}
	abs := math.Abs(got.Value - want.Value)
	rel := 0.0
	if want.Value != 0 {
		rel = abs / math.Abs(want.Value)
	} else if got.Value != 0 {
		rel = math.Inf(1)
	}
	st.sumAbs += abs
	if abs > st.MaxAbs {
		st.MaxAbs = abs
	}
	if rel > st.MaxRel && !math.IsInf(rel, 0) {
		st.MaxRel = rel
	}
	if abs == 0 {
		st.Exact++
	}
	if abs > tol {
		st.Outside++
		return &catMismatch{
			Field:  st.Name,
			Want:   fmt.Sprintf("%.17g", want.Value),
			Got:    fmt.Sprintf("%.17g", got.Value),
			Class:  "C. NUMERICAL FLOATING DIFFERENCE",
			Detail: fmt.Sprintf("abs=%g rel=%g tol=%g", abs, rel, tol),
		}
	}
	return nil
}

func finalizeStat(st *numFieldStat) {
	if st.Compared > 0 {
		st.MeanAbs = st.sumAbs / float64(st.Compared)
	}
}

func logNumStat(t *testing.T, st numFieldStat) {
	t.Helper()
	t.Logf("%-22s compared=%d exact=%d outside_tol=%d max_abs=%.4e max_rel=%.4e mean_abs=%.4e",
		st.Name, st.Compared, st.Exact, st.Outside, st.MaxAbs, st.MaxRel, st.MeanAbs)
}

func compareFeatures(t *testing.T, src []srcObs, evs []domain.VolumeEvent, feats []featRow) (map[string]numFieldStat, []catMismatch) {
	t.Helper()
	byIdx := eventByIndex(src, evs)
	stats := map[string]*numFieldStat{
		"V_RAW":              {Name: "V_RAW"},
		"V_N":                {Name: "V_N"},
		"V1":                 {Name: "V1"},
		"V2":                 {Name: "V2"},
		"interval_mean_vn":   {Name: "interval_mean_vn"},
		"predicted_next_V_N": {Name: "predicted_next_V_N"},
	}
	var mismatches []catMismatch
	for _, row := range feats {
		ev, ok := byIdx[row.Index]
		if !ok {
			mismatches = append(mismatches, catMismatch{
				When: row.Time, Field: "alignment", Want: fmt.Sprintf("index %d", row.Index),
				Got: "missing Engine event", Class: "A. INPUT ALIGNMENT",
			})
			continue
		}
		if !ev.Lineage.IntervalStart.Equal(row.Time) {
			mismatches = append(mismatches, catMismatch{
				When: row.Time, Field: "timestamp", Want: row.Time.Format(time.RFC3339),
				Got: ev.Lineage.IntervalStart.Format(time.RFC3339), Class: "A. INPUT ALIGNMENT",
			})
		}
		if m := addNum(stats["V_RAW"], optionalFloat{Value: row.VRaw, Valid: true}, ev.VRaw, tolVRaw); m != nil {
			m.When = row.Time
			mismatches = append(mismatches, *m)
		}
		if m := addNum(stats["V_N"], row.VN, ev.VN, tolVN); m != nil {
			m.When = row.Time
			mismatches = append(mismatches, *m)
		}
		if m := addNum(stats["V1"], row.V1, ev.V1, tolDeriv); m != nil {
			m.When = row.Time
			mismatches = append(mismatches, *m)
		}
		if m := addNum(stats["V2"], row.V2, ev.V2, tolDeriv); m != nil {
			m.When = row.Time
			mismatches = append(mismatches, *m)
		}
		if m := addNum(stats["interval_mean_vn"], row.MeanVN, ev.IntervalMeanVN, tolMean); m != nil {
			m.When = row.Time
			mismatches = append(mismatches, *m)
		}
		if m := addNum(stats["predicted_next_V_N"], row.Predicted, ev.PredictedNextVN, tolVN); m != nil {
			m.When = row.Time
			mismatches = append(mismatches, *m)
		}
		if ev.PredictedNextVN.Available() && ev.VN.Available() && ev.PredictedNextVN.Value != ev.VN.Value {
			mismatches = append(mismatches, catMismatch{
				When: row.Time, Field: "predicted_next_V_N=V_N",
				Want: fmt.Sprintf("%.17g", ev.VN.Value), Got: fmt.Sprintf("%.17g", ev.PredictedNextVN.Value),
				Class: "F. IMPLEMENTATION DEFECT",
			})
		}
	}
	out := make(map[string]numFieldStat, len(stats))
	for k, st := range stats {
		finalizeStat(st)
		out[k] = *st
		logNumStat(t, *st)
	}
	return out, mismatches
}

func featuresFrom014C(row interpRow) FeatureResult {
	return FeatureResult{
		VN:           qty(row.VN, FeatureAvailable),
		V1:           qty(row.V1, FeatureAvailable),
		V2:           qty(row.V2, FeatureAvailable),
		Predicted:    qty(row.Predicted, FeatureAvailable),
		IntervalMean: qty(row.Activity, FeatureAvailable),
	}
}

// aptfConfirm is a diagnostic twin of frozen HEAD spy_volume_engine.observe.
// After Phase F-R, production confirmColor must match this state-write.
func aptfConfirm(raw string, prior InterpretationState) (cockpit string, next InterpretationState, transition string) {
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
		}
	}
	return color, InterpretationState{Color: color, PendingColor: pendingColor, PendingCount: pendingCount}, transition
}

func replay014C(rows []interpRow, confirm func(string, InterpretationState) (string, InterpretationState, string)) []InterpretationResult {
	var prior InterpretationState
	prevSession := ""
	out := make([]InterpretationResult, 0, len(rows))
	for _, row := range rows {
		if row.SessionKey != prevSession {
			prior = InterpretationState{}
			prevSession = row.SessionKey
		}
		res, next := Interpret(featuresFrom014C(row), prior)
		if confirm != nil {
			cockpit, aptfNext, transition := confirm(res.RawColor, prior)
			res.CockpitColor = cockpit
			res.Transition = transition
			next = aptfNext
		}
		if res.ColorAvailable() {
			prior = next
		}
		out = append(out, res)
	}
	return out
}

func compare014CScience(t *testing.T, rows []interpRow, got []InterpretationResult) (exact map[string][2]int, mismatches []catMismatch, near []string) {
	t.Helper()
	exact = map[string][2]int{}
	inc := func(field string, ok bool) {
		c := exact[field]
		c[1]++
		if ok {
			c[0]++
		}
		exact[field] = c
	}
	for i, row := range rows {
		res := got[i]
		if abs := math.Abs(row.Activity - 0.9); abs < thresholdBand {
			near = append(near, fmt.Sprintf("%s activity=%.17g near 0.9 raw=%s cockpit=%s go_raw=%s go_ind=%s",
				row.Time.Format(time.RFC3339), row.Activity, row.RawColor, row.Cockpit, res.RawColor, res.CockpitColor))
		}
		if abs := math.Abs(row.Activity - 1.1); abs < thresholdBand {
			near = append(near, fmt.Sprintf("%s activity=%.17g near 1.1 raw=%s cockpit=%s go_raw=%s go_ind=%s",
				row.Time.Format(time.RFC3339), row.Activity, row.RawColor, row.Cockpit, res.RawColor, res.CockpitColor))
		}
		if math.Abs(row.V1) <= phaseEpsBand || math.Abs(row.V2) <= phaseEpsBand {
			near = append(near, fmt.Sprintf("%s V1=%.17g V2=%.17g phase=%s go_phase=%s",
				row.Time.Format(time.RFC3339), row.V1, row.V2, row.Phase, res.Phase))
		}
		wantRaw := rawColor(row.Activity)
		inc("raw_color", res.RawColor == row.RawColor && res.RawColor == wantRaw)
		if res.RawColor != row.RawColor {
			mismatches = append(mismatches, catMismatch{
				When: row.Time, Field: "raw_color", Want: row.RawColor, Got: res.RawColor,
				Class:  "D. THRESHOLD/CATEGORICAL DIFFERENCE",
				Detail: fmt.Sprintf("activity=%.17g", row.Activity),
			})
		}
		inc("phase", res.Phase == row.Phase)
		if res.Phase != row.Phase {
			mismatches = append(mismatches, catMismatch{
				When: row.Time, Field: "phase", Want: row.Phase, Got: res.Phase,
				Class:  "D. THRESHOLD/CATEGORICAL DIFFERENCE",
				Detail: fmt.Sprintf("V1=%.17g V2=%.17g", row.V1, row.V2),
			})
		}
		inc("confidence", res.Confidence == row.Confidence)
		if res.Confidence != row.Confidence {
			mismatches = append(mismatches, catMismatch{
				When: row.Time, Field: "confidence", Want: row.Confidence, Got: res.Confidence,
				Class: "D. THRESHOLD/CATEGORICAL DIFFERENCE",
			})
		}
		inc("domain_state", res.DomainState == row.Domain)
		if res.DomainState != row.Domain {
			mismatches = append(mismatches, catMismatch{
				When: row.Time, Field: "domain_state", Want: row.Domain, Got: res.DomainState,
				Class: "D. THRESHOLD/CATEGORICAL DIFFERENCE",
			})
		}
		inc("Indicator/cockpit_color", res.CockpitColor == row.Cockpit)
		if res.CockpitColor != row.Cockpit {
			mismatches = append(mismatches, catMismatch{
				When: row.Time, Field: "Indicator/cockpit_color", Want: row.Cockpit, Got: res.CockpitColor,
				Class:  "F. IMPLEMENTATION DEFECT / confirmation-state bookkeeping",
				Detail: fmt.Sprintf("raw=%s transition_want=%s transition_got=%s activity=%.17g", row.RawColor, row.Transition, res.Transition, row.Activity),
			})
		}
		inc("transition_state", res.Transition == row.Transition)
		if res.Transition != row.Transition {
			mismatches = append(mismatches, catMismatch{
				When: row.Time, Field: "transition_state", Want: row.Transition, Got: res.Transition,
				Class: "F. IMPLEMENTATION DEFECT / confirmation-state bookkeeping",
			})
		}
	}
	return exact, mismatches, near
}

func TestF01FixtureHashes(t *testing.T) {
	t.Log("invariant: vendored Phase F fixtures must not mutate silently")
	requireFileSHA256(t, testdataPath("APTF_TEST_014C_SPY_V_EMISSION_POLICY_V0_1.json"), fixPolicySHA)
	requireFileSHA256(t, testdataPath("level1_source_prefix.csv"), fixSrcSHA)
	requireFileSHA256(t, testdataPath("level1_010_prefix.csv"), fix010SHA)
	requireFileSHA256(t, testdataPath("level1_014c_first_session.csv"), fix014CSHA)
}

func TestF02FrozenPolicyMatchesGoConstants(t *testing.T) {
	t.Log("invariant: vendored 014C policy parameters equal FrozenConfig")
	body, err := os.ReadFile(testdataPath("APTF_TEST_014C_SPY_V_EMISSION_POLICY_V0_1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Parameters struct {
			Confirmation float64 `json:"confirmation_observations"`
			Epsilon      float64 `json:"epsilon"`
			Lower        float64 `json:"lower_threshold"`
			PolicyID     string  `json:"policy_id"`
			StateSource  string  `json:"state_source"`
			Upper        float64 `json:"upper_threshold"`
		} `json:"parameters"`
		Selected string `json:"selected_candidate"`
		Price    bool   `json:"price_inputs_used"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Parameters.PolicyID != InterpretationID || doc.Selected != InterpretationID {
		t.Fatalf("policy %q selected %q", doc.Parameters.PolicyID, doc.Selected)
	}
	if doc.Parameters.StateSource != StateSource {
		t.Fatalf("state_source %q", doc.Parameters.StateSource)
	}
	if doc.Parameters.Lower != LowerThreshold || doc.Parameters.Upper != UpperThreshold {
		t.Fatalf("thresholds %g %g", doc.Parameters.Lower, doc.Parameters.Upper)
	}
	if int(doc.Parameters.Confirmation) != ConfirmationCount {
		t.Fatalf("confirmation %g", doc.Parameters.Confirmation)
	}
	if doc.Parameters.Epsilon != Epsilon {
		t.Fatalf("epsilon %g", doc.Parameters.Epsilon)
	}
	if doc.Price {
		t.Fatal("frozen policy claims price_inputs_used")
	}
}

func TestF03Level1EngineFeatureReplay(t *testing.T) {
	t.Log("Level 1: Engine replay of first 80 source observations vs 010 prefix")
	src := loadLevel1Source(t)
	if len(src) != 80 {
		t.Fatalf("source rows %d", len(src))
	}
	if src[14].Time.Format(time.RFC3339) != "2023-03-30T08:14:00Z" || src[15].Time.Format(time.RFC3339) != "2023-03-30T08:16:00Z" {
		t.Fatalf("expected irregular 08:14→08:16, got %s → %s", src[14].Time, src[15].Time)
	}
	evs := replayEngine(t, src)
	feats := loadFeatCSV(t, testdataPath("level1_010_prefix.csv"))
	if len(feats) == 0 {
		t.Fatal("no 010 prefix rows")
	}
	if feats[0].Index != 16 {
		t.Fatalf("first 010 observation_index=%d want 16", feats[0].Index)
	}
	stats, mismatches := compareFeatures(t, src, evs, feats)
	for _, m := range mismatches {
		t.Errorf("%s %s want %s got %s class=%s %s",
			m.When.Format(time.RFC3339), m.Field, m.Want, m.Got, m.Class, m.Detail)
	}
	for _, name := range []string{"V_RAW", "V_N", "V1", "V2", "interval_mean_vn", "predicted_next_V_N"} {
		if stats[name].Outside != 0 {
			t.Errorf("%s has %d values outside tolerance", name, stats[name].Outside)
		}
	}
	firstVN := evs[14]
	if !firstVN.VN.Available() {
		t.Fatal("first finite V_N should be due at 1-based observation 15")
	}
	if evs[15].V1.Available() {
		t.Fatal("V1 must not be available at 1-based observation 16 (only two finite V_N)")
	}
	if !evs[16].V1.Available() {
		t.Fatal("V1 should be due at 1-based observation 17")
	}
}

func TestF04Level1InterpretationRawPhase(t *testing.T) {
	t.Log("Level 1: session-sliced Interpret raw/phase/confidence/domain vs first 014C session")
	rows := loadInterpCSV(t, testdataPath("level1_014c_first_session.csv"))
	if len(rows) != 82 {
		t.Fatalf("014C first session rows %d", len(rows))
	}
	got := replay014C(rows, nil)
	exact, mismatches, near := compare014CScience(t, rows, got)
	for _, field := range []string{"raw_color", "phase", "confidence", "domain_state"} {
		c := exact[field]
		t.Logf("%s %d/%d exact", field, c[0], c[1])
		if c[0] != c[1] {
			t.Errorf("%s mismatches %d", field, c[1]-c[0])
		}
	}
	for _, m := range mismatches {
		if m.Field == "raw_color" || m.Field == "phase" || m.Field == "confidence" || m.Field == "domain_state" {
			t.Errorf("%s %s want %s got %s %s", m.When.Format(time.RFC3339), m.Field, m.Want, m.Got, m.Detail)
		}
	}
	for _, line := range near {
		t.Logf("threshold/phase-band: %s", line)
	}
}

func TestF05Level1IndicatorMatches014C(t *testing.T) {
	t.Log("Level 1: production Interpret Indicator/transition match first 014C session")
	rows := loadInterpCSV(t, testdataPath("level1_014c_first_session.csv"))
	got := replay014C(rows, nil)
	exact, mismatches, _ := compare014CScience(t, rows, got)
	for _, field := range []string{"raw_color", "phase", "confidence", "domain_state", "Indicator/cockpit_color", "transition_state"} {
		c := exact[field]
		t.Logf("%s %d/%d", field, c[0], c[1])
		if c[0] != c[1] {
			t.Errorf("%s mismatches %d", field, c[1]-c[0])
		}
	}
	for _, m := range mismatches {
		t.Errorf("%s %s want %s got %s %s", m.When.Format(time.RFC3339), m.Field, m.Want, m.Got, m.Detail)
	}
}

func TestF06Level1ProductionMatchesAPTFOracle(t *testing.T) {
	t.Log("Level 1: production confirmColor is identical to the frozen APTF state-write oracle")
	rows := loadInterpCSV(t, testdataPath("level1_014c_first_session.csv"))
	prod := replay014C(rows, nil)
	oracle := replay014C(rows, aptfConfirm)
	if len(prod) != len(oracle) {
		t.Fatalf("len prod=%d oracle=%d", len(prod), len(oracle))
	}
	for i := range prod {
		if prod[i].CockpitColor != oracle[i].CockpitColor || prod[i].Transition != oracle[i].Transition {
			t.Fatalf("%s prod Indicator=%s trans=%s oracle Indicator=%s trans=%s",
				rows[i].Time.Format(time.RFC3339), prod[i].CockpitColor, prod[i].Transition, oracle[i].CockpitColor, oracle[i].Transition)
		}
	}
}

func TestF07ProductionHasNoSessionReset(t *testing.T) {
	t.Log("invariant: production Engine has no date/session reset; Reset clears both Feature and Interpretation")
	body, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, tok := range []string{"PREMARKET", "AFTERHOURS", "date:session", "session_id"} {
		if strings.Contains(text, tok) {
			t.Errorf("engine.go mentions historical session token %s", tok)
		}
	}
	e := mustEngine(t, frozenTestEntity)
	_ = commitBar(t, e, testBar(frozenTestEntity, time.Date(2023, 3, 30, 8, 0, 0, 0, time.UTC), 100, "frozen-reset"))
	if e.Snapshot().RawLen() != 1 {
		t.Fatal("expected one committed raw")
	}
	e.Reset()
	s := e.Snapshot()
	if s.RawLen() != 0 || !s.InterpretationIsEmpty() {
		t.Fatalf("Reset must clear Feature and Interpretation, got %+v", s)
	}
}

func TestF08ProductionDoesNotReadTestdata(t *testing.T) {
	t.Log("invariant: production volume sources do not depend on testdata/CSV")
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(body)
		for _, tok := range []string{"testdata", "encoding/csv", "APTF_TEST_"} {
			if strings.Contains(text, tok) {
				t.Errorf("%s mentions %s", path, tok)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
