package stagetransition

import (
	"time"

	"quantram/internal/domain"
)

// Hub is the process-level StageTransitionPublisher.
//
// Realtime stages call OnFeed / OnIngestion / OnDecision / OnPrice after
// authoritative state exists. Hub derives StageState, compares, and
// publishes only on change. It performs no filesystem I/O.
type Hub struct {
	detector  *Detector
	publisher *Publisher
}

func NewHub() *Hub {
	return &Hub{
		detector:  NewDetector(),
		publisher: NewPublisher(),
	}
}

func (h *Hub) SetClock(now func() time.Time) {
	if h != nil {
		h.detector.SetClock(now)
	}
}

func (h *Hub) Subscribe(buffer int) (uint64, <-chan Event) {
	if h == nil {
		ch := make(chan Event)
		close(ch)
		return 0, ch
	}
	return h.publisher.Subscribe(buffer)
}

func (h *Hub) Unsubscribe(id uint64) {
	if h != nil {
		h.publisher.Unsubscribe(id)
	}
}

func (h *Hub) Stats() Stats {
	if h == nil {
		return Stats{}
	}
	return h.publisher.Stats()
}

func (h *Hub) Close() {
	if h != nil {
		h.publisher.Close()
	}
}

func (h *Hub) ResetEntity(entityID string) {
	if h != nil {
		h.detector.ResetEntity(entityID)
	}
}

func (h *Hub) Latest(stage StageID, entity string) (StageState, bool) {
	if h == nil {
		return StageState{}, false
	}
	return h.detector.Latest(stage, entity)
}

func (h *Hub) LatestAll() map[string]StageState {
	if h == nil {
		return map[string]StageState{}
	}
	return h.detector.LatestAll()
}

func (h *Hub) OnFeed(health domain.FeedHealth) {
	if h == nil {
		return
	}
	state := health.State
	if state == domain.FeedUnspecified {
		return
	}
	ev, ok := h.detector.consider(draft{
		Stage:    StageP01MarketFeed,
		EntityID: EntityGlobal,
		State: StageState{
			Kind:      KindFeed,
			Code:      string(state),
			FeedState: string(state),
		},
		EffectiveEventTime: health.LastMessage,
		ReasonCode:         string(state),
		Feed: &FeedFacts{
			SourceID:          health.SourceID,
			LastError:         health.LastError,
			SubscribedSymbols: append([]string(nil), health.SubscribedSymbols...),
		},
	})
	if ok {
		h.publisher.Publish(ev)
	}
}

type IngestionInput struct {
	FeedState domain.FeedState
	Infer     bool
	Filling   bool
	SourceID  string
}

func IngestionState(in IngestionInput) StageState {
	observe := in.FeedState == domain.FeedHealthy ||
		in.FeedState == domain.FeedDegraded ||
		in.FeedState == domain.FeedRecovering
	cap := "OBSERVE_ONLY"
	switch {
	case in.Filling || in.FeedState == domain.FeedRecovering:
		cap = "RECOVERING"
	case !observe:
		cap = "NOT_READY"
	case in.Infer:
		cap = "OBSERVE_INFER"
	}
	return StageState{Kind: KindIngest, Code: cap, Capability: cap}
}

func (h *Hub) OnIngestion(in IngestionInput, causedBy domain.Bar) {
	if h == nil {
		return
	}
	state := IngestionState(in)
	observe := in.FeedState == domain.FeedHealthy ||
		in.FeedState == domain.FeedDegraded ||
		in.FeedState == domain.FeedRecovering
	ev, ok := h.detector.consider(draft{
		Stage:         StageP02Ingestion,
		EntityID:      EntityGlobal,
		State:         state,
		InitiatingBar: optionalBar(causedBy),
		ReasonCode:    state.Code,
		Ingestion: &IngestionFacts{
			FeedState: string(in.FeedState),
			Observe:   observe,
			Infer:     in.Infer,
			Filling:   in.Filling,
			SourceID:  in.SourceID,
		},
	})
	if ok {
		h.publisher.Publish(ev)
	}
}

func AdaptiveState(ev domain.DecisionEvent) StageState {
	st := StageState{}
	switch {
	case ev.IsDecision():
		st.Kind = KindDecision
		st.Side = string(ev.Decision.Side)
		st.ModelStatus = string(ev.Decision.ModelStatus)
		st.EmitterPosition = string(ev.Decision.EmitterPosition)
		st.Code = "DECISION:" + st.Side
	case ev.IsSkip():
		st.Kind = KindSkip
		st.SkipReason = string(ev.Skip.Reason)
		st.ModelStatus = string(ev.Skip.ModelStatus)
		st.Code = "SKIP:" + st.SkipReason
	default:
		return StageState{}
	}
	return st
}

func (h *Hub) OnDecision(ev domain.DecisionEvent, bar domain.Bar) {
	if h == nil {
		return
	}
	state := AdaptiveState(ev)
	if state.IsAbsent() {
		return
	}
	facts := &AdaptiveFacts{}
	reason := state.Side
	if ev.IsDecision() {
		facts.PathDirection = string(ev.Decision.PathDirection)
		facts.Strength = ev.Decision.Strength
		facts.Confidence = ev.Decision.Confidence
		facts.Uncertainty = ev.Decision.Uncertainty
	} else {
		reason = state.SkipReason
	}
	out, ok := h.detector.consider(draft{
		Stage:              StageP03Adaptive,
		EntityID:           ev.Symbol,
		State:              state,
		InitiatingBar:      optionalBar(bar),
		EffectiveEventTime: ev.IntervalStart,
		MarketSnapshotID:   ev.MarketSnapshotID,
		AcceptedSequence:   ev.AcceptedSequence,
		SourceEventID:      ev.EventID,
		ReasonCode:         reason,
		Adaptive:           facts,
	})
	if ok {
		h.publisher.Publish(out)
	}
}

func PricingState(ev domain.PriceEvent) StageState {
	st := StageState{Kind: KindPricing, PricingStatus: string(ev.Status), DomainExit: ev.DomainExit}
	if ev.Emission != nil {
		st.Color = ev.Emission.Color
		st.TrajectoryPhase = ev.Emission.TrajectoryPhase
		st.DomainState = ev.Emission.DomainState
		st.ConfidenceState = ev.Emission.ConfidenceState
	}
	if ev.Skip != nil {
		st.SkipReason = string(ev.Skip.Reason)
	}
	switch {
	case st.Color != "":
		st.Code = string(ev.Status) + ":" + st.Color
	case st.SkipReason != "":
		st.Code = "SKIP:" + st.SkipReason
	case ev.Status != "":
		st.Code = string(ev.Status)
	default:
		return StageState{}
	}
	return st
}

func (h *Hub) OnPrice(ev domain.PriceEvent, bar domain.Bar) {
	if h == nil {
		return
	}
	state := PricingState(ev)
	if state.IsAbsent() {
		return
	}
	reason := state.Color
	if reason == "" {
		reason = state.SkipReason
	}
	if reason == "" {
		reason = state.PricingStatus
	}
	out, ok := h.detector.consider(draft{
		Stage:              StageP04PriceEngine,
		EntityID:           ev.Symbol,
		State:              state,
		InitiatingBar:      optionalBar(bar),
		EffectiveEventTime: ev.IntervalStart,
		MarketSnapshotID:   ev.MarketSnapshotID,
		AcceptedSequence:   ev.AcceptedSequence,
		SourceEventID:      ev.EventID,
		ReasonCode:         reason,
		Pricing:            &PricingFacts{Emitted: ev.Emitted},
	})
	if ok {
		h.publisher.Publish(out)
	}
}
