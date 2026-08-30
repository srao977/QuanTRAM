package config

import (
	"testing"
)

func TestSplitSymbolsAndCap(t *testing.T) {
	symbols := splitSymbols("aapl, AAPL, nvda")
	if len(symbols) != 2 || symbols[0] != "AAPL" || symbols[1] != "NVDA" {
		t.Fatalf("%v", symbols)
	}
}

func TestLoadRequiresCredentialsForAlpaca(t *testing.T) {
	t.Setenv("QUANTRAM_SOURCE", "alpaca")
	t.Setenv("ALPACA_API_KEY", "")
	t.Setenv("ALPACA_API_SECRET", "")
	t.Setenv("ALPACA_SECRET_KEY", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing credential error")
	}
}

func TestLoadCSVDoesNotRequireKeys(t *testing.T) {
	t.Setenv("QUANTRAM_SOURCE", "csv")
	t.Setenv("QUANTRAM_SYMBOLS", "AAPL")
	t.Setenv("ALPACA_API_KEY", "")
	t.Setenv("ALPACA_API_SECRET", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Source != "csv" || cfg.Symbols[0] != "AAPL" {
		t.Fatalf("%+v", cfg)
	}
}
