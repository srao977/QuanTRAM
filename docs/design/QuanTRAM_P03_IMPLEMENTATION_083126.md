# QuanTRAM P-03 — Implementation Increment

**Date:** August 31, 2026  
**Status:** Ready to implement. Depends on current P-02 quality gate.  
**Design:** [P-03 Adaptive Model Host](QuanTRAM_P03_ADAPTIVE_MODEL_HOST_083126.md)  
**Python reference (do not import at runtime):** `C:\Users\chino\SADE`

## 1. Increment goal

Add a collocated Go adaptive engine that:

1. Subscribes to P-02 finalized bars.
2. Steps SADE D01 → D02 → D04 → emitter mathematics in Go, one symbol at a time.
3. Emits `DecisionVector` or skip.
4. Proves numerical agreement with SADE Unit Run 001 **without SDX**.

Out of scope **for this coding increment:** risk, orders, the unfinished APTF→SADE RK45 migration, joining adaptive output to PriceEngine, Python `Predict` worker, dashboard decision UI.

Those pricing items are the **next scientific increment**, not a discarded design. The destination remains: PriceEngine decisions on RK45 trajectories. See the design doc §2.1.

## 2. Package layout (proposed)

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

internal/modelhost/
  host.go            SubscribeFinalized, infer gate, per-symbol engines, skip/deadline
  host_test.go

internal/domain/decision.go
  DecisionVector, SkipReason  (no proto)

internal/server/     (later in this increment or next)
  Map DecisionVector to proto; register ModelService

cmd/quantram-server/main.go
  Construct host after pipeline; Run host in a goroutine
```

Do **not** create `internal/sade` or a Go module that imports the Python tree. Copy constants and equations; cite SADE paths in comments sparingly.

Suggested config flags (startup env, same style as increment 1):

| Variable | Default | Meaning |
| :--- | :--- | :--- |
| `QUANTRAM_MODEL` | `off` | `off` or `adaptive`. Keep current ingest-only runs working. |
| `QUANTRAM_MODEL_DEADLINE` | `200ms` | Skip if a step exceeds this. |

## 3. Port order (science first, host second)

Implement against **unit tests that call the engine with `domain.Bar`**, then wire the host.

### Phase A — Observation mapper and config

- `mapper.go`: Bar → Observation using §5 of the design doc.
- Offline helper: CSV row `{timestamp,open,high,low,close,volume}` → `domain.Bar` (final, COMPLETE) for tests. This is a **test adapter**, not a revival of SDX.
- Copy `D01V02Config` defaults and fingerprints verbatim.

**Exit:** mapper tests; no engine yet.

### Phase B — D01

Port `sade/d01/v02/` function-for-function. Preserve clamp bounds, `epsilon = 1e-8`, and operation order in `D01V02Model.step`.

Suggested test method: for a short synthetic close/volume series, compare Go vs a thin Python subprocess **or** (preferred for CI) checked-in expected JSON from a one-off SADE script. If a subprocess is used, keep it in `testdata` / an optional `SADE_PYTHON` env so default `go test ./...` does not require SADE on PATH.

**Exit:** D01 state trajectory matches Python within documented tolerance on a ≥50-step series.

### Phase C — D02 + D04 + engine

- `build_return_shape` including 8 samples and `**1.8` / `2^(−τ/hl)`.
- Capturability: `Pow(x, 1.0/3.0)` for structural quality.
- Engine: 15-deque, INITIALIZING vs ACTIONABLE, `_decide` and `_adaptive_properties` from `emitter.py`.
- Position: BUY→LONG, SELL→SHORT, HOLD keeps state.

**Exit:** Full emission compare on Unit Run 001 sequence (100 AAPL bars). `position_decision` exact; scores within tolerance.

### Phase D — Model host

- `Host.Run(ctx)`:
  - `id, ch := pipeline.SubscribeFinalized(2)`
  - on bar: if `!Readiness().Infer` or `!bar.ModelEligible()` → skip
  - `engine := engines[bar.Symbol]` (create on first bar)
  - if `bar.IntervalStart` ≤ last → skip (duplicate/regression)
  - `decision, err := engine.Step(bar)` with deadline
  - fan-out to in-process subscribers / later proto stream
- Depth 2; on overflow skip and log (same policy as P-02 finalized consumer).
- `GetHealth` gains a `model` component: healthy / degraded / unavailable without taking down ingestion.

**Exit:** `QUANTRAM_MODEL=adaptive` + live IEX: after ~16 eligible minutes, host logs ACTIONABLE or HOLD/BUY/SELL; `infer=false` produces skips only.

### Phase E — Proto (can trail Phase D)

Append to `api/proto/quantram/v1/quantram.proto` (do not split the file):

```text
enum DecisionSide { ... UNSPECIFIED, BUY, SELL, HOLD }
enum SkipReason { ... GATE, INITIALIZING, TIMEOUT, ERROR, INFER_OFF }

message DecisionVector {
  string symbol = 1;
  string signal_id = 2;
  string decision_id = 3;
  string market_snapshot_id = 4;
  DecisionSide side = 5;
  double confidence = 6;
  string model_version = 7;
  int64 interval_start_unix_ms = 8;
  bool skipped = 9;
  SkipReason skip_reason = 10;
  double capturability = 11;
  uint32 hard_eligibility = 12;
}

service ModelService {
  rpc StreamDecisions(StreamDecisionsRequest) returns (stream DecisionVector);
  rpc GetModelInfo(GetModelInfoRequest) returns (ModelInfo);
}
```

`Evaluate` (push snapshot) can wait. `ModelInferenceService` waits for pricing/RK45.

Regenerate with `buf generate`. Dashboard may ignore the new RPCs until a later viewer increment.

## 4. SADE file → Go file map

Use this as the port checklist. Skip anything marked **do not port**.

| SADE path | Go target | Port? |
| :--- | :--- | :--- |
| `d01/v02/observations.py` | `observation.go` | Yes |
| `d01/v02/config.py` | `config.go` | Yes |
| `d01/v02/state.py` | `state.go` | Yes |
| `d01/v02/model.py` | `d01.go` | Yes |
| `d01/v02/reference.py` … `forward.py` | matching `*.go` | Yes |
| `d02/v02/builder.py`, `models.py` | `d02.go` | Yes |
| `d04/envelope/capturability_model.py` | `d04.go` | Yes |
| `d04/models/*` | types in `d04.go` or `types.go` | Yes (no pydantic) |
| `adaptive_emitter/emitter.py` | `engine.go` | Yes, minus audit lists |
| `adaptive_emitter/normalizer.py` | `mapper.go` | Yes, from `domain.Bar` |
| `configuration/scientific_baseline.py` | `fingerprints.go` | Yes |
| `adaptive_pipeline/pipeline.py` | — | **No** |
| `input/sdx_client.py` | — | **No** |
| `input/generated/**` | — | **No** |
| `pricing_pipeline/**` | — | **No** (later increment) |
| `unit_run/**`, `__main__.py` | testdata / optional cmd | Offline harness only |

## 5. Offline equivalence without SDX

Unit Run 001 consumed SDX `MarketVector`s from CSV. Recreate that **sequence of OHLCV**, not the gRPC hop.

Options (pick one in Phase C):

1. Check in `testdata/sade_unit_run_001_aapl.csv` copied from the SDX/SADE source that produced the run (header `timestamp,open,high,low,close,volume` or SADE observation columns).
2. Reconstruct bars from `SADE/output/unit_runs/001/observations.csv` (`source_timestamp`, `open`, `high`, `low`, `close`, `volume`, `source_row_index`).

Feed those bars through `mapper` → `AdaptiveEngine`. Compare to:

- checked-in expected decisions JSON generated once from Python, or
- `SADE/output/unit_runs/001/observations.csv` columns `position_decision`, `H`, `C`, `status`.

Do not start `sdx-server` or `python -m sade run` as part of QuanTRAM CI.

## 6. Wiring in `quantram-server`

When `QUANTRAM_MODEL=adaptive`:

```text
pipeline := ...
host := modelhost.New(pipeline, symbols)
go host.Run(ctx)
// existing gRPC serve
```

When `off` (default): server behavior is identical to August 31 ingestion.

P-02 APIs stay unchanged. P-03 is a subscriber.

## 7. What not to copy from SADE (scale blockers)

The investigation is explicit. Do not reintroduce:

1. Per-observation full-history pricing refits (O(n²)).
2. Unbounded audit/trace slices in production.
3. Cross-process protobuf of every vector for a collocated engine.
4. `statistics`/`json` hashing of the entire DMO/FMO tree as a hot-path requirement.
5. A Python adaptive wrapper “until Go is ready.” The adaptive path has no library gap.

## 8. Validation plan

| Test | When |
| :--- | :--- |
| `go test ./internal/adaptive/...` mapper + D01/D02/D04/engine | Every phase |
| Frozen 100-bar decision compare | Phase C exit |
| `go test ./internal/modelhost/...` infer/skip/timeout | Phase D |
| `go test ./...` full repo | Before merge |
| Live IEX, `QUANTRAM_MODEL=adaptive`, one symbol | Manual; market hours |
| Multi-symbol isolation (AAPL step must not move MSFT state) | Phase D unit test |

Paper orders remain forbidden in this increment.

## 9. Suggested first coding session

1. Add `internal/domain/decision.go` and `internal/adaptive/mapper.go` + tests.
2. Port D01 clamps and `step` with a 20-bar fixture.
3. Do not touch proto or `quantram-server` until Phase C is green.

## 10. Change log

| Date | Change |
| :--- | :--- |
| August 31, 2026 | Implementation increment written after SADE + P-02 review. |
