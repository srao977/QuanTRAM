package marketfeed

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"

	"quantram/internal/domain"
)

var csvHeader = []string{"timestamp", "open", "high", "low", "close", "volume"}

type CSVSource struct {
	path   string
	symbol string

	mu     sync.RWMutex
	health domain.FeedHealth
}

func NewCSVSource(path, symbol string) *CSVSource {
	return &CSVSource{
		path:   path,
		symbol: symbol,
		health: domain.FeedHealth{
			SourceID:          SourceID("csv"),
			State:             domain.FeedHealthy,
			SubscribedSymbols: []string{symbol},
		},
	}
}

func (c *CSVSource) Health() domain.FeedHealth {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health
}

func (c *CSVSource) Run(ctx context.Context, symbols []string, out chan<- domain.Bar) error {
	c.setState(domain.FeedHealthy, "")
	file, err := os.Open(c.path)
	if err != nil {
		c.setState(domain.FeedFailed, err.Error())
		return fmt.Errorf("open csv %q: %w", c.path, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("read csv header: %w", err)
	}
	if !equalFields(header, csvHeader) {
		return fmt.Errorf("unexpected csv header %v", header)
	}

	symbol := c.symbol
	if len(symbols) > 0 {
		symbol = symbols[0]
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		record, err := reader.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			c.setState(domain.FeedFailed, err.Error())
			return err
		}
		bar, err := barFromCSV(symbol, record, time.Now().UTC())
		if err != nil {
			return err
		}
		c.recordMessage()
		select {
		case out <- bar:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *CSVSource) setState(state domain.FeedState, lastError string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.health.State = state
	c.health.LastError = lastError
}

func (c *CSVSource) recordMessage() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.health.LastMessage = time.Now()
}

func barFromCSV(symbol string, record []string, receipt time.Time) (domain.Bar, error) {
	if len(record) != len(csvHeader) {
		return domain.Bar{}, fmt.Errorf("csv row has %d fields", len(record))
	}
	values := make([]float64, 4)
	for i, field := range record[1:5] {
		value, err := strconv.ParseFloat(field, 64)
		if err != nil {
			return domain.Bar{}, fmt.Errorf("parse csv field: %w", err)
		}
		values[i] = value
	}
	volume, err := strconv.ParseUint(record[5], 10, 64)
	if err != nil {
		return domain.Bar{}, fmt.Errorf("parse csv volume: %w", err)
	}
	start, err := parseAlpacaTime(record[0])
	if err != nil {
		return domain.Bar{}, err
	}
	instrumentType, tradable := domain.ClassifyInstrument(symbol)
	return domain.Bar{
		Symbol:           symbol,
		InstrumentID:     symbol,
		InstrumentType:   instrumentType,
		Tradable:         tradable,
		Interval:         domain.Interval1Min,
		IntervalStart:    start,
		IntervalEnd:      start.Add(time.Minute),
		Open:             values[0],
		High:             values[1],
		Low:              values[2],
		Close:            values[3],
		Volume:           volume,
		SourceTimestamp:  record[0],
		ReceiptTime:      receipt,
		Source:           SourceID("csv"),
		QualityStatus:    domain.QualityComplete,
		IsFinal:          true,
		MarketSnapshotID: domain.SnapshotID(symbol, SourceID("csv"), record[0], values[0], values[1], values[2], values[3], volume),
	}, nil
}

func equalFields(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
