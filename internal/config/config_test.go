package config

import (
	"testing"
	"time"
)

func TestSplitSymbolsAndCap(t *testing.T) {
	symbols := splitSymbols("aapl, AAPL, nvda")
	if len(symbols) != 2 || symbols[0] != "AAPL" || symbols[1] != "NVDA" {
		t.Fatalf("%v", symbols)
	}
}

func TestLoadRequiresCredentialsForAlpaca(t *testing.T) {
	t.Setenv("QUANTRAM_SOURCE", "alpaca")
	t.Setenv("QUANTRAM_PRICING", "off")
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
	t.Setenv("QUANTRAM_MODEL", "off")
	t.Setenv("QUANTRAM_PRICING", "off")
	t.Setenv("ALPACA_API_KEY", "")
	t.Setenv("ALPACA_API_SECRET", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Source != "csv" || cfg.Symbols[0] != "AAPL" {
		t.Fatalf("%+v", cfg)
	}
	if cfg.Model != ModelOff {
		t.Fatalf("default model %s", cfg.Model)
	}
	if cfg.Pricing != PricingOff {
		t.Fatalf("default pricing %s", cfg.Pricing)
	}
}

func TestLoadRejectsUnknownModel(t *testing.T) {
	t.Setenv("QUANTRAM_SOURCE", "csv")
	t.Setenv("QUANTRAM_SYMBOLS", "AAPL")
	t.Setenv("QUANTRAM_PRICING", "off")
	t.Setenv("QUANTRAM_MODEL", "sidecar")
	if _, err := Load(); err == nil {
		t.Fatal("unknown model must fail startup")
	}
}

func TestLoadRejectsBadDeadline(t *testing.T) {
	t.Setenv("QUANTRAM_SOURCE", "csv")
	t.Setenv("QUANTRAM_SYMBOLS", "AAPL")
	t.Setenv("QUANTRAM_PRICING", "off")
	t.Setenv("QUANTRAM_MODEL", "adaptive")
	t.Setenv("QUANTRAM_MODEL_DEADLINE", "0s")
	if _, err := Load(); err == nil {
		t.Fatal("zero deadline must fail")
	}
	t.Setenv("QUANTRAM_MODEL_DEADLINE", "5s")
	if _, err := Load(); err == nil {
		t.Fatal("deadline above max must fail")
	}
	t.Setenv("QUANTRAM_MODEL_DEADLINE", "200ms")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != ModelAdaptive || cfg.ModelDeadline != 200*time.Millisecond {
		t.Fatalf("%+v", cfg)
	}
}

func TestLoadRejectsUnknownPricing(t *testing.T) {
	t.Setenv("QUANTRAM_SOURCE", "csv")
	t.Setenv("QUANTRAM_SYMBOLS", "AAPL")
	t.Setenv("QUANTRAM_MODEL", "adaptive")
	t.Setenv("QUANTRAM_PRICING", "rk45")
	if _, err := Load(); err == nil {
		t.Fatal("unknown pricing must fail startup")
	}
}

func TestLoadExpmRequiresAdaptive(t *testing.T) {
	t.Setenv("QUANTRAM_SOURCE", "csv")
	t.Setenv("QUANTRAM_SYMBOLS", "AAPL")
	t.Setenv("QUANTRAM_MODEL", "off")
	t.Setenv("QUANTRAM_PRICING", "expm")
	if _, err := Load(); err == nil {
		t.Fatal("expm without adaptive must fail startup")
	}
}

func TestLoadExpmWithAdaptive(t *testing.T) {
	t.Setenv("QUANTRAM_SOURCE", "csv")
	t.Setenv("QUANTRAM_SYMBOLS", "AAPL")
	t.Setenv("QUANTRAM_MODEL", "adaptive")
	t.Setenv("QUANTRAM_PRICING", "expm")
	t.Setenv("QUANTRAM_MODEL_DEADLINE", "200ms")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Pricing != PricingExpm || cfg.Model != ModelAdaptive {
		t.Fatalf("%+v", cfg)
	}
}
