package adaptive

func adaptHalfLife(current, persistence, strength, uncertainty float64, perturbationClass string, cfg HalfLifeConfig) float64 {
	reinforceGain := cfg.ReinforcementMultiplierBounds.Clamp(1.0 + (persistence * strength * 0.2))
	contradictLoss := cfg.ContradictionMultiplierBounds.Clamp(1.0 - (uncertainty * 0.35))
	updated := current * reinforceGain * contradictLoss
	if isAdversePerturbation(perturbationClass) && cfg.PerturbationResetPolicy == "SHORTEN" {
		updated *= 0.75
	}
	return clamp(updated, cfg.Min, cfg.Max)
}
