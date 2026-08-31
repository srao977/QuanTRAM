package adaptive

import "math"

const (
	PerturbationNone          = "NONE"
	PerturbationReinforcing   = "REINFORCING"
	PerturbationContradicting = "CONTRADICTING"
	PerturbationReversing     = "REVERSING"
	PerturbationStructural    = "STRUCTURAL/UNKNOWN"
)

func direction(value, epsilon float64) int {
	if value > epsilon {
		return 1
	}
	if value < -epsilon {
		return -1
	}
	return 0
}

func inferPerturbationClass(innovationResidual, priorLevel, prevVelocity, velocity, directionalEpsilon float64) string {
	stateDirection := direction(priorLevel, directionalEpsilon)
	if stateDirection == 0 {
		stateDirection = direction(prevVelocity, directionalEpsilon)
	}
	evidenceDirection := direction(innovationResidual, directionalEpsilon)
	if evidenceDirection == 0 {
		evidenceDirection = direction(velocity-prevVelocity, directionalEpsilon)
	}
	if stateDirection == 0 || evidenceDirection == 0 {
		return PerturbationStructural
	}
	currentDirection := direction(velocity, directionalEpsilon)
	if evidenceDirection == -stateDirection {
		if currentDirection == -stateDirection {
			return PerturbationReversing
		}
		return PerturbationContradicting
	}
	if evidenceDirection == stateDirection {
		return PerturbationReinforcing
	}
	return PerturbationStructural
}

func classifyPerturbation(
	innovation, prevVelocity, velocity, sourceQuality float64,
	cfg PerturbationConfig,
	numericalEpsilon float64,
	innovationResidual float64,
	priorLevel float64,
) (class string, magnitude, multiplier float64) {
	q := clamp(innovation/(1.0+innovation), 0, 1)
	magMultiplier := cfg.AdaptationMultiplierBounds.Clamp(1.0 + q*(cfg.AdaptationMultiplierBounds.Hi-1.0))
	if sourceQuality < cfg.StructuralQualityFloor {
		return PerturbationStructural, q, magMultiplier
	}
	materialityFloor := math.Sqrt(max(0.0, numericalEpsilon))
	if q <= materialityFloor {
		return PerturbationNone, q, magMultiplier
	}
	class = inferPerturbationClass(innovationResidual, priorLevel, prevVelocity, velocity, 1e-15)
	return class, q, magMultiplier
}

func isAdversePerturbation(class string) bool {
	switch class {
	case PerturbationContradicting, PerturbationReversing, PerturbationStructural:
		return true
	default:
		return false
	}
}
