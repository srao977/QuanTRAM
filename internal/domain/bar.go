package domain

import (
	"crypto/sha256"
	"fmt"
	"time"
)

const Interval1Min = "1Min"

type InstrumentType string

const (
	InstrumentUnspecified InstrumentType = ""
	InstrumentStock       InstrumentType = "STOCK"
	InstrumentETF         InstrumentType = "ETF"
	InstrumentIndex       InstrumentType = "INDEX"
)

type QualityStatus string

const (
	QualityUnspecified   QualityStatus = ""
	QualityComplete      QualityStatus = "COMPLETE"
	QualityDegraded      QualityStatus = "DEGRADED"
	QualityStale         QualityStatus = "STALE"
	QualityPartial       QualityStatus = "PARTIAL"
	QualityReconstructed QualityStatus = "RECONSTRUCTED"
	QualityInvalid       QualityStatus = "INVALID"
)

type Bar struct {
	Symbol           string         `bson:"symbol"`
	InstrumentID     string         `bson:"instrument_id"`
	InstrumentType   InstrumentType `bson:"instrument_type"`
	Tradable         bool           `bson:"tradable"`
	Interval         string         `bson:"interval"`
	IntervalStart    time.Time      `bson:"interval_start_unix_ms"`
	IntervalEnd      time.Time      `bson:"interval_end_unix_ms"`
	Open             float64        `bson:"open"`
	High             float64        `bson:"high"`
	Low              float64        `bson:"low"`
	Close            float64        `bson:"close"`
	Volume           uint64         `bson:"volume"`
	EventCount       uint32         `bson:"event_count"`
	SourceTimestamp  string         `bson:"source_timestamp"`
	ReceiptTime      time.Time      `bson:"receipt_unix_ms"`
	Source           string         `bson:"source"`
	QualityStatus    QualityStatus  `bson:"quality_status"`
	IsFinal          bool           `bson:"is_final"`
	IsBackfilled     bool           `bson:"is_backfilled"`
	SourceTransition bool           `bson:"source_transition"`
	MarketSnapshotID string         `bson:"market_snapshot_id"`
}

func (b Bar) DataAge(now time.Time) time.Duration {
	if b.IntervalStart.IsZero() {
		return 0
	}
	return now.Sub(b.IntervalStart)
}

func (b Bar) DedupKey() string {
	return b.Symbol + "|" + b.IntervalStart.UTC().Format(time.RFC3339Nano)
}

func SnapshotID(symbol, source, sourceTimestamp string, open, high, low, close float64, volume uint64) string {
	payload := fmt.Sprintf("%s|%s|%s|%.8f|%.8f|%.8f|%.8f|%d", symbol, source, sourceTimestamp, open, high, low, close, volume)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(payload)))
}

func ClassifyInstrument(symbol string) (InstrumentType, bool) {
	switch symbol {
	case "":
		return InstrumentUnspecified, false
	default:
		return InstrumentStock, true
	}
}
