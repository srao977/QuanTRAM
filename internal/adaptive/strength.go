package adaptive

import "math"

func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

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
