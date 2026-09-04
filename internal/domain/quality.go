package domain

import "time"

const (
	// MinInferContiguous is the minimum causal finalized history required for inference.
	MinInferContiguous = 2
	// MaxFinalLateness is the live-source freshness allowance after interval end.
	MaxFinalLateness = 90 * time.Second
)

// ModelEligible reports whether a bar may enter the live model path.
func (b Bar) ModelEligible() bool {
	return b.IsFinal && b.QualityStatus == QualityComplete && !b.IsBackfilled
}

// LiveFresh reports whether a live bar is within the finalization watermark.
// Replay sources are exempt from wall-clock freshness.
func (b Bar) LiveFresh(now time.Time) bool {
	if !liveSource(b.Source) {
		return true
	}
	if b.IntervalEnd.IsZero() {
		return false
	}
	if now.Before(b.IntervalEnd) {
		return true
	}
	return now.Sub(b.IntervalEnd) <= MaxFinalLateness
}

func liveSource(source string) bool {
	return source == "ALPACA_IEX" || source == "ALPACA_TEST"
}

// Finalized returns finalized bars in their input order.
func Finalized(bars []Bar) []Bar {
	out := make([]Bar, 0, len(bars))
	for _, bar := range bars {
		if bar.IsFinal {
			out = append(out, bar)
		}
	}
	return out
}

// LastFinalized returns the last finalized bar in input order.
func LastFinalized(bars []Bar) (Bar, bool) {
	for i := len(bars) - 1; i >= 0; i-- {
		if bars[i].IsFinal {
			return bars[i], true
		}
	}
	return Bar{}, false
}

// ContiguousFinalized reports whether the last n finalized bars are in causal
// IntervalStart order. A skipped provider minute is allowed.
func ContiguousFinalized(bars []Bar, n int) bool {
	if n <= 0 {
		return true
	}
	finals := Finalized(bars)
	if len(finals) < n {
		return false
	}
	tail := finals[len(finals)-n:]
	for i := 1; i < len(tail); i++ {
		if !tail[i].IntervalStart.After(tail[i-1].IntervalStart) {
			return false
		}
	}
	return true
}

// InferReady is the P-02 quality gate. Continuity is causal observation
// order (strictly increasing IntervalStart), not fixed one-minute adjacency.
func InferReady(bars []Bar, now time.Time) bool {
	if !ContiguousFinalized(bars, MinInferContiguous) {
		return false
	}
	last, ok := LastFinalized(bars)
	if !ok {
		return false
	}
	return last.ModelEligible() && last.LiveFresh(now)
}
