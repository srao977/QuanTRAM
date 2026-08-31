package domain

import (
	"testing"
	"time"
)

func testBar(start time.Time, quality QualityStatus, final, backfilled bool, source string) Bar {
	return Bar{
		Symbol:        "AAPL",
		Interval:      Interval1Min,
		IntervalStart: start,
		IntervalEnd:   start.Add(time.Minute),
		QualityStatus: quality,
		IsFinal:       final,
		IsBackfilled:  backfilled,
		Source:        source,
		Close:         100,
	}
}

func TestModelEligible(t *testing.T) {
	start := time.Date(2026, 8, 31, 16, 52, 0, 0, time.UTC)
	live := testBar(start, QualityComplete, true, false, "ALPACA_IEX")
	if !live.ModelEligible() {
		t.Fatal("complete live bar should be model eligible")
	}
	partial := testBar(start, QualityPartial, false, false, "ALPACA_IEX")
	if partial.ModelEligible() {
		t.Fatal("partial bar must not be model eligible")
	}
	reconstructed := testBar(start, QualityReconstructed, true, true, "ALPACA_IEX")
	if reconstructed.ModelEligible() {
		t.Fatal("reconstructed bar must not be model eligible")
	}
}

func TestInferReadyRequiresContiguousLiveHead(t *testing.T) {
	now := time.Date(2026, 8, 31, 16, 53, 10, 0, time.UTC)
	first := testBar(time.Date(2026, 8, 31, 16, 51, 0, 0, time.UTC), QualityComplete, true, false, "ALPACA_IEX")
	second := testBar(time.Date(2026, 8, 31, 16, 52, 0, 0, time.UTC), QualityComplete, true, false, "ALPACA_IEX")
	if InferReady([]Bar{first}, now) {
		t.Fatal("single bar is not enough continuity")
	}
	if !InferReady([]Bar{first, second}, now) {
		t.Fatal("two adjacent complete bars should enable infer")
	}

	gapped := testBar(time.Date(2026, 8, 31, 16, 53, 0, 0, time.UTC), QualityComplete, true, false, "ALPACA_IEX")
	if InferReady([]Bar{first, gapped}, now.Add(time.Minute)) {
		t.Fatal("gapped series must not enable infer")
	}

	filled := testBar(first.IntervalStart, QualityReconstructed, true, true, "ALPACA_IEX")
	if InferReady([]Bar{first, testBar(second.IntervalStart, QualityReconstructed, true, true, "ALPACA_IEX")}, now) {
		t.Fatal("reconstructed head must not enable infer")
	}
	if !InferReady([]Bar{filled, second}, now) {
		t.Fatal("reconstructed prior bar may prove continuity for a live head")
	}

	forming := testBar(time.Date(2026, 8, 31, 16, 53, 0, 0, time.UTC), QualityPartial, false, false, "ALPACA_IEX")
	if !InferReady([]Bar{first, second, forming}, now) {
		t.Fatal("a forming next minute must not disable infer on the last finalized bar")
	}
}

func TestLiveFreshWatermark(t *testing.T) {
	start := time.Date(2026, 8, 31, 16, 52, 0, 0, time.UTC)
	bar := testBar(start, QualityComplete, true, false, "ALPACA_IEX")
	if !bar.LiveFresh(start.Add(90 * time.Second)) {
		t.Fatal("bar should be fresh at interval end")
	}
	if !bar.LiveFresh(start.Add(time.Minute + MaxFinalLateness)) {
		t.Fatal("bar should remain fresh through allowed lateness")
	}
	if bar.LiveFresh(start.Add(time.Minute + MaxFinalLateness + time.Second)) {
		t.Fatal("bar should go stale past allowed lateness")
	}

	csv := testBar(start, QualityComplete, true, false, "CSV")
	if !csv.LiveFresh(start.Add(24 * time.Hour)) {
		t.Fatal("CSV replay must not use wall-clock freshness")
	}
}
