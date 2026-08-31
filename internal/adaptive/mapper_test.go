package adaptive

import (
	"math"
	"testing"
	"time"

	"quantram/internal/domain"
)

func TestObservationEventTimeIsIntervalStart(t *testing.T) {
	start := time.Date(2022, 9, 30, 4, 0, 0, 0, time.UTC)
	bar := domain.Bar{
		Symbol:          "aapl",
		IntervalStart:   start,
		IntervalEnd:     start.Add(time.Minute),
		Close:           143.49,
		Volume:          4060,
		SourceTimestamp: "not-the-event-time",
		ReceiptTime:     start.Add(2 * time.Second),
		QualityStatus:   domain.QualityComplete,
		IsFinal:         true,
	}
	obs := ObservationFromBar(bar, 0)
	if obs.EntityID != "AAPL" {
		t.Fatalf("entity_id=%s", obs.EntityID)
	}
	if obs.EventTime != float64(start.Unix()) {
		t.Fatalf("event_time=%g want %d from IntervalStart", obs.EventTime, start.Unix())
	}
	if obs.Price != 143.49 {
		t.Fatalf("price=%g", obs.Price)
	}
	if obs.Volume != 4060 {
		t.Fatalf("volume=%g", obs.Volume)
	}
	if obs.Session != "UNKNOWN" {
		t.Fatalf("session=%s", obs.Session)
	}
	if obs.SourceQuality != 1 {
		t.Fatalf("source_quality=%g", obs.SourceQuality)
	}
	if obs.ReceiveTime == obs.EventTime {
		t.Fatal("receive_time must not replace event_time")
	}
}

func TestObservationQualityFollowsModelEligible(t *testing.T) {
	start := time.Date(2026, 8, 31, 16, 52, 0, 0, time.UTC)
	partial := domain.Bar{
		Symbol:        "MSFT",
		IntervalStart: start,
		Close:         400,
		QualityStatus: domain.QualityPartial,
		IsFinal:       false,
	}
	obs := ObservationFromBar(partial, 3)
	if obs.SourceQuality != 0 {
		t.Fatalf("ineligible bar quality=%g", obs.SourceQuality)
	}
	if obs.SequenceID != 3 {
		t.Fatalf("sequence_id=%d", obs.SequenceID)
	}
}

func TestBarFromOHLCVUsesUTCIntervalStart(t *testing.T) {
	bar, err := BarFromOHLCV("AAPL", "2022-09-30 04:00:00", 143.59, 143.59, 143.1, 143.49, 4060)
	if err != nil {
		t.Fatal(err)
	}
	if !bar.ModelEligible() {
		t.Fatal("fixture bar must be model eligible")
	}
	want := time.Date(2022, 9, 30, 4, 0, 0, 0, time.UTC)
	if !bar.IntervalStart.Equal(want) {
		t.Fatalf("IntervalStart=%s", bar.IntervalStart)
	}
	obs := ObservationFromBar(bar, 0)
	if math.Abs(obs.EventTime-float64(want.Unix())) > 0 {
		t.Fatalf("event_time=%g", obs.EventTime)
	}
}

func TestConfigFingerprintMatchesSADE(t *testing.T) {
	if got := DefaultConfig().SHA256(); got != DefaultConfigSHA256 {
		t.Fatalf("config hash %s want %s", got, DefaultConfigSHA256)
	}
	if BaselineRuleFingerprint == "" || BaselineImplementationFingerprint == "" {
		t.Fatal("baseline fingerprints must be set")
	}
}
