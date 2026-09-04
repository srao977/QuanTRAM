package pricing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"

	"quantram/internal/domain"
)

// pricingFixtureSHA256 freezes the reference P-04 output independently of the
// OHLCV fixture joined by the harness.
const pricingFixtureSHA256 = "4b3b8783108988e71c4bf2cec9b6f8a4c6bf929fb93a4be27706a16ef4c1752a"

// equivalenceManifest records both warmup topology and row-level semantic
// matches so a count-only pass cannot hide an earlier divergence.
type equivalenceManifest struct {
	FixtureSHA256        string `json:"fixture_sha256"`
	GonumVersion         string `json:"gonum_version"`
	BarsConsidered       int    `json:"bars_considered"`
	WarmupDerivative     int    `json:"warmup_derivative"`
	WarmupF4             int    `json:"warmup_f4"`
	Emitted              int    `json:"emitted"`
	ProjectionSuccess    int    `json:"projection_success"`
	ProjectionFailure    int    `json:"projection_failure"`
	DomainExit           int    `json:"domain_exit"`
	StatusMatches        int    `json:"status_matches"`
	ColorMatches         int    `json:"color_matches"`
	PhaseMatches         int    `json:"phase_matches"`
	ConfidenceMatches    int    `json:"confidence_matches"`
	RKSuccessMatches     int    `json:"rk_success_matches"`
	DomainExitMatches    int    `json:"domain_exit_matches"`
	CockpitOutputMatches int    `json:"cockpit_output_matches"`
	SemanticEquivalence  string `json:"semantic_equivalence"`
	FirstDivergence      string `json:"first_divergence,omitempty"`
}

func TestPricingUnitRun001Equivalence(t *testing.T) {
	pricingPath := filepath.Join("testdata", "pricing_unit_run_001_observations.csv")
	ohlcvPath := filepath.Join("testdata", "unit_run_001_ohlcv.csv")
	sum, err := FileSHA256(pricingPath)
	if err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	t.Logf("pricing_unit_run_001_observations.csv sha256=%s", sum)
	if pricingFixtureSHA256 != "compute-on-copy" && sum != pricingFixtureSHA256 {
		t.Fatalf("fixture sha256 %s want %s", sum, pricingFixtureSHA256)
	}

	rows, err := loadPricingFixture(pricingPath, ohlcvPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 100 {
		t.Fatalf("rows=%d want 100", len(rows))
	}

	eng, err := NewEngine("AAPL")
	if err != nil {
		t.Fatal(err)
	}

	m := equivalenceManifest{
		FixtureSHA256:  sum,
		GonumVersion:   "v0.17.0",
		BarsConsidered: len(rows),
	}
	var first string
	for i, row := range rows {
		bar, err := row.Bar()
		if err != nil {
			t.Fatalf("row %d bar: %v", i, err)
		}
		ev := eng.Step(bar)
		if d := pricingDivergence(row, ev); d != "" && first == "" {
			first = d
			t.Errorf("first divergence at %s: %s status got=%s want=%s", row.SourceTimestamp, d, ev.Status, row.PricingStatus)
		}
		switch ev.Status {
		case domain.PricingStatusWarmupDerivative:
			m.WarmupDerivative++
		case domain.PricingStatusWarmupF4:
			m.WarmupF4++
		case domain.PricingStatusEmitted:
			m.Emitted++
		case domain.PricingStatusProjectionFailure:
			m.ProjectionFailure++
		}
		if ev.RKSuccess {
			m.ProjectionSuccess++
		}
		if ev.DomainExit {
			m.DomainExit++
		}
		if string(ev.Status) == row.PricingStatus {
			m.StatusMatches++
		}
		if ev.Emission != nil && ev.Emission.Color == row.PricingColor {
			m.ColorMatches++
		} else if ev.Emission == nil && row.PricingColor == "" {
			m.ColorMatches++
		}
		if ev.Emission != nil && ev.Emission.TrajectoryPhase == row.PricingPhase {
			m.PhaseMatches++
		} else if ev.Emission == nil && row.PricingPhase == "" {
			m.PhaseMatches++
		}
		if ev.Emission != nil && ev.Emission.ConfidenceState == row.PricingConfidence {
			m.ConfidenceMatches++
		} else if ev.Emission == nil && row.PricingConfidence == "" {
			m.ConfidenceMatches++
		}
		if row.RKSuccess == nil || ev.RKSuccess == *row.RKSuccess {
			m.RKSuccessMatches++
		}
		if row.DomainExit == nil || ev.DomainExit == *row.DomainExit {
			m.DomainExitMatches++
		}
		hasCockpit := ev.Cockpit != nil
		if hasCockpit == row.CockpitOutput {
			m.CockpitOutputMatches++
		}
	}
	m.FirstDivergence = first
	if first == "" && m.WarmupDerivative == 15 && m.WarmupF4 == 30 && m.Emitted == 55 {
		m.SemanticEquivalence = "PASS"
	} else {
		m.SemanticEquivalence = "FAIL"
	}
	raw, _ := json.MarshalIndent(m, "", "  ")
	_ = os.WriteFile(filepath.Join("testdata", "p04_equivalence_manifest.json"), raw, 0o644)
	t.Logf("manifest %s", raw)
	if m.SemanticEquivalence != "PASS" {
		t.Fatalf("semantic equivalence FAIL warmup_d=%d warmup_f4=%d emitted=%d first=%s",
			m.WarmupDerivative, m.WarmupF4, m.Emitted, first)
	}
}

func pricingDivergence(row pricingRow, ev domain.PriceEvent) string {
	if string(ev.Status) != row.PricingStatus {
		return "pricing_status"
	}
	if ev.Emitted != row.PricingEmitted && (ev.Status == domain.PricingStatusEmitted || ev.Status == domain.PricingStatusProjectionFailure) {
		if ev.Emission == nil && row.PricingEmitted {
			return "pricing_emitted"
		}
	}
	if row.PricingEmitted {
		if ev.Emission == nil {
			return "missing_emission"
		}
		if ev.Emission.Color != row.PricingColor {
			return "pricing_color"
		}
		if ev.Emission.TrajectoryPhase != row.PricingPhase {
			return "pricing_phase"
		}
		if ev.Emission.ConfidenceState != row.PricingConfidence {
			return "pricing_confidence"
		}
		if row.RKSuccess != nil && ev.RKSuccess != *row.RKSuccess {
			return "rk_success"
		}
		if row.DomainExit != nil && ev.DomainExit != *row.DomainExit {
			return "domain_exit"
		}
		if (ev.Cockpit != nil) != row.CockpitOutput {
			return "price_cockpit_output"
		}
	}
	return ""
}

func gonumVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, m := range info.Deps {
		if m.Path == "gonum.org/v1/gonum" {
			return m.Version
		}
	}
	return "unknown"
}
