package adaptive

import (
	"math"
	"path/filepath"
	"strconv"
	"testing"

	"quantram/internal/domain"
)

// Unit Run 001 fixture identity. Origin: SADE/output/unit_runs/001/observations.csv
// (AAPL, 100 vectors, generated 2026-08-25). Recreated as OHLCV bars; no SDX.
const unitRun001SHA256 = "6c98e15df41f71d4369c22d4211f3fd651eda829a5046371faa38c426381f33a"

// Tolerance vs CPython SADE Unit Run 001. Exact for status, path, side, H.
// Transcendental (exp, log1p, pow): 16 ulp or 1e-12 absolute, whichever is looser.
var unitRunTolerance = map[string]float64{
	"Q_G":                   1e-12,
	"Q_S":                   1e-12,
	"Q_R":                   1e-12,
	"C":                     1e-12,
	"strength":              1e-12,
	"coherence":             1e-12,
	"persistence":           1e-12,
	"uncertainty":           1e-12,
	"reversal_propensity":   1e-12,
	"terminal_displacement": 1e-12,
}

func TestUnitRun001Equivalence(t *testing.T) {
	path := filepath.Join("testdata", "unit_run_001_observations.csv")
	sum, err := FileSHA256(path)
	if err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	if unitRun001SHA256 != "compute-on-copy" && sum != unitRun001SHA256 {
		t.Fatalf("fixture sha256 %s want %s", sum, unitRun001SHA256)
	}
	t.Logf("unit_run_001_observations.csv sha256=%s", sum)

	rows, err := LoadUnitRun001(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 100 {
		t.Fatalf("rows=%d want 100", len(rows))
	}

	eng := NewEngine("AAPL")
	var buys, sells, holds, initializing int
	for i, row := range rows {
		bar, err := row.Bar()
		if err != nil {
			t.Fatalf("row %d bar: %v", i+1, err)
		}
		ev := eng.Step(bar)
		if d := firstDivergence(row, ev, eng.LastEval()); d != "" {
			t.Fatalf("first divergence at sequence %d (%s): %s pre=%s post=%s",
				row.Index, row.SourceTimestamp, d, ev.PreStateHash, ev.PostStateHash)
		}
		switch {
		case ev.IsSkip() && ev.Skip.Reason == domain.SkipInitializing:
			initializing++
		case ev.IsDecision() && ev.Decision.Side == domain.SideBuy:
			buys++
		case ev.IsDecision() && ev.Decision.Side == domain.SideSell:
			sells++
		case ev.IsDecision() && ev.Decision.Side == domain.SideHold:
			holds++
		default:
			t.Fatalf("row %d unexpected outcome %+v / %+v", i+1, ev.Decision, ev.Skip)
		}
	}
	if initializing != 15 || buys != 8 || sells != 10 || holds != 67 {
		t.Fatalf("counts initializing=%d BUY=%d SELL=%d HOLD=%d want 15/8/10/67",
			initializing, buys, sells, holds)
	}
	if eng.PositionState() != domain.EmitterShort {
		t.Fatalf("terminal position %s want SHORT", eng.PositionState())
	}
}

func firstDivergence(row UnitRunRow, ev domain.DecisionEvent, snap EvalSnapshot) string {
	if row.Status == "INITIALIZING" {
		if !ev.IsSkip() || ev.Skip.Reason != domain.SkipInitializing {
			return fieldDiff("status", row.Status, "not INITIALIZING skip")
		}
	} else if row.Status == "ACTIONABLE" {
		if !ev.IsDecision() || ev.Decision.ModelStatus != domain.StatusActionable {
			return fieldDiff("status", row.Status, "not ACTIONABLE decision")
		}
		if string(ev.Decision.Side) != row.PositionDecision {
			return fieldDiff("side", row.PositionDecision, string(ev.Decision.Side))
		}
	} else {
		return fieldDiff("status", row.Status, "unknown expected status")
	}
	if snap.Path != row.PathDirection {
		return fieldDiff("path_direction", string(row.PathDirection), string(snap.Path))
	}
	if snap.H != row.H {
		return fieldDiff("H", strconv.Itoa(row.H), strconv.Itoa(snap.H))
	}
	for _, item := range []struct {
		name string
		got  float64
		want float64
	}{
		{"Q_G", snap.QG, row.QG},
		{"Q_S", snap.QS, row.QS},
		{"Q_R", snap.QR, row.QR},
		{"C", snap.C, row.C},
		{"strength", snap.Strength, row.Strength},
		{"coherence", snap.Coherence, row.Coherence},
		{"persistence", snap.Persistence, row.Persistence},
		{"uncertainty", snap.Uncertainty, row.Uncertainty},
		{"reversal_propensity", snap.Reversal, row.Reversal},
		{"terminal_displacement", snap.TerminalDisplacement, row.TerminalDisplacement},
	} {
		if !withinTolerance(item.got, item.want, unitRunTolerance[item.name]) {
			return fieldDiff(item.name, strconv.FormatFloat(item.want, 'g', 17, 64), strconv.FormatFloat(item.got, 'g', 17, 64))
		}
	}
	return ""
}

func withinTolerance(got, want, absTol float64) bool {
	if got == want {
		return true
	}
	if math.Abs(got-want) <= absTol {
		return true
	}
	return ulpDiff(got, want) <= 16
}

func ulpDiff(a, b float64) int {
	if a == b {
		return 0
	}
	if math.IsNaN(a) || math.IsNaN(b) || math.IsInf(a, 0) || math.IsInf(b, 0) {
		return 1 << 20
	}
	ai := math.Float64bits(a)
	bi := math.Float64bits(b)
	if (ai^bi)>>63 != 0 {
		return 1 << 20
	}
	if ai > bi {
		return int(ai - bi)
	}
	return int(bi - ai)
}

func fieldDiff(name, want, got string) string {
	return name + " expected=" + want + " actual=" + got
}
