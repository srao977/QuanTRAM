package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	DefaultGRPCPort     = "50051"
	DefaultSource       = "alpaca"
	DefaultFeed         = "iex"
	DefaultInterval     = "1Min"
	MaxSymbolsBasicPlan = 30
	IEXStreamURL        = "wss://stream.data.alpaca.markets/v2/iex"
	TestStreamURL       = "wss://stream.data.alpaca.markets/v2/test"
	DataRESTURL         = "https://data.alpaca.markets"
	HeartbeatInterval   = time.Second
	HeartbeatMaxRTT     = 1500 * time.Millisecond
	HeartbeatMaxMisses  = 3
	ReconnectBase       = 100 * time.Millisecond
	ReconnectCap        = 30 * time.Second
	StreamReadIdle      = 90 * time.Second
	WindowLimit         = 64
	SubscriberQueue     = 16
	ConsumerQueue       = 2
)

type Config struct {
	GRPCPort  string
	Source    string
	Feed      string
	StreamURL string
	DataREST  string
	APIKey    string
	APISecret string
	Symbols   []string
	CSVPath   string
	Interval  string
}

func Load() (Config, error) {
	source := strings.ToLower(environmentOrDefault("QUANTRAM_SOURCE", DefaultSource))
	feed := strings.ToLower(environmentOrDefault("QUANTRAM_FEED", DefaultFeed))
	symbols := splitSymbols(environmentOrDefault("QUANTRAM_SYMBOLS", "AAPL"))
	if feed == "test" && environmentOrDefault("QUANTRAM_SYMBOLS", "") == "" {
		symbols = []string{"FAKEPACA"}
	}
	if len(symbols) == 0 {
		return Config{}, fmt.Errorf("QUANTRAM_SYMBOLS must contain at least one symbol")
	}
	if len(symbols) > MaxSymbolsBasicPlan {
		return Config{}, fmt.Errorf("Basic plan allows at most %d symbols, got %d", MaxSymbolsBasicPlan, len(symbols))
	}

	cfg := Config{
		GRPCPort:  environmentOrDefault("GRPC_PORT", DefaultGRPCPort),
		Source:    source,
		Feed:      feed,
		StreamURL: streamURL(feed),
		DataREST:  environmentOrDefault("ALPACA_DATA_REST", DataRESTURL),
		APIKey:    trimCredential(firstEnv("ALPACA_API_KEY")),
		APISecret: trimCredential(firstEnv("ALPACA_API_SECRET", "ALPACA_SECRET_KEY")),
		Symbols:   symbols,
		CSVPath:   environmentOrDefault("QUANTRAM_CSV_PATH", "AAPL_1min_firstratedata.csv"),
		Interval:  environmentOrDefault("QUANTRAM_INTERVAL", DefaultInterval),
	}
	if source == "alpaca" && (cfg.APIKey == "" || cfg.APISecret == "") {
		return Config{}, fmt.Errorf("ALPACA_API_KEY and ALPACA_API_SECRET (or ALPACA_SECRET_KEY) are required for source=alpaca")
	}
	if source != "alpaca" && source != "csv" {
		return Config{}, fmt.Errorf("QUANTRAM_SOURCE must be alpaca or csv")
	}
	return cfg, nil
}

func streamURL(feed string) string {
	switch feed {
	case "test":
		return TestStreamURL
	default:
		return IEXStreamURL
	}
}

func splitSymbols(value string) []string {
	parts := strings.Split(value, ",")
	symbols := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		symbol := strings.ToUpper(strings.TrimSpace(part))
		if symbol == "" {
			continue
		}
		if _, duplicate := seen[symbol]; duplicate {
			continue
		}
		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}
	return symbols
}

func trimCredential(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"'`)
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}
	return ""
}

func environmentOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
