package adaptive

import (
	"math"
	"testing"
)

func TestD01FirstStepZeroLevel(t *testing.T) {
	m := NewModel("AAPL", DefaultConfig())
	dmo, fmo, err := m.Step(Observation{
		EntityID: "AAPL", EventTime: 1_664_510_400, SequenceID: 0,
		Price: 143.49, Volume: 4060, SourceQuality: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if dmo.StateLevel != 0 {
		t.Fatalf("first level=%g want 0", dmo.StateLevel)
	}
	if dmo.ConfigHash != DefaultConfigSHA256 {
		t.Fatalf("config_hash=%s", dmo.ConfigHash)
	}
	if len(fmo.Samples) != 8 {
		t.Fatalf("samples=%d", len(fmo.Samples))
	}
	if fmo.Samples[len(fmo.Samples)-1].Tau != fmo.IntervalLength {
		t.Fatal("terminal tau must equal interval")
	}
	if m.State().Sequence != 1 {
		t.Fatalf("sequence=%d", m.State().Sequence)
	}
}

func TestD01StepIsTransactionalOnError(t *testing.T) {
	m := NewModel("AAPL", DefaultConfig())
	_, _, err := m.Step(Observation{
		EntityID: "AAPL", EventTime: 100, SequenceID: 0,
		Price: 10, Volume: 1, SourceQuality: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	hash := m.StateHash()
	_, _, err = m.Step(Observation{
		EntityID: "AAPL", EventTime: 90, SequenceID: 1,
		Price: 11, Volume: 1, SourceQuality: 1,
	})
	if err == nil {
		t.Fatal("out-of-order event must fail")
	}
	if m.StateHash() != hash {
		t.Fatal("failed step must not commit state")
	}
	if m.State().Sequence != 1 {
		t.Fatalf("sequence mutated to %d", m.State().Sequence)
	}
}

func TestD01RejectsNonMonotonicSequence(t *testing.T) {
	m := NewModel("AAPL", DefaultConfig())
	_, _, err := m.Step(Observation{
		EntityID: "AAPL", EventTime: 100, SequenceID: 0,
		Price: 10, Volume: 1, SourceQuality: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = m.Step(Observation{
		EntityID: "AAPL", EventTime: 160, SequenceID: 0,
		Price: 11, Volume: 1, SourceQuality: 1,
	})
	if err == nil {
		t.Fatal("duplicate sequence must fail")
	}
}

func TestKinematicsClipCounts(t *testing.T) {
	level, vel, acc, curv, clipped := computeKinematics(
		100, 0, 1e-8, 0, 0, 1e-9,
		DefaultConfig().Kinematics, 1e-8,
	)
	if !finite(level) || !finite(vel) || !finite(acc) || !finite(curv) {
		t.Fatal("kinematics produced non-finite")
	}
	if clipped == 0 {
		t.Fatal("extreme dt should clip")
	}
}

func TestStructuralQualityUsesPowNotCbrt(t *testing.T) {
	shape := ReturnShape{Strength: 0.5, Coherence: 0.4, Persistence: 0.3}
	got := structuralQuality(shape)
	want := math.Pow(0.5*0.4*0.3, 1.0/3.0)
	if got != want {
		t.Fatalf("structural=%g want Pow %g (Cbrt would be %g)", got, want, math.Cbrt(0.5*0.4*0.3))
	}
}
