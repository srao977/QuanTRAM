package marketfeed

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBarFromAlpaca(t *testing.T) {
	raw := alpacaBar{
		Type:      "b",
		Symbol:    "AAPL",
		Open:      143.59,
		High:      143.59,
		Low:       143.10,
		Close:     143.49,
		Volume:    json.Number("4060"),
		Timestamp: "2022-09-30T08:00:00Z",
		Trades:    json.Number("12"),
	}
	bar, err := barFromAlpaca(raw, "ALPACA_IEX", time.Date(2022, 9, 30, 8, 0, 1, 0, time.UTC), false)
	if err != nil {
		t.Fatalf("barFromAlpaca: %v", err)
	}
	if bar.Symbol != "AAPL" || bar.SourceTimestamp != raw.Timestamp || !bar.IsFinal || bar.IsBackfilled {
		t.Fatalf("unexpected bar: %+v", bar)
	}
	if bar.Volume != 4060 || bar.Close != 143.49 || bar.MarketSnapshotID == "" {
		t.Fatalf("unexpected numeric/id fields: %+v", bar)
	}
	if bar.IntervalEnd.Sub(bar.IntervalStart) != time.Minute {
		t.Fatalf("interval width %s", bar.IntervalEnd.Sub(bar.IntervalStart))
	}
}

func TestBarFromRawDoesNotConfuseTypeAndTimestamp(t *testing.T) {
	raw := json.RawMessage(`{"T":"b","S":"FAKEPACA","o":1,"h":2,"l":0.5,"c":1.5,"v":10,"t":"2026-08-30T16:48:00Z","n":3}`)
	bar, err := barFromRaw(raw, "ALPACA_TEST", time.Unix(0, 0).UTC(), false)
	if err != nil {
		t.Fatal(err)
	}
	if bar.SourceTimestamp != "2026-08-30T16:48:00Z" || bar.Symbol != "FAKEPACA" || bar.Volume != 10 {
		t.Fatalf("%+v", bar)
	}
}

func TestDecodeMessageArrayAndControl(t *testing.T) {
	raw := []byte(`[{"T":"success","msg":"authenticated"}]`)
	messages, err := decodeMessageArray(raw)
	if err != nil || len(messages) != 1 {
		t.Fatalf("decode array: %v len=%d", err, len(messages))
	}
	control, err := decodeControl(messages[0])
	if err != nil {
		t.Fatalf("decode control: %v", err)
	}
	if control.Type != "success" || control.Message != "authenticated" {
		t.Fatalf("control %+v", control)
	}
}

func TestParseAlpacaTimeNaiveCSV(t *testing.T) {
	parsed, err := parseAlpacaTime("2022-09-30 04:00:00")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Hour() != 4 || parsed.Minute() != 0 {
		t.Fatalf("parsed %s", parsed)
	}
}
