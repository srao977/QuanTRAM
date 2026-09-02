package server

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	quantramv1 "quantram/gen/quantram/v1"
	"quantram/internal/config"
	"quantram/internal/domain"
	"quantram/internal/ingestion"
	"quantram/internal/modelhost"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestToProtoDecisionEventOneof(t *testing.T) {
	now := time.Date(2026, 9, 1, 13, 45, 0, 0, time.UTC)
	decision := domain.DecisionEvent{
		EventID:          "AAPL:evt:16",
		SignalID:         "sig",
		DecisionID:       "dec",
		Symbol:           "AAPL",
		IntervalStart:    now,
		MarketSnapshotID: "snap",
		AcceptedSequence: 16,
		ReceivedAt:       now,
		CompletedAt:      now.Add(2 * time.Millisecond),
		Latency:          2 * time.Millisecond,
		ModelVersion:     "0.2",
		SchemaVersion:    "quantram.adaptive.v1",
		PreStateHash:     "pre",
		PostStateHash:    "post",
		Decision: &domain.Decision{
			Side:            domain.SideHold,
			Confidence:      0.5,
			H:               1,
			QG:              0.1,
			QS:              0.2,
			QR:              0.3,
			PathDirection:   domain.PathFlat,
			ModelStatus:     domain.StatusActionable,
			EmitterPosition: domain.EmitterLong,
		},
	}
	out := toProtoDecisionEvent(decision)
	if out.GetSkip() != nil || out.GetDecision() == nil {
		t.Fatalf("decision must set decision oneof only, skip=%v", out.GetSkip())
	}
	if out.GetDecision().GetSide() != quantramv1.Side_SIDE_HOLD {
		t.Fatalf("HOLD is a decision, got %s", out.GetDecision().GetSide())
	}
	if out.GetDecision().GetH() != 1 || out.GetLatencyMs() != 2 {
		t.Fatalf("H/latency %+v", out)
	}

	skip := domain.DecisionEvent{
		EventID:       "AAPL:host:1",
		Symbol:        "AAPL",
		IntervalStart: now,
		Skip:          &domain.Skip{Reason: domain.SkipInitializing, ModelStatus: domain.StatusInitializing},
	}
	skipped := toProtoDecisionEvent(skip)
	if skipped.GetDecision() != nil || skipped.GetSkip() == nil {
		t.Fatal("skip must set skip oneof only")
	}
	if skipped.GetSkip().GetReason() != quantramv1.SkipReason_SKIP_REASON_INITIALIZING {
		t.Fatalf("got %s", skipped.GetSkip().GetReason())
	}
}

func TestStreamDecisionsOffAndUnavailable(t *testing.T) {
	pipeline := ingestion.NewPipeline(nil, nil, "TEST", []string{"AAPL"})
	if err := New(pipeline, nil).StreamDecisions(&quantramv1.StreamDecisionsRequest{}, nil); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("off want FailedPrecondition, got %v", err)
	}
	if err := New(pipeline, modelhost.Unavailable{}).StreamDecisions(&quantramv1.StreamDecisionsRequest{}, nil); status.Code(err) != codes.Unavailable {
		t.Fatalf("unavailable want Unavailable, got %v", err)
	}
}

func TestStreamDecisionsLive(t *testing.T) {
	pipeline := ingestion.NewPipeline(nil, nil, "TEST", []string{"AAPL"})
	host, err := modelhost.New(pipeline, []string{"AAPL"}, modelhost.Options{
		Mode:     config.ModelAdaptive,
		Deadline: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = host.Run(ctx) }()
	waitUntil(t, time.Second, host.Started)

	lis := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	quantramv1.RegisterModelServiceServer(grpcServer, New(pipeline, host))
	go func() { _ = grpcServer.Serve(lis) }()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = lis.Close()
	})

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	pipeline.MarkFeedHealthy()
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	pipeline.InjectBar(domain.Bar{
		Symbol:           "AAPL",
		Interval:         domain.Interval1Min,
		IntervalStart:    start,
		IntervalEnd:      start.Add(time.Minute),
		Close:            100,
		Volume:           1000,
		QualityStatus:    domain.QualityComplete,
		IsFinal:          true,
		Source:           "TEST",
		MarketSnapshotID: "snap-e",
	})
	waitUntil(t, 2*time.Second, func() bool { return len(host.LastEvents()) > 0 })

	client := quantramv1.NewModelServiceClient(conn)
	streamCtx, streamCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer streamCancel()
	stream, err := client.StreamDecisions(streamCtx, &quantramv1.StreamDecisionsRequest{MaxEvents: 1})
	if err != nil {
		t.Fatal(err)
	}
	ev, err := stream.Recv()
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if ev.GetSymbol() != "AAPL" || ev.GetSkip() == nil {
		t.Fatalf("want skip for first bar, got %+v", ev)
	}
	reason := ev.GetSkip().GetReason()
	if reason != quantramv1.SkipReason_SKIP_REASON_INITIALIZING && reason != quantramv1.SkipReason_SKIP_REASON_INFER_OFF {
		t.Fatalf("unexpected skip %s", reason)
	}
}

func TestStreamPriceEventsOffAndUnavailable(t *testing.T) {
	pipeline := ingestion.NewPipeline(nil, nil, "TEST", []string{"AAPL"})
	if err := New(pipeline, nil).StreamPriceEvents(&quantramv1.StreamPriceEventsRequest{}, nil); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("off want FailedPrecondition, got %v", err)
	}
	adaptiveOnly, err := modelhost.New(pipeline, []string{"AAPL"}, modelhost.Options{
		Mode:     config.ModelAdaptive,
		Deadline: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := New(pipeline, adaptiveOnly).StreamPriceEvents(&quantramv1.StreamPriceEventsRequest{}, nil); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("pricing off want FailedPrecondition, got %v", err)
	}
}

func TestStreamPriceEventsLive(t *testing.T) {
	src := &liveBars{
		bars:  make(chan domain.Bar, 8),
		infer: true,
	}
	host, err := modelhost.New(src, []string{"AAPL"}, modelhost.Options{
		Mode:     config.ModelAdaptive,
		Pricing:  config.PricingExpm,
		Deadline: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = host.Run(ctx) }()
	waitUntil(t, time.Second, host.Started)

	lis := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	quantramv1.RegisterModelServiceServer(grpcServer, New(ingestion.NewPipeline(nil, nil, "TEST", []string{"AAPL"}), host))
	go func() { _ = grpcServer.Serve(lis) }()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = lis.Close()
	})

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	src.bars <- domain.Bar{
		Symbol:           "AAPL",
		Interval:         domain.Interval1Min,
		IntervalStart:    start,
		IntervalEnd:      start.Add(time.Minute),
		Close:            100,
		Volume:           1000,
		QualityStatus:    domain.QualityComplete,
		IsFinal:          true,
		Source:           "TEST",
		MarketSnapshotID: "snap-p",
	}
	waitUntil(t, 2*time.Second, func() bool { return len(host.LastPriceEvents()) > 0 })

	client := quantramv1.NewModelServiceClient(conn)
	streamCtx, streamCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer streamCancel()
	stream, err := client.StreamPriceEvents(streamCtx, &quantramv1.StreamPriceEventsRequest{MaxEvents: 1})
	if err != nil {
		t.Fatal(err)
	}
	ev, err := stream.Recv()
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if ev.GetSymbol() != "AAPL" || ev.GetSkip() == nil {
		t.Fatalf("want pricing skip for first bar, got %+v", ev)
	}
	if ev.GetStatus() != quantramv1.PricingStatus_PRICING_STATUS_WARMUP_DERIVATIVE {
		t.Fatalf("want WARMUP_DERIVATIVE, got %s", ev.GetStatus())
	}
}

type liveBars struct {
	bars  chan domain.Bar
	infer bool
}

func (f *liveBars) SubscribeModelBars(int) (uint64, <-chan domain.Bar) { return 1, f.bars }
func (f *liveBars) Unsubscribe(uint64)                                {}
func (f *liveBars) Readiness() domain.Readiness {
	return domain.Readiness{Ready: true, Observe: true, Infer: f.infer}
}
func (f *liveBars) ReadinessFor(string) domain.Readiness { return f.Readiness() }
func (f *liveBars) ModelPathStatus(string) ingestion.ModelPathStatus {
	return ingestion.ModelPathStatus{}
}

func TestToProtoPriceEventWarmup(t *testing.T) {
	ev := domain.PriceEvent{
		EventID:          "AAPL:price:1",
		Symbol:           "AAPL",
		IntervalStart:    time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC),
		AcceptedSequence: 1,
		Status:           domain.PricingStatusWarmupDerivative,
		Skip:             &domain.PricingSkip{Reason: domain.PricingSkipWarmupDerivative},
	}
	out := toProtoPriceEvent(ev)
	if out.GetEmission() != nil || out.GetSkip() == nil {
		t.Fatal("warmup must carry skip without emission")
	}
	if out.GetStatus() != quantramv1.PricingStatus_PRICING_STATUS_WARMUP_DERIVATIVE {
		t.Fatalf("status %s", out.GetStatus())
	}
}

func waitUntil(t *testing.T, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out")
}
