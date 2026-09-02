package domain

import (
	"testing"
	"time"
)

func TestClassifyBarContinuity(t *testing.T) {
	start := time.Date(2026, 9, 1, 13, 31, 0, 0, time.UTC)

	class, elapsed := ClassifyBarContinuity(time.Time{}, false, start)
	if class != ContinuityFirst || elapsed != 0 {
		t.Fatalf("first: %s %s", class, elapsed)
	}

	class, elapsed = ClassifyBarContinuity(start, true, start.Add(time.Minute))
	if class != ContinuityNormal || elapsed != time.Minute {
		t.Fatalf("normal: %s %s", class, elapsed)
	}

	class, elapsed = ClassifyBarContinuity(start, true, start.Add(2*time.Minute))
	if class != ContinuityIrregular || elapsed != 2*time.Minute {
		t.Fatalf("irregular 2m: %s %s", class, elapsed)
	}

	class, elapsed = ClassifyBarContinuity(start, true, start.Add(5*time.Minute))
	if class != ContinuityIrregular || elapsed != 5*time.Minute {
		t.Fatalf("irregular 5m: %s %s", class, elapsed)
	}

	class, elapsed = ClassifyBarContinuity(start, true, start)
	if class != ContinuityDuplicate || elapsed != 0 {
		t.Fatalf("duplicate: %s %s", class, elapsed)
	}

	class, elapsed = ClassifyBarContinuity(start.Add(3*time.Minute), true, start.Add(2*time.Minute))
	if class != ContinuityRegression || elapsed != -time.Minute {
		t.Fatalf("regression: %s %s", class, elapsed)
	}

	class, _ = ClassifyBarContinuity(start, true, start.Add(90*time.Second))
	if class != ContinuityUnaligned {
		t.Fatalf("unaligned mid-minute: %s", class)
	}
}
