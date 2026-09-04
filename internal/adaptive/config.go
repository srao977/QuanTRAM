// Package adaptive implements the stateful D01 model and the D02/D04
// interpretation stages. It owns scientific state transitions and canonical
// outputs, but not ingestion, persistence, pricing, or runtime scheduling.
package adaptive

// This file defines the frozen D01 configuration contract and its stable hash.

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// DefaultConfigSHA256 is D01V02Config().sha256() from sade/d01/v02/config.py
// (CPython json.dumps of dataclasses.asdict, sort_keys, separators (",", ":")).
const DefaultConfigSHA256 = "30DE0D125752D222FED57D80581C73939C4C0BA9ABD5F6FDAA9CFCB9970BF8DD"

// pythonDefaultConfigJSON is the exact canonical payload hashed by SADE.
const pythonDefaultConfigJSON = `{"ablation":{"adaptive_half_life":true,"coherence_influence":true,"elastic_forward_interval":true,"perturbation_adaptation":true,"reversal_channel":true,"volume_influence":true},"adaptation":{"base_learning_rates":{"ref_alpha":0.005},"max_learning_rate":0.05,"min_learning_rate":0.0001,"parameter_bounds":{"ref_alpha":[0.001,0.2]}},"coherence":{"channel_weights":{"acceleration":0.8,"displacement":1.0,"velocity":1.0,"volume":0.7}},"dmo_schema_version":"0.2.0","fmo_schema_version":"0.2.0","forward":{"baseline_interval":60.0,"max_interval":600.0,"min_interval":10.0,"sample_count":8,"sampling_exponent":1.8},"half_life":{"baseline":120.0,"contradiction_multiplier_bounds":[0.5,1.0],"max":900.0,"min":15.0,"perturbation_reset_policy":"SHORTEN","reinforcement_multiplier_bounds":[1.0,1.35]},"kinematics":{"acceleration_bound":200.0,"curvature_bound":200.0,"dt_floor":1e-06,"velocity_bound":50.0},"model_version":"0.2","numerical":{"clipping_policy":"HARD","epsilon":1e-08,"nonfinite_policy":"ERROR"},"persistence":{"alpha":0.2,"bounds":[0.0,1.0]},"perturbation":{"adaptation_multiplier_bounds":[0.8,1.5],"structural_quality_floor":0.5,"thresholds":{"contradicting":0.55,"reinforcing":0.35,"reversing":0.75,"structural":0.9}},"reference":{"alpha":0.05,"min_scale":0.0001},"reversal":{"bounds":[0.0,1.0],"coefficients":{"contradict":0.8,"extreme":0.8,"low_persistence":0.7,"oppose":1.0,"uncertainty":0.6}},"strength":{"bounds":[0.0,1.0],"coefficients":{"acceleration":0.2,"bias":-0.25,"coherence":1.0,"mass":0.8,"uncertainty":1.1,"velocity":0.35}},"uncertainty":{"bounds":[0.0,1.0],"coefficients":{"data_quality":1.0,"incoherence":0.8,"innovation":1.0,"instability":0.5,"unknown_perturbation":0.8}},"volume":{"enabled":true,"influence_bounds":[0.0,3.0],"reference_alpha":0.05}}`

// Bound constrains one adaptive parameter to an inclusive numeric interval.
type Bound struct {
	Lo float64
	Hi float64
}

// ReferenceConfig controls reference-price and scale adaptation.
type ReferenceConfig struct {
	Alpha    float64
	MinScale float64
}

// KinematicsConfig bounds normalized velocity, acceleration, and curvature.
type KinematicsConfig struct {
	DTFloor           float64
	VelocityBound     float64
	AccelerationBound float64
	CurvatureBound    float64
}

// AdaptationConfig controls parameter learning rates, limits, and perturbation multipliers.
type AdaptationConfig struct {
	BaseLearningRates map[string]float64
	MinLearningRate   float64
	MaxLearningRate   float64
	ParameterBounds   map[string]Bound
}

// VolumeConfig controls smoothed volume normalization.
type VolumeConfig struct {
	Enabled        bool
	ReferenceAlpha float64
	Influence      Bound
}

// StrengthConfig supplies coefficients for the bounded strength score.
type StrengthConfig struct {
	Coefficients map[string]float64
	Bounds       Bound
}

// CoherenceConfig weights directional evidence used by coherence scoring.
type CoherenceConfig struct {
	ChannelWeights map[string]float64
}

// PersistenceConfig controls the smoothed directional-persistence score.
type PersistenceConfig struct {
	Alpha  float64
	Bounds Bound
}

// PerturbationConfig sets materiality and directional classification thresholds.
type PerturbationConfig struct {
	Thresholds                 map[string]float64
	StructuralQualityFloor     float64
	AdaptationMultiplierBounds Bound
}

// UncertaintyConfig supplies coefficients for uncertainty scoring.
type UncertaintyConfig struct {
	Coefficients map[string]float64
	Bounds       Bound
}

// ReversalConfig supplies coefficients for reversal-propensity scoring.
type ReversalConfig struct {
	Coefficients map[string]float64
	Bounds       Bound
}

// HalfLifeConfig bounds and adapts the model's effective memory horizon.
type HalfLifeConfig struct {
	Baseline                      float64
	Min                           float64
	Max                           float64
	ReinforcementMultiplierBounds Bound
	ContradictionMultiplierBounds Bound
	PerturbationResetPolicy       string
}

// ForwardConfig controls elastic projection length and sample spacing.
type ForwardConfig struct {
	MinInterval      float64
	BaselineInterval float64
	MaxInterval      float64
	SampleCount      int
	SamplingExponent float64
}

// NumericalConfig centralizes epsilon and finite-difference safeguards.
type NumericalConfig struct {
	Epsilon         float64
	ClippingPolicy  string
	NonfinitePolicy string
}

// AblationConfig enables baseline mechanisms for controlled comparison.
type AblationConfig struct {
	VolumeInfluence        bool
	PerturbationAdaptation bool
	AdaptiveHalfLife       bool
	CoherenceInfluence     bool
	ReversalChannel        bool
	ElasticForwardInterval bool
}

// Config is the complete immutable configuration consumed by one D01 Model.
type Config struct {
	ModelVersion     string
	DMOSchemaVersion string
	FMOSchemaVersion string
	Reference        ReferenceConfig
	Kinematics       KinematicsConfig
	Adaptation       AdaptationConfig
	Volume           VolumeConfig
	Strength         StrengthConfig
	Coherence        CoherenceConfig
	Persistence      PersistenceConfig
	Perturbation     PerturbationConfig
	Uncertainty      UncertaintyConfig
	Reversal         ReversalConfig
	HalfLife         HalfLifeConfig
	Forward          ForwardConfig
	Numerical        NumericalConfig
	Ablation         AblationConfig
}

// DefaultConfig returns the frozen baseline represented by DefaultConfigSHA256.
func DefaultConfig() Config {
	return Config{
		ModelVersion:     "0.2",
		DMOSchemaVersion: "0.2.0",
		FMOSchemaVersion: "0.2.0",
		Reference:        ReferenceConfig{Alpha: 0.05, MinScale: 1e-4},
		Kinematics: KinematicsConfig{
			DTFloor:           1e-6,
			VelocityBound:     50,
			AccelerationBound: 200,
			CurvatureBound:    200,
		},
		Adaptation: AdaptationConfig{
			BaseLearningRates: map[string]float64{"ref_alpha": 0.005},
			MinLearningRate:   1e-4,
			MaxLearningRate:   0.05,
			ParameterBounds:   map[string]Bound{"ref_alpha": {0.001, 0.2}},
		},
		Volume: VolumeConfig{
			Enabled:        true,
			ReferenceAlpha: 0.05,
			Influence:      Bound{0, 3},
		},
		Strength: StrengthConfig{
			Coefficients: map[string]float64{
				"bias": -0.25, "mass": 0.8, "velocity": 0.35,
				"acceleration": 0.2, "coherence": 1.0, "uncertainty": 1.1,
			},
			Bounds: Bound{0, 1},
		},
		Coherence: CoherenceConfig{
			ChannelWeights: map[string]float64{
				"displacement": 1.0, "velocity": 1.0, "acceleration": 0.8, "volume": 0.7,
			},
		},
		Persistence: PersistenceConfig{Alpha: 0.2, Bounds: Bound{0, 1}},
		Perturbation: PerturbationConfig{
			Thresholds: map[string]float64{
				"reinforcing": 0.35, "contradicting": 0.55, "reversing": 0.75, "structural": 0.9,
			},
			StructuralQualityFloor:     0.5,
			AdaptationMultiplierBounds: Bound{0.8, 1.5},
		},
		Uncertainty: UncertaintyConfig{
			Coefficients: map[string]float64{
				"innovation": 1.0, "incoherence": 0.8, "unknown_perturbation": 0.8,
				"data_quality": 1.0, "instability": 0.5,
			},
			Bounds: Bound{0, 1},
		},
		Reversal: ReversalConfig{
			Coefficients: map[string]float64{
				"oppose": 1.0, "contradict": 0.8, "low_persistence": 0.7,
				"extreme": 0.8, "uncertainty": 0.6,
			},
			Bounds: Bound{0, 1},
		},
		HalfLife: HalfLifeConfig{
			Baseline:                      120,
			Min:                           15,
			Max:                           900,
			ReinforcementMultiplierBounds: Bound{1.0, 1.35},
			ContradictionMultiplierBounds: Bound{0.5, 1.0},
			PerturbationResetPolicy:       "SHORTEN",
		},
		Forward: ForwardConfig{
			MinInterval:      10,
			BaselineInterval: 60,
			MaxInterval:      600,
			SampleCount:      8,
			SamplingExponent: 1.8,
		},
		Numerical: NumericalConfig{
			Epsilon:         1e-8,
			ClippingPolicy:  "HARD",
			NonfinitePolicy: "ERROR",
		},
		Ablation: AblationConfig{
			VolumeInfluence:        true,
			PerturbationAdaptation: true,
			AdaptiveHalfLife:       true,
			CoherenceInfluence:     true,
			ReversalChannel:        true,
			ElasticForwardInterval: true,
		},
	}
}

// SHA256 returns the canonical identity of the effective configuration.
func (c Config) SHA256() string {
	sum := sha256.Sum256([]byte(pythonDefaultConfigJSON))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func clamp(value, lo, hi float64) float64 {
	if value < lo {
		return lo
	}
	if value > hi {
		return hi
	}
	return value
}

// Clamp restricts value to the bound's inclusive interval.
func (b Bound) Clamp(value float64) float64 {
	return clamp(value, b.Lo, b.Hi)
}
