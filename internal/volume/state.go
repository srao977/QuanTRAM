package volume

import (
	"math"
	"strings"
	"time"

	"quantram/internal/domain"
)

// FeatureState is bounded positional Volume Feature State.
//
// Raw, VN, and Times are aligned 1:1. Index i is one scientific observation
// position. An unavailable derived V_N is stored as NaN at that position; it
// is not omitted to harvest finite values.
//
// Length is at most RawWindow (15). O(1) versus runtime length.
type FeatureState struct {
	Raw   []float64
	VN    []float64
	Times []time.Time
}

// InterpretationState is bounded Volume Interpretation State.
//
// Color is the carried historical Indicator (frozen APTF state.color /
// cockpit_color). It is the last emitted Indicator adopted on commit, not
// an independent memory of the last raw color that completed confirmation.
// While a transition is pending, Color is AMBER because AMBER is the
// emitted Indicator written into the next state.
//
// Pending fields hold the confirmation candidate. Confirmation mathematics
// lives in Interpret.
type InterpretationState struct {
	Color        string
	PendingColor string
	PendingCount int
}

// State is per-entity VolumeState: Feature + Interpretation.
//
// Ownership: one instance per symbol. Not shared with Adaptive or Price.
// Clone copies rings so a future candidate cannot alias committed storage.
// Reset clears both substates. There is no independent interpretation reset.
type State struct {
	Entity         string
	Feature        FeatureState
	Interpretation InterpretationState
}

// NewState constructs empty VolumeState for one entity.
func NewState(entity string) State {
	return State{Entity: strings.ToUpper(strings.TrimSpace(entity))}
}

// Clone returns a deep copy of Feature rings and a value copy of interpretation.
func (s State) Clone() State {
	out := s
	out.Feature = s.Feature.clone()
	return out
}

func (f FeatureState) clone() FeatureState {
	return FeatureState{
		Raw:   append([]float64(nil), f.Raw...),
		VN:    append([]float64(nil), f.VN...),
		Times: append([]time.Time(nil), f.Times...),
	}
}

// Reset clears Feature State and Interpretation State. Entity is kept.
// Maturation will restart when later phases consume observations again.
func (s *State) Reset() {
	if s == nil {
		return
	}
	s.Feature = FeatureState{}
	s.Interpretation = InterpretationState{}
}

// AppendPositional records one causal observation position.
//
// vn may be NaN to hold an unavailable derived value in sequence. This is a
// storage primitive, not Volume Feature Mathematics. Rings stay aligned and
// bounded at RawWindow.
func (s *State) AppendPositional(raw, vn float64, at time.Time) {
	if s == nil {
		return
	}
	s.Feature.Raw = append(s.Feature.Raw, raw)
	s.Feature.VN = append(s.Feature.VN, vn)
	s.Feature.Times = append(s.Feature.Times, at)
	if overflow := len(s.Feature.Raw) - RawWindow; overflow > 0 {
		s.Feature.Raw = s.Feature.Raw[overflow:]
		s.Feature.VN = s.Feature.VN[overflow:]
		s.Feature.Times = s.Feature.Times[overflow:]
	}
}

// RecordObservation appends mapped V_RAW and a positional NaN V_N placeholder.
// Later phases will write computed V_N into the same position; Phase A does not.
func (s *State) RecordObservation(obs Observation) {
	s.AppendPositional(obs.VRaw, math.NaN(), obs.Lineage.IntervalStart)
}

// RawLen is the number of stored positional raw observations.
func (s State) RawLen() int { return len(s.Feature.Raw) }

// VNLen is the number of stored positional normalized slots (including NaN).
func (s State) VNLen() int { return len(s.Feature.VN) }

// AliasesCommitted reports whether other's Feature rings share backing arrays
// with s. Used by tests to prove Clone isolation.
func (s State) aliasesFeature(other State) bool {
	return sameFloatBacking(s.Feature.Raw, other.Feature.Raw) ||
		sameFloatBacking(s.Feature.VN, other.Feature.VN) ||
		sameTimeBacking(s.Feature.Times, other.Feature.Times)
}

func sameFloatBacking(a, b []float64) bool {
	return len(a) > 0 && len(b) > 0 && &a[0] == &b[0]
}

func sameTimeBacking(a, b []time.Time) bool {
	return len(a) > 0 && len(b) > 0 && &a[0] == &b[0]
}

// InterpretationIsEmpty reports a cleared interpretation machine.
func (s State) InterpretationIsEmpty() bool {
	return s.Interpretation.Color == domain.VolumeColorUnset &&
		s.Interpretation.PendingColor == domain.VolumeColorUnset &&
		s.Interpretation.PendingCount == 0
}
