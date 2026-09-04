package adaptive

// This file scores reversal propensity from opposing and unstable evidence.

// computeReversalPropensity combines motion opposition, adverse perturbation,
// low persistence, extreme level, and uncertainty through a bounded sigmoid.
func computeReversalPropensity(
	velocity, acceleration float64,
	perturbationClass string,
	persistence, level, uncertainty float64,
	cfg ReversalConfig,
) float64 {
	oppose := 0.0
	if velocity*acceleration < 0.0 {
		oppose = 1.0
	}
	contradict := 0.0
	if isAdversePerturbation(perturbationClass) {
		contradict = 1.0
	}
	extreme := min(1.0, abs(level)/4.0)
	c := cfg.Coefficients
	raw := c["oppose"]*oppose +
		c["contradict"]*contradict +
		c["low_persistence"]*(1.0-persistence) +
		c["extreme"]*extreme +
		c["uncertainty"]*uncertainty -
		1.2
	return cfg.Bounds.Clamp(sigmoid(raw))
}
