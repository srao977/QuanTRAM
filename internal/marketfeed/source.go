// Package marketfeed adapts live, replay, and historical providers into domain bars.
package marketfeed

import (
	"context"
	"time"

	"quantram/internal/domain"
)

// BarRangeRequest selects a symbol and optional time bounds for historical bars.
type BarRangeRequest struct {
	Symbol string
	From   time.Time
	To     time.Time
	Feed   string
}

// LiveBarSource streams normalized bars and exposes current source health.
type LiveBarSource interface {
	Run(ctx context.Context, symbols []string, out chan<- domain.Bar) error
	Health() domain.FeedHealth
}

// HistoricalBarSource retrieves normalized bars for recovery and gap filling.
type HistoricalBarSource interface {
	Bars(ctx context.Context, request BarRangeRequest) ([]domain.Bar, error)
}

// Credentials contains provider authentication material.
type Credentials struct {
	Key    string
	Secret string
}

// SourceID maps a configured feed name to its canonical provenance label.
func SourceID(feed string) string {
	if feed == "test" {
		return "ALPACA_TEST"
	}
	if feed == "csv" {
		return "CSV"
	}
	return "ALPACA_IEX"
}
