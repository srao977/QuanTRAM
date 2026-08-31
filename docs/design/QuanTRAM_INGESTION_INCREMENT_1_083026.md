# QuanTRAM Increment 1 — Data Ingestion

**Date:** August 30, 2026  
**Status:** Implemented and locally validated (CSV + Alpaca test feed). Live IEX RTH and P-02 quality gating are recorded in [P-02 Data Quality (August 31)](QuanTRAM_INGESTION_P02_DATA_QUALITY_083126.md). REST gap-fill against a live disconnect is still open.  
**Scope:** P-01 Market Feed and P-02 Ingestion only  
**Parent:** [QuanTRAM Process Model](QuanTRAM_PROCESS_MODEL_082926.md)  
**Sequence:** Process-model steps **S0** (ingestion services only) and **S1** (partial)

## 1. Increment goal

Stand up a Go gRPC ingestion server that connects to Alpaca on the Basic plan, emits a normalized 1-minute bar stream, reconnects with gap fill, and can be validated without the model, risk, or paper-order path.

Out of scope: Databento, SADE_Go, OMS, execution, ledger, benchmark, and the paper-trading sample in `dataIngestion_and_paper_trading.md`.

## 2. Basic-plan constraints (from local Alpaca notes)

| Constraint | Increment 1 behavior |
| :--- | :--- |
| One WebSocket connection | Single Alpaca session in P-01 |
| IEX realtime | Default stream `wss://stream.data.alpaca.markets/v2/iex` |
| Explicit symbol list | Subscribe only configured symbols |
| 30-symbol cap | Reject subscribe lists longer than 30 |
| Paper keys | Same key pair authenticates data and (later) paper trading; this increment uses them only for market data |

Credentials come from `ALPACA_API_KEY` and `ALPACA_API_SECRET` or `ALPACA_SECRET_KEY`. They are never logged, never placed in proto messages, and never committed.

Alpaca also exposes `wss://stream.data.alpaca.markets/v2/test` with symbol `FAKEPACA`, which streams outside regular hours. That feed is the weekday-off / Sunday validation path.

## 3. What this increment owns

**P-01** — Alpaca IEX (or test) WebSocket, Alpaca REST historical bars, CSV replay adapter, credential provider, provider decode.

**P-02** — Heartbeat evaluation, circuit-breaker state (no failover source yet), last-bar tracking, REST gap fill, dedup, bounded bar window, gRPC bar stream and health queries.

Provider-supplied 1-minute bars (`T=b`) are the increment-1 live input. Local tick aggregation is deferred (DI-01 remains open). Bars still carry QuanTRAM quality and receipt metadata.

## 4. Runtime topology

```text
Alpaca IEX/test WS ──┐
Alpaca REST bars ────┤   P-01 adapters
CSV replay ──────────┘
         │  domain.Bar (not proto)
         ▼
   P-02 pipeline  (breaker, window, gap fill, fan-out)
         │
         ▼
   gRPC  :50051
     MarketFeedService
     IngestionService
     OperationsService
```

Collocated in `cmd/quantram-server`. Domain packages do not import generated proto. The gRPC adapter in `internal/server` maps at the edge, same rule as SDX.

## 5. Contracts

`api/proto/quantram/v1/quantram.proto` (`quantram.v1`) defines only the increment-1 services. Later increments append to this file.

| Service | RPC | Role |
| :--- | :--- | :--- |
| `MarketFeedService` | `GetFeedHealth`, `GetActiveSource` | Northbound feed state |
| `IngestionService` | `StreamBars`, `GetBarWindow`, `TriggerGapFill` | Bar stream and recovery |
| `OperationsService` | `GetHealth`, `GetReadiness` | Process liveness / ready to observe |

`source_timestamp` is forwarded verbatim. QuanTRAM does not derive session or model time from it. `receipt_unix_ms` is local receive time.

## 6. Resilience (thin reconnect only)

Increment 1 keeps a **minimal reconnect path**. The architecture circuit breaker is **not completed** and is **deferred until much later** (after IEX E2E and the model/paper slice).

What exists now:

- WebSocket ping every 1000 ms. A measured pong RTT above 1500 ms marks the feed failed. Missing *data* is not a disconnect.
- Read deadline 45 s so a quiet minute does not tear down the session.
- Exponential reconnect backoff, 100 ms × 2ⁿ, cap 30 s. Handshake: `connected` → auth → `authenticated` → subscribe.
- REST gap-fill helper and in-window dedup. Not proven on a real IEX drop.

What is explicitly **not done** (deferred):

- Failover to Databento or any second live source
- Production heartbeat / completeness / error-rate trip rules from the parent spec
- DI-03 source-transition and mid-bar reconciliation
- Treating current `CircuitBreaker` state as a production guardrail

Until that work is scheduled, a failed Alpaca session means reconnect-or-degrade on the same source, not a circuit trip to a backup feed.

## 7. Validation

| Path | Result | Date |
| :--- | :--- | :--- |
| `go test ./...` | Pass (decode, breaker, gap-fill dedup, CSV, REST httptest) | 2026-08-30 |
| CSV → gRPC | Five sample AAPL bars streamed; health `CSV` / `HEALTHY`; window catch-up works after replay ends | 2026-08-30 |
| Alpaca `v2/test` + `FAKEPACA` | Auth and subscribe healthy; received bar `2026-08-30T16:50:00Z` OHLCV 132.65 / 136 / 132.12 / 134.65 / 205 | 2026-08-30 |
| Alpaca IEX `AAPL` | Not run (Sunday / outside RTH). Closed on 2026-08-31; see the P-02 data-quality note. | — |
| REST gap-fill after a live disconnect | Unit-tested against a fake HTTP server; not yet proven on a real IEX drop | — |

Paper orders are not part of this validation. The test feed emits about one minute bar per minute; allow at least one minute per requested bar.

## 8. Implementation notes

Lessons from the first live connect:

1. **Handshake.** The first control message is `connected`, not `authenticated`. Treating `success` as login caused subscribe `402`.
2. **JSON `T` vs `t`.** Go’s `encoding/json` matches struct tags case-insensitively, so Alpaca’s type (`T`) and timestamp (`t`) collided and bars were dropped. Decode uses exact object keys.
3. **Quiet is not dead.** Do not fail the socket because no bar arrived in 3 s. Heartbeat is ping/write failure or high pong RTT, not bar cadence.
4. **`StreamBars` catch-up.** A late client is sent the current window first, then live bars. Required for CSV (finite replay) and for joining an already-running Alpaca session.
5. **Credentials.** `ALPACA_API_KEY` plus `ALPACA_API_SECRET` or `ALPACA_SECRET_KEY`. Values are trimmed of whitespace and quotes. Never log or commit them. Rotate any key that appeared in `Alpaca Usage.pdf`.

## 9. Remaining for S1 (near term)

- IEX validation during regular hours — **done 2026-08-31**; see [P-02 Data Quality](QuanTRAM_INGESTION_P02_DATA_QUALITY_083126.md).
- Live reconnect + REST gap-fill proof on IEX still open.
- Local tick aggregation still deferred (DI-01).
- No model, risk, or paper-order wiring.

## 9.1 Deferred (much later — not S1 exit)

| Item | Status |
| :--- | :--- |
| Full circuit breaker (thresholds, failover, failback) | **Not completed — deferred** |
| Databento adapter | Deferred with S6 |
| Live reconnect + REST gap-fill proof on IEX | Deferred with breaker hardening |
| DI-03 mid-bar / overlap reconciliation | Deferred with Gate A |

## 10. Layout

```text
cmd/quantram-server
cmd/quantram-ingest-client
internal/config
internal/domain
internal/marketfeed
internal/ingestion
internal/server
api/proto/quantram/v1/quantram.proto
gen/quantram/v1
```

Buf remote plugins match SDX (`buf.yaml`, `buf.gen.yaml`). Run book is in the repository `README.md`.

## 11. Change log

| Date | Change |
| :--- | :--- |
| August 30, 2026 | Initial increment design. |
| August 30, 2026 | Recorded implementation: proto + Go server/client, CSV and Alpaca test-feed validation, handshake and `T`/`t` decode fixes. |
| August 30, 2026 | Marked full circuit breaker not completed and deferred until after IEX E2E and the model/paper slice. |
| August 31, 2026 | Pointed IEX RTH and P-02 quality work at `QuanTRAM_INGESTION_P02_DATA_QUALITY_083126.md`. |
