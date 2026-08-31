package ingestion

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"quantram/internal/domain"
	"quantram/internal/marketfeed"
)

type stubHistorical struct {
	bars []domain.Bar
}

func (s stubHistorical) Bars(context.Context, marketfeed.BarRangeRequest) ([]domain.Bar, error) {
	return s.bars, nil
}

func TestWindowDedupAndPipelineCSV(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "aapl_sample.csv")
	live := marketfeed.NewCSVSource(path, "AAPL")
	pipeline := NewPipeline(live, nil, "CSV", []string{"AAPL"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	id, bars := pipeline.Subscribe(8)
	defer pipeline.Unsubscribe(id)

	done := make(chan error, 1)
	go func() {
		done <- pipeline.Run(ctx)
	}()

	var received []domain.Bar
	deadline := time.After(2 * time.Second)
	for len(received) < 5 {
		select {
		case bar := <-bars:
			received = append(received, bar)
		case <-deadline:
			t.Fatalf("timed out receiving bars, got %d", len(received))
		}
	}
	if received[0].SourceTimestamp != "2022-09-30 04:00:00" {
		t.Fatalf("first timestamp %s", received[0].SourceTimestamp)
	}
	if received[0].Open != 143.59 || received[4].Volume != 309 {
		t.Fatalf("fidelity failed: %+v %+v", received[0], received[4])
	}

	window := pipeline.Window("AAPL", 3)
	if len(window) != 3 || window[2].Close != 143.32 {
		t.Fatalf("window %+v", window)
	}

	cancel()
	<-done
}

func TestGapFillDedup(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "aapl_sample.csv")
	live := marketfeed.NewCSVSource(path, "AAPL")
	existing := domain.Bar{
		Symbol:          "AAPL",
		IntervalStart:   time.Date(2022, 9, 30, 4, 0, 0, 0, time.UTC),
		SourceTimestamp: "2022-09-30 04:00:00",
		Close:           143.49,
	}
	pipeline := NewPipeline(live, stubHistorical{bars: []domain.Bar{existing, existing}}, "CSV", []string{"AAPL"})
	fetched, injected, deduped, err := pipeline.GapFill(context.Background(), "AAPL", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if fetched != 2 || injected != 1 || deduped != 1 {
		t.Fatalf("fetched=%d injected=%d deduped=%d", fetched, injected, deduped)
	}
}

func TestInferGatedOnQuality(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "aapl_sample.csv")
	live := marketfeed.NewCSVSource(path, "AAPL")
	pipeline := NewPipeline(live, nil, "CSV", []string{"AAPL"})
	pipeline.breaker.MarkHealthy()

	now := time.Now().UTC().Truncate(time.Minute)
	minute := now.Add(-2 * time.Minute)
	first := liveBar(minute, 313.10, domain.QualityComplete, true, false)
	second := liveBar(minute.Add(time.Minute), 313.20, domain.QualityComplete, true, false)
	partial := liveBar(minute.Add(2*time.Minute), 313.30, domain.QualityPartial, false, false)
	reconstructed := liveBar(minute.Add(time.Minute), 313.00, domain.QualityReconstructed, true, true)

	pipeline.accept(first)
	if pipeline.Readiness().Infer {
		t.Fatal("one bar must not enable infer")
	}
	pipeline.accept(second)
	ready := pipeline.Readiness()
	if !ready.Infer || !ready.Observe {
		t.Fatalf("adjacent complete bars should enable infer: %+v", ready)
	}

	pipeline.accept(partial)
	if !pipeline.Readiness().Infer {
		t.Fatal("forming next minute should not disable infer")
	}
	window := pipeline.Window("AAPL", 0)
	if len(window) != 3 || window[2].QualityStatus != domain.QualityPartial {
		t.Fatalf("expected partial in observe window: %+v", window)
	}

	id, bars := pipeline.SubscribeFinalized(4)
	defer pipeline.Unsubscribe(id)
	pipeline.accept(liveBar(minute.Add(2*time.Minute), 313.40, domain.QualityComplete, true, false))
	select {
	case bar := <-bars:
		if !bar.IsFinal || bar.Close != 313.40 {
			t.Fatalf("consumer got %+v", bar)
		}
	case <-time.After(time.Second):
		t.Fatal("finalized subscriber did not receive closed bar")
	}

	gapped := NewPipeline(live, nil, "CSV", []string{"AAPL"})
	gapped.breaker.MarkHealthy()
	gapped.accept(first)
	gapped.accept(liveBar(minute.Add(3*time.Minute), 314, domain.QualityComplete, true, false))
	if gapped.Readiness().Infer {
		t.Fatal("gapped series must not enable infer")
	}

	filled := NewPipeline(live, stubHistorical{bars: []domain.Bar{reconstructed}}, "CSV", []string{"AAPL"})
	filled.accept(first)
	if _, _, _, err := filled.GapFill(context.Background(), "AAPL", time.Time{}, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if filled.Readiness().Infer {
		t.Fatal("reconstructed head must not enable infer")
	}
}

func liveBar(start time.Time, close float64, quality domain.QualityStatus, final, backfilled bool) domain.Bar {
	return domain.Bar{
		Symbol:           "AAPL",
		Interval:         domain.Interval1Min,
		IntervalStart:    start,
		IntervalEnd:      start.Add(time.Minute),
		Close:            close,
		QualityStatus:    quality,
		IsFinal:          final,
		IsBackfilled:     backfilled,
		Source:           "ALPACA_IEX",
		MarketSnapshotID: domain.SnapshotID("AAPL", "ALPACA_IEX", start.Format(time.RFC3339), 0, 0, 0, close, 0),
	}
}
