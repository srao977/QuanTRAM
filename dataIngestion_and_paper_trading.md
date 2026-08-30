# Algorithmic Trading Starter Guide: Go, WebSockets & Alpaca API

Welcome to your starter guide for building a low-latency data ingestion pipeline and automated paper trading system using **Go (Golang)** and the **Alpaca Market Data & Trading APIs**.

---

## 📑 Table of Contents
1. [Architecture Overview](#1-architecture-overview)
2. [Prerequisites & Account Setup](#2-prerequisites--account-setup)
3. [Go Environment & Dependencies](#3-go-environment--dependencies)
4. [Project Directory Layout](#4-project-directory-layout)
5. [Complete Source Code](#5-complete-source-code)
   - [Configuration (`config.go`)](#a-configuration-configgo)
   - [WebSocket Ingestion Engine (`ingestion.go`)](#b-websocket-ingestion-engine-ingestiongo)
   - [Trading Execution Engine (`trader.go`)](#c-trading-execution-engine-tradergo)
   - [Application Entry Point (`main.go`)](#d-application-entry-point-maingo)
6. [Step-by-Step Running & Testing Guide](#6-step-by-step-running--testing-guide)
7. [Key Concepts for Beginners in Go](#7-key-concepts-for-beginners-in-go)
8. [Next Steps & Enhancements](#8-next-steps--enhancements)

---

## 1. Architecture Overview

When building high-performance trading software in Go, separating concerns between **network reception**, **data processing**, and **order execution** is critical.

```
+------------------------------+
|  Alpaca Market Data Stream   | (wss://stream.data.alpaca.markets/v2/iex)
+--------------+---------------+
               |
               | (WebSocket Stream)
               v
+--------------+---------------+
|     Data Ingestion Engine    | (Goroutine 1: Listens for incoming bars/trades)
+--------------+---------------+
               |
               | (Go Channel: chan MarketBar)
               v
+--------------+---------------+
|    Strategy / Pipeline       | (Goroutine 2: Evaluates signals on incoming data)
+--------------+---------------+
               |
               | (HTTP REST Call)
               v
+--------------+---------------+
|   Alpaca Paper Trading API   | (https://paper-api.alpaca.markets)
+------------------------------+
```

### Why Go Channels and Goroutines?
* **Non-Blocking Ingestion:** The WebSocket reader goroutine stays lightweight and never blocks on slow calculations or REST calls.
* **Thread-Safe Memory:** Go channels allow goroutines to pass market data safely without relying on complex mutex locking.

---

## 2. Prerequisites & Account Setup

1. **Install Go:** Download and install Go (v1.20+) from [golang.org](https://golang.org/dl/).
2. **Alpaca API Keys:** Obtain your **Paper Trading** API Key ID and Secret Key from your Alpaca dashboard:
   - Ensure you are viewing the **Paper Trading** dashboard (`https://app.alpaca.markets/paper/dashboard`).
   - Copy the **API Key ID** and **Secret Key**.

---

## 3. Go Environment & Dependencies

This project relies on two core packages:
* `github.com/gorilla/websocket`: The standard, industry-proven WebSocket implementation for Go.
* Standard Go Library (`net/http`, `encoding/json`, `sync`, `context`).

---

## 4. Project Directory Layout

Create a folder named `alpaca-go-trader` and structure your project files as follows:

```text
alpaca-go-trader/
├── go.mod
├── config.go
├── ingestion.go
├── trader.go
└── main.go
```

To initialize your module, open a terminal in that directory and run:

```bash
go mod init alpaca-go-trader
go get github.com/gorilla/websocket
```

---

## 5. Complete Source Code

### A. Configuration (`config.go`)

This file holds credentials, endpoints, and symbol configuration.

```go
package main

import (
	"os"
)

type Config struct {
	APIKey       string
	APISecret    string
	PaperBaseURL string
	StreamURL    string
	Symbols      []string
}

func LoadConfig() Config {
	// Fallback defaults or load from Environment Variables
	apiKey := os.Getenv("ALPACA_API_KEY")
	apiSecret := os.Getenv("ALPACA_SECRET_KEY")

	if apiKey == "" {
		apiKey = "YOUR_PAPER_API_KEY_HERE"
	}
	if apiSecret == "" {
		apiSecret = "YOUR_PAPER_SECRET_KEY_HERE"
	}

	return Config{
		APIKey:       apiKey,
		APISecret:    apiSecret,
		PaperBaseURL: "https://paper-api.alpaca.markets",
		StreamURL:    "wss://stream.data.alpaca.markets/v2/iex", // Free tier WebSocket endpoint
		Symbols:      []string{"AAPL", "NVDA", "TSLA"},
	}
}
```

---

### B. WebSocket Ingestion Engine (`ingestion.go`)

This module manages the connection lifecycle, authenticates, subscribes to minute bars, and emits data onto a channel.

```go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

// MarketBar represents an incoming minute bar from Alpaca's WebSocket feed.
type MarketBar struct {
	Type      string  `json:"T"`
	Symbol    string  `json:"S"`
	Open      float64 `json:"o"`
	High      float64 `json:"h"`
	Low       float64 `json:"l"`
	Close     float64 `json:"c"`
	Volume    uint64  `json:"v"`
	Timestamp string  `json:"t"`
}

// DataIngestion handles the WebSocket lifecycle.
type DataIngestion struct {
	cfg     Config
	barChan chan<- MarketBar
}

func NewDataIngestion(cfg Config, barChan chan<- MarketBar) *DataIngestion {
	return &DataIngestion{
		cfg:     cfg,
		barChan: barChan,
	}
}

// Start connects, authenticates, subscribes, and reads incoming messages in a loop.
func (d *DataIngestion) Start() {
	headers := http.Header{}
	conn, _, err := websocket.DefaultDialer.Dial(d.cfg.StreamURL, headers)
	if err != nil {
		log.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()

	log.Println("Connected to WebSocket server.")

	// Step 1: Authenticate
	authMsg := map[string]interface{}{
		"action": "auth",
		"key":    d.cfg.APIKey,
		"secret": d.cfg.APISecret,
	}
	if err := conn.WriteJSON(authMsg); err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}

	// Read auth response
	_, responseBytes, err := conn.ReadMessage()
	if err != nil {
		log.Fatalf("Error reading auth response: %v", err)
	}
	log.Printf("Auth Response: %s", string(responseBytes))

	// Step 2: Subscribe to Minute Bars ("bars")
	subMsg := map[string]interface{}{
		"action": "subscribe",
		"bars":   d.cfg.Symbols,
	}
	if err := conn.WriteJSON(subMsg); err != nil {
		log.Fatalf("Subscription request failed: %v", err)
	}

	// Step 3: Listen Loop
	log.Printf("Subscribed to bars for: %v. Listening for live data...", d.cfg.Symbols)
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("WebSocket read error: %v", err)
			return
		}

		var rawEvents []map[string]interface{}
		if err := json.Unmarshal(msg, &rawEvents); err != nil {
			continue
		}

		for _, raw := range rawEvents {
			eventType, ok := raw["T"].(string)
			if !ok {
				continue
			}

			// If message type is "b" (Bar), parse into struct and push to channel
			if eventType == "b" {
				barData, _ := json.Marshal(raw)
				var bar MarketBar
				if err := json.Unmarshal(barData, &bar); err == nil {
					d.barChan <- bar
				}
			}
		}
	}
}
```

---

### C. Trading Execution Engine (`trader.go`)

This file submits orders via HTTP REST calls to the Alpaca Paper Trading REST endpoint.

```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type OrderRequest struct {
	Symbol      string `json:"symbol"`
	Qty         string `json:"qty"`
	Side        string `json:"side"` // "buy" or "sell"
	Type        string `json:"type"` // "market", "limit", etc.
	TimeInForce string `json:"time_in_force"` // "day", "gtc"
}

type OrderResponse struct {
	ID        string `json:"id"`
	Symbol    string `json:"symbol"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type PaperTrader struct {
	cfg        Config
	httpClient *http.Client
}

func NewPaperTrader(cfg Config) *PaperTrader {
	return &PaperTrader{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SubmitOrder sends a paper trading order request to Alpaca REST API.
func (pt *PaperTrader) SubmitOrder(symbol, qty, side string) (*OrderResponse, error) {
	url := fmt.Sprintf("%s/v2/orders", pt.cfg.PaperBaseURL)

	reqBody := OrderRequest{
		Symbol:      symbol,
		Qty:         qty,
		Side:        side,
		Type:        "market",
		TimeInForce: "day",
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling order request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Add required authentication headers
	req.Header.Set("APCA-API-KEY-ID", pt.cfg.APIKey)
	req.Header.Set("APCA-API-SECRET-KEY", pt.cfg.APISecret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := pt.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error executing request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("alpaca API error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var orderResp OrderResponse
	if err := json.Unmarshal(respBytes, &orderResp); err != nil {
		return nil, fmt.Errorf("error unmarshaling order response: %w", err)
	}

	return &orderResp, nil
}
```

---

### D. Application Entry Point (`main.go`)

`main.go` ties everything together by initializing channels, launching the goroutines, processing bars, and invoking sample trades.

```go
package main

import (
	"fmt"
	"log"
)

func main() {
	cfg := LoadConfig()

	log.Println("Starting Alpaca Go Data Ingestion & Paper Trader Engine...")

	// Create a buffered channel to communicate between ingestion & processing logic
	barChannel := make(chan MarketBar, 100)

	// Initialize components
	ingestion := NewDataIngestion(cfg, barChannel)
	trader := NewPaperTrader(cfg)

	// Launch Data Ingestion in a separate Goroutine
	go ingestion.Start()

	// Sample threshold counter to demonstrate auto-trading
	barCounts := make(map[string]int)

	log.Println("Listening for market events...")

	// Main execution loop: Consumes messages from barChannel
	for bar := range barChannel {
		fmt.Printf("[MARKET BAR] Symbol: %-5s | Close: $%8.2f | Vol: %-6d | Time: %s
",
			bar.Symbol, bar.Close, bar.Volume, bar.Timestamp)

		barCounts[bar.Symbol]++

		// Example Strategy Trigger: Place a test paper trade every 5 bars received for a symbol
		if barCounts[bar.Symbol]%5 == 0 {
			log.Printf("--> Strategy Signal: Triggering paper order for %s", bar.Symbol)
			
			order, err := trader.SubmitOrder(bar.Symbol, "1", "buy")
			if err != nil {
				log.Printf("❌ Order Placement Failed: %v", err)
			} else {
				log.Printf("✅ Order Placed Successfully! Order ID: %s | Status: %s", order.ID, order.Status)
			}
		}
	}
}
```

---

## 6. Step-by-Step Running & Testing Guide

### Option 1: Passing Environment Variables (Recommended)
Set your keys in your terminal before running:

**On Linux/macOS:**
```bash
export ALPACA_API_KEY="PKXXXXXXXXXXXXXXXX"
export ALPACA_SECRET_KEY="YOUR_SECRET_KEY_HERE"
go run .
```

**On Windows (PowerShell):**
```powershell
$env:ALPACA_API_KEY="PKXXXXXXXXXXXXXXXX"
$env:ALPACA_SECRET_KEY="YOUR_SECRET_KEY_HERE"
go run .
```

### Option 2: Hardcoding Keys for Local Testing
Edit `config.go` directly and replace `"YOUR_PAPER_API_KEY_HERE"` and `"YOUR_PAPER_SECRET_KEY_HERE"` with your credentials, then execute:

```bash
go run .
```

---

## 7. Key Concepts for Beginners in Go

1. **Goroutines (`go ingestion.Start()`):**
   Goroutines are lightweight threads managed by the Go runtime. Running `ingestion.Start()` in a goroutine keeps the WebSocket connection open and reading without freezing your main program logic.

2. **Channels (`make(chan MarketBar, 100)`):**
   Channels are Go’s native mechanism for communicating between goroutines safely. The ingestion engine pushes bars *into* `barChan <- bar`, while the main function reads them out *from* `for bar := range barChannel`.

3. **Struct Tags (`json:"S"`):**
   Alpaca returns compact JSON keys like `S` for Symbol and `c` for Close price. Struct tags tell Go how to map those JSON fields directly into readable struct variables.

4. **Zero-Dependency REST Calling:**
   Using `net/http` to send JSON payloads ensures your code stays fast, light, and easy to debug without requiring third-party SDK abstractions.

---

## 8. Next Steps & Enhancements

* **Moving Averages & Indicators:** Build a state struct in Go to maintain a rolling window of recent close prices to calculate Simple Moving Averages (SMA) or RSI.
* **Stream Handling Resilience:** Add auto-reconnect logic to `ingestion.go` using a reconnect loop with exponential backoff if the WebSocket loses connectivity.
* **Stop-Loss & Take-Profit:** Modify `OrderRequest` in `trader.go` to support bracket orders (`order_class: "bracket"`) to manage risk automatically on Alpaca's servers.
