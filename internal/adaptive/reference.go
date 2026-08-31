package adaptive

func updateReferenceAndScale(price, prevReference, prevScale float64, cfg ReferenceConfig) (float64, float64) {
	ref := prevReference + cfg.Alpha*(price-prevReference)
	absErr := abs(price - ref)
	scale := max(cfg.MinScale, (0.95*prevScale)+(0.05*absErr))
	return ref, scale
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
