package adaptive

// This file centralizes floating-point finiteness checks.

import "math"

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
