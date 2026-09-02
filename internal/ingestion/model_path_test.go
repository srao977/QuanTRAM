package ingestion

import (
	"testing"
	"time"

	"quantram/internal/domain"
)

func TestSubscribeFinalizedStillDropOldest(t *testing.T) {
	pipeline := newTestPipeline("AAPL")
	id, bars := pipeline.SubscribeFinalized(2)
	defer pipeline.Unsubscribe(id)

	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	first := liveBar(start, 1, domain.QualityComplete, true, false)
	second := liveBar(start.Add(time.Minute), 2, domain.QualityComplete, true, false)
	third := liveBar(start.Add(2*time.Minute), 3, domain.QualityComplete, true, false)
	pipeline.accept(first)
	pipeline.accept(second)
	pipeline.accept(third)

	got := drainBars(bars)
	if len(got) != 2 || got[0].Close != 2 || got[1].Close != 3 {
		t.Fatalf("lossy finalized path should drop oldest and keep [2,3], got %v", closes(got))
	}
}

func TestSubscribeModelBarsNoSilentDrop(t *testing.T) {
	pipeline := newTestPipeline("AAPL")
	id, bars := pipeline.SubscribeModelBars(2)
	defer pipeline.Unsubscribe(id)

	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	n0 := liveBar(start, 100, domain.QualityComplete, true, false)
	n1 := liveBar(start.Add(time.Minute), 101, domain.QualityComplete, true, false)
	n2 := liveBar(start.Add(2*time.Minute), 102, domain.QualityComplete, true, false)
	pipeline.accept(n0)
	pipeline.accept(n1)
	pipeline.accept(n2)

	status := pipeline.ModelPathStatus("AAPL")
	if !status.Discontinuous || status.Reason != domain.SkipQueueOverflow {
		t.Fatalf("expected QUEUE_OVERFLOW, got %+v", status)
	}
	got := drainBars(bars)
	if len(got) != 2 || got[0].Close != 100 || got[1].Close != 101 {
		t.Fatalf("model path must keep in-order prefix, got %+v", closes(got))
	}
	if containsClose(got, 102) {
		t.Fatal("overflowed bar must not replace an earlier bar")
	}

	later := liveBar(start.Add(3*time.Minute), 103, domain.QualityComplete, true, false)
	pipeline.accept(later)
	if extra := drainBars(bars); len(extra) != 0 {
		t.Fatalf("later bar must not evaluate after discontinuity, got %+v", closes(extra))
	}
	if pipeline.ReadinessFor("AAPL").Infer {
		t.Fatal("ReadinessFor must gate infer after overflow")
	}
}

func TestSubscribeModelBarsIrregularIntervalDelivers(t *testing.T) {
	pipeline := newTestPipeline("AAPL")
	id, bars := pipeline.SubscribeModelBars(8)
	defer pipeline.Unsubscribe(id)

	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	pipeline.accept(liveBar(start, 100, domain.QualityComplete, true, false))
	pipeline.accept(liveBar(start.Add(2*time.Minute), 102, domain.QualityComplete, true, false))

	status := pipeline.ModelPathStatus("AAPL")
	if status.Discontinuous {
		t.Fatalf("irregular interval must not latch discontinuity, got %+v", status)
	}
	got := drainBars(bars)
	if len(got) != 2 || got[0].Close != 100 || got[1].Close != 102 {
		t.Fatalf("10:32-absent then 10:34 must deliver both bars, got %+v", closes(got))
	}
}

func TestSubscribeModelBarsIgnoresPartials(t *testing.T) {
	pipeline := newTestPipeline("AAPL")
	id, bars := pipeline.SubscribeModelBars(4)
	defer pipeline.Unsubscribe(id)
	observeID, observe := pipeline.Subscribe(4)
	defer pipeline.Unsubscribe(observeID)

	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	partial := liveBar(start, 100, domain.QualityPartial, false, false)
	final := liveBar(start, 101, domain.QualityComplete, true, false)
	pipeline.accept(partial)
	pipeline.accept(final)

	modelGot := drainBars(bars)
	if len(modelGot) != 1 || !modelGot[0].IsFinal {
		t.Fatalf("model path should see only the final bar, got %+v", modelGot)
	}
	observeGot := drainBars(observe)
	if len(observeGot) != 2 {
		t.Fatalf("observe path should still see partial and final, got %d", len(observeGot))
	}
}

func TestSubscribeModelBarsIgnoresBackfillAndKeepsLive(t *testing.T) {
	pipeline := newTestPipeline("AAPL")
	id, bars := pipeline.SubscribeModelBars(8)
	defer pipeline.Unsubscribe(id)

	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	for i := 0; i < 15; i++ {
		pipeline.accept(liveBar(start.Add(time.Duration(i)*time.Minute), float64(100+i), domain.QualityReconstructed, true, true))
	}
	if pipeline.ModelPathStatus("AAPL").Discontinuous {
		t.Fatal("REST backfill must not mark the model path discontinuous")
	}
	if got := drainBars(bars); len(got) != 0 {
		t.Fatalf("backfill must not enter the model path, got %+v", closes(got))
	}

	live := liveBar(start.Add(15*time.Minute), 200, domain.QualityComplete, true, false)
	pipeline.accept(live)
	got := drainBars(bars)
	if len(got) != 1 || got[0].Close != 200 || pipeline.ModelPathStatus("AAPL").Discontinuous {
		t.Fatalf("first live eligible bar should cold-start, got %+v status=%+v", closes(got), pipeline.ModelPathStatus("AAPL"))
	}
}

func TestResetModelPathAllowsResume(t *testing.T) {
	pipeline := newTestPipeline("AAPL")
	id, bars := pipeline.SubscribeModelBars(2)
	defer pipeline.Unsubscribe(id)
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	pipeline.accept(liveBar(start, 1, domain.QualityComplete, true, false))
	pipeline.accept(liveBar(start.Add(time.Minute), 2, domain.QualityComplete, true, false))
	pipeline.accept(liveBar(start.Add(2*time.Minute), 3, domain.QualityComplete, true, false))
	drainBars(bars)
	pipeline.ResetModelPath("AAPL")
	if pipeline.ModelPathStatus("AAPL").Discontinuous {
		t.Fatal("reset should clear discontinuity")
	}
	resume := liveBar(start.Add(10*time.Minute), 10, domain.QualityComplete, true, false)
	pipeline.accept(resume)
	got := drainBars(bars)
	if len(got) != 1 || got[0].Close != 10 {
		t.Fatalf("after reset, cold-start bar should deliver, got %+v", closes(got))
	}
}

func newTestPipeline(symbols ...string) *Pipeline {
	p := NewPipeline(nil, nil, "TEST", symbols)
	p.breaker.MarkHealthy()
	return p
}

func drainBars(ch <-chan domain.Bar) []domain.Bar {
	var out []domain.Bar
	for {
		select {
		case bar := <-ch:
			out = append(out, bar)
		default:
			return out
		}
	}
}

func containsClose(bars []domain.Bar, close float64) bool {
	for _, bar := range bars {
		if bar.Close == close {
			return true
		}
	}
	return false
}

func closes(bars []domain.Bar) []float64 {
	out := make([]float64, len(bars))
	for i, bar := range bars {
		out[i] = bar.Close
	}
	return out
}
