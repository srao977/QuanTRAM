package adaptive

// This file maps motion support and uncertainty to bounded signal strength.

import "math"

func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

// computeStrength applies configured linear evidence weights before the
// logistic transform and final configured bounds.
func computeStrength(effectiveMass, velocity, acceleration, coherence, uncertainty float64, cfg StrengthConfig) float64 {
	c := cfg.Coefficients
	raw := c["bias"] +
		c["mass"]*effectiveMass +
		c["velocity"]*abs(velocity) +
		c["acceleration"]*abs(acceleration) +
		c["coherence"]*coherence -
		c["uncertainty"]*uncertainty
	return cfg.Bounds.Clamp(sigmoid(raw))
}
