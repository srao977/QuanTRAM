package adaptive

// This file maps accepted bars and offline fixtures into adaptive inputs.

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"quantram/internal/domain"
)

// ObservationFromBar maps a P-02 bar to a D01 observation.
// event_time is IntervalStart (UTC unix seconds), never SourceTimestamp.
func ObservationFromBar(bar domain.Bar, sequenceID int) Observation {
	quality := 0.0
	if bar.ModelEligible() {
		quality = 1.0
	}
	session := "UNKNOWN"
	return Observation{
		EntityID:      strings.ToUpper(bar.Symbol),
		EventTime:     eventTimeSeconds(bar.IntervalStart),
		ReceiveTime:   eventTimeSeconds(bar.ReceiptTime),
		SequenceID:    sequenceID,
		Price:         bar.Close,
		Volume:        float64(bar.Volume),
		Session:       session,
		SourceQuality: quality,
		AvailabilityMask: map[string]bool{
			"price":  true,
			"volume": true,
			"bid":    false,
			"ask":    false,
		},
	}
}

func eventTimeSeconds(t time.Time) float64 {
	if t.IsZero() {
		return 0
	}
	return float64(t.UTC().UnixNano()) / 1e9
}

// BarFromOHLCV is the offline fixture adapter. It is not a live source.
func BarFromOHLCV(symbol, timestamp string, open, high, low, close float64, volume uint64) (domain.Bar, error) {
	start, err := parseFixtureTimestamp(timestamp)
	if err != nil {
		return domain.Bar{}, err
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	return domain.Bar{
		Symbol:           symbol,
		InstrumentType:   domain.InstrumentStock,
		Tradable:         true,
		Interval:         domain.Interval1Min,
		IntervalStart:    start,
		IntervalEnd:      start.Add(time.Minute),
		Open:             open,
		High:             high,
		Low:              low,
		Close:            close,
		Volume:           volume,
		SourceTimestamp:  timestamp,
		Source:           "UNIT_RUN_001",
		QualityStatus:    domain.QualityComplete,
		IsFinal:          true,
		IsBackfilled:     false,
		MarketSnapshotID: domain.SnapshotID(symbol, "UNIT_RUN_001", timestamp, open, high, low, close, volume),
	}, nil
}

func parseFixtureTimestamp(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty fixture timestamp")
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable fixture timestamp %q", value)
}

// ParseVolume parses a finite, nonnegative fixture volume and truncates any
// fractional component to the domain's integer representation.
func ParseVolume(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("empty volume")
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil || !finite(f) || f < 0 {
		return 0, fmt.Errorf("invalid volume %q", value)
	}
	return uint64(f), nil
}
