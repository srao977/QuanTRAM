package adaptive

// This file evaluates whether a validated return shape is capturable.

import (
	"fmt"
	"math"
	"sort"

	"quantram/internal/domain"
)

// EnvelopeContext supplies evaluation-time and optional market eligibility
// constraints to capturability scoring.
type EnvelopeContext struct {
	EvaluationTime float64
	MarketEligible *bool
}

// ProductionContext creates an envelope context without a market eligibility
// override.
func ProductionContext(evaluationTime float64) EnvelopeContext {
	return EnvelopeContext{EvaluationTime: evaluationTime}
}

// CapturabilityResult contains the hard gate, component qualities, final score,
// and deterministic reason codes for one return shape.
type CapturabilityResult struct {
	HardEligibility        int      `bson:"hard_eligibility"`
	GeometryQuality        float64  `bson:"geometry_quality"`
	StructuralQuality      float64  `bson:"structural_quality"`
	RiskQuality            float64  `bson:"risk_quality"`
	BaseCapturabilityScore float64  `bson:"base_capturability_score"`
	CapturabilityScore     float64  `bson:"capturability_score"`
	ReasonCodes            []string `bson:"reason_codes"`
}

// ValidateReturnShape verifies that stored geometry agrees exactly with the
// forward samples from which it was derived.
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

// geometryQuality is terminal displacement divided by maximum excursion.
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

// EvaluateCapturability multiplies geometry, structural, and risk quality,
// then applies projection freshness and market eligibility as a hard gate.
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
