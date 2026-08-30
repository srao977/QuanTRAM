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
	Symbol           string
	InstrumentID     string
	InstrumentType   InstrumentType
	Tradable         bool
	Interval         string
	IntervalStart    time.Time
	IntervalEnd      time.Time
	Open             float64
	High             float64
	Low              float64
	Close            float64
	Volume           uint64
	EventCount       uint32
	SourceTimestamp  string
	ReceiptTime      time.Time
	Source           string
	QualityStatus    QualityStatus
	IsFinal          bool
	IsBackfilled     bool
	SourceTransition bool
	MarketSnapshotID string
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
