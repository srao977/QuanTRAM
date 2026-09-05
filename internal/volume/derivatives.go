package volume

import "time"

// Causal quadratic derivatives of positional V_N (P-04V Phase C).
//
// Purpose: compute frozen V1/V2 from the last three positional V_N values
// and their actual IntervalStart times.
//
// Scientific inputs: last DerivativeWindow (3) aligned VN and Times entries.
//
// Scientific outputs: V1, V2 as Quantity values (available / insufficient /
// undefined). No phase label, no color.
//
// Mathematical model:
//
//	V_N(τ) = aτ² + bτ + c
//	V1 = b
//	V2 = 2a
//
// with τ_current = 0. Design matrix rows are [τ², τ, 1].
//
// Parameters: DerivativeWindow = 3. Not an environment knob.
//
// Time-coordinate semantics: τ_i = (t_i − t_current).Minutes() using the
// stored IntervalStart instants. Irregular and fractional spacing are kept.
// Positional indices [-2,-1,0] are never substituted for elapsed time.
//
// Numerical solver: lstsq in linalg.go (gonum SVD, NumPy rcond=None cutoff).
// Require rank == 3 and finite a, b, c, V1, V2.
//
// State ownership: read-only on caller slices. Copies before forming the
// design. Does not append history.
//
// Failure: < 3 positions → FeatureInsufficient. Three positions with a
// nonfinite V_N, invalid/duplicate/non-increasing time, rank ≠ 3, or
// nonfinite coefficients → FeatureUndefined. No imputation.
//
// Causal invariants: only the current and prior two positional observations
// participate. NaN occupies its index. Times/VN order is not rewritten.
//
// Concurrency: pure; caller serializes State.
//
// Non-responsibilities: interpretation, confirmation, Engine PrepareStep/
// Commit, modelhost, Adaptive/Price science, proto.

// DerivativeResult is the Phase C V1/V2 readiness pair.
type DerivativeResult struct {
	V1 Quantity
	V2 Quantity
}

// derivativesFromState fits V1/V2 on the last three positional VN/Times.
func derivativesFromState(vn []float64, times []time.Time) DerivativeResult {
	if len(vn) < DerivativeWindow || len(times) < DerivativeWindow {
		return undefinedDerivatives(FeatureInsufficient)
	}
	if len(vn) != len(times) {
		return undefinedDerivatives(FeatureUndefined)
	}

	y := append([]float64(nil), vn[len(vn)-DerivativeWindow:]...)
	t := append([]time.Time(nil), times[len(times)-DerivativeWindow:]...)

	for _, v := range y {
		if !finite(v) {
			return undefinedDerivatives(FeatureUndefined)
		}
	}

	tau, ok := elapsedMinutes(t)
	if !ok {
		return undefinedDerivatives(FeatureUndefined)
	}

	design := make([]float64, DerivativeWindow*3)
	for i := 0; i < DerivativeWindow; i++ {
		design[i*3+0] = tau[i] * tau[i]
		design[i*3+1] = tau[i]
		design[i*3+2] = 1
	}

	coeff, rank, solved := lstsq(design, DerivativeWindow, 3, y)
	if !solved || rank != 3 {
		return undefinedDerivatives(FeatureUndefined)
	}
	a, b := coeff[0], coeff[1]
	v1 := b
	v2 := 2 * a
	if !finite(a) || !finite(b) || !finite(coeff[2]) || !finite(v1) || !finite(v2) {
		return undefinedDerivatives(FeatureUndefined)
	}
	return DerivativeResult{
		V1: Quantity{Value: v1, Status: FeatureAvailable},
		V2: Quantity{Value: v2, Status: FeatureAvailable},
	}
}

// elapsedMinutes maps three IntervalStart values to τ with τ_current = 0.
//
// Requires strictly increasing, non-zero times. Does not reorder.
func elapsedMinutes(times []time.Time) ([]float64, bool) {
	if len(times) != DerivativeWindow {
		return nil, false
	}
	current := times[DerivativeWindow-1]
	if current.IsZero() {
		return nil, false
	}
	tau := make([]float64, DerivativeWindow)
	for i, tm := range times {
		if tm.IsZero() {
			return nil, false
		}
		if i > 0 && !tm.After(times[i-1]) {
			return nil, false
		}
		tau[i] = tm.Sub(current).Minutes()
		if !finite(tau[i]) {
			return nil, false
		}
	}
	if tau[DerivativeWindow-1] != 0 {
		return nil, false
	}
	return tau, true
}

func undefinedDerivatives(status FeatureStatus) DerivativeResult {
	return DerivativeResult{
		V1: unavailable(status),
		V2: unavailable(status),
	}
}
