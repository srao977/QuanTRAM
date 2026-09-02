package pricing

import (
	"math"

	"gonum.org/v1/gonum/mat"
)

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func allFinite(values ...float64) bool {
	for _, v := range values {
		if !finite(v) {
			return false
		}
	}
	return true
}

// lstsq matches numpy.linalg.lstsq(..., rcond=None): SVD least squares,
// rank cutoff eps * max(M, N) * max(S).
func lstsq(design []float64, rows, cols int, y []float64) (coeff []float64, rank int, ok bool) {
	a := mat.NewDense(rows, cols, append([]float64(nil), design...))
	var svd mat.SVD
	if !svd.Factorize(a, mat.SVDThin) {
		return nil, 0, false
	}
	values := svd.Values(nil)
	if len(values) == 0 || values[0] == 0 {
		return nil, 0, false
	}
	eps := 2.220446049250313e-16
	mn := rows
	if cols > mn {
		mn = cols
	}
	tol := eps * float64(mn) * values[0]
	for _, v := range values {
		if v > tol {
			rank++
		}
	}
	b := mat.NewVecDense(rows, append([]float64(nil), y...))
	var x mat.VecDense
	if err := x.SolveVec(a, b); err != nil {
		return nil, rank, false
	}
	coeff = make([]float64, cols)
	copy(coeff, x.RawVector().Data[:cols])
	for _, v := range coeff {
		if !finite(v) {
			return coeff, rank, false
		}
	}
	return coeff, rank, true
}

func solveSymmetric(a []float64, n int, b []float64) ([]float64, bool) {
	m := mat.NewDense(n, n, append([]float64(nil), a...))
	rhs := mat.NewVecDense(n, append([]float64(nil), b...))
	var x mat.VecDense
	if err := x.SolveVec(m, rhs); err != nil {
		return nil, false
	}
	out := make([]float64, n)
	copy(out, x.RawVector().Data[:n])
	for _, v := range out {
		if !finite(v) {
			return out, false
		}
	}
	return out, true
}

func cond2(design []float64, rows, cols int) float64 {
	a := mat.NewDense(rows, cols, append([]float64(nil), design...))
	return mat.Cond(a, 2)
}

func maxRealEigenvalue(a []float64, n int) (float64, bool) {
	m := mat.NewDense(n, n, append([]float64(nil), a...))
	var eig mat.Eigen
	if !eig.Factorize(m, mat.EigenNone) {
		return math.NaN(), false
	}
	vals := eig.Values(nil)
	maxRe := math.Inf(-1)
	for _, z := range vals {
		if real(z) > maxRe {
			maxRe = real(z)
		}
	}
	if !finite(maxRe) {
		return math.NaN(), false
	}
	return maxRe, true
}

func matrixExp(a []float64, n int) (*mat.Dense, bool) {
	m := mat.NewDense(n, n, append([]float64(nil), a...))
	var exp mat.Dense
	exp.Exp(m)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if !finite(exp.At(i, j)) {
				return nil, false
			}
		}
	}
	return &exp, true
}

func maxColumnNorm2(m *mat.Dense, rows, cols int) float64 {
	maxN := 0.0
	for j := 0; j < cols; j++ {
		col := mat.Col(nil, j, m)
		n := 0.0
		for i := 0; i < rows; i++ {
			n += col[i] * col[i]
		}
		n = math.Sqrt(n)
		if n > maxN {
			maxN = n
		}
	}
	return maxN
}

func popStd(values []float64) (mean, std float64, ok bool) {
	n := float64(len(values))
	if n == 0 {
		return 0, 0, false
	}
	for _, v := range values {
		mean += v
	}
	mean /= n
	var acc float64
	for _, v := range values {
		d := v - mean
		acc += d * d
	}
	std = math.Sqrt(acc / n) // ddof=0
	if !finite(mean) || !finite(std) {
		return mean, std, false
	}
	return mean, std, true
}
