package adaptive

// This file defines D01 observations and their causal input validation.

import (
	"fmt"
)

// Observation is one timestamped market input to the stateful D01 model.
type Observation struct {
	EntityID         string
	EventTime        float64
	ReceiveTime      float64
	SequenceID       int
	Price            float64
	Volume           float64
	Bid              *float64
	Ask              *float64
	BidSize          *float64
	AskSize          *float64
	Session          string
	SourceQuality    float64
	AvailabilityMask map[string]bool
}

// WithDefaults returns an observation with a complete availability mask and a
// non-empty session label.
func (o Observation) WithDefaults() Observation {
	out := o
	mask := make(map[string]bool, len(o.AvailabilityMask)+4)
	for k, v := range o.AvailabilityMask {
		mask[k] = v
	}
	if _, ok := mask["price"]; !ok {
		mask["price"] = true
	}
	if _, ok := mask["volume"]; !ok {
		mask["volume"] = true
	}
	if _, ok := mask["bid"]; !ok {
		mask["bid"] = o.Bid != nil
	}
	if _, ok := mask["ask"]; !ok {
		mask["ask"] = o.Ask != nil
	}
	out.AvailabilityMask = mask
	if out.Session == "" {
		out.Session = "UNKNOWN"
	}
	return out
}

// Clone returns a deep copy of the observation's pointers and availability map.
func (o Observation) Clone() Observation {
	out := o
	if o.Bid != nil {
		v := *o.Bid
		out.Bid = &v
	}
	if o.Ask != nil {
		v := *o.Ask
		out.Ask = &v
	}
	if o.BidSize != nil {
		v := *o.BidSize
		out.BidSize = &v
	}
	if o.AskSize != nil {
		v := *o.AskSize
		out.AskSize = &v
	}
	if o.AvailabilityMask != nil {
		out.AvailabilityMask = make(map[string]bool, len(o.AvailabilityMask))
		for k, v := range o.AvailabilityMask {
			out.AvailabilityMask[k] = v
		}
	}
	return out
}

// AssertCausalSequence requires nondecreasing event time and strictly
// increasing sequence identifiers after the first observation.
func AssertCausalSequence(previous *Observation, current Observation) error {
	if previous == nil {
		return nil
	}
	if current.EventTime < previous.EventTime {
		return fmt.Errorf("OUT_OF_ORDER_EVENT_TIME")
	}
	if current.SequenceID <= previous.SequenceID {
		return fmt.Errorf("NON_MONOTONIC_SEQUENCE")
	}
	return nil
}

// FiniteInputs rejects non-finite price, volume, or event time.
func FiniteInputs(obs Observation) error {
	if !finite(obs.Price) || !finite(obs.Volume) || !finite(obs.EventTime) {
		return fmt.Errorf("non-finite price, volume, or event_time")
	}
	return nil
}
