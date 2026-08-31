package adaptive

func computeCoherence(evidence, weights map[string]float64, epsilon float64) float64 {
	num := 0.0
	den := 0.0
	for key, value := range evidence {
		w := weights[key]
		num += w * value
		den += w * abs(value)
	}
	if den <= epsilon {
		return 0.0
	}
	return clamp(abs(num)/(den+epsilon), 0, 1)
}
