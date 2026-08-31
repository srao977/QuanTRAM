package ingestion

import (
	"testing"
	"time"

	"quantram/internal/domain"
)

func windowBar(start time.Time, close float64, quality domain.QualityStatus, final, backfilled bool) domain.Bar {
	return domain.Bar{
		Symbol:           "AAPL",
		Interval:         domain.Interval1Min,
		IntervalStart:    start,
		IntervalEnd:      start.Add(time.Minute),
		Close:            close,
		QualityStatus:    quality,
		IsFinal:          final,
		IsBackfilled:     backfilled,
		MarketSnapshotID: domain.SnapshotID("AAPL", "ALPACA_IEX", start.Format(time.RFC3339), 0, 0, 0, close, 0),
		Source:           "ALPACA_IEX",
	}
}

func TestWindowReplacesUpdatedBarThenFinal(t *testing.T) {
	store := NewWindowStore(8)
	start := time.Date(2026, 8, 31, 16, 52, 0, 0, time.UTC)
	partial := windowBar(start, 313.64, domain.QualityPartial, false, false)
	if !store.Add(partial) {
		t.Fatal("expected first partial to be stored")
	}
	updated := windowBar(start, 313.70, domain.QualityPartial, false, false)
	if !store.Add(updated) {
		t.Fatal("expected updated bar to replace partial")
	}
	final := windowBar(start, 313.80, domain.QualityComplete, true, false)
	if !store.Add(final) {
		t.Fatal("expected final bar to replace partial")
	}
	got := store.Window("AAPL", 0)
	if len(got) != 1 || !got[0].IsFinal || got[0].Close != 313.80 {
		t.Fatalf("window %+v", got)
	}
	lateUpdate := windowBar(start, 313.81, domain.QualityPartial, false, false)
	if store.Add(lateUpdate) {
		t.Fatal("final bar must not be replaced by a later partial")
	}
}

func TestWindowKeepsLiveCompleteOverReconstructed(t *testing.T) {
	store := NewWindowStore(8)
	start := time.Date(2026, 8, 31, 16, 52, 0, 0, time.UTC)
	live := windowBar(start, 313.80, domain.QualityComplete, true, false)
	if !store.Add(live) {
		t.Fatal("expected live bar")
	}
	fill := windowBar(start, 313.10, domain.QualityReconstructed, true, true)
	if store.Add(fill) {
		t.Fatal("reconstructed fill must not replace live complete")
	}
	if store.Window("AAPL", 0)[0].Close != 313.80 {
		t.Fatalf("unexpected %+v", store.Window("AAPL", 0))
	}
}

func TestWindowInsertsOutOfOrderChronologically(t *testing.T) {
	store := NewWindowStore(8)
	later := windowBar(time.Date(2026, 8, 31, 16, 53, 0, 0, time.UTC), 2, domain.QualityComplete, true, false)
	earlier := windowBar(time.Date(2026, 8, 31, 16, 52, 0, 0, time.UTC), 1, domain.QualityReconstructed, true, true)
	if !store.Add(later) || !store.Add(earlier) {
		t.Fatal("expected both bars stored")
	}
	got := store.Window("AAPL", 0)
	if len(got) != 2 || !got[0].IntervalStart.Before(got[1].IntervalStart) || got[0].Close != 1 || got[1].Close != 2 {
		t.Fatalf("order %+v", got)
	}
}
