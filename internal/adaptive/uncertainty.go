package adaptive

func computeUncertainty(
	innovationMag, coherence, unknownPerturbation, dataQualityDegradation, instability float64,
	cfg UncertaintyConfig,
) float64 {
	c := cfg.Coefficients
	raw := c["innovation"]*innovationMag +
		c["incoherence"]*(1.0-coherence) +
		c["unknown_perturbation"]*unknownPerturbation +
		c["data_quality"]*dataQualityDegradation +
		c["instability"]*instability
	return cfg.Bounds.Clamp(sigmoid(raw - 1.0))
}
