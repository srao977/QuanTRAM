package domain

import "time"

// ContinuityClass is a runtime observation-continuity label.
// It is not a scientific D01/D02/D04 state.
type ContinuityClass string

// Continuity classes distinguish causal, irregular, and invalid timestamp transitions.
const (
	ContinuityFirst      ContinuityClass = "first"
	ContinuityNormal     ContinuityClass = "normal"
	ContinuityIrregular  ContinuityClass = "irregular"
	ContinuityDuplicate  ContinuityClass = "duplicate"
	ContinuityRegression ContinuityClass = "regression"
	ContinuityUnaligned  ContinuityClass = "unaligned"
)

// MinuteAligned reports whether t sits on a UTC one-minute boundary.
func MinuteAligned(t time.Time) bool {
	utc := t.UTC()
	return utc.Equal(utc.Truncate(time.Minute))
}

// ClassifyBarContinuity classifies causal observation order.
// A timestamp delta greater than one minute is irregular, not automatically a gap.
func ClassifyBarContinuity(prev time.Time, hasPrev bool, curr time.Time) (ContinuityClass, time.Duration) {
	if curr.IsZero() || !MinuteAligned(curr) {
		return ContinuityUnaligned, 0
	}
	if !hasPrev {
		return ContinuityFirst, 0
	}
	elapsed := curr.Sub(prev)
	switch {
	case elapsed == 0:
		return ContinuityDuplicate, 0
	case elapsed < 0:
		return ContinuityRegression, elapsed
	case elapsed%time.Minute != 0:
		return ContinuityUnaligned, elapsed
	case elapsed == time.Minute:
		return ContinuityNormal, elapsed
	default:
		return ContinuityIrregular, elapsed
	}
}
