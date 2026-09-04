package pricing

// This file fits the local F4 jerk dynamics used for forward projection.

import "math"

// f4Fit retains both standardized regression coefficients and their physical
// coordinate equivalent, together with the fitted training envelope.
type f4Fit struct {
	Standardized [4]float64
	Physical     [4]float64
	Means        [3]float64
	Scales       [3]float64
	Minimum      [3]float64
	Maximum      [3]float64
	Condition    float64
}

// fitF4AtIndex ridge-fits jerk against standardized P, P1, and P2 over the
// trailing window. Scaling uses population standard deviation (ddof=0).
func fitF4AtIndex(p, p1, p2, jp []float64, index, window int, ridgeLambda float64) *f4Fit {
	if index < window-1 || index < 0 || index >= len(p)-1 {
		return nil
	}
	start := index - window + 1
	for i := start; i <= index; i++ {
		if !finite(jp[i]) {
			return nil
		}
	}
	vals := make([][3]float64, window)
	for i := 0; i < window; i++ {
		src := start + i
		vals[i] = [3]float64{p[src], p1[src], p2[src]}
	}
	var means, scales, minimum, maximum [3]float64
	for dim := 0; dim < 3; dim++ {
		col := make([]float64, window)
		minimum[dim] = math.Inf(1)
		maximum[dim] = math.Inf(-1)
		for i := 0; i < window; i++ {
			v := vals[i][dim]
			col[i] = v
			if v < minimum[dim] {
				minimum[dim] = v
			}
			if v > maximum[dim] {
				maximum[dim] = v
			}
		}
		m, s, ok := popStd(col)
		if !ok || s <= 0 {
			return nil
		}
		means[dim] = m
		scales[dim] = s
	}
	// design: [1, z0, z1, z2]  window x 4
	design := make([]float64, window*4)
	target := make([]float64, window)
	for i := 0; i < window; i++ {
		design[i*4+0] = 1
		for dim := 0; dim < 3; dim++ {
			design[i*4+1+dim] = (vals[i][dim] - means[dim]) / scales[dim]
		}
		target[i] = jp[start+i]
	}
	// (XᵀX + λ R) β = Xᵀ jp, R = diag(0,1,1,1)
	xtx := make([]float64, 16)
	xty := make([]float64, 4)
	for i := 0; i < window; i++ {
		for r := 0; r < 4; r++ {
			xty[r] += design[i*4+r] * target[i]
			for c := 0; c < 4; c++ {
				xtx[r*4+c] += design[i*4+r] * design[i*4+c]
			}
		}
	}
	xtx[1*4+1] += ridgeLambda
	xtx[2*4+2] += ridgeLambda
	xtx[3*4+3] += ridgeLambda
	beta, ok := solveSymmetric(xtx, 4, xty)
	if !ok {
		return nil
	}
	var slopes [3]float64
	for dim := 0; dim < 3; dim++ {
		slopes[dim] = beta[1+dim] / scales[dim]
	}
	phys0 := beta[0] - (slopes[0]*means[0] + slopes[1]*means[1] + slopes[2]*means[2])
	var std [4]float64
	copy(std[:], beta)
	return &f4Fit{
		Standardized: std,
		Physical:     [4]float64{phys0, slopes[0], slopes[1], slopes[2]},
		Means:        means,
		Scales:       scales,
		Minimum:      minimum,
		Maximum:      maximum,
		Condition:    cond2(design, window, 4),
	}
}
