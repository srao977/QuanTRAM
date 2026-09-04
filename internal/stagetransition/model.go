// Package stagetransition publishes compact facts when a realtime stage's
// meaningful categorical state changes.
//
// Purpose:
//
//	Sideways output from existing authoritative realtime state. A subscriber
//	that receives every Event for (StageID, EntityID) in sequence can
//	reconstruct the latest meaningful StageState. It cannot reconstruct
//	continuously varying scientific values that were never published.
//
// Inputs:
//
//	Already-computed FeedHealth, ingestion capability, DecisionEvent,
//	PriceEvent, and the accepted domain.Bar that caused a Bar-driven
//	evaluation. Scientific engines are never re-run here.
//
// Outputs:
//
//	Immutable Event values. TXT diagnostic is one subscriber. A future
//	Snapshot Service may be another.
//
// Ownership:
//
//	This package owns Event, StageState equality, sequence, identity,
//	bounded pub/sub, and the TXT subscriber. Realtime stages own when to
//	call Hub after commit.
//
// State equality:
//
//	Typed field comparison of authoritative StageState dimensions only.
//	InitiatingBar, TransitionFacts, timestamps, IDs, and floats are excluded.
//
// Causal Bar:
//
//	domain.Bar is a value struct with no maps/slices/pointers. CopyBar is
//	assignment. The Bar is lineage, not StageState. A different Bar with
//	the same StageState does not publish.
//
// Lifecycle / concurrency / failure:
//
//	One Hub per process. Hub methods are concurrent-safe. Publish never
//	blocks on disk or subscriber work. Zero subscribers is valid.
//
// Non-responsibilities:
//
//	Snapshot Service, Persistence, MongoDB, Aperture, proto, dashboard,
//	P-05–P-08, and any scientific recomputation.
package stagetransition

import (
	"fmt"
	"strings"
	"time"

	"quantram/internal/domain"
)

const (
	ContractVersion = "1.1"

	EntityGlobal = "GLOBAL"
	StateAbsent  = "ABSENT"

	DefaultSubscriberBuffer = 128

	KindFeed     = "FEED"
	KindIngest   = "INGESTION"
	KindDecision = "DECISION"
	KindSkip     = "SKIP"
	KindPricing  = "PRICING"
)

type StageID string

const (
	StageP01MarketFeed  StageID = "P01_MARKET_FEED"
	StageP02Ingestion   StageID = "P02_INGESTION"
	StageP03Adaptive    StageID = "P03_ADAPTIVE"
	StageP04PriceEngine StageID = "P04_PRICE_ENGINE"
)

func (id StageID) Name() string {
	switch id {
	case StageP01MarketFeed:
		return "Market Feed"
	case StageP02Ingestion:
		return "Ingestion"
	case StageP03Adaptive:
		return "Adaptive Model"
	case StageP04PriceEngine:
		return "Price Engine"
	default:
		if id == "" {
			return ""
		}
		return string(id)
	}
}

// StageState is the reconstructable meaningful state of one stage/entity.
// Equality uses these typed fields only. Code is a display summary.
type StageState struct {
	Kind            string
	Code            string
	FeedState       string
	Capability      string
	Side            string
	SkipReason      string
	ModelStatus     string
	EmitterPosition string
	PricingStatus   string
	Color           string
	TrajectoryPhase string
	DomainState     string
	ConfidenceState string
	DomainExit      bool
}

func (s StageState) Equal(other StageState) bool {
	return s.Kind == other.Kind &&
		s.FeedState == other.FeedState &&
		s.Capability == other.Capability &&
		s.Side == other.Side &&
		s.SkipReason == other.SkipReason &&
		s.ModelStatus == other.ModelStatus &&
		s.EmitterPosition == other.EmitterPosition &&
		s.PricingStatus == other.PricingStatus &&
		s.Color == other.Color &&
		s.TrajectoryPhase == other.TrajectoryPhase &&
		s.DomainState == other.DomainState &&
		s.ConfidenceState == other.ConfidenceState &&
		s.DomainExit == other.DomainExit
}

func (s StageState) IsAbsent() bool {
	return s.Kind == "" &&
		s.Code == "" &&
		s.FeedState == "" &&
		s.Capability == "" &&
		s.Side == "" &&
		s.SkipReason == "" &&
		s.ModelStatus == "" &&
		s.EmitterPosition == "" &&
		s.PricingStatus == "" &&
		s.Color == "" &&
		s.TrajectoryPhase == "" &&
		s.DomainState == "" &&
		s.ConfidenceState == "" &&
		!s.DomainExit
}

func (s StageState) Display() string {
	if s.IsAbsent() {
		return StateAbsent
	}
	if s.Code != "" {
		return s.Code
	}
	return s.Kind
}

func (s StageState) Lines() []string {
	if s.IsAbsent() {
		return []string{StateAbsent}
	}
	var lines []string
	add := func(label, v string) {
		if v != "" {
			lines = append(lines, label+": "+v)
		}
	}
	add("Kind", s.Kind)
	add("Code", s.Code)
	add("Feed State", s.FeedState)
	add("Capability", s.Capability)
	add("Decision", s.Side)
	add("Skip Reason", s.SkipReason)
	add("Model Status", s.ModelStatus)
	add("Emitter", s.EmitterPosition)
	add("Pricing Status", s.PricingStatus)
	add("Color", s.Color)
	add("Trajectory Phase", s.TrajectoryPhase)
	add("Domain State", s.DomainState)
	add("Confidence State", s.ConfidenceState)
	if s.Kind == KindPricing {
		lines = append(lines, fmt.Sprintf("Domain Exit: %t", s.DomainExit))
	}
	if len(lines) == 0 {
		return []string{StateAbsent}
	}
	return lines
}

// FeedFacts are P-01 transition facts. They do not participate in equality.
type FeedFacts struct {
	SourceID          string
	LastError         string
	SubscribedSymbols []string
}

// IngestionFacts are P-02 transition facts. Capability is in StageState.
type IngestionFacts struct {
	FeedState string
	Observe   bool
	Infer     bool
	Filling   bool
	SourceID  string
}

// AdaptiveFacts are P-03 contextual values captured at transition time.
// PathDirection and continuous scores are NOT reconstructable after later
// non-transitioning bars.
type AdaptiveFacts struct {
	PathDirection string
	Strength      float64
	Confidence    float64
	Uncertainty   float64
}

// PricingFacts are P-04 contextual values captured at transition time.
// Emitted is implied by status/color and is not an equality dimension.
type PricingFacts struct {
	Emitted bool
}

// Event is the provider-neutral StageTransitionEvent envelope.
type Event struct {
	TransitionID       string
	Sequence           uint64
	StageID            StageID
	StageName          string
	EntityID           string
	Previous           StageState
	Current            StageState
	InitiatingBar      *domain.Bar
	EffectiveEventTime time.Time
	PublishedTime      time.Time
	MarketSnapshotID   string
	AcceptedSequence   int
	SourceEventID      string
	ReasonCode         string
	ContractVersion    string
	Feed               *FeedFacts
	Ingestion          *IngestionFacts
	Adaptive           *AdaptiveFacts
	Pricing            *PricingFacts
}

func (e Event) HasInitiatingBar() bool { return e.InitiatingBar != nil }

func (e Event) BarAgrees() bool {
	if e.InitiatingBar == nil {
		return true
	}
	b := e.InitiatingBar
	if e.MarketSnapshotID != "" && e.MarketSnapshotID != b.MarketSnapshotID {
		return false
	}
	if !e.EffectiveEventTime.IsZero() && !b.IntervalStart.IsZero() && !e.EffectiveEventTime.Equal(b.IntervalStart) {
		return false
	}
	if e.EntityID != EntityGlobal && e.EntityID != "" && b.Symbol != "" && e.EntityID != b.Symbol {
		return false
	}
	return true
}

// CopyBar copies the canonical domain.Bar by value.
// Bar contains only value-safe fields (no maps, slices, or pointers).
func CopyBar(b domain.Bar) domain.Bar {
	return b
}

func optionalBar(bar domain.Bar) *domain.Bar {
	if bar.Symbol == "" && bar.IntervalStart.IsZero() {
		return nil
	}
	copied := CopyBar(bar)
	return &copied
}

func formatStateLines(indent string, s StageState) string {
	var b strings.Builder
	for _, line := range s.Lines() {
		b.WriteString(indent)
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
