package adaptive

import (
	"fmt"
	"math"

	"quantram/internal/domain"
)

type ForwardSample struct {
	Tau                float64
	Level              float64
	Velocity           float64
	Uncertainty        float64
	Strength           float64
	Persistence        float64
	ReversalPropensity float64
}

type ReturnShape struct {
	ModelTime                   float64
	EntityID                    string
	SourceModelVersion          string
	CurrentLevel                float64
	ProjectionInterval          float64
	ForwardHalfLife             float64
	ForwardSamples              []ForwardSample
	TerminalDisplacement        float64
	MaximumAbsoluteDisplacement float64
	PathDirection               domain.PathDirection
	TerminalDecayFactor         float64
	Strength                    float64
	Coherence                   float64
	Persistence                 float64
	Uncertainty                 float64
	ReversalPropensity          float64
	StateSupportRatio           float64
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
