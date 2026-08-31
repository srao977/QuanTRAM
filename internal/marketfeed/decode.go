package marketfeed

import (
	"encoding/json"
	"fmt"
	"time"

	"quantram/internal/domain"
)

type alpacaControl struct {
	Type    string   `json:"T"`
	Message string   `json:"msg"`
	Code    int      `json:"code"`
	Bars    []string `json:"bars"`
}

type alpacaBar struct {
	Type      string      `json:"T"`
	Symbol    string      `json:"S"`
	Open      float64     `json:"o"`
	High      float64     `json:"h"`
	Low       float64     `json:"l"`
	Close     float64     `json:"c"`
	Volume    json.Number `json:"v"`
	Timestamp string      `json:"t"`
	Trades    json.Number `json:"n"`
}

type restBarsResponse struct {
	Bars          map[string][]restBar `json:"bars"`
	NextPageToken *string              `json:"next_page_token"`
}

type restBar struct {
	Timestamp string  `json:"t"`
	Open      float64 `json:"o"`
	High      float64 `json:"h"`
	Low       float64 `json:"l"`
	Close     float64 `json:"c"`
	Volume    uint64  `json:"v"`
	Trades    uint32  `json:"n"`
}

func decodeMessageArray(raw []byte) ([]json.RawMessage, error) {
	var messages []json.RawMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		return nil, fmt.Errorf("decode alpaca array: %w", err)
	}
	return messages, nil
}

func decodeControl(raw json.RawMessage) (alpacaControl, error) {
	fields, err := rawObject(raw)
	if err != nil {
		return alpacaControl{}, err
	}
	var control alpacaControl
	control.Type = rawString(fields, "T")
	control.Message = rawString(fields, "msg")
	control.Code = rawInt(fields, "code")
	if bars, ok := fields["bars"]; ok {
		_ = json.Unmarshal(bars, &control.Bars)
	}
	return control, nil
}

func rawObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	return fields, nil
}

func rawString(fields map[string]json.RawMessage, key string) string {
	value, ok := fields[key]
	if !ok {
		return ""
	}
	var parsed string
	if err := json.Unmarshal(value, &parsed); err != nil {
		return ""
	}
	return parsed
}

func rawInt(fields map[string]json.RawMessage, key string) int {
	value, ok := fields[key]
	if !ok {
		return 0
	}
	var parsed int
	if err := json.Unmarshal(value, &parsed); err != nil {
		return 0
	}
	return parsed
}

func barFromRaw(raw json.RawMessage, source string, receipt time.Time, backfilled bool) (domain.Bar, error) {
	fields, err := rawObject(raw)
	if err != nil {
		return domain.Bar{}, err
	}
	open, err := requiredFloat(fields, "o")
	if err != nil {
		return domain.Bar{}, err
	}
	high, err := requiredFloat(fields, "h")
	if err != nil {
		return domain.Bar{}, err
	}
	low, err := requiredFloat(fields, "l")
	if err != nil {
		return domain.Bar{}, err
	}
	closePx, err := requiredFloat(fields, "c")
	if err != nil {
		return domain.Bar{}, err
	}
	volume, err := requiredNumber(fields, "v")
	if err != nil {
		return domain.Bar{}, err
	}
	return barFromAlpaca(alpacaBar{
		Type:      rawString(fields, "T"),
		Symbol:    rawString(fields, "S"),
		Open:      open,
		High:      high,
		Low:       low,
		Close:     closePx,
		Volume:    volume,
		Timestamp: rawString(fields, "t"),
		Trades:    json.Number(rawNumber(fields, "n")),
	}, source, receipt, backfilled)
}

func requiredFloat(fields map[string]json.RawMessage, key string) (float64, error) {
	value, ok := fields[key]
	if !ok {
		return 0, fmt.Errorf("alpaca bar missing %s", key)
	}
	var parsed float64
	if err := json.Unmarshal(value, &parsed); err != nil {
		return 0, fmt.Errorf("alpaca bar unreadable %s", key)
	}
	return parsed, nil
}

func requiredNumber(fields map[string]json.RawMessage, key string) (json.Number, error) {
	if _, ok := fields[key]; !ok {
		return "", fmt.Errorf("alpaca bar missing %s", key)
	}
	return json.Number(rawNumber(fields, key)), nil
}

func rawNumber(fields map[string]json.RawMessage, key string) string {
	value, ok := fields[key]
	if !ok {
		return "0"
	}
	var asString string
	if err := json.Unmarshal(value, &asString); err == nil {
		return asString
	}
	return string(value)
}

func barFromAlpaca(raw alpacaBar, source string, receipt time.Time, backfilled bool) (domain.Bar, error) {
	if raw.Symbol == "" {
		return domain.Bar{}, fmt.Errorf("alpaca bar missing symbol")
	}
	if raw.Timestamp == "" {
		return domain.Bar{}, fmt.Errorf("alpaca bar missing timestamp")
	}
	start, err := parseAlpacaTime(raw.Timestamp)
	if err != nil {
		return domain.Bar{}, fmt.Errorf("parse alpaca timestamp %q: %w", raw.Timestamp, err)
	}
	if err := validateOHLC(raw.Open, raw.High, raw.Low, raw.Close); err != nil {
		return domain.Bar{}, err
	}
	volume, err := parseUint(raw.Volume)
	if err != nil {
		return domain.Bar{}, fmt.Errorf("parse alpaca volume: %w", err)
	}
	trades, _ := parseUint32(raw.Trades)
	instrumentType, tradable := domain.ClassifyInstrument(raw.Symbol)
	quality, isFinal := classifyAlpacaBar(raw.Type, backfilled)
	return domain.Bar{
		Symbol:           raw.Symbol,
		InstrumentID:     raw.Symbol,
		InstrumentType:   instrumentType,
		Tradable:         tradable,
		Interval:         domain.Interval1Min,
		IntervalStart:    start,
		IntervalEnd:      start.Add(time.Minute),
		Open:             raw.Open,
		High:             raw.High,
		Low:              raw.Low,
		Close:            raw.Close,
		Volume:           volume,
		EventCount:       trades,
		SourceTimestamp:  raw.Timestamp,
		ReceiptTime:      receipt,
		Source:           source,
		QualityStatus:    quality,
		IsFinal:          isFinal,
		IsBackfilled:     backfilled,
		MarketSnapshotID: domain.SnapshotID(raw.Symbol, source, raw.Timestamp, raw.Open, raw.High, raw.Low, raw.Close, volume),
	}, nil
}

func validateOHLC(open, high, low, close float64) error {
	for _, value := range []float64{open, high, low, close} {
		if value <= 0 || value != value {
			return fmt.Errorf("alpaca bar incomplete ohlc")
		}
	}
	if high < low || high < open || high < close || low > open || low > close {
		return fmt.Errorf("alpaca bar inconsistent ohlc")
	}
	return nil
}

func classifyAlpacaBar(messageType string, backfilled bool) (domain.QualityStatus, bool) {
	if backfilled {
		return domain.QualityReconstructed, true
	}
	if messageType == "u" {
		return domain.QualityPartial, false
	}
	return domain.QualityComplete, true
}

func parseUint(value json.Number) (uint64, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := value.Int64()
	if err != nil {
		f, ferr := value.Float64()
		if ferr != nil || f < 0 {
			return 0, err
		}
		return uint64(f), nil
	}
	if parsed < 0 {
		return 0, fmt.Errorf("negative volume")
	}
	return uint64(parsed), nil
}

func parseUint32(value json.Number) (uint32, error) {
	parsed, err := parseUint(value)
	return uint32(parsed), err
}

func parseAlpacaTime(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC(), nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC(), nil
	}
	if parsed, err := time.Parse("2006-01-02 15:04:05", value); err == nil {
		return parsed.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp %q", value)
}
