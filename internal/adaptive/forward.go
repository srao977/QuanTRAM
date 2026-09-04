package adaptive

// This file selects the elastic horizon and samples the forward trajectory.

import "math"

// computeForwardInterval contracts the horizon under uncertainty or
// perturbation and expands it under persistent, strong evidence.
func computeForwardInterval(baseline, persistence, strength, uncertainty, perturbationMagnitude float64, cfg ForwardConfig) float64 {
	length := baseline * (0.7 + 0.6*persistence) * (0.7 + 0.6*strength) * (1.1 - 0.8*uncertainty) * (1.0 - 0.35*perturbationMagnitude)
	return clamp(length, cfg.MinInterval, cfg.MaxInterval)
}

// forwardSamples distributes samples by a power law and preserves the horizon
// itself as the terminal sample.
func forwardSamples(length float64, sampleCount int, exponent float64) []float64 {
	if sampleCount <= 0 {
		return nil
	}
	points := make([]float64, 0, sampleCount)
	n := float64(sampleCount)
	for idx := 1; idx <= sampleCount; idx++ {
		tau := length * math.Pow(float64(idx)/n, exponent)
		points = append(points, tau)
	}
	return points
}

// propagateLevel evaluates constant-acceleration kinematics at tau.
func propagateLevel(level, velocity, acceleration, tau float64) float64 {
	return level + velocity*tau + 0.5*acceleration*tau*tau
}
