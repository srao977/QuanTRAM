package pricing

import (
	"testing"
	"time"

	"quantram/internal/domain"
)

func TestPrepareStepDoesNotCommitUntilAsked(t *testing.T) {
	eng, err := NewEngine("AAPL")
	if err != nil {
		t.Fatal(err)
	}
	bar := domain.Bar{
		Symbol:        "AAPL",
		Interval:      domain.Interval1Min,
		IntervalStart: time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC),
		Close:         100,
		Volume:        1000,
	}
	hash := eng.StateHash()
	ev, working, commit := eng.PrepareStep(bar)
	if !commit || working == nil {
		t.Fatalf("warmup should be committable, ev=%+v", ev)
	}
	if eng.StateHash() != hash || eng.Received() != 0 {
		t.Fatal("PrepareStep mutated committed engine")
	}
	eng.Commit(working)
	if eng.Received() != 1 || eng.StateHash() == hash {
		t.Fatal("Commit did not adopt working state")
	}
}

func TestStepCommitsWarmup(t *testing.T) {
	eng, err := NewEngine("AAPL")
	if err != nil {
		t.Fatal(err)
	}
	bar := domain.Bar{
		Symbol:        "AAPL",
		Interval:      domain.Interval1Min,
		IntervalStart: time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC),
		Close:         100,
		Volume:        1000,
	}
	ev := eng.Step(bar)
	if ev.Status != domain.PricingStatusWarmupDerivative || eng.Received() != 1 {
		t.Fatalf("status=%s received=%d", ev.Status, eng.Received())
	}
}
