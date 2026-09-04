package stagetransition

import (
	"fmt"
	"sync"
	"time"

	"quantram/internal/domain"
)

type stateKey struct {
	stage  StageID
	entity string
}

// Detector remembers the last published StageState per (StageID, EntityID)
// and assigns a monotonic TransitionSequence at that scope.
//
// Ownership: Hub. Not used by subscribers.
// PublishedTime is excluded from TransitionID so replay identity stays
// deterministic when sequence and current state match.
type Detector struct {
	mu   sync.Mutex
	last map[stateKey]StageState
	seq  map[stateKey]uint64
	now  func() time.Time
}

func NewDetector() *Detector {
	return &Detector{
		last: make(map[stateKey]StageState),
		seq:  make(map[stateKey]uint64),
		now:  time.Now,
	}
}

func (d *Detector) SetClock(now func() time.Time) {
	if d == nil || now == nil {
		return
	}
	d.mu.Lock()
	d.now = now
	d.mu.Unlock()
}

type draft struct {
	Stage              StageID
	EntityID           string
	State              StageState
	InitiatingBar      *domain.Bar
	EffectiveEventTime time.Time
	MarketSnapshotID   string
	AcceptedSequence   int
	SourceEventID      string
	ReasonCode         string
	Feed               *FeedFacts
	Ingestion          *IngestionFacts
	Adaptive           *AdaptiveFacts
	Pricing            *PricingFacts
}

func (d *Detector) consider(in draft) (Event, bool) {
	if d == nil {
		return Event{}, false
	}
	if in.EntityID == "" {
		in.EntityID = EntityGlobal
	}
	key := stateKey{stage: in.Stage, entity: in.EntityID}

	d.mu.Lock()
	defer d.mu.Unlock()

	prev, seen := d.last[key]
	if seen && prev.Equal(in.State) {
		return Event{}, false
	}
	next := d.seq[key] + 1
	d.seq[key] = next
	d.last[key] = in.State
	published := d.now()
	effective := in.EffectiveEventTime
	if effective.IsZero() {
		effective = published
	}
	return Event{
		TransitionID:       transitionID(in.Stage, in.EntityID, next),
		Sequence:           next,
		StageID:            in.Stage,
		StageName:          in.Stage.Name(),
		EntityID:           in.EntityID,
		Previous:           prev,
		Current:            in.State,
		InitiatingBar:      in.InitiatingBar,
		EffectiveEventTime: effective,
		PublishedTime:      published,
		MarketSnapshotID:   in.MarketSnapshotID,
		AcceptedSequence:   in.AcceptedSequence,
		SourceEventID:      in.SourceEventID,
		ReasonCode:         in.ReasonCode,
		ContractVersion:    ContractVersion,
		Feed:               in.Feed,
		Ingestion:          in.Ingestion,
		Adaptive:           in.Adaptive,
		Pricing:            in.Pricing,
	}, true
}

func (d *Detector) Latest(stage StageID, entity string) (StageState, bool) {
	if d == nil {
		return StageState{}, false
	}
	if entity == "" {
		entity = EntityGlobal
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	st, ok := d.last[stateKey{stage: stage, entity: entity}]
	return st, ok
}

func (d *Detector) LatestAll() map[string]StageState {
	out := make(map[string]StageState)
	if d == nil {
		return out
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, st := range d.last {
		out[string(k.stage)+"|"+k.entity] = st
	}
	return out
}

func (d *Detector) ResetEntity(entityID string) {
	if d == nil || entityID == "" || entityID == EntityGlobal {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, stage := range []StageID{StageP03Adaptive, StageP04PriceEngine} {
		delete(d.last, stateKey{stage: stage, entity: entityID})
	}
}

func transitionID(stage StageID, entity string, seq uint64) string {
	return fmt.Sprintf("%s:%s:%d", stage, entity, seq)
}
