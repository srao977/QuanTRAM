package adaptive

func updatePersistence(prevPersistence, velocity, prevVelocity, acceleration float64, perturbationClass string, cfg PersistenceConfig) float64 {
	directionAgreement := 0.0
	if velocity*prevVelocity >= 0.0 {
		directionAgreement = 1.0
	}
	accelPenalty := min(1.0, abs(acceleration)/(1.0+abs(acceleration)))
	pertPenalty := 0.0
	if isAdversePerturbation(perturbationClass) {
		pertPenalty = 0.4
	}
	agreement := clamp(directionAgreement-accelPenalty*0.25-pertPenalty, 0, 1)
	value := (1.0-cfg.Alpha)*prevPersistence + cfg.Alpha*agreement
	return cfg.Bounds.Clamp(value)
}
