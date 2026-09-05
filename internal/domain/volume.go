// Package domain (volume types) holds P-04V causal-lineage and representation
// constants shared by later Volume Output.
//
// Purpose: preserve initiating-Bar identity so a future Price Output and
// Volume Output can be proven to have been caused by the same published B_t.
//
// Scope: lineage fields, Volume Output representation labels, and the
// internal first-class VolumeEvent. No Volume mathematics, no proto,
// no StageTransition, no engine.
//
// Inputs: copied from an immutable domain.Bar.
// Outputs: VolumeLineage and VolumeEvent used by internal/volume.
//
// Parameters: none. Color strings are representation labels, not knobs.
//
// Ownership: value types; no per-entity scientific state lives here.
//
// Lifecycle: constructed when a Bar is mapped; discarded with the observation.
//
// Concurrency: immutable after construction; safe to copy.
//
// Failure: empty MarketSnapshotID is preserved as observed. This package does
// not invent a second correlation identifier.
//
// Invariants:
//   - Lineage answers "which published Bar caused this opportunity?"
//   - Lineage is not Volume scientific effective time.
//   - Same initiating Bar ⇒ same Symbol, MarketSnapshotID, IntervalStart.
//
// Non-responsibilities: V_N / V1 / V2 / interpretation transitions, proto
// field numbers, Adaptive/Price science, host integration.
package domain

import "time"

// VolumeLineage is the initiating published Bar identity for P-04V.
// It is not Volume scientific effective time and must not be required to
// equal any later engine EffectiveTime.
type VolumeLineage struct {
	Symbol           string
	MarketSnapshotID string
	IntervalStart    time.Time
	IntervalEnd      time.Time
	SourceTimestamp  string
}

// SameInitiatingBar reports causal-lineage equality with another engine's
// initiating Symbol, MarketSnapshotID, and IntervalStart. Scientific
// effective times are intentionally not compared.
func (l VolumeLineage) SameInitiatingBar(symbol, marketSnapshotID string, intervalStart time.Time) bool {
	return l.Symbol == symbol &&
		l.MarketSnapshotID == marketSnapshotID &&
		l.IntervalStart.Equal(intervalStart)
}

// Volume interpretation color labels. Representation only; no hysteresis.
const (
	VolumeColorUnset   = ""
	VolumeColorGreen   = "GREEN"
	VolumeColorAmber   = "AMBER"
	VolumeColorRed     = "RED"
	VolumeColorInvalid = "INVALID"
)

// Volume phase labels. Representation only; no classification here.
const (
	VolumePhaseUnset                  = ""
	VolumePhaseStationary             = "ACTIVITY_STATIONARY"
	VolumePhaseIncreasingAccelerating = "ACTIVITY_INCREASING_ACCELERATING"
	VolumePhaseIncreasingDecelerating = "ACTIVITY_INCREASING_DECELERATING"
	VolumePhaseDecreasingAccelerating = "ACTIVITY_DECREASING_ACCELERATING"
	VolumePhaseDecreasingDecelerating = "ACTIVITY_DECREASING_DECELERATING"
)

// Volume confidence and domain labels. Representation only.
const (
	VolumeConfidenceUnset   = ""
	VolumeConfidenceHigh    = "HIGH"
	VolumeDomainUnset       = ""
	VolumeDomainCausalLocal = "CAUSAL_LOCAL_VOLUME"
)

// Volume engine/output status. Internal strings; not proto enums.
const (
	VolumeStatusMaturing  = "MATURING"
	VolumeStatusAvailable = "AVAILABLE"
	VolumeStatusInvalid   = "INVALID"
	VolumeStatusError     = "ENGINE_ERROR"
)

// Volume quantity readiness. Distinguishes a computed zero from unavailability.
const (
	VolumeQtyInsufficient = "INSUFFICIENT"
	VolumeQtyAvailable    = "AVAILABLE"
	VolumeQtyUndefined    = "UNDEFINED"
)

// VolumeQuantity is one Volume feature value plus why it is or is not present.
type VolumeQuantity struct {
	Value  float64
	Status string
}

// Available reports a finite scientifically computed quantity.
func (q VolumeQuantity) Available() bool {
	return q.Status == VolumeQtyAvailable
}

// VolumeEvent is the internal first-class P-04V Volume Output.
//
// Indicator is the canonical interpreted categorical state. Historical APTF
// artifacts call the same field cockpit_color. This is not a proto message.
type VolumeEvent struct {
	Lineage         VolumeLineage
	Status          string
	Reason          string
	VRaw            VolumeQuantity
	VN              VolumeQuantity
	V1              VolumeQuantity
	V2              VolumeQuantity
	IntervalMeanVN  VolumeQuantity
	PredictedNextVN VolumeQuantity
	RawColor        string
	Indicator       string
	Phase           string
	Confidence      string
	DomainState     string
	Transition      string
}
