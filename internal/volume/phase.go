package volume

import (
	"math"

	"quantram/internal/domain"
)

// Frozen V1/V2 phase classification (P-04V Phase D).
//
// Purpose: map finite V1/V2 to the historical ACTIVITY_* phase label.
//
// Scientific inputs: available V1 and V2. Callers must not pass unavailable
// derivatives as zero.
//
// Scientific outputs: one of the five frozen phase strings.
//
// Model:
//
//	|V1| <= ε                 → ACTIVITY_STATIONARY
//	V1 > 0  and V2 >  ε       → ACTIVITY_INCREASING_ACCELERATING
//	V1 > 0  and V2 <= ε       → ACTIVITY_INCREASING_DECELERATING
//	V1 < 0  and V2 <  -ε      → ACTIVITY_DECREASING_ACCELERATING
//	V1 < 0  and V2 >= -ε      → ACTIVITY_DECREASING_DECELERATING
//
// Frozen constant: Epsilon = 1e-12. Not an environment knob.
//
// Ownership: pure function; no VolumeState.
//
// Failure: classifyPhase assumes finite inputs. Readiness is decided by
// classifyPhaseQuantity.
//
// Causal invariants: no prior V1/V2; no Price; no color.
//
// Lifecycle / concurrency: stateless.
//
// Non-responsibilities: raw/cockpit color, confirmation, color-age, proto.

// classifyPhaseQuantity applies frozen inequalities only when both derivatives
// are finite and available. Insufficient and undefined are not collapsed.
func classifyPhaseQuantity(v1, v2 Quantity) (string, FeatureStatus) {
	if v1.Available() && v2.Available() {
		return classifyPhase(v1.Value, v2.Value), FeatureAvailable
	}
	if v1.Status == FeatureUndefined || v2.Status == FeatureUndefined {
		return domain.VolumePhaseUnset, FeatureUndefined
	}
	return domain.VolumePhaseUnset, FeatureInsufficient
}

// classifyPhase is the frozen five-way V1/V2 classifier.
func classifyPhase(v1, v2 float64) string {
	if math.Abs(v1) <= Epsilon {
		return domain.VolumePhaseStationary
	}
	if v1 > 0 {
		if v2 > Epsilon {
			return domain.VolumePhaseIncreasingAccelerating
		}
		return domain.VolumePhaseIncreasingDecelerating
	}
	if v2 < -Epsilon {
		return domain.VolumePhaseDecreasingAccelerating
	}
	return domain.VolumePhaseDecreasingDecelerating
}
