package modelhost

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"quantram/internal/adaptive"
	"quantram/internal/config"
	"quantram/internal/domain"
	"quantram/internal/ingestion"
)

type fakeSource struct {
	mu     sync.Mutex
	bars   chan domain.Bar
	infer  bool
	status map[string]ingestion.ModelPathStatus
}

func newFake(buffer int) *fakeSource {
	return &fakeSource{
		bars:   make(chan domain.Bar, buffer),
		infer:  true,
		status: map[string]ingestion.ModelPathStatus{},
	}
}

func (f *fakeSource) SubscribeModelBars(int) (uint64, <-chan domain.Bar) {
	return 1, f.bars
}
func (f *fakeSource) Unsubscribe(uint64) {}
func (f *fakeSource) Readiness() domain.Readiness {
	f.mu.Lock()
	defer f.mu.Unlock()
	return domain.Readiness{Ready: true, Observe: true, Infer: f.infer}
}
func (f *fakeSource) ReadinessFor(string) domain.Readiness { return f.Readiness() }
func (f *fakeSource) ModelPathStatus(symbol string) ingestion.ModelPathStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status[symbol]
}
func (f *fakeSource) setInfer(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.infer = v
}
func (f *fakeSource) setStatus(symbol string, st ingestion.ModelPathStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[symbol] = st
}
func (f *fakeSource) push(bar domain.Bar) { f.bars <- bar }

func finalBar(symbol string, start time.Time, close float64) domain.Bar {
	return domain.Bar{
		Symbol:           symbol,
		Interval:         domain.Interval1Min,
		IntervalStart:    start,
		IntervalEnd:      start.Add(time.Minute),
		Close:            close,
		Volume:           1000,
		QualityStatus:    domain.QualityComplete,
		IsFinal:          true,
		Source:           "ALPACA_IEX",
		MarketSnapshotID: fmt.Sprintf("snap-%s-%s-%.4f", symbol, start.UTC().Format(time.RFC3339Nano), close),
	}
}

func collect(t *testing.T, events <-chan domain.DecisionEvent, n int, timeout time.Duration) []domain.DecisionEvent {
	t.Helper()
	out := make([]domain.DecisionEvent, 0, n)
	deadline := time.After(timeout)
	for len(out) < n {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("events closed after %d, want %d", len(out), n)
			}
			out = append(out, ev)
		case <-deadline:
			t.Fatalf("timed out after %d events, want %d", len(out), n)
		}
	}
	return out
}

func startHost(t *testing.T, src BarSource, symbols []string, opts Options) (*Host, context.CancelFunc) {
	t.Helper()
	if opts.Deadline == 0 {
		opts.Deadline = 200 * time.Millisecond
	}
	opts.Mode = config.ModelAdaptive
	host, err := New(src, symbols, opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = host.Run(ctx) }()
	deadline := time.Now().Add(time.Second)
	for !host.started.Load() {
		if time.Now().After(deadline) {
			t.Fatal("host did not subscribe")
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Cleanup(cancel)
	return host, cancel
}

func TestColdStartInitializing(t *testing.T) {
	src := newFake(32)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{})
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	for i := 0; i < adaptive.ContextLength; i++ {
		src.push(finalBar("AAPL", start.Add(time.Duration(i)*time.Minute), 100+float64(i)))
		ev := collect(t, host.Events(), 1, time.Second)[0]
		if !ev.IsSkip() || ev.Skip.Reason != domain.SkipInitializing {
			t.Fatalf("step %d want INITIALIZING, got %+v", i+1, ev.Skip)
		}
	}
	h := host.SymbolHealth("AAPL")
	if h.Status != StatusInitializing || h.Accepted != adaptive.ContextLength {
		t.Fatalf("health %+v", h)
	}
	src.push(finalBar("AAPL", start.Add(time.Duration(adaptive.ContextLength)*time.Minute), 120))
	ev := collect(t, host.Events(), 1, time.Second)[0]
	if !ev.IsDecision() || ev.Decision.ModelStatus != domain.StatusActionable {
		t.Fatalf("16th should be ACTIONABLE, got %+v / %+v", ev.Decision, ev.Skip)
	}
}

func TestInferPauseDoesNotReset(t *testing.T) {
	src := newFake(8)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{})
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	src.push(finalBar("AAPL", start, 100))
	src.push(finalBar("AAPL", start.Add(time.Minute), 101))
	_ = collect(t, host.Events(), 2, time.Second)
	hash := host.WorkerHash("AAPL")
	count := host.WorkerCompleted("AAPL")
	src.setInfer(false)
	src.push(finalBar("AAPL", start.Add(2*time.Minute), 102))
	ev := collect(t, host.Events(), 1, time.Second)[0]
	if !ev.IsSkip() || ev.Skip.Reason != domain.SkipInferOff {
		t.Fatalf("want INFER_OFF, got %+v", ev.Skip)
	}
	if host.WorkerHash("AAPL") != hash || host.WorkerCompleted("AAPL") != count {
		t.Fatal("infer pause reset scientific state")
	}
	if host.SymbolHealth("AAPL").Status != StatusPaused {
		t.Fatalf("status=%s", host.SymbolHealth("AAPL").Status)
	}
}

func TestDuplicateOrRegressionSkip(t *testing.T) {
	src := newFake(8)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{})
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	bar := finalBar("AAPL", start, 100)
	src.push(bar)
	_ = collect(t, host.Events(), 1, time.Second)
	hash := host.WorkerHash("AAPL")
	src.push(bar)
	ev := collect(t, host.Events(), 1, time.Second)[0]
	if !ev.IsSkip() || ev.Skip.Reason != domain.SkipDuplicateOrRegression {
		t.Fatalf("want DUPLICATE_OR_REGRESSION, got %+v", ev.Skip)
	}
	if host.WorkerHash("AAPL") != hash {
		t.Fatal("duplicate committed state")
	}
}

func TestHostInputGapMarksDiscontinuous(t *testing.T) {
	src := newFake(8)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{})
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	src.push(finalBar("AAPL", start, 100))
	_ = collect(t, host.Events(), 1, time.Second)
	src.push(finalBar("AAPL", start.Add(3*time.Minute), 103))
	ev := collect(t, host.Events(), 1, time.Second)[0]
	if !ev.IsSkip() || ev.Skip.Reason != domain.SkipInputGap {
		t.Fatalf("want INPUT_GAP, got %+v", ev.Skip)
	}
	src.push(finalBar("AAPL", start.Add(4*time.Minute), 104))
	ev = collect(t, host.Events(), 1, time.Second)[0]
	if !ev.IsSkip() || ev.Skip.Reason != domain.SkipStateDiscontinuous {
		t.Fatalf("later bar must be STATE_DISCONTINUOUS, got %+v", ev.Skip)
	}
}

func TestTimeoutDoesNotCommit(t *testing.T) {
	src := newFake(8)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{Deadline: 20 * time.Millisecond, Delay: 60 * time.Millisecond})
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	hash := host.WorkerHash("AAPL")
	src.push(finalBar("AAPL", start, 100))
	ev := collect(t, host.Events(), 1, time.Second)[0]
	if !ev.IsSkip() || ev.Skip.Reason != domain.SkipTimeout {
		t.Fatalf("want TIMEOUT, got %+v", ev.Skip)
	}
	if ev.IsDecision() {
		t.Fatal("timeout must not carry a decision")
	}
	if host.WorkerHash("AAPL") != hash || host.WorkerCompleted("AAPL") != 0 {
		t.Fatal("timeout committed state")
	}
}

func TestPanicIsolatesSymbol(t *testing.T) {
	src := newFake(16)
	host, _ := startHost(t, src, []string{"AAPL", "MSFT"}, Options{PanicOn: "AAPL"})
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	src.push(finalBar("AAPL", start, 100))
	src.push(finalBar("MSFT", start, 200))
	got := collect(t, host.Events(), 2, 2*time.Second)
	var aapl, msft *domain.DecisionEvent
	for i := range got {
		switch got[i].Symbol {
		case "AAPL":
			aapl = &got[i]
		case "MSFT":
			msft = &got[i]
		}
	}
	if aapl == nil || !aapl.IsSkip() || aapl.Skip.Reason != domain.SkipEnginePanic {
		t.Fatalf("AAPL want ENGINE_PANIC, got %+v", aapl)
	}
	if msft == nil || !msft.IsSkip() || msft.Skip.Reason != domain.SkipInitializing {
		t.Fatalf("MSFT should still initialize, got %+v", msft)
	}
}

func TestReconstructedNeverEvaluates(t *testing.T) {
	src := newFake(8)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{})
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	bar := finalBar("AAPL", start, 100)
	bar.IsBackfilled = true
	bar.QualityStatus = domain.QualityReconstructed
	src.push(bar)
	ev := collect(t, host.Events(), 1, time.Second)[0]
	if !ev.IsSkip() || ev.Skip.Reason != domain.SkipNotModelEligible {
		t.Fatalf("want NOT_MODEL_ELIGIBLE, got %+v", ev.Skip)
	}
	if host.WorkerCompleted("AAPL") != 0 {
		t.Fatal("reconstructed bar evaluated")
	}
}

func TestWorkerOverflowSkip(t *testing.T) {
	src := newFake(16)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{Delay: 80 * time.Millisecond, InboxSize: 2})
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	for i := 0; i < 6; i++ {
		src.push(finalBar("AAPL", start.Add(time.Duration(i)*time.Minute), 100+float64(i)))
	}
	deadline := time.After(2 * time.Second)
	var sawOverflow bool
	for !sawOverflow {
		select {
		case ev := <-host.Events():
			if ev.Skip != nil && ev.Skip.Reason == domain.SkipQueueOverflow {
				sawOverflow = true
			}
		case <-deadline:
			t.Fatal("expected QUEUE_OVERFLOW")
		}
	}
}

func TestShutdownDoesNotSendOnClosed(t *testing.T) {
	src := newFake(4)
	host, err := New(src, []string{"AAPL"}, Options{Mode: config.ModelAdaptive, Deadline: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- host.Run(ctx) }()
	src.push(finalBar("AAPL", time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC), 100))
	_ = collect(t, host.Events(), 1, time.Second)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("host did not stop")
	}
	for range host.Events() {
	}
}

func TestPartialNeverReachesHostViaPipeline(t *testing.T) {
	pipeline := ingestion.NewPipeline(nil, nil, "TEST", []string{"AAPL"})
	host, cancel := startHost(t, pipeline, []string{"AAPL"}, Options{})
	defer cancel()
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	partial := finalBar("AAPL", start, 100)
	partial.IsFinal = false
	partial.QualityStatus = domain.QualityPartial
	pipeline.InjectBar(partial)
	select {
	case ev := <-host.Events():
		t.Fatalf("partial reached host: %+v", ev)
	case <-time.After(80 * time.Millisecond):
	}
	pipeline.MarkFeedHealthy()
	complete := finalBar("AAPL", start, 101)
	pipeline.InjectBar(complete)
	ev := collect(t, host.Events(), 1, time.Second)[0]
	if ev.Skip == nil || ev.Skip.Reason != domain.SkipInitializing {
		if ev.Skip != nil && ev.Skip.Reason == domain.SkipInferOff {
			return
		}
		if ev.IsSkip() && ev.Skip.Reason == domain.SkipInitializing {
			return
		}
		t.Fatalf("expected initializing or infer-off for first complete, got %+v", ev.Skip)
	}
}

func TestNewNilWhenOff(t *testing.T) {
	host, err := New(newFake(1), []string{"AAPL"}, Options{Mode: config.ModelOff})
	if err != nil || host != nil {
		t.Fatalf("off must return nil host, got %v %v", host, err)
	}
}

func TestUnknownModeRejected(t *testing.T) {
	_, err := New(newFake(1), []string{"AAPL"}, Options{Mode: "nope", Deadline: time.Millisecond})
	if err == nil {
		t.Fatal("expected unknown mode error")
	}
}

func TestUnavailableHealthIsNotOff(t *testing.T) {
	h := Unavailable{}.Health()
	if h.State != domain.ComponentUnavailable || h.Detail != "unavailable" {
		t.Fatalf("%+v", h)
	}
	off := (*Host)(nil).Health()
	if off.State != domain.ComponentHealthy || off.Detail != "off" {
		t.Fatalf("nil host should report off, got %+v", off)
	}
}

func TestPanicDoesNotStopPipelineObserve(t *testing.T) {
	pipeline := ingestion.NewPipeline(nil, nil, "TEST", []string{"AAPL", "MSFT"})
	id, bars := pipeline.Subscribe(8)
	defer pipeline.Unsubscribe(id)
	host, cancel := startHost(t, pipeline, []string{"AAPL", "MSFT"}, Options{PanicOn: "AAPL"})
	defer cancel()
	pipeline.MarkFeedHealthy()
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	pipeline.InjectBar(finalBar("AAPL", start, 100))
	pipeline.InjectBar(finalBar("MSFT", start, 200))

	saw := map[string]bool{}
	deadline := time.After(2 * time.Second)
	for len(saw) < 2 {
		select {
		case bar := <-bars:
			saw[bar.Symbol] = true
		case <-deadline:
			t.Fatalf("P-02 observe missed bars after AAPL panic: %v", saw)
		}
	}

	got := collect(t, host.Events(), 2, 2*time.Second)
	var aapl, msft *domain.DecisionEvent
	for i := range got {
		switch got[i].Symbol {
		case "AAPL":
			aapl = &got[i]
		case "MSFT":
			msft = &got[i]
		}
	}
	if aapl == nil || !aapl.IsSkip() || aapl.Skip.Reason != domain.SkipEnginePanic {
		t.Fatalf("AAPL want ENGINE_PANIC, got %+v", aapl)
	}
	if msft == nil || !msft.IsSkip() {
		t.Fatalf("MSFT should still emit after AAPL panic, got %+v", msft)
	}
	if msft.Skip.Reason != domain.SkipInitializing && msft.Skip.Reason != domain.SkipInferOff {
		t.Fatalf("MSFT should initialize or infer-off, got %+v", msft.Skip)
	}
}

func TestSubscribeEventsFanout(t *testing.T) {
	src := newFake(8)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{})
	id1, ch1 := host.SubscribeEvents(4)
	id2, ch2 := host.SubscribeEvents(4)
	defer host.UnsubscribeEvents(id1)
	defer host.UnsubscribeEvents(id2)
	src.push(finalBar("AAPL", time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC), 100))
	e1 := collect(t, ch1, 1, time.Second)[0]
	e2 := collect(t, ch2, 1, time.Second)[0]
	if e1.EventID == "" || e1.EventID != e2.EventID {
		t.Fatalf("fanout mismatch %q vs %q", e1.EventID, e2.EventID)
	}
}
