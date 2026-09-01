# QuanTRAM P-03 — Implementation Increment

**Date:** August 31, 2026  
**Last updated:** September 1, 2026  
**Status:** Phases A–E complete in-process (Go adaptive box + host + `ModelService.StreamDecisions`). Default `QUANTRAM_MODEL=off`. Live IEX DecisionEvents observed 1 Sep (HOLD then BUY on IEX; Adaptive Pipeline viewer in `quantram-dashboard`). EXPM / paper orders remain out of scope.  
**Design:** [P-03 Adaptive Model Host](QuanTRAM_P03_ADAPTIVE_MODEL_HOST_083126.md)  
**Review:** [P03_feedback_083126.md](../../P03_feedback_083126.md)  
**Python reference (do not import at runtime):** `C:\Users\chino\SADE`

## 1. Increment goal

Add a collocated Go adaptive engine that:

1. Subscribes to P-02 finalized bars.
2. Steps SADE D01 → D02 → D04 → emitter mathematics in Go, one symbol at a time.
3. Emits one `DecisionEvent` per considered bar (decision or typed skip).
4. Proves numerical agreement with SADE Unit Run 001 **without SDX**.

Out of scope **for this coding increment:** risk, orders, Go EXPM / F4 / PriceEngine, joining adaptive output to PriceEngine, Python `Predict` worker. Adaptive Pipeline decision UI lives in `quantram-dashboard` (landed 1 Sep); this repo does not host frontend.

Those pricing items are the **next scientific increment**, not a discarded design. **Go production destination:** PriceEngine decisions on EXPM trajectories (`time_term == false` only). Offline Gold Nugget oracle is SADE `sade/pricing_pipeline/projection.py::solve_cover_rk45_reference` (frozen copy of APTF `diagnostics/run_test_013b_qqq_validation.py::solve_cover`). Do not port RK45, do not wire it, and do not make QuanTRAM depend on the APTF repo. See the design doc §2.1.

## 2. Package layout

Stay in this repo. Domain packages do not import `gen/`.

```text
internal/adaptive/
  config.go          D01/D02/D04 constants from sade/d01/v02/config.py
  observation.go     NormalizedObservation
  state.go           RuntimeState, HalfLifeState
  d01.go             D01V02Model.step and helpers
  kinematics.go
  reference.go
  volume.go
  coherence.go
  strength.go
  persistence.go
  uncertainty.go
  reversal.go
  perturbation.go
  half_life.go
  adaptation.go
  forward.go
  health.go
  d02.go             build_return_shape
  d04.go             CapturabilityModelV0_2
  engine.go          AdaptiveEngine (emitter + 15-context + decide)
  fingerprints.go    SADE baseline rule/code hashes
  mapper.go          domain.Bar → Observation (live + offline)

internal/adaptive/*_test.go
  Equivalence tests that load testdata bars, not gRPC.
  testdata/unit_run_001_observations.csv (+ origin + SHA-256)

internal/modelhost/          (Phase D — done 1 Sep)
  host.go            keyed per-symbol workers, SubscribeModelBars, cold start
  host_test.go       overflow, gap, timeout atomicity, infer pause, panic isolation

internal/domain/decision.go
  DecisionEvent, Decision, Skip, SkipReason  (domain freeze; proto on ModelService)

internal/server/
  GetHealth model component; StreamDecisions maps domain → proto oneof

cmd/quantram-server/main.go
  Construct host after pipeline; register ModelService; Run host when adaptive
```

**Present (1 Sep):** `internal/domain/decision.go`, `internal/adaptive/`, `SubscribeModelBars`, `internal/modelhost/`, `QUANTRAM_MODEL`, `ModelService.StreamDecisions`. Adaptive Pipeline viewer in `quantram-dashboard` (own proto copy + `ModelService` client). **Not present:** `Evaluate` / `ModelInferenceService`, EXPM.

Do **not** create `internal/sade` or a Go module that imports the Python tree. Copy constants and equations; cite SADE paths in comments sparingly.

Suggested config flags (startup env, same style as increment 1):

| Variable | Default | Meaning |
| :--- | :--- | :--- |
| `QUANTRAM_MODEL` | `off` | `off` or `adaptive`. Unknown values: **fail startup**. |
| `QUANTRAM_MODEL_DEADLINE` | `200ms` | Reject `<= 0` or unreasonably large (e.g. `> 2s`). |

When `adaptive` is requested but fingerprints/config fail: model component **unavailable**, do **not** silently fall back to `off`. Ingestion stays up. When `off`, do not subscribe the model path.

## 3. Port order (science first, host second)

Implement against **unit tests that call the engine with `domain.Bar`**, then wire the host.

### Phase A — Observation mapper and config — **done (31 Aug)**

- `mapper.go`: Bar → Observation using design §5.3. **`event_time` = `IntervalStart`.**
- Offline helper: CSV row `{timestamp,open,high,low,close,volume}` → `domain.Bar` (final, COMPLETE) for tests. This is a **test adapter**, not a revival of SDX.
- `D01V02Config` defaults and SADE baseline fingerprints copied verbatim (`config.go`, `fingerprints.go`). Config SHA-256 matches CPython `D01V02Config().sha256()`.
- `internal/domain/decision.go`: `DecisionEvent` / `Decision` / `Skip` / `SkipReason` (no proto).

**Exit:** mapper tests green (`event_time` is `IntervalStart`, not `SourceTimestamp`).

### Phase B — D01 — **done (31 Aug)**

Ported `sade/d01/v02/` function-for-function. Clamp bounds, `epsilon = 1e-8`, and `D01V02Model.step` operation order preserved. `Step` is copy-compute-commit (failed causal / non-finite steps do not mutate committed state).

CI uses checked-in Unit Run 001 expectations (Phase C). No Python subprocess and no `SADE_PYTHON` dependency.

**Exit:** D01 unit tests plus the 100-bar frozen trajectory (Phase C).

### Phase C — D02 + D04 + engine — **done (31 Aug)**

- `build_return_shape` including 8 samples and `**1.8` / `2^(−τ/hl)`.
- Capturability: `Pow(x, 1.0/3.0)` for structural quality (**not** `math.Cbrt`).
- Engine: 15-deque, INITIALIZING vs ACTIONABLE, `_decide` and `_adaptive_properties` from `emitter.py`.
- `emitter_position_state`: BUY→LONG, SELL→SHORT, HOLD keeps prior.
- `Step` is copy-compute-commit. Validate outputs; non-finite → skip / no commit.
- Domain types in `internal/domain/decision.go` (`DecisionEvent`) **before** proto.

**Exit (met):** Frozen Unit Run 001 compare. Fixture: `internal/adaptive/testdata/unit_run_001_observations.csv` reconstructed from `SADE/output/unit_runs/001/observations.csv`. SHA-256 `6c98e15df41f71d4369c22d4211f3fd651eda829a5046371faa38c426381f33a`. Origin note in `testdata/unit_run_001.origin.txt`.

Result: 100 AAPL bars → 15 `INITIALIZING`, 8 BUY / 10 SELL / 67 HOLD. Exact `model_status`, `path_direction`, `side`, `H`; scores within the Phase C tolerance table (`1e-12` abs or 16 ulp). First-divergence report on mismatch. `go test ./...` green. `go test -race` not run on this workstation (cgo unavailable).

### Phase D0 — P-02 model-consumer path — **done (1 Sep)**

Do **not** call `SubscribeFinalized(2)` for P-03. `FinalizedBarConsumer` / `SubscribeModelBars` is in `internal/ingestion/model_path.go`:

- finalized-only; no silent drop-oldest
- mailbox overflow → `QUEUE_OVERFLOW`; non-adjacent `IntervalStart` → `INPUT_GAP`; symbol stays discontinuous until `ResetModelPath`
- later bars for that symbol are not delivered (so they cannot be evaluated)
- `Subscribe` / `SubscribeFinalized` remain lossy observe hooks

**Exit (met):** stall consumer, depth-2 mailbox, three finalized minutes: in-order prefix `[N, N+1]` retained; N+2 marks `QUEUE_OVERFLOW` and is not substituted for N. Adjacent-gap test marks `INPUT_GAP` and does not deliver the jumped bar. Partials still reach observe only.

### Phase D — Model host — **done (1 Sep)**

- Keyed worker per symbol (`internal/modelhost`); mailbox depth = window (64), not 2. `SubscribeModelBars` same depth. Model path is **live eligible only** (no REST backfill).
- Cold start only (no window seed). First 15 accepted eligible minutes emit `INITIALIZING`; 16th is `ACTIONABLE`.
- Phase 0: global `infer` pauses all evaluation, does not reset state.
- Deadline: `PrepareStep` on a clone; `Commit` only if finished within `QUANTRAM_MODEL_DEADLINE` (default 200 ms). Timeout / panic / skip leave committed scientific state unchanged.
- Health: design §8.2 (`off` / `cold` / `initializing` / `ready` / `paused` / `discontinuous` / `error`). `GetHealth` appends a model component. Adaptive requested but fingerprints fail → `unavailable`, not silent `off`.
- Config: unknown `QUANTRAM_MODEL` or bad deadline **fails startup**. Default remains `off` (ingestion unchanged).

**Exit tests (met):** overflow, missing interval, duplicate/regression, `infer` pause-not-reset, timeout unchanged hash, AAPL panic does not stop MSFT or P-02 observe, shutdown without send-on-closed, reconstructed head never evaluates, partial `u` never reaches host, cold-start `INITIALIZING n/16`. `go test ./...` green. `go test -race` not run on this workstation (cgo unavailable).

Live acceptance (regular hours, observed 1 Sep on IEX AAPL/MSFT/NVDA after mailbox/backfill fix): set `QUANTRAM_MODEL=adaptive`, warm-up crosses 16 accepted eligible minutes; outcomes are `ModelService.StreamDecisions` events with `model_status=ACTIONABLE` and `side` in {BUY,SELL,HOLD} **or** typed skips — assert proto fields, not log text. Dashboard Adaptive Pipeline is the operator view of the same stream.

### Phase E — Proto — **done (1 Sep)**

`quantram.proto` `ModelService.StreamDecisions` streams `DecisionEvent` with `oneof outcome { Decision, Skip }`. HOLD is a decision. Includes H, Q_G, Q_S, Q_R, path_direction, `emitter_position_state`, skip reason, hashes, versions. Host fan-out via `SubscribeEvents`. Last-per-symbol catch-up on connect (no durable history). Off → `FailedPrecondition`; unavailable → `Unavailable`.

`Evaluate` and `ModelInferenceService` wait for the pricing increment (Go EXPM + PriceEngine join).

```powershell
$env:QUANTRAM_MODEL = "adaptive"
.\scripts\Start-QuantramIngestion.ps1 -Feed iex -Symbols AAPL,MSFT,NVDA
# other terminal:
go run ./cmd/quantram-ingest-client -operation decisions -symbols AAPL -max-bars 0 -timeout 6h
```

## 4. SADE file → Go file map

Use this as the port checklist. Skip anything marked **do not port**.

| SADE path | Go target | Port? | Status |
| :--- | :--- | :--- | :--- |
| `d01/v02/observations.py` | `observation.go` | Yes | Done |
| `d01/v02/config.py` | `config.go` | Yes | Done |
| `d01/v02/state.py` | `state.go` | Yes | Done |
| `d01/v02/model.py` | `d01.go` | Yes | Done |
| `d01/v02/reference.py` … `forward.py` | matching `*.go` | Yes | Done |
| `d02/v02/builder.py`, `models.py` | `d02.go` | Yes | Done |
| `d04/envelope/capturability_model.py` | `d04.go` | Yes | Done |
| `d04/models/*` | types in `d04.go` | Yes (no pydantic) | Done |
| `adaptive_emitter/emitter.py` | `engine.go` | Yes, minus audit lists | Done |
| `adaptive_emitter/normalizer.py` | `mapper.go` | Yes, from `domain.Bar` | Done |
| `configuration/scientific_baseline.py` | `fingerprints.go` | Yes | Done |
| `adaptive_pipeline/pipeline.py` | — | **No** | — |
| `input/sdx_client.py` | — | **No** | — |
| `input/generated/**` | — | **No** | — |
| `pricing_pipeline/**` | — | **No** in P-03. Later: port EXPM `solve_cover` / `analytic_affine_trajectory` only; do **not** port `solve_cover_rk45_reference` into Go. That SADE function is the offline oracle (APTF `run_test_013b_qqq_validation.py::solve_cover` lineage). | Deferred |
| `unit_run/**`, `__main__.py` | testdata | Offline harness only | Fixture checked in |

## 5. Offline equivalence without SDX

Unit Run 001 consumed SDX `MarketVector`s from CSV. Recreate that **sequence of OHLCV**, not the gRPC hop.

**Chosen (Phase C):** option 2. Reconstruct bars from a checked-in copy of `SADE/output/unit_runs/001/observations.csv` (`source_timestamp`, OHLCV, `source_row_index`). Feed through `mapper` → `Engine.Step`. Compare to the same file’s `status`, `path_direction`, `position_decision`, `H`, `Q_G`, `Q_S`, `Q_R`, `C`, and related scores.

Do not start `sdx-server` or `python -m sade run` as part of QuanTRAM CI.

## 6. Wiring in `quantram-server`

When `QUANTRAM_MODEL=adaptive`:

```text
pipeline := ...
host := modelhost.New(pipeline, symbols, Options{Mode, Deadline})
go host.Run(ctx)
// existing gRPC serve; GetHealth includes model
```

When `off` (default): `modelhost.New` returns `(nil, nil)`; no model-path subscribe; observe APIs stay unchanged. Startup log includes `model=off|adaptive`.

P-02 **observe** APIs stay unchanged. P-03 uses `SubscribeModelBars`. Default remains `off` until a regular-hours live DecisionEvent run is accepted. To enable:

```powershell
$env:QUANTRAM_MODEL = "adaptive"
.\scripts\Start-QuantramIngestion.ps1 -Feed iex -Symbols AAPL,MSFT,NVDA
```

## 7. What not to copy from SADE (scale blockers)

The investigation is explicit. Do not reintroduce:

1. Per-observation full-history pricing refits (O(n²)).
2. Unbounded audit/trace slices in production.
3. Cross-process protobuf of every vector for a collocated engine.
4. `statistics`/`json` hashing of the entire DMO/FMO tree as a hot-path requirement.
5. A Python adaptive wrapper “until Go is ready.” The adaptive path has no library gap.

## 8. Validation plan

| Test | When | Status |
| :--- | :--- | :--- |
| Mapper + `IntervalStart` event time | Phase A | Done |
| D01/D02/D04/engine + I/O validation | Phases B–C | Done |
| Frozen 100-bar compare + SHA-256 fixtures + tolerance table | Phase C exit | Done |
| `SubscribeModelBars` overflow / gap | Phase D0 | Done (1 Sep) |
| Host: infer pause, timeout atomicity, panic isolation, cold start | Phase D | Done (1 Sep) |
| `go test ./...` | Before merge | Green (1 Sep) |
| `go test -race ./...` | Before merge | Blocked here (cgo); retry on host |
| Live IEX field-level DecisionEvents + latency | Manual; regular hours | Observed 1 Sep (`QUANTRAM_MODEL=adaptive` + `StreamDecisions`; Adaptive Pipeline in `quantram-dashboard`) |
| Config reject unknown mode / bad deadline | Phase D | Done (1 Sep) |
| `ModelService.StreamDecisions` oneof + off/unavailable | Phase E | Done (1 Sep) |

Phases A–E are green in-process. Adaptive Pipeline landed in `quantram-dashboard` (1 Sep; proto copy + `StreamDecisions` SSE). Paper orders remain forbidden. Do not start EXPM / RK45.

## 9. Suggested first coding session — **done (31 Aug)**

1. Check in Unit Run 001 fixtures (SHA-256) and `internal/domain/decision.go` (`DecisionEvent`).
2. Mapper with `event_time = IntervalStart`.
3. Port D01 `Step` as copy-compute-commit.
4. Do not call `SubscribeFinalized` or add proto until Phase C and D0 are green.

**Next session:** paper/risk/orders still forbidden. Do not start EXPM / RK45. Still do not call `SubscribeFinalized` for P-03. Candlestick BUY/SELL markers and durable decision history are dashboard follow-ups, not this repo.

## 10. Change log

| Date | Change |
| :--- | :--- |
| August 31, 2026 | Implementation increment written after SADE + P-02 review. |
| August 31, 2026 | Incorporated P-03 feedback: Phase D0 model-consumer path, DecisionEvent, transactional Step, race/overflow tests, cold start. |
| August 31, 2026 | Phases A–C implemented in-repo: `internal/domain/decision.go`, `internal/adaptive/` (D01→D02→D04→emitter), Unit Run 001 fixture + equivalence (15 / 8 / 10 / 67). Server and proto unwired. Phase D0 not started. |
| August 31, 2026 | Pricing destination amended: next increment is Go EXPM (`time_term == false`; reject `true`), validated against Python EXPM and frozen RK45. No RK45 Go port or Python RK45 wiring. |
| August 31, 2026 | Oracle path named: SADE `solve_cover_rk45_reference` (from APTF `run_test_013b_qqq_validation.py::solve_cover`). Go validation must not import or depend on the APTF repository. |
| September 1, 2026 | Phase D0: `SubscribeModelBars` / `FinalizedBarConsumer`. No silent drop-oldest; overflow→`QUEUE_OVERFLOW`, gap→`INPUT_GAP`. Observe `SubscribeFinalized` unchanged. Host (Phase D) not started. |
| September 1, 2026 | Phase D: `internal/modelhost` keyed workers, `PrepareStep`/`Commit` deadline, infer pause-not-reset, panic isolation, `QUANTRAM_MODEL` validation, `GetHealth` model component. Default still `off`. Proto deferred to Phase E. |
| September 1, 2026 | Phase E: `ModelService.StreamDecisions`, domain→proto oneof, host `SubscribeEvents` fan-out, ingest-client `-operation decisions`. Default still `off`. |
| September 1, 2026 | Live fix: model path ignores backfill; mailbox 64. Restored gap-fill/reconnect no longer permanently pauses P-03 while P-02 observe stays healthy. |
| September 1, 2026 | Live IEX DecisionEvents observed (HOLD then BUY). Adaptive Pipeline viewer landed in `quantram-dashboard` (copied proto + `ModelService` client). This repo still has no frontend. |
