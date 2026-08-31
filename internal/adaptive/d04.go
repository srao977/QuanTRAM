package adaptive

import (
	"fmt"
	"math"
	"sort"

	"quantram/internal/domain"
)

type EnvelopeContext struct {
	EvaluationTime float64
	MarketEligible *bool
}

func ProductionContext(evaluationTime float64) EnvelopeContext {
	return EnvelopeContext{EvaluationTime: evaluationTime}
}

type CapturabilityResult struct {
	HardEligibility        int
	GeometryQuality        float64
	StructuralQuality      float64
	RiskQuality            float64
	BaseCapturabilityScore float64
	CapturabilityScore     float64
	ReasonCodes            []string
}

func ValidateReturnShape(shape ReturnShape) error {
	if len(shape.ForwardSamples) == 0 {
		return fmt.Errorf("INVALID_RETURNSHAPE")
	}
	terminal := shape.ForwardSamples[len(shape.ForwardSamples)-1].Level - shape.CurrentLevel
	maximum := 0.0
	for _, sample := range shape.ForwardSamples {
		d := abs(sample.Level - shape.CurrentLevel)
		if d > maximum {
			maximum = d
		}
	}
	var direction domain.PathDirection
	switch {
	case shape.TerminalDisplacement > 0:
		direction = domain.PathUpward
	case shape.TerminalDisplacement < 0:
		direction = domain.PathDownward
	default:
		direction = domain.PathFlat
	}
	if abs(shape.TerminalDisplacement) > shape.MaximumAbsoluteDisplacement ||
		shape.TerminalDisplacement != terminal ||
		shape.MaximumAbsoluteDisplacement != maximum ||
		shape.PathDirection != direction {
		return fmt.Errorf("INVALID_RETURNSHAPE")
	}
	return nil
}

func geometryQuality(shape ReturnShape) float64 {
	if shape.MaximumAbsoluteDisplacement == 0 {
		return 0
	}
	return abs(shape.TerminalDisplacement) / shape.MaximumAbsoluteDisplacement
}

func structuralQuality(shape ReturnShape) float64 {
	// Must match Python (x ** (1.0 / 3.0)), not math.Cbrt.
	return math.Pow(shape.Strength*shape.Coherence*shape.Persistence, 1.0/3.0)
}

func riskQuality(shape ReturnShape) float64 {
	return math.Sqrt((1.0 - shape.Uncertainty) * (1.0 - shape.ReversalPropensity))
}

func EvaluateCapturability(shape ReturnShape, ctx EnvelopeContext) (CapturabilityResult, error) {
	if err := ValidateReturnShape(shape); err != nil {
		return CapturabilityResult{}, err
	}
	geometry := geometryQuality(shape)
	structural := structuralQuality(shape)
	risk := riskQuality(shape)
	base := geometry * structural * risk
	projectionValid := ctx.EvaluationTime <= shape.ModelTime+shape.ProjectionInterval
	hard := 0
	if projectionValid && (ctx.MarketEligible == nil || *ctx.MarketEligible) {
		hard = 1
	}
	final := float64(hard) * base

	reasons := make([]string, 0, 5)
	if shape.MaximumAbsoluteDisplacement == 0 {
		reasons = append(reasons, "ZERO_GEOMETRY")
	}
	if shape.Uncertainty > 0.5 {
		reasons = append(reasons, "UNCERTAINTY_HIGH")
	}
	if shape.ReversalPropensity > 0.5 {
		reasons = append(reasons, "REVERSAL_PROPENSITY_HIGH")
	}
	if !projectionValid {
		reasons = append(reasons, "SHAPE_STALE")
	}
	if ctx.MarketEligible != nil && !*ctx.MarketEligible {
		reasons = append(reasons, "MARKET_INELIGIBLE")
	}
	sort.Strings(reasons)

	return CapturabilityResult{
		HardEligibility:        hard,
		GeometryQuality:        geometry,
		StructuralQuality:      structural,
		RiskQuality:            risk,
		BaseCapturabilityScore: base,
		CapturabilityScore:     final,
		ReasonCodes:            reasons,
	}, nil
}
