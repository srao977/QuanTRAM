package pricing

// This file maps accepted bars and offline OHLCV fixtures into pricing inputs.

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"quantram/internal/domain"
)

// Observation is the live/offline pricing input. Time is IntervalStart minutes.
type Observation struct {
	Entity    string
	Timestamp string
	Minutes   float64
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	Session   string
	Source    string
	Snapshot  string
	Interval  time.Time
}

// ObservationFromBar maps a P-02/P-03 accepted bar.
// Minutes = IntervalStart unix ms / 60_000. Does not read Decision.side.
func ObservationFromBar(bar domain.Bar, cfg Config) Observation {
	session := cfg.DefaultSession
	source := bar.Source
	if source == "" {
		source = cfg.DefaultSourceProvider
	}
	return Observation{
		Entity:    strings.ToUpper(bar.Symbol),
		Timestamp: bar.SourceTimestamp,
		Minutes:   intervalMinutes(bar.IntervalStart),
		Open:      bar.Open,
		High:      bar.High,
		Low:       bar.Low,
		Close:     bar.Close,
		Volume:    float64(bar.Volume),
		Session:   session,
		Source:    source,
		Snapshot:  bar.MarketSnapshotID,
		Interval:  bar.IntervalStart,
	}
}

func intervalMinutes(t time.Time) float64 {
	if t.IsZero() {
		return 0
	}
	return float64(t.UTC().UnixMilli()) / 60_000.0
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
		Source:           "PRICING_UNIT_RUN_001",
		QualityStatus:    domain.QualityComplete,
		IsFinal:          true,
		IsBackfilled:     false,
		MarketSnapshotID: domain.SnapshotID(symbol, "PRICING_UNIT_RUN_001", timestamp, open, high, low, close, volume),
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

func parseVolume(value string) (uint64, error) {
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
