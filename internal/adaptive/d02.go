package adaptive

// This file converts paired DMO/FMO outputs into a validated return shape.

import (
	"fmt"
	"math"

	"quantram/internal/domain"
)

// ForwardSample is one projected state sample carried into a ReturnShape.
type ForwardSample struct {
	Tau                float64 `bson:"tau"`
	Level              float64 `bson:"level"`
	Velocity           float64 `bson:"velocity"`
	Uncertainty        float64 `bson:"uncertainty"`
	Strength           float64 `bson:"strength"`
	Persistence        float64 `bson:"persistence"`
	ReversalPropensity float64 `bson:"reversal_propensity"`
}

// ReturnShape summarizes the geometry and scientific support of one forward
// projection for downstream capturability evaluation.
type ReturnShape struct {
	ModelTime                   float64              `bson:"model_time"`
	EntityID                    string               `bson:"entity_id"`
	SourceModelVersion          string               `bson:"source_model_version"`
	CurrentLevel                float64              `bson:"current_level"`
	ProjectionInterval          float64              `bson:"projection_interval"`
	ForwardHalfLife             float64              `bson:"forward_half_life"`
	ForwardSamples              []ForwardSample      `bson:"forward_samples"`
	TerminalDisplacement        float64              `bson:"terminal_displacement"`
	MaximumAbsoluteDisplacement float64              `bson:"maximum_absolute_displacement"`
	PathDirection               domain.PathDirection `bson:"path_direction"`
	TerminalDecayFactor         float64              `bson:"terminal_decay_factor"`
	Strength                    float64              `bson:"strength"`
	Coherence                   float64              `bson:"coherence"`
	Persistence                 float64              `bson:"persistence"`
	Uncertainty                 float64              `bson:"uncertainty"`
	ReversalPropensity          float64              `bson:"reversal_propensity"`
	StateSupportRatio           float64              `bson:"state_support_ratio"`
}

func requireFinite(name string, value float64) error {
	if !finite(value) {
		return fmt.Errorf("%s must be finite", name)
	}
	return nil
}

func requireBounded(name string, value, lower, upper float64) error {
	if err := requireFinite(name, value); err != nil {
		return err
	}
	if value < lower || value > upper {
		return fmt.Errorf("%s must be in [%g, %g]", name, lower, upper)
	}
	return nil
}

// BuildReturnShape validates a paired DMO/FMO result and derives terminal
// displacement, maximum excursion, path direction, and terminal decay.
func BuildReturnShape(dmo DMOOutput, fmo FMOOutput) (ReturnShape, error) {
	if err := validateD02Input(dmo, fmo); err != nil {
		return ReturnShape{}, err
	}

	samples := make([]ForwardSample, len(fmo.Samples))
	for i, sample := range fmo.Samples {
		samples[i] = ForwardSample{
			Tau:                sample.Tau,
			Level:              sample.Level,
			Velocity:           sample.Velocity,
			Uncertainty:        sample.Uncertainty,
			Strength:           sample.Strength,
			Persistence:        sample.Persistence,
			ReversalPropensity: sample.ReversalPropensity,
		}
	}

	terminalDisplacement := samples[len(samples)-1].Level - dmo.StateLevel
	maxAbs := 0.0
	for _, sample := range samples {
		d := abs(sample.Level - dmo.StateLevel)
		if d > maxAbs {
			maxAbs = d
		}
	}
	var direction domain.PathDirection
	switch {
	case terminalDisplacement > 0:
		direction = domain.PathUpward
	case terminalDisplacement < 0:
		direction = domain.PathDownward
	default:
		direction = domain.PathFlat
	}
	terminalDecay := math.Pow(2.0, -fmo.IntervalLength/dmo.ForwardHalfLife)

	shape := ReturnShape{
		ModelTime:                   dmo.ModelTime,
		EntityID:                    dmo.EntityID,
		SourceModelVersion:          dmo.ModelVersion,
		CurrentLevel:                dmo.StateLevel,
		ProjectionInterval:          fmo.IntervalLength,
		ForwardHalfLife:             dmo.ForwardHalfLife,
		ForwardSamples:              samples,
		TerminalDisplacement:        terminalDisplacement,
		MaximumAbsoluteDisplacement: maxAbs,
		PathDirection:               direction,
		TerminalDecayFactor:         terminalDecay,
		Strength:                    dmo.Strength,
		Coherence:                   dmo.Coherence,
		Persistence:                 dmo.Persistence,
		Uncertainty:                 dmo.Uncertainty,
		ReversalPropensity:          dmo.ReversalPropensity,
		StateSupportRatio:           dmo.StateSupportRatio,
	}
	if err := validateReturnShape(shape); err != nil {
		return ReturnShape{}, err
	}
	return shape, nil
}

func validateD02Input(dmo DMOOutput, fmo FMOOutput) error {
	if dmo.ModelTime != fmo.ModelTime {
		return fmt.Errorf("DMO and FMO model_time must match")
	}
	if dmo.EntityID != fmo.EntityID {
		return fmt.Errorf("DMO and FMO entity_id must match")
	}
	if dmo.ModelVersion != "0.2" {
		return fmt.Errorf("D02 v0.2 requires D01 model_version 0.2")
	}
	if dmo.EntityID == "" {
		return fmt.Errorf("entity_id must be non-empty")
	}
	if err := requireFinite("model_time", dmo.ModelTime); err != nil {
		return err
	}
	if err := requireFinite("state_level", dmo.StateLevel); err != nil {
		return err
	}
	if err := requireBounded("forward_interval", fmo.IntervalLength, 10.0, 600.0); err != nil {
		return err
	}
	if err := requireBounded("forward_half_life", dmo.ForwardHalfLife, 15.0, 900.0); err != nil {
		return err
	}
	for _, name := range []struct {
		label string
		value float64
	}{
		{"strength", dmo.Strength},
		{"coherence", dmo.Coherence},
		{"persistence", dmo.Persistence},
		{"uncertainty", dmo.Uncertainty},
		{"reversal_propensity", dmo.ReversalPropensity},
	} {
		if err := requireBounded(name.label, name.value, 0, 1); err != nil {
			return err
		}
	}
	if err := requireFinite("state_support_ratio", dmo.StateSupportRatio); err != nil {
		return err
	}
	if dmo.StateSupportRatio < 0 {
		return fmt.Errorf("state_support_ratio must be nonnegative")
	}
	if len(fmo.Samples) == 0 {
		return fmt.Errorf("forward_samples must be non-empty")
	}
	previousTau := 0.0
	for i, sample := range fmo.Samples {
		prefix := fmt.Sprintf("forward_samples[%d]", i)
		if err := requireFinite(prefix+".tau", sample.Tau); err != nil {
			return err
		}
		if sample.Tau <= previousTau {
			return fmt.Errorf("forward sample tau values must be strictly increasing")
		}
		if sample.Tau > fmo.IntervalLength {
			return fmt.Errorf("forward sample tau cannot exceed projection interval")
		}
		previousTau = sample.Tau
		if err := requireFinite(prefix+".level", sample.Level); err != nil {
			return err
		}
		if err := requireBounded(prefix+".velocity", sample.Velocity, -50, 50); err != nil {
			return err
		}
		for _, name := range []struct {
			label string
			value float64
		}{
			{prefix + ".uncertainty", sample.Uncertainty},
			{prefix + ".strength", sample.Strength},
			{prefix + ".persistence", sample.Persistence},
			{prefix + ".reversal_propensity", sample.ReversalPropensity},
		} {
			if err := requireBounded(name.label, name.value, 0, 1); err != nil {
				return err
			}
		}
	}
	if fmo.Samples[len(fmo.Samples)-1].Tau != fmo.IntervalLength {
		return fmt.Errorf("terminal forward sample tau must equal projection interval")
	}
	return nil
}

func validateReturnShape(shape ReturnShape) error {
	if shape.EntityID == "" {
		return fmt.Errorf("entity_id must be non-empty")
	}
	if shape.SourceModelVersion != "0.2" {
		return fmt.Errorf("source_model_version must be 0.2")
	}
	if shape.MaximumAbsoluteDisplacement < 0 {
		return fmt.Errorf("maximum_absolute_displacement must be nonnegative")
	}
	if !(shape.TerminalDecayFactor > 0 && shape.TerminalDecayFactor < 1) {
		return fmt.Errorf("terminal_decay_factor must be in (0, 1)")
	}
	return nil
}
