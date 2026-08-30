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
