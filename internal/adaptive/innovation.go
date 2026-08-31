package adaptive

import "math"

func innovationMagnitude(level, prevLevel, prevVelocity, dt, epsilon float64) (residual, magnitude float64) {
	expected := prevLevel + prevVelocity*dt
	residual = level - expected
	magnitude = math.Sqrt((residual * residual) / max(dt+epsilon, epsilon))
	return residual, magnitude
}
