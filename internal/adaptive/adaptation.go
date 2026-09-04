package adaptive

// This file applies bounded parameter learning after each D01 observation.

// updateParameters scales each base learning rate by confidence and
// perturbation, then reports both effective movement and bound contacts.
func updateParameters(params map[string]float64, uncertainty, strength, perturbationMultiplier float64, cfg AdaptationConfig) (updated map[string]float64, magnitudes map[string]float64, boundHits int) {
	updated = make(map[string]float64, len(params))
	magnitudes = make(map[string]float64, len(params))
	for name, current := range params {
		eta0, ok := cfg.BaseLearningRates[name]
		if !ok {
			eta0 = cfg.MinLearningRate
		}
		eta := eta0 * max(0.2, 1.0-uncertainty) * max(0.5, strength) * perturbationMultiplier
		eta = clamp(eta, cfg.MinLearningRate, cfg.MaxLearningRate)
		gradient := (strength - uncertainty) * 0.1
		proposal := current + eta*gradient
		bounds, ok := cfg.ParameterBounds[name]
		if !ok {
			bounds = Bound{current - 1.0, current + 1.0}
		}
		clipped := bounds.Clamp(proposal)
		if clipped != proposal {
			boundHits++
		}
		updated[name] = clipped
		magnitudes[name] = abs(clipped - current)
	}
	return updated, magnitudes, boundHits
}
