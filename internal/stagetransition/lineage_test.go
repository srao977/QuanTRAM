package stagetransition

import (
	"testing"
	"time"

	"quantram/internal/domain"
)

func TestFullInitiatingBarPreserved(t *testing.T) {
	h := NewHub()
	id, ch := h.Subscribe(8)
	defer h.Unsubscribe(id)
	start := time.Date(2026, 9, 4, 13, 30, 0, 0, time.UTC)
	want := causalBar("AAPL", start, 187.25)
	ev := decision("AAPL", domain.SideBuy, start, "a:1")
	ev.MarketSnapshotID = want.MarketSnapshotID
	h.OnDecision(ev, want)
	got := drain(t, ch, 1)[0]
	if got.InitiatingBar == nil {
		t.Fatal("P-03 missing initiating bar")
	}
	if *got.InitiatingBar != want {
		t.Fatalf("bar %+v want %+v", *got.InitiatingBar, want)
	}
	if !got.BarAgrees() {
		t.Fatal("correlation disagrees with initiating bar")
	}
}

func TestBarCopyIsIndependent(t *testing.T) {
	h := NewHub()
	id, ch := h.Subscribe(4)
	defer h.Unsubscribe(id)
	start := time.Date(2026, 9, 4, 13, 30, 0, 0, time.UTC)
	src := causalBar("AAPL", start, 10)
	ev := decision("AAPL", domain.SideHold, start, "a:1")
	ev.MarketSnapshotID = src.MarketSnapshotID
	h.OnDecision(ev, src)
	got := drain(t, ch, 1)[0]
	src.Close = 999
	src.Volume = 1
	src.MarketSnapshotID = "mutated"
	if got.InitiatingBar.Close == 999 || got.InitiatingBar.Volume == 1 || got.InitiatingBar.MarketSnapshotID == "mutated" {
		t.Fatal("initiating bar aliased the source")
	}
	if got.InitiatingBar.Close != 10 || got.InitiatingBar.Volume != 1000 {
		t.Fatalf("copy lost values %+v", got.InitiatingBar)
	}
}

func TestDifferentBarSameStateDoesNotPublish(t *testing.T) {
	h := NewHub()
	id, ch := h.Subscribe(8)
	defer h.Unsubscribe(id)
	t0 := time.Date(2026, 9, 4, 13, 30, 0, 0, time.UTC)
	b1 := causalBar("AAPL", t0, 101)
	b2 := causalBar("AAPL", t0.Add(time.Minute), 102)
	b3 := causalBar("AAPL", t0.Add(2*time.Minute), 103)
	b4 := causalBar("AAPL", t0.Add(3*time.Minute), 104)
	hold := func(b domain.Bar, id string) {
		ev := decision("AAPL", domain.SideHold, b.IntervalStart, id)
		ev.MarketSnapshotID = b.MarketSnapshotID
		ev.AcceptedSequence = ev.AcceptedSequence + 1
		h.OnDecision(ev, b)
	}
	hold(b1, "1")
	hold(b2, "2")
	hold(b3, "3")
	first := drain(t, ch, 1)[0]
	if first.InitiatingBar == nil || first.InitiatingBar.Close != 101 {
		t.Fatalf("initial bar %+v", first.InitiatingBar)
	}
	select {
	case extra := <-ch:
		t.Fatalf("bar-only change published %s close=%v", extra.Current.Code, extra.InitiatingBar.Close)
	default:
	}
	buy := decision("AAPL", domain.SideBuy, b4.IntervalStart, "4")
	buy.MarketSnapshotID = b4.MarketSnapshotID
	h.OnDecision(buy, b4)
	changed := drain(t, ch, 1)[0]
	if changed.InitiatingBar == nil || *changed.InitiatingBar != b4 {
		t.Fatalf("BUY must carry bar 4, got %+v", changed.InitiatingBar)
	}
}

func TestP03P04SameBarLineage(t *testing.T) {
	h := NewHub()
	id, ch := h.Subscribe(8)
	defer h.Unsubscribe(id)
	start := time.Date(2026, 9, 4, 13, 30, 0, 0, time.UTC)
	b := causalBar("AAPL", start, 150)
	d := decision("AAPL", domain.SideHold, start, "d:1")
	d.MarketSnapshotID = b.MarketSnapshotID
	p := price("AAPL", domain.PricingStatusEmitted, "GREEN", start, "p:1")
	p.MarketSnapshotID = b.MarketSnapshotID
	p.Emission.TrajectoryPhase = "ACCELERATING"
	h.OnPrice(p, b)
	h.OnDecision(d, b)
	evs := drain(t, ch, 2)
	if evs[0].StageID != StageP04PriceEngine || evs[1].StageID != StageP03Adaptive {
		t.Fatalf("order %s %s", evs[0].StageID, evs[1].StageID)
	}
	if evs[0].InitiatingBar == nil || evs[1].InitiatingBar == nil {
		t.Fatal("missing bars")
	}
	if *evs[0].InitiatingBar != b || *evs[1].InitiatingBar != b {
		t.Fatal("both must carry accepted bar B")
	}
	if *evs[0].InitiatingBar != *evs[1].InitiatingBar {
		t.Fatal("P-03 and P-04 bars differ")
	}
	if !evs[0].BarAgrees() || !evs[1].BarAgrees() {
		t.Fatal("correlation mismatch")
	}
}

func TestP01HasNoInitiatingBar(t *testing.T) {
	h := NewHub()
	id, ch := h.Subscribe(4)
	defer h.Unsubscribe(id)
	h.OnFeed(domain.FeedHealth{State: domain.FeedHealthy, SourceID: "ALPACA_IEX"})
	ev := drain(t, ch, 1)[0]
	if ev.InitiatingBar != nil {
		t.Fatal("P-01 must not invent a bar")
	}
	if ev.Current.FeedState != "HEALTHY" {
		t.Fatalf("%+v", ev.Current)
	}
}

func TestStateVsFactsVsBarEquality(t *testing.T) {
	h := NewHub()
	id, ch := h.Subscribe(16)
	defer h.Unsubscribe(id)
	start := time.Date(2026, 9, 4, 13, 30, 0, 0, time.UTC)

	d1 := decision("AAPL", domain.SideHold, start, "1")
	b1 := causalBar("AAPL", start, 10)
	d1.MarketSnapshotID = b1.MarketSnapshotID
	h.OnDecision(d1, b1)
	_ = drain(t, ch, 1)

	d2 := decision("AAPL", domain.SideHold, start.Add(time.Minute), "2")
	d2.Decision.Strength = 0.99
	d2.Decision.Confidence = 0.11
	d2.Decision.Uncertainty = 0.77
	d2.Decision.PathDirection = domain.PathUpward
	b2 := causalBar("AAPL", start.Add(time.Minute), 20)
	d2.MarketSnapshotID = b2.MarketSnapshotID
	h.OnDecision(d2, b2)
	select {
	case ev := <-ch:
		t.Fatalf("fact/bar-only change published %+v", ev.Current)
	default:
	}

	d3 := decision("AAPL", domain.SideHold, start.Add(2*time.Minute), "3")
	d3.Decision.EmitterPosition = domain.EmitterLong
	b3 := causalBar("AAPL", start.Add(2*time.Minute), 30)
	d3.MarketSnapshotID = b3.MarketSnapshotID
	h.OnDecision(d3, b3)
	got := drain(t, ch, 1)[0]
	if got.Current.EmitterPosition != string(domain.EmitterLong) {
		t.Fatalf("emitter change must publish %+v", got.Current)
	}

	p1 := price("SPY", domain.PricingStatusEmitted, "GREEN", start, "p1")
	p1.Emission.TrajectoryPhase = "ACCELERATING"
	pb := causalBar("SPY", start, 1)
	p1.MarketSnapshotID = pb.MarketSnapshotID
	h.OnPrice(p1, pb)
	_ = drain(t, ch, 1)
	p2 := price("SPY", domain.PricingStatusEmitted, "GREEN", start.Add(time.Minute), "p2")
	p2.Emission.TrajectoryPhase = "ACCELERATING"
	p2.Emitted = true
	pb2 := causalBar("SPY", start.Add(time.Minute), 2)
	p2.MarketSnapshotID = pb2.MarketSnapshotID
	h.OnPrice(p2, pb2)
	select {
	case ev := <-ch:
		t.Fatalf("P-04 fact/bar-only published %+v", ev.Current)
	default:
	}
	p3 := price("SPY", domain.PricingStatusEmitted, "GREEN", start.Add(2*time.Minute), "p3")
	p3.Emission.TrajectoryPhase = "DECELERATING"
	pb3 := causalBar("SPY", start.Add(2*time.Minute), 3)
	p3.MarketSnapshotID = pb3.MarketSnapshotID
	h.OnPrice(p3, pb3)
	phase := drain(t, ch, 1)[0]
	if phase.Current.TrajectoryPhase != "DECELERATING" {
		t.Fatalf("phase change must publish %+v", phase.Current)
	}

	h.OnFeed(domain.FeedHealth{State: domain.FeedHealthy, SourceID: "ALPACA_IEX", LastError: "x"})
	_ = drain(t, ch, 1)
	h.OnFeed(domain.FeedHealth{State: domain.FeedHealthy, SourceID: "OTHER", LastError: "y"})
	select {
	case ev := <-ch:
		t.Fatalf("P-01 fact-only published %+v", ev)
	default:
	}
	h.OnFeed(domain.FeedHealth{State: domain.FeedFailed, SourceID: "ALPACA_IEX"})
	if drain(t, ch, 1)[0].Current.FeedState != "FAILED" {
		t.Fatal("P-01 state change must publish")
	}
}

func TestReconstructionMatchesDetector(t *testing.T) {
	h := NewHub()
	id, ch := h.Subscribe(32)
	defer h.Unsubscribe(id)
	start := time.Date(2026, 9, 4, 13, 30, 0, 0, time.UTC)
	decide(h, "AAPL", domain.SideHold, start, "a1")
	decide(h, "AAPL", domain.SideBuy, start.Add(time.Minute), "a2")
	decide(h, "SPY", domain.SideSell, start, "s1")
	priceOut(h, "AAPL", domain.PricingStatusEmitted, "AMBER", start, "p1")
	priceOut(h, "AAPL", domain.PricingStatusEmitted, "GREEN", start.Add(time.Minute), "p2")
	h.OnFeed(domain.FeedHealth{State: domain.FeedFailed})
	h.OnFeed(domain.FeedHealth{State: domain.FeedHealthy})
	h.OnIngestion(IngestionInput{FeedState: domain.FeedFailed}, domain.Bar{})
	h.OnIngestion(IngestionInput{FeedState: domain.FeedHealthy, Infer: true}, domain.Bar{})

	latest := map[string]StageState{}
	for _, ev := range drain(t, ch, 9) {
		latest[string(ev.StageID)+"|"+ev.EntityID] = ev.Current
	}
	want := h.LatestAll()
	if len(want) == 0 {
		t.Fatal("detector empty")
	}
	for key, st := range want {
		got, ok := latest[key]
		if !ok || !got.Equal(st) {
			t.Fatalf("reconstruct %s got %+v want %+v", key, got, st)
		}
	}
}
