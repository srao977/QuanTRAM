// Package volume is the P-04V Volume Engine foundation.
//
// Purpose: own frozen Volume scientific constants, Bar→observation mapping,
// and bounded per-entity VolumeState. Later phases add Feature and
// Interpretation mathematics and host join.
//
// Scope (Phase A–E): constants, mapper, positional rings, VolumeState,
// feature/derivative/interpretation mathematics, and Engine PrepareStep/
// Commit. Not in this package yet: modelhost, proto.
//
// Inputs: canonical domain.Bar (immutable) and an entity id.
// Outputs: Observation, Config, State, FeatureResult, DerivativeResult,
// InterpretationResult, domain.VolumeEvent.
//
// Scientific constants: frozen identities and numeric values below. They are
// not environment variables, flags, or live tuning knobs.
//
// Ownership: one State per entity/symbol. No global or cross-engine state.
//
// Lifecycle: NewEngine / FrozenConfig construct; PrepareStep clones;
// Commit adopts; Reset clears scientific substate. Host ResetSymbol is not wired.
//
// Concurrency: one State is not safe for concurrent mutation. Future host
// keyed workers serialize per entity.
//
// Failure: Validate rejects empty entity. Mapping never mutates the Bar.
//
// Invariants: windows are positional and bounded; zero raw volume is 0;
// initiating-Bar lineage is preserved; Adaptive/Price packages are not imported.
//
// Non-responsibilities: P-03 D01 updateVolumeInfluence, Price science,
// ingestion/modelhost, Snapshot/Persistence, Volume Policy architecture.
package volume

import (
	"fmt"
	"strings"
)

// Frozen Volume Feature Science.
const (
	NormalizationID     = "ROLLING_MEDIAN_RATIO_15"
	RawWindow           = 15
	DerivativeWindow    = 3
	IntervalMeanWindow  = 15
	ProjectionID        = "VOLUME_POINT"
	InterpretationID    = "V_INTERVAL_B10_C2"
	InterpretationLabel = "V_EMISSION_V0_1"
	StateSource         = "INTERVAL_MEAN_V_N"
	LowerThreshold      = 0.9
	UpperThreshold      = 1.1
	ConfirmationCount   = 2
	Epsilon             = 1e-12
)

// Config is the frozen P-04V scientific identity plus the owning entity.
// Window and threshold fields are copies of the frozen constants so later
// code can read them from one value. They must not be retuned at runtime.
type Config struct {
	Entity             string
	NormalizationID    string
	RawWindow          int
	DerivativeWindow   int
	IntervalMeanWindow int
	ProjectionID       string
	InterpretationID   string
	StateSource        string
	LowerThreshold     float64
	UpperThreshold     float64
	ConfirmationCount  int
	Epsilon            float64
}

// FrozenConfig returns the approved scientific constants for one entity.
func FrozenConfig(entity string) Config {
	return Config{
		Entity:             strings.ToUpper(strings.TrimSpace(entity)),
		NormalizationID:    NormalizationID,
		RawWindow:          RawWindow,
		DerivativeWindow:   DerivativeWindow,
		IntervalMeanWindow: IntervalMeanWindow,
		ProjectionID:       ProjectionID,
		InterpretationID:   InterpretationID,
		StateSource:        StateSource,
		LowerThreshold:     LowerThreshold,
		UpperThreshold:     UpperThreshold,
		ConfirmationCount:  ConfirmationCount,
		Epsilon:            Epsilon,
	}
}

// Validate checks entity presence and that scientific fields still match the
// frozen constants. It does not read the environment.
func (c Config) Validate() error {
	if c.Entity == "" {
		return fmt.Errorf("volume config: entity must be non-empty")
	}
	if c.NormalizationID != NormalizationID ||
		c.RawWindow != RawWindow ||
		c.DerivativeWindow != DerivativeWindow ||
		c.IntervalMeanWindow != IntervalMeanWindow ||
		c.ProjectionID != ProjectionID ||
		c.InterpretationID != InterpretationID ||
		c.StateSource != StateSource ||
		c.LowerThreshold != LowerThreshold ||
		c.UpperThreshold != UpperThreshold ||
		c.ConfirmationCount != ConfirmationCount ||
		c.Epsilon != Epsilon {
		return fmt.Errorf("volume config: scientific constants must remain frozen")
	}
	return nil
}
