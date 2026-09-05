package volume

import (
	"strings"

	"quantram/internal/domain"
)

// Observation is the side-effect-free Volume input mapped from one domain.Bar.
//
// Responsibility: capture V_RAW and initiating-Bar lineage. It does not
// normalize, differentiate, interpret, or read Adaptive/Price output.
//
// Time: IntervalStart is retained as canonical causal time. Future V1/V2
// elapsed-minute coordinates are derived from these times; they are not
// precomputed here, so float minutes are not stored as a second clock.
type Observation struct {
	Entity  string
	VRaw    float64
	Lineage domain.VolumeLineage
}

// ObservationFromBar maps an immutable Bar to a Volume observation.
// V_RAW is float64(Bar.Volume), including exact 0. The Bar is not mutated.
func ObservationFromBar(bar domain.Bar) Observation {
	symbol := strings.ToUpper(strings.TrimSpace(bar.Symbol))
	return Observation{
		Entity: symbol,
		VRaw:   float64(bar.Volume),
		Lineage: domain.VolumeLineage{
			Symbol:           symbol,
			MarketSnapshotID: bar.MarketSnapshotID,
			IntervalStart:    bar.IntervalStart,
			IntervalEnd:      bar.IntervalEnd,
			SourceTimestamp:  bar.SourceTimestamp,
		},
	}
}
