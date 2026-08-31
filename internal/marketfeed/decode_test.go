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
	if bar.QualityStatus != "COMPLETE" {
		t.Fatalf("quality %s", bar.QualityStatus)
	}
}

func TestBarFromAlpacaUpdatedIsPartial(t *testing.T) {
	raw := alpacaBar{
		Type:      "u",
		Symbol:    "AAPL",
		Open:      313.64,
		High:      313.70,
		Low:       313.62,
		Close:     313.65,
		Volume:    json.Number("100"),
		Timestamp: "2026-08-31T16:52:00Z",
	}
	bar, err := barFromAlpaca(raw, "ALPACA_IEX", time.Date(2026, 8, 31, 16, 52, 20, 0, time.UTC), false)
	if err != nil {
		t.Fatal(err)
	}
	if bar.IsFinal || bar.QualityStatus != "PARTIAL" {
		t.Fatalf("updated bar should be partial: %+v", bar)
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

func TestBarFromRawSkipsIncompleteOrUnreadable(t *testing.T) {
	receipt := time.Unix(0, 0).UTC()
	cases := []string{
		`{"T":"b","S":"AAPL","h":2,"l":1,"c":1.5,"v":10,"t":"2026-08-31T16:52:00Z"}`,
		`{"T":"b","S":"AAPL","o":"x","h":2,"l":1,"c":1.5,"v":10,"t":"2026-08-31T16:52:00Z"}`,
		`{"T":"b","S":"AAPL","o":0,"h":2,"l":1,"c":1.5,"v":10,"t":"2026-08-31T16:52:00Z"}`,
		`{"T":"b","S":"AAPL","o":1.5,"h":1,"l":1.2,"c":1.3,"v":10,"t":"2026-08-31T16:52:00Z"}`,
	}
	for _, raw := range cases {
		if _, err := barFromRaw(json.RawMessage(raw), "ALPACA_IEX", receipt, false); err == nil {
			t.Fatalf("expected skip for %s", raw)
		}
	}
}

func TestBarFromAlpacaRejectsIncompleteOHLC(t *testing.T) {
	raw := alpacaBar{
		Type:      "b",
		Symbol:    "AAPL",
		Open:      0,
		High:      313.7,
		Low:       313.6,
		Close:     313.65,
		Volume:    json.Number("100"),
		Timestamp: "2026-08-31T16:52:00Z",
	}
	if _, err := barFromAlpaca(raw, "ALPACA_IEX", time.Time{}, false); err == nil {
		t.Fatal("expected incomplete ohlc to be rejected")
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
