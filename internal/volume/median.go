package volume

import (
	"math"
	"sort"
)

// rollingMedian15 computes the frozen ROLLING_MEDIAN_RATIO_15 denominator.
//
// Purpose: deterministic odd-N median over exactly 15 positional V_RAW values.
//
// Inputs: a slice that must already be the last 15 causal raw observations,
// including the current observation. Callers must not pass a finite-harvested
// or compacted window.
//
// Outputs: (median, true) when all 15 values are finite; otherwise
// (NaN, false). Status (insufficient vs undefined) is assigned by the caller.
//
// Parameters: N is frozen at RawWindow (15). median = sorted[7].
//
// Ownership: copies before sorting; does not retain or mutate the input.
//
// Lifecycle: pure function. No state.
//
// Concurrency: safe; no shared storage.
//
// Failure: len != 15 or any nonfinite member → unavailable.
//
// Invariants: causal window order is not reordered in place; no even-N average;
// no skip-back for finite replacements.
//
// Non-responsibilities: V_N division, interval mean, V1/V2, interpretation.

// rollingMedian15 returns sorted[7] of a copied 15-value window.
func rollingMedian15(values []float64) (float64, bool) {
	if len(values) != RawWindow {
		return math.NaN(), false
	}
	for _, v := range values {
		if !finite(v) {
			return math.NaN(), false
		}
	}
	copied := append([]float64(nil), values...)
	sort.Float64s(copied)
	return copied[7], true
}
