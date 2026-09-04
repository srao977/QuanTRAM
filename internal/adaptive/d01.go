package adaptive

// This file owns the D01 observation-to-DMO/FMO state transition.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Model holds the D01 configuration and its last committed runtime state.
type Model struct {
	Config     Config
	ConfigHash string
	state      RuntimeState
}

// NewModel creates a D01 model with fresh runtime state for entityID.
func NewModel(entityID string, cfg Config) *Model {
	return &Model{
		Config:     cfg,
		ConfigHash: cfg.SHA256(),
		state:      NewRuntimeState(entityID, cfg.HalfLife.Baseline),
	}
}

// Clone returns an independent copy of the model and its mutable state.
func (m *Model) Clone() *Model {
	return &Model{
		Config:     m.Config,
		ConfigHash: m.ConfigHash,
		state:      m.state.Clone(),
	}
}

// State returns a defensive copy of the last committed runtime state.
func (m *Model) State() RuntimeState {
	return m.state.Clone()
}

// StateHash returns the canonical hash of the last committed scientific state.
func (m *Model) StateHash() string {
	return stateHash(m.state)
}

// stateHash intentionally includes only the canonical scientific state vector
// and half-lives, excluding counters and transport metadata.
func stateHash(state RuntimeState) string {
	payload := map[string]float64{
		"acceleration":          state.StateVector.Acceleration,
		"curvature":             state.StateVector.Curvature,
		"forward_half_life":     state.HalfLife.ForwardHalfLife,
		"level":                 state.StateVector.Level,
		"observation_half_life": state.HalfLife.ObservationHalfLife,
		"persistence":           state.StateVector.Persistence,
		"reversal_propensity":   state.StateVector.ReversalPropensity,
		"strength":              state.StateVector.Strength,
		"uncertainty":           state.StateVector.Uncertainty,
		"velocity":              state.StateVector.Velocity,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// Step applies one D01 observation on a state copy and commits only on success.
func (m *Model) Step(observation Observation) (DMOOutput, FMOOutput, error) {
	working := m.state.Clone()
	dmo, fmo, err := stepD01(m.Config, m.ConfigHash, &working, observation)
	if err != nil {
		return DMOOutput{}, FMOOutput{}, err
	}
	m.state = working
	return dmo, fmo, nil
}

// stepD01 applies the complete causal scientific pipeline to state. Its caller
// supplies a working copy so any validation or finite-result error is atomic.
func stepD01(cfg Config, configHash string, state *RuntimeState, observation Observation) (DMOOutput, FMOOutput, error) {
	obs := observation.WithDefaults()
	if err := FiniteInputs(obs); err != nil {
		return DMOOutput{}, FMOOutput{}, err
	}
	if err := AssertCausalSequence(state.LastObservation, obs); err != nil {
		return DMOOutput{}, FMOOutput{}, err
	}

	dataQuality := clamp(obs.SourceQuality, 0, 1)
	epsilon := cfg.Numerical.Epsilon

	var dt float64
	if state.LastEventTime == nil {
		dt = 1.0
	} else {
		dt = max(0.0, obs.EventTime-*state.LastEventTime)
		if dt > 5.0 {
			state.DataGapCount++
		}
	}

	if state.Sequence == 0 {
		state.AdaptiveReference = obs.Price
		state.AdaptiveScale = max(cfg.Reference.MinScale, state.AdaptiveScale)
	} else {
		state.AdaptiveReference, state.AdaptiveScale = updateReferenceAndScale(
			obs.Price, state.AdaptiveReference, state.AdaptiveScale, cfg.Reference,
		)
	}

	level, velocity, acceleration, curvature, clipped := computeKinematics(
		obs.Price, state.AdaptiveReference, state.AdaptiveScale,
		state.PrevLevel, state.PrevVelocity, dt, cfg.Kinematics, epsilon,
	)
	state.ClippingCount += clipped

	residual, innovationMag := innovationMagnitude(level, state.PrevLevel, state.PrevVelocity, dt, epsilon)
	if innovationMag > 5.0 {
		state.InnovationExtremeCount++
	}

	volumeEnabled := cfg.Volume.Enabled && cfg.Ablation.VolumeInfluence
	volumeInfluence := 0.0
	if volumeEnabled {
		state.VolumeReference, volumeInfluence = updateVolumeInfluence(
			obs.Volume, state.VolumeReference, cfg.Volume, epsilon,
		)
	}

	perturbationClass, perturbationMagnitude, perturbationMultiplier := classifyPerturbation(
		innovationMag, state.PrevVelocity, velocity, dataQuality, cfg.Perturbation,
		epsilon, residual, state.PrevLevel,
	)

	evidence := map[string]float64{
		"displacement": level,
		"velocity":     velocity,
		"acceleration": acceleration,
		"volume":       0,
	}
	if volumeEnabled {
		evidence["volume"] = volumeInfluence
	}
	coherence := computeCoherence(evidence, cfg.Coherence.ChannelWeights, epsilon)
	if !cfg.Ablation.CoherenceInfluence {
		coherence = 0.5
	}

	strength := computeStrength(volumeInfluence, velocity, acceleration, coherence, state.StateVector.Uncertainty, cfg.Strength)

	persistence := updatePersistence(
		state.StateVector.Persistence, velocity, state.PrevVelocity, acceleration, perturbationClass, cfg.Persistence,
	)

	unknownPerturbation := 0.0
	if perturbationClass == PerturbationStructural {
		unknownPerturbation = 1.0
	}
	uncertainty := computeUncertainty(
		innovationMag, coherence, unknownPerturbation, 1.0-dataQuality, min(1.0, abs(residual)), cfg.Uncertainty,
	)

	reversal := 0.0
	if cfg.Ablation.ReversalChannel {
		reversal = computeReversalPropensity(velocity, acceleration, perturbationClass, persistence, level, uncertainty, cfg.Reversal)
	}

	obsHL := cfg.HalfLife.Baseline
	fwdHL := cfg.HalfLife.Baseline
	if cfg.Ablation.AdaptiveHalfLife {
		obsHL = adaptHalfLife(state.HalfLife.ObservationHalfLife, persistence, strength, uncertainty, perturbationClass, cfg.HalfLife)
		fwdHL = adaptHalfLife(state.HalfLife.ForwardHalfLife, persistence, strength, uncertainty, perturbationClass, cfg.HalfLife)
	}

	adaptiveMult := 1.0
	if cfg.Ablation.PerturbationAdaptation {
		adaptiveMult = perturbationMultiplier
	}
	updatedParams, updateMag, boundHits := updateParameters(state.ParameterState, uncertainty, strength, adaptiveMult, cfg.Adaptation)
	state.ParameterState = updatedParams
	state.ParameterUpdateMagnitude = updateMag
	state.ParameterBoundHits += boundHits
	refAlpha, ok := state.ParameterState["ref_alpha"]
	if !ok {
		refAlpha = cfg.Reference.Alpha
	}
	state.ParameterState["ref_alpha"] = clamp(refAlpha, 0.001, 0.2)

	state.ModelTime = obs.EventTime
	state.Sequence++
	eventTime := obs.EventTime
	state.LastEventTime = &eventTime
	cloned := obs.Clone()
	state.LastObservation = &cloned
	state.PrevLevel = level
	state.PrevVelocity = velocity
	state.StateVector.Level = level
	state.StateVector.Velocity = velocity
	state.StateVector.Acceleration = acceleration
	state.StateVector.Curvature = curvature
	state.StateVector.Strength = strength
	state.StateVector.Persistence = persistence
	state.StateVector.PerturbationMagnitude = perturbationMagnitude
	state.StateVector.Uncertainty = uncertainty
	state.StateVector.ReversalPropensity = reversal
	state.HalfLife.ObservationHalfLife = obsHL
	state.HalfLife.ForwardHalfLife = fwdHL

	intervalLength := cfg.Forward.BaselineInterval
	if cfg.Ablation.ElasticForwardInterval {
		intervalLength = computeForwardInterval(
			cfg.Forward.BaselineInterval, persistence, strength, uncertainty, perturbationMagnitude, cfg.Forward,
		)
	}
	taus := forwardSamples(intervalLength, cfg.Forward.SampleCount, cfg.Forward.SamplingExponent)
	samples := make([]FMOSample, 0, len(taus))
	for _, tau := range taus {
		decay := math.Pow(2.0, -tau/max(epsilon, fwdHL))
		projUnc := clamp(uncertainty+(1.0-decay)*0.15, 0, 1)
		samples = append(samples, FMOSample{
			Tau:                tau,
			Level:              propagateLevel(level, velocity, acceleration, tau),
			Velocity:           velocity * decay,
			Uncertainty:        projUnc,
			Strength:           clamp(strength*decay, 0, 1),
			Persistence:        clamp(persistence*decay, 0, 1),
			ReversalPropensity: clamp(reversal+(1.0-decay)*0.1, 0, 1),
		})
	}

	healthStatus := evaluateHealth(state)
	stateSupportRatio := (strength * persistence) / max(epsilon, uncertainty+reversal)
	traceID := fmt.Sprintf("%s:%d", state.EntityID, state.Sequence)

	paramsCopy := make(map[string]float64, len(state.ParameterState))
	for k, v := range state.ParameterState {
		paramsCopy[k] = v
	}
	magCopy := make(map[string]float64, len(state.ParameterUpdateMagnitude))
	for k, v := range state.ParameterUpdateMagnitude {
		magCopy[k] = v
	}

	dmo := DMOOutput{
		ModelTime:                state.ModelTime,
		EntityID:                 state.EntityID,
		ModelVersion:             cfg.ModelVersion,
		StateLevel:               level,
		StateVelocity:            velocity,
		StateAcceleration:        acceleration,
		StateCurvature:           curvature,
		Strength:                 strength,
		Coherence:                coherence,
		Persistence:              persistence,
		PerturbationMagnitude:    perturbationMagnitude,
		PerturbationClass:        perturbationClass,
		Uncertainty:              uncertainty,
		ReversalPropensity:       reversal,
		StateSupportRatio:        stateSupportRatio,
		ObservationHalfLife:      obsHL,
		ForwardHalfLife:          fwdHL,
		ParameterState:           paramsCopy,
		ParameterUpdateMagnitude: magCopy,
		DataQuality:              dataQuality,
		ModelHealth:              healthStatus,
		DMOSchemaVersion:         cfg.DMOSchemaVersion,
		FMOSchemaVersion:         cfg.FMOSchemaVersion,
		ConfigHash:               configHash,
		StateHash:                stateHash(*state),
		TraceID:                  traceID,
	}
	fmo := FMOOutput{
		ModelTime:      state.ModelTime,
		EntityID:       state.EntityID,
		IntervalLength: intervalLength,
		Samples:        samples,
	}
	if !finite(stateSupportRatio) {
		return DMOOutput{}, FMOOutput{}, fmt.Errorf("non-finite state_support_ratio")
	}
	return dmo, fmo, nil
}
