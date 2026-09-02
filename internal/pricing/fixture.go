package pricing

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"quantram/internal/domain"
)

type pricingRow struct {
	SourceRowIndex    int
	SourceTimestamp   string
	EntityID          string
	Close             float64
	PricingStatus     string
	PricingEmitted    bool
	PricingColor      string
	PricingPhase      string
	PricingConfidence string
	RKSuccess         *bool
	DomainExit        *bool
	CockpitOutput     bool
	Open              float64
	High              float64
	Low               float64
	Volume            uint64
}

func FileSHA256(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func loadPricingFixture(pricingPath, ohlcvPath string) ([]pricingRow, error) {
	ohlcv, err := loadOHLCV(ohlcvPath)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(pricingPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	idx := headerIndex(header)
	var rows []pricingRow
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		ts := rec[idx["source_timestamp"]]
		oh, ok := ohlcv[ts]
		if !ok {
			return nil, fmt.Errorf("no OHLCV for %s", ts)
		}
		row := pricingRow{
			SourceRowIndex:    atoi(rec[idx["source_row_index"]]),
			SourceTimestamp:   ts,
			EntityID:          rec[idx["entity_id"]],
			Close:             atof(rec[idx["close"]]),
			PricingStatus:     rec[idx["pricing_status"]],
			PricingEmitted:    rec[idx["pricing_emitted"]] == "True",
			PricingColor:      rec[idx["pricing_color"]],
			PricingPhase:      rec[idx["pricing_phase"]],
			PricingConfidence: rec[idx["pricing_confidence"]],
			CockpitOutput:     rec[idx["price_cockpit_output"]] == "True",
			Open:              oh.open,
			High:              oh.high,
			Low:               oh.low,
			Volume:            oh.volume,
		}
		if v := rec[idx["rk_success"]]; v == "True" || v == "False" {
			b := v == "True"
			row.RKSuccess = &b
		}
		if v := rec[idx["domain_exit"]]; v == "True" || v == "False" {
			b := v == "True"
			row.DomainExit = &b
		}
		rows = append(rows, row)
	}
	return rows, nil
}

type ohlcv struct {
	open, high, low float64
	volume          uint64
}

func loadOHLCV(path string) (map[string]ohlcv, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	idx := headerIndex(header)
	out := map[string]ohlcv{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		vol, err := parseVolume(rec[idx["volume"]])
		if err != nil {
			return nil, err
		}
		out[rec[idx["source_timestamp"]]] = ohlcv{
			open:   atof(rec[idx["open"]]),
			high:   atof(rec[idx["high"]]),
			low:    atof(rec[idx["low"]]),
			volume: vol,
		}
	}
	return out, nil
}

func (row pricingRow) Bar() (domain.Bar, error) {
	return BarFromOHLCV(row.EntityID, row.SourceTimestamp, row.Open, row.High, row.Low, row.Close, row.Volume)
}

func headerIndex(header []string) map[string]int {
	out := make(map[string]int, len(header))
	for i, name := range header {
		out[strings.TrimSpace(name)] = i
	}
	return out
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}
