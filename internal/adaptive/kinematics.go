package adaptive

// This file derives bounded normalized price kinematics.

import "math"

func clip(value, bound float64) (float64, bool) {
	if value > bound {
		return bound, true
	}
	if value < -bound {
		return -bound, true
	}
	return value, false
}

// computeKinematics normalizes price around the adaptive reference, computes
// finite-difference velocity and acceleration using the dt floor, and bounds
// each derivative while counting numerical clipping events.
func computeKinematics(
	price, reference, scale, prevLevel, prevVelocity, dt float64,
	cfg KinematicsConfig,
	epsilon float64,
) (level, velocity, acceleration, curvature float64, clipped int) {
	dtEff := max(cfg.DTFloor, dt)
	level = (price - reference) / max(scale, epsilon)
	velocity = (level - prevLevel) / (dtEff + epsilon)
	var hit bool
	velocity, hit = clip(velocity, cfg.VelocityBound)
	if hit {
		clipped++
	}
	acceleration = (velocity - prevVelocity) / (dtEff + epsilon)
	acceleration, hit = clip(acceleration, cfg.AccelerationBound)
	if hit {
		clipped++
	}
	curvature = acceleration / math.Pow(1.0+velocity*velocity, 1.5)
	curvature, hit = clip(curvature, cfg.CurvatureBound)
	if hit {
		clipped++
	}
	return level, velocity, acceleration, curvature, clipped
}
