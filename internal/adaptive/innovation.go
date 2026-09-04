package adaptive

// This file measures innovation against constant-velocity expectation.

import "math"

// innovationMagnitude returns the residual and its time-normalized magnitude,
// sqrt(residual^2 / max(dt+epsilon, epsilon)).
func innovationMagnitude(level, prevLevel, prevVelocity, dt, epsilon float64) (residual, magnitude float64) {
	expected := prevLevel + prevVelocity*dt
	residual = level - expected
	magnitude = math.Sqrt((residual * residual) / max(dt+epsilon, epsilon))
	return residual, magnitude
}
