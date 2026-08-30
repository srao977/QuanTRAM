package marketfeed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAlpacaRESTBars(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/stocks/bars" {
			t.Fatalf("path %s", r.URL.Path)
		}
		if r.URL.Query().Get("symbols") != "AAPL" || r.URL.Query().Get("feed") != "iex" {
			t.Fatalf("query %s", r.URL.RawQuery)
		}
		if r.Header.Get("APCA-API-KEY-ID") == "" {
			t.Fatal("missing key header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"bars":{"AAPL":[{"t":"2022-09-30T08:00:00Z","o":143.59,"h":143.59,"l":143.1,"c":143.49,"v":4060,"n":4}]}}`))
	}))
	defer server.Close()

	client := NewAlpacaREST(server.URL, "iex", Credentials{Key: "k", Secret: "s"})
	bars, err := client.Bars(context.Background(), BarRangeRequest{
		Symbol: "AAPL",
		From:   time.Date(2022, 9, 30, 8, 0, 0, 0, time.UTC),
		To:     time.Date(2022, 9, 30, 8, 5, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 || !bars[0].IsBackfilled || bars[0].Close != 143.49 {
		t.Fatalf("bars %+v", bars)
	}
}
