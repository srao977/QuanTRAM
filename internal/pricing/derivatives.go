package pricing

import "math"

// causalQuadraticAtIndex is SADE derivatives.causal_quadratic_at_index.
func causalQuadraticAtIndex(timesMinutes, prices []float64, index, window int) (p1, p2 float64, failures int) {
	if index < window-1 || index < 0 || index >= len(prices) {
		return math.NaN(), math.NaN(), 0
	}
	x := make([]float64, window)
	y := make([]float64, window)
	t0 := timesMinutes[index]
	for i := 0; i < window; i++ {
		src := index - window + 1 + i
		x[i] = timesMinutes[src] - t0
		y[i] = prices[src]
	}
	design := make([]float64, window*3)
	for i := 0; i < window; i++ {
		design[i*3+0] = x[i] * x[i]
		design[i*3+1] = x[i]
		design[i*3+2] = 1
	}
	coeff, rank, ok := lstsq(design, window, 3, y)
	if !ok || rank != 3 {
		return math.NaN(), math.NaN(), 1
	}
	return coeff[1], 2.0 * coeff[0], 0
}
