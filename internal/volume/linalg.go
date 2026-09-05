package volume

import (
	"gonum.org/v1/gonum/mat"
)

// lstsq is the Volume copy of the P-04 NumPy rcond=None least-squares contract.
//
// Purpose: solve Xβ ≈ y for the 3×3 causal quadratic design without importing
// Price science.
//
// Inputs: row-major design (rows×cols), right-hand side y.
//
// Outputs: coefficient vector, numerical rank, and ok=false when SVD/solve
// fails or a coefficient is nonfinite.
//
// Model: SVD least squares. Rank cutoff is
//
//	eps * max(M, N) * max(S)
//
// with eps = 2.220446049250313e-16, matching numpy.linalg.lstsq(..., rcond=None)
// and internal/pricing/linalg.go. The algorithm is copied; Price state is not.
//
// Parameters: none. Window size is supplied by the caller (Volume uses 3).
//
// Ownership: copies design and y; does not retain caller slices.
//
// Lifecycle: pure function.
//
// Concurrency: safe; no shared storage.
//
// Failure: Factorize/Solve failure, empty/zero singular values, or nonfinite
// coefficients → ok=false.
//
// Invariants: same cutoff and finite-coefficient rule as P-04 lstsq.
//
// Non-responsibilities: V_N construction, interpretation, EXPM, Price windows.

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
