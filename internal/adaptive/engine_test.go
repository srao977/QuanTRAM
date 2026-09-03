package adaptive

import (
	"math"
	"reflect"
	"testing"
	"time"

	"quantram/internal/domain"
)

func syntheticBar(i int, close float64, volume uint64) domain.Bar {
	start := time.Date(2022, 9, 30, 4, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Minute)
	bar, err := BarFromOHLCV("AAPL", start.Format("2006-01-02 15:04:05"), close, close, close, close, volume)
	if err != nil {
		panic(err)
	}
	return bar
}

func TestEngineWarmupThenActionable(t *testing.T) {
	eng := NewEngine("AAPL")
	price := 100.0
	for i := 0; i < ContextLength; i++ {
		price += 0.1
		ev := eng.Step(syntheticBar(i, price, 1000))
		if !ev.IsSkip() || ev.Skip.Reason != domain.SkipInitializing {
			t.Fatalf("step %d: want INITIALIZING skip, got %+v", i+1, ev)
		}
		if ev.Skip.Detail != "" && ev.Skip.ModelStatus != domain.StatusInitializing {
			t.Fatalf("step %d status=%s", i+1, ev.Skip.ModelStatus)
		}
		if ev.Decision != nil {
			t.Fatal("INITIALIZING must not carry a decision")
		}
	}
	if eng.CompletedCount() != ContextLength {
		t.Fatalf("completed=%d", eng.CompletedCount())
	}
	ev := eng.Step(syntheticBar(ContextLength, price+0.2, 1200))
	if !ev.IsDecision() {
		t.Fatalf("16th step should be a decision, got skip %+v", ev.Skip)
	}
	if ev.Decision.ModelStatus != domain.StatusActionable {
		t.Fatalf("status=%s", ev.Decision.ModelStatus)
	}
	switch ev.Decision.Side {
	case domain.SideBuy, domain.SideSell, domain.SideHold:
	default:
		t.Fatalf("side=%s", ev.Decision.Side)
	}
	if ev.Decision.Side == domain.SideHold && ev.Decision.EmitterPosition != domain.EmitterFlat {
		t.Fatalf("HOLD must keep prior position, got %s", ev.Decision.EmitterPosition)
	}
}

func TestEngineInvalidInputDoesNotCommit(t *testing.T) {
	eng := NewEngine("AAPL")
	_ = eng.Step(syntheticBar(0, 100, 10))
	hash := eng.StateHash()
	count := eng.CompletedCount()
	bad := syntheticBar(1, 101, 10)
	bad.Close = math.NaN()
	ev := eng.Step(bad)
	if !ev.IsSkip() || ev.Skip.Reason != domain.SkipInvalidInput {
		t.Fatalf("want INVALID_INPUT, got %+v", ev)
	}
	if eng.StateHash() != hash || eng.CompletedCount() != count {
		t.Fatal("invalid input committed state")
	}
}

func TestEngineDuplicateTimeDoesNotCommit(t *testing.T) {
	eng := NewEngine("AAPL")
	first := syntheticBar(0, 100, 10)
	_ = eng.Step(first)
	hash := eng.StateHash()
	ev := eng.Step(first)
	if !ev.IsSkip() || ev.Skip.Reason != domain.SkipDuplicateOrRegression {
		t.Fatalf("want DUPLICATE_OR_REGRESSION, got %+v", ev)
	}
	if eng.StateHash() != hash {
		t.Fatal("duplicate committed state")
	}
}

func TestEngineRetainsCanonicalPipelineOutputsOnCommit(t *testing.T) {
	eng := NewEngine("AAPL")
	bar := syntheticBar(0, 100, 10)
	event, working, commit := eng.PrepareStep(bar)
	if !commit {
		t.Fatal("valid evaluation was not prepared for commit")
	}
	if !event.IsSkip() || event.Skip.Reason != domain.SkipInitializing {
		t.Fatalf("want committed INITIALIZING event, got %+v", event)
	}
	want := working.LastPipelineOutputs()
	eng.Commit(working)
	got := eng.LastPipelineOutputs()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("committed outputs differ from prepared canonical outputs\ngot:  %+v\nwant: %+v", got, want)
	}

	got.DMO.ParameterState["ref_alpha"] = -1
	got.FMO.Samples[0].Level = -1
	got.ReturnShape.ForwardSamples[0].Level = -1
	if reflect.DeepEqual(got, eng.LastPipelineOutputs()) {
		t.Fatal("retained outputs must be defensively copied")
	}

	before := eng.LastPipelineOutputs()
	bad := syntheticBar(1, 101, 10)
	bad.Close = math.NaN()
	_ = eng.Step(bad)
	if !reflect.DeepEqual(before, eng.LastPipelineOutputs()) {
		t.Fatal("failed evaluation changed retained pipeline outputs")
	}
}

func TestDecidePredicate(t *testing.T) {
	adaptive := AdaptiveProperties{Prior15MedianC: 0.2, UpCount: 8, DownCount: 7}
	side, _ := decide(domain.PathUpward, 1, 0.3, adaptive)
	if side != domain.SideBuy {
		t.Fatalf("side=%s", side)
	}
	side, _ = decide(domain.PathDownward, 1, 0.3, AdaptiveProperties{Prior15MedianC: 0.2, UpCount: 4, DownCount: 11})
	if side != domain.SideSell {
		t.Fatalf("side=%s", side)
	}
	side, _ = decide(domain.PathUpward, 1, 0.1, adaptive)
	if side != domain.SideHold {
		t.Fatalf("low C should HOLD, got %s", side)
	}
}
