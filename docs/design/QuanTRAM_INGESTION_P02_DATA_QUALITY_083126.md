# QuanTRAM Increment 1 continuation — P-02 Data Quality

**Date:** August 31, 2026  
**Status:** Implemented and validated on live Alpaca IEX during regular hours  
**Scope:** P-02 data-quality gate on the existing increment-1 ingestion path (P-01 Alpaca market feed + P-02 pipeline). CSV replay was not the target of this slice.  
**Parent:** [QuanTRAM Process Model](QuanTRAM_PROCESS_MODEL_082926.md)  
**Prior increment:** [QuanTRAM Ingestion Increment 1](QuanTRAM_INGESTION_INCREMENT_1_083026.md)  
**Integrity gaps addressed in part:** DI-02 (finalization / lateness), DI-06 (quality as gating)

## 1. Why this slice exists

August 30 stood up a working ingest server (CSV, Alpaca test feed, gRPC window/stream/health). Quality fields existed on `Bar`, but they were **labels**, not a **gate**. `infer` became true as soon as the socket looked healthy. Alpaca `u` updates were discarded (first-bar-wins). A quiet IEX minute could look like a dead feed. Incomplete OHLC could be stored as `COMPLETE` with zeros.

The August 31 work makes a bar eligible for the next pipeline component (P-03 Adaptive Model Host) without requiring durable storage of every bar.

## 2. Decisions recorded today

| Decision | Outcome |
| :--- | :--- |
| Persistence of every bar | **Not required.** The live gate is the in-memory 64-bar window plus `infer`. |
| “Hardening” in the 31 Aug code spec | Broader than Data Quality (lossy queues, health UX, persistence). Today’s work is **P-02 Data Quality**, not that whole list. |
| Next consumer | P-03. `infer=true` is the contract that a bar is ready to consume. `observe` / `ready` remain the dashboard path. |
| Bad or incomplete Alpaca bars | Do **not** invent values. Skip the message and wait for the next complete, consistent bar. |
| Rolling window | 64 one-minute bars per symbol is the current trailing snapshot. That is acceptable for model input. |
| Full circuit breaker / Databento | Still deferred. |

## 3. Consumer contract (what “bar ready” means)

`OperationsService.GetReadiness` now distinguishes observation from inference:

| Flag | Meaning after this slice |
| :--- | :--- |
| `ready` | Same as `observe` (feed is up enough to watch). Dashboard-compatible. |
| `observe` | Feed state is healthy, degraded, or recovering. |
| `infer` | **Data-quality gate.** P-03 may consume finalized complete bars. |
| `message` | When observable but not inferable: `observable; inference gated on data quality`. |

`infer` is true only when **all** of the following hold (`internal/domain/quality.go`):

1. The shared feed breaker is `HEALTHY`.
2. Every configured symbol has **two adjacent** finalized 1-minute bars (no skipped minute between the last two finals).
3. The latest finalized bar is **model-eligible**: `is_final=true`, `quality_status=COMPLETE`, `is_backfilled=false`.
4. For live Alpaca (`ALPACA_IEX` or `ALPACA_TEST`), that last bar’s `interval_end` is within **90 seconds** of now (`MaxFinalLateness`). CSV replay does not use wall-clock freshness.

A forming `u` for the next minute does **not** disable `infer` on the last closed complete bar. A reconstructed REST bar may sit in the window to prove continuity, but a reconstructed **head** cannot turn `infer` on. Gap-fill after reconnect leaves `infer` false until live complete bars resume.

If a finalized-only subscriber overflows its depth-2 queue, `infer` is cleared. The dashboard observe path remains lossy (depth 16) and does not drive the gate.

## 4. Window and update policy (DI-02)

The per-symbol window is still 64 bars. Behavior changed from first-bar-wins to **last-write-wins** with precedence (`internal/ingestion/window.go`):

| Incoming vs stored (same symbol + interval start) | Result |
| :--- | :--- |
| Later `u` after `u` | Replace (forming minute updates). |
| `b` after `u` | Replace. Minute is finalized. |
| `u` after `b` | Reject. A closed bar is not un-finalized. |
| REST reconstructed after live complete | Reject. Live complete wins. |
| Identical snapshot / identical OHLCV+flags | No-op (dedup). |
| New interval, out of arrival order | Insert in **chronological** `interval_start` order, then evict the oldest if over 64. |

Alpaca classification (`classifyAlpacaBar`):

| Provider message | `is_final` | `quality_status` |
| :--- | :--- | :--- |
| `T=u` | false | `PARTIAL` |
| `T=b` (live) | true | `COMPLETE` |
| REST historical (`is_backfilled=true`) | true | `RECONSTRUCTED` |

Default `StreamBars` still fans out partials for observation. `StreamBarsRequest.finalized_only=true` (proto field 3) and `Pipeline.SubscribeFinalized` deliver only `is_final` bars. The ingest client flag is `-finalized-only`.

The window is **64 accepted bars**, not 64 wall-clock minutes with holes invented. After a clean IEX stretch it is the current trailing hour. A skipped bad bar or a reconnect gap means fewer complete calendar minutes in that slot. Process restart starts empty; REST gap-fill may inject reconstructed recent minutes.

## 5. Incomplete or unreadable Alpaca bars

Market-feed policy: **skip, do not repair.** The next complete accurate bar is the input. `QUALITY_STATUS_INVALID` remains unused; invalid live messages never enter the window.

A WebSocket or REST bar is rejected when:

- symbol or timestamp is missing, or the timestamp cannot be parsed
- `o`, `h`, `l`, `c`, or `v` is missing or unreadable (live JSON path)
- any OHLC price is `<= 0` or NaN
- OHLC is internally inconsistent (`high < low`, `high` below open/close, `low` above open/close)
- volume cannot be parsed as a non-negative integer

Volume `0` is allowed (quiet minute). Trade count `n` is optional.

On the WebSocket, rejection logs `skip alpaca bar: …` and the session continues. REST gap-fill skips the row without logging. A skipped closed minute is a continuity hole, so `infer` stays false until two adjacent complete bars exist again. CSV decode was not changed.

Unreadable non-array WebSocket payloads are ignored. An Alpaca stream `error` control still ends the session so reconnect can run.

## 6. Feed liveness vs bar cadence

August 30 already stated that a quiet minute is not a dead socket. Two implementation bugs still treated quiet IEX as failure:

1. **Read deadline 45 s** was armed once per `ReadMessage`. Control pongs do not complete that call, so a 1-minute bar gap timed out and the client re-subscribed. Idle deadline is now **90 s** (`StreamReadIdle`), reset on each read and in the pong handler.
2. **Breaker “stale” at 3 s** used `LastMessage` age (`3 × HeartbeatInterval`). A connected feed with no JSON bar was marked `FAILED`. That check is removed. Breaker failure is ping/write misses or pong RTT above 1500 ms only.

The server has **no N-hour ingest timer**. `.\scripts\Start-QuantramIngestion.ps1 -Feed iex -Symbols AAPL` runs until Ctrl+C. `-Timeout` / `-MaxBars` apply to the client / `-SmokeTest` only. IEX publishes 1-minute bars during regular hours; after the close the process can stay up without a new bar every minute. The trailing window remains the last 64 accepted bars.

## 7. Validation (August 31, 2026, RTH)

`go test ./...` passed after the quality, window, breaker, and decode changes.

### 7.1 First live IEX pull (before the quality gate)

Used to prove the market was open and increment 1 could receive changing IEX data. Two consecutive AAPL minutes:

| `source_timestamp` (UTC) | O / H / L / C | Volume |
| :--- | :--- | ---: |
| `2026-08-31T16:52:00Z` | 313.64 / 313.715 / 313.62 / 313.645 | 914 |
| `2026-08-31T16:53:00Z` | 313.635 / 313.90 / 313.565 / 313.795 | 4920 |

Source `ALPACA_IEX`, `final=true`, `quality=COMPLETE`. The session **re-subscribed twice** (~45 s), which motivated the idle-deadline fix. `GetReadiness` immediately after connect was `infer=false` because no bar had been accepted yet.

### 7.2 Live IEX after the quality gate

Before any closed bar: `ready=true observe=true infer=false`, message `observable; inference gated on data quality`.

Then two adjacent finalized complete AAPL minutes (`-finalized-only`):

| `source_timestamp` (UTC) | O / H / L / C | Volume | Snapshot prefix |
| :--- | :--- | ---: | :--- |
| `2026-08-31T17:00:00Z` | 313.78 / 313.78 / 313.70 / 313.70 | 1938 | `84b9dcffc384` |
| `2026-08-31T17:01:00Z` | 313.685 / 313.685 / 313.485 / 313.565 | 2748 | `b1203a94e57e` |

After those two bars: `infer=true`, message `HEALTHY`. Server log showed **one** `alpaca subscribed` and no 45-second re-subscribe during the wait.

REST gap-fill after a **real** IEX disconnect is still only unit-tested against a fake HTTP server.

## 8. Code and contract changes

| Area | Change |
| :--- | :--- |
| `internal/domain/quality.go` | Model eligibility, 90 s live watermark, contiguous finalized check, `InferReady`. |
| `internal/ingestion/window.go` | Last-write-wins, chronological insert, live-complete vs reconstructed precedence. |
| `internal/ingestion/pipeline.go` | Per-window `infer` refresh; `SubscribeFinalized`; drop of finalized consumer bars clears `infer`. |
| `internal/ingestion/breaker.go` | Removed 3-second last-message stale trip. |
| `internal/marketfeed/decode.go` | `u` → `PARTIAL`; required OHLC/volume fields; `validateOHLC`. |
| `internal/marketfeed/alpaca_ws.go` | 90 s read idle; pong resets deadline. |
| `api/proto/quantram/v1/quantram.proto` | `StreamBarsRequest.finalized_only`. Regenerated `gen/quantram/v1`. |
| `internal/server/server.go` | Catch-up and live stream honor `finalized_only`. |
| `cmd/quantram-ingest-client` | `-finalized-only`. |
| Tests | Domain quality, window replace/order, pipeline infer gating, decode skip cases, REST reconstructed, breaker quiet interval. |

## 9. Explicitly not done (unchanged deferrals)

- P-03 model host, P-04 inference, risk, paper/live orders, ledger
- Durable bar history or restart recovery of the window
- Local tick aggregation (DI-01)
- Session / calendar / halt semantics (DI-04)
- Full circuit breaker, Databento, mid-bar source reconciliation (DI-03)
- Using `QUALITY_STATUS_INVALID` as a stored bar state (rejected Alpaca messages never enter the window)
- In-process non-lossy P-03 subscription distinct from gRPC (only `SubscribeFinalized` depth 2 exists as a hook)

## 10. How to run the live path

Credentials in the session: `ALPACA_API_KEY` and `ALPACA_API_SECRET` (or `ALPACA_SECRET_KEY`).

```powershell
.\scripts\Start-QuantramIngestion.ps1 -Feed iex -Symbols AAPL
```

In another terminal:

```powershell
go run ./cmd/quantram-ingest-client -operation ready
go run ./cmd/quantram-ingest-client -operation stream -symbols AAPL -max-bars 0 -timeout 6h -finalized-only
```

Default script feed is still `test` / `FAKEPACA`. Use `-Feed iex` for regular-hours U.S. equities.

## 11. Remaining for S1 after this slice

- Live reconnect plus REST gap-fill **proof** on a real IEX drop (helper exists; not demonstrated today).
- DI-01 canonical bars from ticks, if ever required instead of provider aggregates.
- Wire P-03 to `infer` + finalized window as the next increment.

## 12. Change log

| Date | Change |
| :--- | :--- |
| August 31, 2026 | P-02 quality gate: last-write-wins, `infer` from contiguous complete live bars, skip incomplete Alpaca OHLC, IEX quiet-minute reconnect fix, live IEX RTH validation. |
