# QuanTRAM

Quantitative Trading Adaptive Model. Increment 1 is the Go data-ingestion layer: Alpaca IEX (or test) WebSocket, REST gap-fill, optional CSV replay, and gRPC bar/health APIs.

Do not commit Alpaca keys. Use environment variables. Rotate any key that appeared in local notes or PDFs.

## Generate, test, build

```powershell
buf lint
buf generate
go test ./...
go build ./cmd/quantram-server ./cmd/quantram-ingest-client
```

## CSV validation (anytime)

```powershell
$env:QUANTRAM_SOURCE="csv"
$env:QUANTRAM_CSV_PATH="AAPL_1min_firstratedata.csv"
$env:QUANTRAM_SYMBOLS="AAPL"
go run ./cmd/quantram-server
```

In another terminal:

```powershell
go run ./cmd/quantram-ingest-client -operation stream -symbols AAPL -max-bars 5
go run ./cmd/quantram-ingest-client -operation health
go run ./cmd/quantram-ingest-client -operation window -symbols AAPL
go run ./cmd/quantram-ingest-client -operation decisions -symbols AAPL -max-bars 5
```

`StreamDecisions` requires `QUANTRAM_MODEL=adaptive`. Default is `off` (`FailedPrecondition`).

`StreamPriceEvents` requires `QUANTRAM_PRICING=expm` **and** `QUANTRAM_MODEL=adaptive`. Default pricing is `off` (`FailedPrecondition`). Unknown `QUANTRAM_PRICING` fails startup. Price Engine color needs 45 consecutive accepted eligible minutes after a cold start. A host-gate `INPUT_GAP` is a missing adjacent minute (not silence); restart the server to resume that symbol.

## Alpaca test feed (outside regular hours)

```powershell
$env:QUANTRAM_SOURCE="alpaca"
$env:QUANTRAM_FEED="test"
$env:QUANTRAM_SYMBOLS="FAKEPACA"
$env:ALPACA_API_KEY="..."
$env:ALPACA_API_SECRET="..."
go run ./cmd/quantram-server
```

```powershell
go run ./cmd/quantram-ingest-client -operation source
go run ./cmd/quantram-ingest-client -operation stream -symbols FAKEPACA -max-bars 3 -timeout 4m
```

## Alpaca IEX (regular hours, Basic plan)

```powershell
$env:QUANTRAM_SOURCE="alpaca"
$env:QUANTRAM_FEED="iex"
$env:QUANTRAM_SYMBOLS="AAPL"
$env:ALPACA_API_KEY="..."
$env:ALPACA_API_SECRET="..."
go run ./cmd/quantram-server
```

Paper trading is not started in this increment.
