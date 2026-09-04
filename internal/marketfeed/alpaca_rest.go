package marketfeed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"quantram/internal/domain"
)

// AlpacaREST retrieves paginated historical bars from Alpaca's data API.
type AlpacaREST struct {
	baseURL     string
	feed        string
	credentials Credentials
	client      *http.Client
}

// NewAlpacaREST constructs a historical source with a bounded HTTP timeout.
func NewAlpacaREST(baseURL, feed string, credentials Credentials) *AlpacaREST {
	if feed == "test" {
		feed = "iex"
	}
	return &AlpacaREST{
		baseURL:     baseURL,
		feed:        feed,
		credentials: credentials,
		client:      &http.Client{Timeout: 15 * time.Second},
	}
}

// Bars retrieves all pages for a request and marks normalized bars as backfilled.
func (a *AlpacaREST) Bars(ctx context.Context, request BarRangeRequest) ([]domain.Bar, error) {
	if request.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	feed := request.Feed
	if feed == "" || feed == "test" {
		feed = a.feed
	}
	var collected []domain.Bar
	pageToken := ""
	for {
		page, next, err := a.fetchPage(ctx, request, feed, pageToken)
		if err != nil {
			return nil, err
		}
		collected = append(collected, page...)
		if next == "" {
			return collected, nil
		}
		pageToken = next
	}
}

func (a *AlpacaREST) fetchPage(ctx context.Context, request BarRangeRequest, feed, pageToken string) ([]domain.Bar, string, error) {
	endpoint, err := url.Parse(a.baseURL + "/v2/stocks/bars")
	if err != nil {
		return nil, "", err
	}
	query := endpoint.Query()
	query.Set("symbols", request.Symbol)
	query.Set("timeframe", domain.Interval1Min)
	query.Set("feed", feed)
	query.Set("limit", "10000")
	if !request.From.IsZero() {
		query.Set("start", request.From.UTC().Format(time.RFC3339))
	}
	if !request.To.IsZero() {
		query.Set("end", request.To.UTC().Format(time.RFC3339))
	}
	if pageToken != "" {
		query.Set("page_token", pageToken)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("APCA-API-KEY-ID", a.credentials.Key)
	req.Header.Set("APCA-API-SECRET-KEY", a.credentials.Secret)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("alpaca bars request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("alpaca bars status %d", resp.StatusCode)
	}

	var payload restBarsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, "", fmt.Errorf("decode alpaca bars: %w", err)
	}
	receipt := time.Now().UTC()
	source := SourceID(feed)
	bars := make([]domain.Bar, 0, len(payload.Bars[request.Symbol]))
	for _, raw := range payload.Bars[request.Symbol] {
		bar, err := barFromAlpaca(alpacaBar{
			Type:      "b",
			Symbol:    request.Symbol,
			Open:      raw.Open,
			High:      raw.High,
			Low:       raw.Low,
			Close:     raw.Close,
			Volume:    json.Number(strconv.FormatUint(raw.Volume, 10)),
			Timestamp: raw.Timestamp,
			Trades:    json.Number(strconv.FormatUint(uint64(raw.Trades), 10)),
		}, source, receipt, true)
		if err != nil {
			continue
		}
		bars = append(bars, bar)
	}
	next := ""
	if payload.NextPageToken != nil {
		next = *payload.NextPageToken
	}
	return bars, next, nil
}
