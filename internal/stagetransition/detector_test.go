package stagetransition

import (
	"strings"
	"testing"
	"time"

	"quantram/internal/domain"
)

func TestFirstStateThenSameThenChange(t *testing.T) {
	h := NewHub()
	fixed := time.Date(2026, 9, 4, 14, 0, 0, 0, time.UTC)
	h.SetClock(func() time.Time { return fixed.Add(time.Millisecond) })

	id, ch := h.Subscribe(8)
	defer h.Unsubscribe(id)

	bar := time.Date(2026, 9, 4, 13, 30, 0, 0, time.UTC)
	decide(h, "AAPL", domain.SideHold, bar, "a:1")
	decide(h, "AAPL", domain.SideHold, bar.Add(time.Minute), "a:2")
	decide(h, "AAPL", domain.SideBuy, bar.Add(2*time.Minute), "a:3")

	evs := drain(t, ch, 2)
	if evs[0].Previous.Display() != StateAbsent || evs[0].Current.Code != "DECISION:HOLD" {
		t.Fatalf("first %+v", evs[0])
	}
	if evs[0].Sequence != 1 || evs[0].TransitionID != "P03_ADAPTIVE:AAPL:1" {
		t.Fatalf("id/seq %+v", evs[0])
	}
	if evs[1].Previous.Code != "DECISION:HOLD" || evs[1].Current.Code != "DECISION:BUY" {
		t.Fatalf("change %+v", evs[1])
	}
	if evs[1].Sequence != 2 {
		t.Fatalf("seq %d", evs[1].Sequence)
	}
	if evs[0].EffectiveEventTime.Equal(evs[0].PublishedTime) {
		t.Fatal("event time must stay distinct from published time when IntervalStart is set")
	}
	if !evs[0].EffectiveEventTime.Equal(bar) {
		t.Fatalf("effective %s", evs[0].EffectiveEventTime)
	}
	if evs[0].PublishedTime.IsZero() {
		t.Fatal("published time missing")
	}
	select {
	case extra := <-ch:
		t.Fatalf("same-state published %s", extra.Current.Code)
	default:
	}
}

func TestEntityAndStageIsolation(t *testing.T) {
	h := NewHub()
	id, ch := h.Subscribe(16)
	defer h.Unsubscribe(id)

	bar := time.Date(2026, 9, 4, 13, 30, 0, 0, time.UTC)
	decide(h, "AAPL", domain.SideHold, bar, "a:1")
	decide(h, "SPY", domain.SideHold, bar, "s:1")
	priceOut(h, "AAPL", domain.PricingStatusWarmupDerivative, "", bar, "p:1")
	decide(h, "AAPL", domain.SideHold, bar.Add(time.Minute), "a:2")

	evs := drain(t, ch, 3)
	seen := map[string]uint64{}
	for _, ev := range evs {
		seen[string(ev.StageID)+"|"+ev.EntityID] = ev.Sequence
	}
	if seen["P03_ADAPTIVE|AAPL"] != 1 || seen["P03_ADAPTIVE|SPY"] != 1 || seen["P04_PRICE_ENGINE|AAPL"] != 1 {
		t.Fatalf("%v", seen)
	}
}

func TestResetEntityAllowsRepublishSameState(t *testing.T) {
	h := NewHub()
	id, ch := h.Subscribe(8)
	defer h.Unsubscribe(id)
	bar := time.Now().UTC().Truncate(time.Minute)
	decide(h, "AAPL", domain.SideHold, bar, "a:1")
	decide(h, "AAPL", domain.SideHold, bar.Add(time.Minute), "a:2")
	h.ResetEntity("AAPL")
	decide(h, "AAPL", domain.SideHold, bar.Add(2*time.Minute), "a:3")
	evs := drain(t, ch, 2)
	if evs[1].Sequence != 2 || evs[1].Current.Code != "DECISION:HOLD" {
		t.Fatalf("%+v", evs[1])
	}
}

func TestFeedAndIngestionStates(t *testing.T) {
	h := NewHub()
	id, ch := h.Subscribe(8)
	defer h.Unsubscribe(id)

	h.OnFeed(domain.FeedHealth{State: domain.FeedFailed, SourceID: "ALPACA_IEX"})
	h.OnFeed(domain.FeedHealth{State: domain.FeedFailed, SourceID: "ALPACA_IEX"})
	h.OnFeed(domain.FeedHealth{State: domain.FeedRecovering, SourceID: "ALPACA_IEX"})
	h.OnIngestion(IngestionInput{FeedState: domain.FeedFailed, Infer: false}, domain.Bar{})
	h.OnIngestion(IngestionInput{FeedState: domain.FeedFailed, Infer: false}, domain.Bar{})
	h.OnIngestion(IngestionInput{FeedState: domain.FeedHealthy, Infer: true, SourceID: "ALPACA_IEX"}, domain.Bar{})

	evs := drain(t, ch, 4)
	if evs[0].Current.Code != "FAILED" || evs[1].Current.Code != "RECOVERING" {
		t.Fatalf("feed %+v %+v", evs[0].Current, evs[1].Current)
	}
	if evs[2].Current.Code != "NOT_READY" || evs[3].Current.Code != "OBSERVE_INFER" {
		t.Fatalf("ingest %+v %+v", evs[2].Current, evs[3].Current)
	}
	if evs[0].StageID != StageP01MarketFeed || evs[2].StageID != StageP02Ingestion {
		t.Fatal("stage ids")
	}
}

func TestPricingColorChangeOnly(t *testing.T) {
	h := NewHub()
	id, ch := h.Subscribe(8)
	defer h.Unsubscribe(id)
	bar := time.Date(2026, 9, 4, 13, 30, 0, 0, time.UTC)
	priceOut(h, "SPY", domain.PricingStatusEmitted, "AMBER", bar, "p:1")
	priceOut(h, "SPY", domain.PricingStatusEmitted, "AMBER", bar.Add(time.Minute), "p:2")
	priceOut(h, "SPY", domain.PricingStatusEmitted, "GREEN", bar.Add(2*time.Minute), "p:3")
	evs := drain(t, ch, 2)
	if evs[0].Current.Code != "EMITTED:AMBER" || evs[1].Current.Code != "EMITTED:GREEN" {
		t.Fatalf("%s -> %s", evs[0].Current.Code, evs[1].Current.Code)
	}
}

func TestUnspecifiedFeedDoesNotPublish(t *testing.T) {
	h := NewHub()
	id, ch := h.Subscribe(4)
	defer h.Unsubscribe(id)
	h.OnFeed(domain.FeedHealth{})
	select {
	case ev := <-ch:
		t.Fatalf("unexpected %s", ev.Current.Code)
	default:
	}
}

func TestTransitionIDExcludesPublishedTime(t *testing.T) {
	h := NewHub()
	n := 0
	h.SetClock(func() time.Time {
		n++
		return time.Date(2026, 9, 4, 15, 0, 0, n, time.UTC)
	})
	id, ch := h.Subscribe(4)
	defer h.Unsubscribe(id)
	decide(h, "AAPL", domain.SideHold, time.Date(2026, 9, 4, 13, 30, 0, 0, time.UTC), "a:1")
	ev := drain(t, ch, 1)[0]
	if ev.TransitionID != "P03_ADAPTIVE:AAPL:1" {
		t.Fatalf("%s", ev.TransitionID)
	}
	if strings.Contains(ev.TransitionID, ev.PublishedTime.Format(time.RFC3339Nano)) {
		t.Fatal("published time leaked into identity")
	}
}

func TestConcurrentSymbolsSafe(t *testing.T) {
	h := NewHub()
	id, ch := h.Subscribe(64)
	defer h.Unsubscribe(id)
	bar := time.Date(2026, 9, 4, 13, 30, 0, 0, time.UTC)
	done := make(chan struct{}, 2)
	go func() {
		decide(h, "AAPL", domain.SideHold, bar, "a:1")
		done <- struct{}{}
	}()
	go func() {
		decide(h, "MSFT", domain.SideHold, bar, "m:1")
		done <- struct{}{}
	}()
	<-done
	<-done
	evs := drain(t, ch, 2)
	if evs[0].EntityID == evs[1].EntityID {
		t.Fatal("expected isolated entities")
	}
}

func causalBar(symbol string, start time.Time, close float64) domain.Bar {
	return domain.Bar{
		Symbol:           symbol,
		Interval:         domain.Interval1Min,
		IntervalStart:    start,
		IntervalEnd:      start.Add(time.Minute),
		Open:             close,
		High:             close + 1,
		Low:              close - 1,
		Close:            close,
		Volume:           1000,
		Source:           "ALPACA_IEX",
		SourceTimestamp:  start.UTC().Format(time.RFC3339),
		QualityStatus:    domain.QualityComplete,
		IsFinal:          true,
		MarketSnapshotID: domain.SnapshotID(symbol, "ALPACA_IEX", start.UTC().Format(time.RFC3339), close, close+1, close-1, close, 1000),
	}
}

func decide(h *Hub, symbol string, side domain.Side, start time.Time, eventID string) domain.Bar {
	b := causalBar(symbol, start, 100)
	ev := decision(symbol, side, start, eventID)
	ev.MarketSnapshotID = b.MarketSnapshotID
	h.OnDecision(ev, b)
	return b
}

func priceOut(h *Hub, symbol string, status domain.PricingStatus, color string, start time.Time, eventID string) domain.Bar {
	b := causalBar(symbol, start, 100)
	ev := price(symbol, status, color, start, eventID)
	ev.MarketSnapshotID = b.MarketSnapshotID
	h.OnPrice(ev, b)
	return b
}

func decision(symbol string, side domain.Side, start time.Time, eventID string) domain.DecisionEvent {
	return domain.DecisionEvent{
		EventID:          eventID,
		Symbol:           symbol,
		IntervalStart:    start,
		MarketSnapshotID: "snap-" + symbol,
		AcceptedSequence: 3,
		Decision: &domain.Decision{
			Side:            side,
			ModelStatus:     domain.StatusActionable,
			EmitterPosition: domain.EmitterFlat,
			PathDirection:   domain.PathFlat,
			Strength:        0.4,
			Confidence:      0.5,
		},
	}
}

func price(symbol string, status domain.PricingStatus, color string, start time.Time, eventID string) domain.PriceEvent {
	ev := domain.PriceEvent{
		EventID:          eventID,
		Symbol:           symbol,
		IntervalStart:    start,
		MarketSnapshotID: "snap-" + symbol,
		AcceptedSequence: 3,
		Status:           status,
		Emitted:          color != "",
	}
	if color != "" {
		ev.Emission = &domain.PriceEmission{Color: color, TrajectoryPhase: "ACCELERATING"}
	} else {
		ev.Skip = &domain.PricingSkip{Reason: domain.PricingSkipWarmupDerivative}
	}
	return ev
}

func drain(t *testing.T, ch <-chan Event, n int) []Event {
	t.Helper()
	out := make([]Event, 0, n)
	deadline := time.After(2 * time.Second)
	for len(out) < n {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("closed after %d want %d", len(out), n)
			}
			out = append(out, ev)
		case <-deadline:
			t.Fatalf("timeout after %d want %d", len(out), n)
		}
	}
	return out
}
