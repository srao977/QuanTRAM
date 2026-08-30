package marketfeed

import (
	"context"
	"time"

	"quantram/internal/domain"
)

type BarRangeRequest struct {
	Symbol string
	From   time.Time
	To     time.Time
	Feed   string
}

type LiveBarSource interface {
	Run(ctx context.Context, symbols []string, out chan<- domain.Bar) error
	Health() domain.FeedHealth
}

type HistoricalBarSource interface {
	Bars(ctx context.Context, request BarRangeRequest) ([]domain.Bar, error)
}

type Credentials struct {
	Key    string
	Secret string
}

func SourceID(feed string) string {
	if feed == "test" {
		return "ALPACA_TEST"
	}
	if feed == "csv" {
		return "CSV"
	}
	return "ALPACA_IEX"
}
