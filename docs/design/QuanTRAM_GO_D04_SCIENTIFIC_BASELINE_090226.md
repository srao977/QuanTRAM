# QuanTRAM Go D04 Scientific Baseline

**Date:** September 2, 2026  
**Frozen at:** 2026-09-02 11:07:01 -07:00  
**Status:** FROZEN PRE-INTERVAL-CONTINUITY-FIX BASELINE  

This is a **FROZEN BASELINE**, not a scientifically final system. It preserves the accepted Go adaptive path (D01 → D02 → D04 → Emitter) immediately before any interval-aware continuity change. It does **not** claim that current one-minute adjacency semantics are correct.

## Purpose

Immutable comparison point before changing runtime observation-continuity semantics.

Do **not** treat this document as a fix. The known `INPUT_GAP` / `STATE_DISCONTINUOUS` latch remains unfixed.

## 1. Repository identity

| Field | Value |
| :--- | :--- |
| Path | `C:\Users\chino\QuanTRAM` |
| Module | `quantram` |
| OS / platform | windows / amd64 |
| Go version | `go1.25.3 windows/amd64` (`C:\Program Files\Go`) |
| Gonum | `gonum.org/v1/gonum v0.17.0` |
| gRPC | `google.golang.org/grpc v1.83.2` |
| Protobuf | `google.golang.org/protobuf v1.36.12` |

## 2. Commit / hash (pre-freeze ancestor)

Working tree at freeze start sat on:

| Field | Value |
| :--- | :--- |
| Branch | `main` (tracks `origin/main`) |
| Pre-freeze HEAD | `042356aa15ee7589f4d614621826852163ad2329` (`P03 completed`) |

The freeze commit that contains this document is the baseline commit. After tag creation, `git rev-parse HEAD` and `git rev-list -n 1 <tag>` must match.

## 3. Branch

`main`

## 4. Go / toolchain versions

See §1. Race detector: `go test -race ./...` is **environment blocked** (`-race requires cgo`; `CGO_ENABLED` not set).

## 5. Current implemented pipeline

```text
Market Feed (Alpaca IEX / test / CSV)
    -> P-02 ingestion / quality / model path
    -> P-03 Model Host (keyed worker)
        -> D01 Adaptive Model
        -> Dynamic Model Outputs
        -> D02 Return Shape
        -> D04 Trading Envelope
        -> Adaptive Emitter / DecisionEvent
        -> ModelService.StreamDecisions
```

Observed live 2 Sep 2026 on IEX (AAPL / MSFT / NVDA) through `quantram-dashboard` Adaptive Pipeline. Example: AAPL accepted sequence 20 at 2:01 PM ET, Departures CLEARED (BUY / Upward / scientific LONG) while Price Engine still BOARDING (minute 20 of 45).

## 6. Scientific modules included

| Module | Status |
| :--- | :--- |
| D01 | INCLUDED (`internal/adaptive/d01.go` and helpers) |
| D02 | INCLUDED (`internal/adaptive/d02.go`) |
| D04 | INCLUDED (`internal/adaptive/d04.go`) |
| Adaptive Emitter | INCLUDED (`internal/adaptive/engine.go`) |
| Host / DecisionEvent | INCLUDED (`internal/modelhost`, `internal/domain/decision.go`) |
| P-04 Price Engine | EXISTING ONLY in this tree (already landed 2 Sep; **not modified by this freeze**). Default `QUANTRAM_PRICING=off`. |

This freeze did not begin P-04, did not change P-04 mathematics, and did not change D01/D02/D04 mathematics.

## 7. Test results

Recorded 2 Sep 2026, no code changes to obtain green.

| Command | Result |
| :--- | :--- |
| `go test ./...` | **PASS** (`adaptive`, `config`, `domain`, `ingestion`, `marketfeed`, `modelhost`, `pricing`, `server`) |
| `go test -race ./...` | **ENVIRONMENT BLOCKED** (`go: -race requires cgo; enable cgo by setting CGO_ENABLED=1`) |

## 8. Existing equivalence evidence

No new acceptance criteria were invented for this freeze.

| Test | Result | Evidence |
| :--- | :--- | :--- |
| `TestUnitRun001Equivalence` (`internal/adaptive`) | PASS | Fixture SHA-256 `6c98e15df41f71d4369c22d4211f3fd651eda829a5046371faa38c426381f33a`. Counts **15 / 8 / 10 / 67** (INITIALIZING / BUY / SELL / HOLD). Terminal emitter SHORT. |
| `TestPricingUnitRun001Equivalence` (`internal/pricing`) | PASS | Fixture SHA-256 `4b3b8783108988e71c4bf2cec9b6f8a4c6bf929fb93a4be27706a16ef4c1752a`. Semantic **15 / 30 / 55**. Manifest `internal/pricing/testdata/p04_equivalence_manifest.json`. |

## 9. Relevant runtime configuration

| Variable | Default | Live observation (2 Sep) |
| :--- | :--- | :--- |
| `QUANTRAM_MODEL` | `off` | `adaptive` |
| `QUANTRAM_PRICING` | `off` | `expm` (opt-in; not required for this D04 freeze) |
| `QUANTRAM_MODEL_DEADLINE` | `200ms` | default |
| `QUANTRAM_SOURCE` | — | `alpaca` |
| `QUANTRAM_FEED` | — | `iex` |
| `QUANTRAM_SYMBOLS` | — | `AAPL,MSFT,NVDA` |

`expm` requires `adaptive`. Unknown model or pricing values fail startup.

## 10. File / manifest fingerprints

Companion inventory (source/docs/scripts present on disk at freeze write, 107 files):  
`docs/design/quantram_go_d04_baseline_inventory_090226.sha256.txt`  
SHA-256 of that file: `28fa92f96f5e1bbefe45d6ec7ec5db0ec1f3b60fb16cc1793bc1add27e60ab11`

Principal scientific / runtime files (SHA-256 of working-tree bytes):

| File | SHA-256 |
| :--- | :--- |
| `internal/adaptive/d01.go` | `c6c055fad61e4b7a7e57c19e7f70b7400466bfb52ac88c99fd460e5f8b1d3386` |
| `internal/adaptive/d02.go` | `44148f08968d32ac2bab79d8f9ffff4c12b2584f5762cbeaf22b20e371f995d3` |
| `internal/adaptive/d04.go` | `70e993a27f072da9259ea41064deda13a7ff01f491ff5af0a905228eaccefd49` |
| `internal/adaptive/engine.go` | `ebbc327866e42cf4b20bb6163a6b7ca37e4146266f1da5c34ae82bb97d1c9419` |
| `internal/adaptive/equivalence_test.go` | `45fbc64221b5fde5b518d141170a7926c5bf1e8b23567e67746ffb9ba3df5de1` |
| `internal/adaptive/mapper.go` | `ec893645e76d9b8850016b4fdfaada6ef7db61f6597f53b5d8281ecc28a0d594` |
| `internal/adaptive/testdata/unit_run_001_observations.csv` | `6c98e15df41f71d4369c22d4211f3fd651eda829a5046371faa38c426381f33a` |
| `internal/domain/decision.go` | `74c9b21cb9ec4597b53b0f6bb531cc376a87d64a13830e5f85b64e53d7d4b786` |
| `internal/ingestion/model_path.go` | `f5de211876599fd970b59febb433aeb2e0bfdd28ae3d53cfad57e6506a6c9a09` |
| `internal/modelhost/host.go` | `dcfbfd9c6c803cbb5425cf8070193a5493ad8d5225640952d1a482a11023c291` |
| `internal/config/config.go` | `8524cec37c643c26e799671f5392fa46c44a7cd38a4ccb487e267a7216e2f0e4` |
| `api/proto/quantram/v1/quantram.proto` | `34651eced569fed9db4969c0a9627e2629b115d4f32b4de8f07d3a202a625b77` |
| `cmd/quantram-server/main.go` | `7a445f2a918d7d9a2d4c1d05a7e6f6bcbe5d2721b43ad4f18a2efd72abf8d059` |
| `go.mod` | `1ca2625ef5ee0412b2a9877e70f581d53f98cdfefe1c72ec567923857808e083` |

P-04 files are fingerprinted in the companion inventory. They exist in this baseline commit because they were already in the accepted working tree. This freeze did not edit them.

## 11. Known limitations

- This baseline is **not** a scientifically final system.
- No live `ResetModelPath` RPC. Recovery from a latched symbol is process restart (cold start: Adaptive 15 consecutive accepted eligible minutes; Price Engine color 45).
- `go test -race` not run here (cgo unavailable).
- Decision / price history is last-per-symbol only. Viewer boards clear on refresh.
- P-05 / P-06 / paper orders are not started.
- Live IEX Price Engine color was not claimed at freeze time (warm-up in progress).

## 12. Explicit continuity issue (unfixed)

**KNOWN BASELINE ISSUE:** Fixed-time adjacency may incorrectly classify legitimate non-uniform provider observation intervals as scientific discontinuity.

Observed chain (IEX, 2 Sep 2026):

```text
valid/live provider stream
    ->
non-adjacent nominal one-minute timestamp
    ->
INPUT_GAP
    ->
STATE_DISCONTINUOUS latch
    ->
subsequent bars rejected
    ->
full process restart currently required
    ->
adaptive warm-up repeats
```

Implementation (unchanged by this freeze):

| Location | Behavior |
| :--- | :--- |
| `internal/ingestion/model_path.go` `Pipeline.fanoutModel` | If `bar.IntervalStart.Sub(last) != time.Minute`, set `modelDisc[symbol] = INPUT_GAP`, log `model path INPUT_GAP`, **do not forward** later bars. |
| `internal/modelhost/host.go` `Host.handle` | If `bar.IntervalStart.Sub(w.lastAccepted) != time.Minute`, `markDisc(SkipInputGap)` and emit `INPUT_GAP`. |
| `internal/modelhost/host.go` `Host.discontinuousSkip` | After latch, later bars emit `STATE_DISCONTINUOUS` with detail `INPUT_GAP`. |
| `internal/modelhost/host.go` `Host.refreshPathStatus` | Ingestion-path disc copies into the host worker. |

Host-gate log: `model skip … STATE_DISCONTINUOUS`. Ingestion-path log: `model path skip … INPUT_GAP` (later bars never reach the host).

Silence (no new bar) does not emit a gap. A later eligible bar whose `IntervalStart` is not exactly one minute after the last accepted bar does.

**KNOWN ISSUE FIXED: NO**

## 13. Issue remains unfixed

This freeze records the behavior. It does **not** implement interval-aware continuity. It does **not** change adjacency tests that currently require exact `time.Minute`.

## 14. P-04 not changed by this freeze

P-04 Go EXPM already existed in the working tree (Phases A–I, 2 Sep). This freeze committed that existing state so the tree is reproducible. It did **not** start P-04, did not alter pricing mathematics, and did not join BUY/SELL to color.

## 15. Freeze / tag identity

Preferred annotated tag:

`quantram-go-d04-pre-interval-fix-2026-09-02`

Tag message intent:

> QuanTRAM Go D04 scientific/runtime baseline before interval-aware continuity changes. Includes accepted Go adaptive path through D01, D02, D04 and Emitter. Known issue: nominal one-minute timestamp non-adjacency may incorrectly produce INPUT_GAP / STATE_DISCONTINUOUS for legitimate irregular provider observations. This tag intentionally preserves that behavior for regression comparison.

Do not move or overwrite this tag.

## 16. Change log

| Date | Change |
| :--- | :--- |
| September 2, 2026 | Freeze document written. No science change. No interval-continuity fix. P-04 not modified. |

## Intentional exclusions

Not part of the freeze commit:

- `Alpaca Usage.pdf` (ignored)
- `bin/`, `quantram-server.exe` (local binaries)
- Alpaca credentials (environment only; never committed)
