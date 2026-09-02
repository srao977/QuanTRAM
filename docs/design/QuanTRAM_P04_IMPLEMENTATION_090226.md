# QuanTRAM P-04 — Implementation Increment

**Date:** September 2, 2026  
**Status:** Governing summary accepted 2 Sep. Phases A–I landed (2 Sep). Viewer Price Engine card and airport boards included.  
**Parents:** [P-03 Implementation](QuanTRAM_P03_IMPLEMENTATION_083126.md), [Process Model](QuanTRAM_PROCESS_MODEL_082926.md)  
**Python reference (do not import at runtime):** `C:\Users\chino\SADE`

## 1. Increment goal

Add a collocated Go pricing engine that:

1. Steps on the **same accepted eligible bars** as P-03 (OHLCV + `IntervalStart`), including adaptive `INITIALIZING` minutes.
2. Runs SADE derivatives → F4 → EXPM cover → numerical → PriceEngine → cockpit, one symbol at a time, **without RK45**.
3. Emits one `PriceEvent` per considered bar (emission or typed pricing skip).
4. Proves agreement with SADE Pricing Unit Run 001 **without SDX**.

Out of scope **for this coding increment:** risk, orders, Python `Predict`, `ModelInferenceService`, RK45 Go port, joining BUY/SELL to color into an order. Viewer Price Engine cards and airport boards landed 2 Sep in `quantram-dashboard`.

**Go production destination:** PriceEngine on EXPM trajectories (`time_term == false` only). Offline Gold Nugget (not in this repo, not in CI): SADE `projection.py::solve_cover_rk45_reference`. QuanTRAM CI compares to checked-in SADE **EXPM** artifacts.

## 2. Package layout

Stay in this repo. Domain packages do not import `gen/`. Adaptive packages do not import pricing (pricing may read `domain.Bar` / accepted-bar facts only).

```text
internal/domain/price.go
  PriceEvent, PriceEmission, PricingSkip, PricingSkipReason

internal/pricing/
  config.go          PricingPipelineConfig constants (windows, epsilon, ridge, policy medians)
  observation.go     close, IntervalStart minutes, OHLCV envelope
  history.go         bounded deques (31)
  derivatives.go     causal_quadratic_at_index
  dynamics.go        allocate_fit, fit_f4_at_index (ddof=0, ridge)
  projection.go      analytic_affine_trajectory, solve_cover (EXPM only)
  numerical.go       build_numerical_row (eigvals, 3×3 expm amplification)
  policy.go          EmissionPolicy, PolicyState, PolicyConfig
  engine.go          PriceEngine.observe coherence
  cockpit.go         PriceCockpitInterpreter
  pipeline.go        PricingEngine.Step (copy-compute-commit)
  fingerprints.go    config / policy hashes
  mapper.go          domain.Bar → pricing observation (live + offline)

internal/pricing/*_test.go
  Equivalence tests that load testdata bars, not gRPC.
  testdata/pricing_unit_run_001_observations.csv (+ origin + SHA-256)

internal/modelhost/          (Phase H — landed 2 Sep)
  Same keyed worker PrepareStep/Commit for adaptive + pricing
  GetHealth pricing component; QUANTRAM_PRICING validation
  SubscribePriceEvents fan-out (Phase I)

api/proto/.../quantram.proto (Phase I)
  ModelService.StreamPriceEvents + PriceEvent / PriceEmission / PricingSkip
```

Do **not** create `internal/sade`. Do **not** add `cmd/quantram-model-worker`. `gonum.org/v1/gonum v0.17.0` is declared in `go.mod`.

Suggested flags (startup env):

| Variable | Default | Meaning |
| :--- | :--- | :--- |
| `QUANTRAM_PRICING` | `off` | `off` or `expm`. Unknown: **fail startup**. |
| `QUANTRAM_MODEL` | `off` | Must be `adaptive` if pricing is `expm`. |

When `expm` is requested but fingerprints fail: pricing component **unavailable**; do not silent-off. P-02 and P-03 stay up. When `off`, do not allocate pricing history.

## 3. Port order (science first, host second)

Implement against **unit tests that call the pipeline with `domain.Bar`**, then wire the host. Same discipline as P-03 A–C before D.

### Phase A — Observation mapper and config

- `mapper.go`: `Bar` → pricing observation. **`event_time` minutes = `IntervalStart` unix ms / 60_000.**
- Offline helper: CSV `{source_timestamp,open,high,low,close,volume}` → `domain.Bar` (final, COMPLETE) — reuse P-03 test adapter style. This is not SDX.
- Copy `PricingPipelineConfig` defaults and `PolicyConfig` medians verbatim.
- `internal/domain/price.go`: `PriceEvent` / emission / skip (no proto).

**Exit:** mapper tests; consecutive minute `x` differences are `-14 … 0` on the 15-window.

### Phase B — Causal quadratic

Port `causal_quadratic_at_index` only for production. Full-history `causal_quadratic` may exist in tests to prove at-index equivalence (Finding 002), not on the live Step path.

gonum: SVD / lstsq matching `numpy.linalg.lstsq(..., rcond=None)`. Rank ≠ 3 or non-finite → NaN.

**Exit:** at-index p1/p2 vs a frozen window from pricing_001 (tolerance table).

### Phase C — F4

Port `fit_f4_at_index`. Population std `ddof=0`. Ridge `diag([0,1,1,1])`, `lambda=1`. Reject newest index. Store physical, means, scales, min/max, condition.

**Exit:** physical coefficients vs fixture window; `valid_fit` false cases return skip `F4_FIT_UNAVAILABLE`.

### Phase D — EXPM cover

Port `analytic_affine_trajectory` + production `solve_cover`. Grid 11. `time_term` true → error, never a trajectory. **Do not copy** `solve_cover_rk45_reference`.

gonum: `(*mat.Dense).Exp` on the 4×4 augmented matrix.

**Exit:** 11-point `[p,p1,p2]` vs SADE EXPM on the same initial+physical (absolute/ulp table). Unstable and envelope-exit packaging match Python.

### Phase E — Numerical row

3×3 companion eigenvalues (`max Re`), `perturbation_amplification` from **3×3** `expm(A)` column norms. Projected endpoint from cover. Keep SADE key `rk_success` in testdata mapping.

Investigation F.2.5: cond / eigenvalue / amplification can flip policy colors near thresholds. Record observed deltas; do not “fix” policy cutovers by changing medians.

**Exit:** numerical fields vs fixture rows that reached `EMITTED`.

### Phase F — PriceEngine + cockpit

Port `EmissionPolicy.emit`, debounce `PolicyState`, `PriceEngine.observe` symbol/timestamp check, cockpit. No BUY/SELL/HOLD mapping.

**Exit:** `color`, `trajectory_phase`, `turning_tendency`, `confidence_state` vs pricing_001 for emitted rows.

### Phase G — Pipeline + frozen unit run

`PricingEngine.Step`: copy-compute-commit; non-finite / panic path does not mutate committed deques or policy state. Bounded history 31. Status machine §5.5 of the design doc.

Fixture: copy `SADE/output/unit_runs/pricing_001/observations.csv` (and origin note). SHA-256 at check-in. Compare `pricing_status`, `pricing_emitted`, `pricing_color`, `pricing_phase`, `pricing_confidence`, `rk_success`, `domain_exit`. Expect **15 / 30 / 55**.

Do not start `sdx-server` or `python -m sade.unit_run.run_pricing_001` in CI.

**Exit:** `go test ./internal/pricing/...` green. Host unwired. Proto unwired.

### Phase H — Host join

- Same keyed worker; **no second mailbox**.
- Call pricing only after the bar is accepted for the symbol (eligible, adjacent, infer on). Adaptive skip INITIALIZING **does** call pricing.
- `infer` pause: skip pricing Step, do not reset history (same as adaptive).
- One transactional pair: `PrepareStep` adaptive + pricing, then commit **both** or **neither**. Shared `QUANTRAM_MODEL_DEADLINE`. Injected P-04 failure after P-03 prepare rolls back both; replay of the same bar matches a clean run.
- Health: `GetHealth` component `pricing` = `off` / `cold` / `initializing` / `ready` / `paused` / `discontinuous` / `error`. Pricing initializing/cold does not paint overall market red. Pricing unavailable degrades overall health but does not take ingest or P-03 down.
- Default `off`. `expm` without `adaptive` fails startup. Unknown `QUANTRAM_PRICING` fails startup.

**Exit tests:** overflow/gap pause pricing; INITIALIZING bars still price-warmup; reconstructed/backfill bars never price; rollback/replay; `QUANTRAM_PRICING` unknown fails startup. **Done (2 Sep).**

### Phase I — Proto and viewer

`StreamPriceEvents` on `ModelService`. Off → `FailedPrecondition`; unavailable → `Unavailable`. Last-per-symbol catch-up, no durable history. Dashboard proto copy + Adaptive Pipeline Price Engine **stage** minicard, output card, Departures / Arrivals airport boards. **Done (2 Sep).**

Do **not** add `Evaluate` / `ModelInferenceService` in this increment.

### Continuity (updated 2 Sep, after freeze)

Model continuity is **causal observation continuity**, not fixed one-minute timestamp adjacency. Classifier: `internal/domain/continuity.go`.

A provider-omitted minute (`10:31 → 10:34`) is a valid irregular interval. Adaptive and Price Engine both step on that accepted bar. Actual `IntervalStart` is preserved (P-04 `x = t - t_index` in minutes). The one-minute EXPM cover is a **projection horizon**, not an incoming-bar spacing rule. No synthetic flat bar is inserted.

`INPUT_GAP` / `STATE_DISCONTINUOUS` remain for **proven** loss: queue overflow, isolated panic, or a harness-proven missing eligible bar. Silence (no new bar) is not a gap.

`Host.ResetSymbol` reinitializes one symbol (adaptive + pricing) and restarts warm-up. There is no operator reset RPC yet. Process restart is no longer required for a skipped IEX minute.

## 4. SADE file → Go file map

| SADE path | Go target | Port? | Status |
| :--- | :--- | :--- | :--- |
| `pricing_pipeline/derivatives.py` `causal_quadratic_at_index` | `derivatives.go` | Yes | Done (2 Sep) |
| `causal_quadratic` full history | tests only | Test | Done |
| `pricing_pipeline/dynamics.py` `fit_f4_at_index` | `dynamics.go` | Yes | Done (2 Sep) |
| `fit_f4` full history | tests only | Test | Done |
| `projection.py` `analytic_affine_trajectory`, `solve_cover` | `projection.go` | Yes | Done (2 Sep) |
| `projection.py` `solve_cover_rk45_reference` | — | **No** | — |
| `numerical.py` `build_numerical_row` | `numerical.go` | Yes | Done (2 Sep) |
| `price_engine/contracts.py` | `domain/price.go` | Yes | Done (2 Sep) |
| `price_engine/engine.py` | `engine.go` | Yes | Done (2 Sep) |
| `price_engine/policy.py` | `policy.go` | Yes | Done (2 Sep) |
| `price_engine/cockpit.py` | `cockpit.go` | Yes | Done (2 Sep) |
| `pricing_pipeline/pipeline.py` | `pipeline.go` | Yes, minus SDX/session defaults that lie | Done (2 Sep) |
| `unit_run/run_pricing_001.py` | testdata | Offline harness only | Fixture checked in |
| `input/sdx_client.py` | — | **No** | — |
| Adaptive D01–D04 / emitter | already P-03 | **No** (do not re-port) | Done |

## 5. Offline equivalence without SDX

Pricing Unit Run 001 consumed adaptive output rows from the same SDX CSV as Unit Run 001. Recreate the **OHLCV sequence**, not the gRPC hop and not BUY/SELL as inputs.

**Chosen:** reconstruct bars from a checked-in copy of `SADE/output/unit_runs/pricing_001/observations.csv`. Feed through mapper → `PricingEngine.Step`. Compare pricing columns listed in Phase G. Adaptive columns in that file are context only.

Do not run SADE as part of QuanTRAM CI.

## 6. Wiring in `quantram-server`

When `QUANTRAM_MODEL=adaptive` and `QUANTRAM_PRICING=expm`:

```text
pipeline := ...
host := modelhost.New(..., Options{Mode: adaptive, Pricing: expm, Deadline})
go host.Run(ctx)
```

When pricing `off` (default): no pricing deques; health component `off`. Observe APIs unchanged. Startup log includes `model=` and `pricing=`.

P-03 `StreamDecisions` unchanged. `StreamPriceEvents` is the northbound PriceEvent fan-out. After a QuanTRAM proto change, copy `quantram.proto` into `quantram-dashboard` and regenerate; restart Next.js so `/api/prices` loads.

## 7. What not to copy from SADE (scale blockers)

Same list as P-03, plus:

1. Per-observation full-history derivative/F4 refits (O(n²)).
2. `solve_ivp` / RK45 / `scipy.integrate`.
3. `time_term` jerk branch.
4. Unbounded close/p1/p2 history (SADE already bounded to 31 — keep that).
5. A Python pricing sidecar “until gonum is ready.”
6. Recalibrating policy medians to hide EXPM/gonum drift.

## 8. Validation plan

| Test | When | Status |
| :--- | :--- | :--- |
| Mapper + IntervalStart minutes | Phase A | Done (2 Sep) |
| Quadratic at-index | Phase B | Done (2 Sep) |
| F4 at-index + ddof=0 | Phase C | Done (2 Sep) |
| EXPM 11-point vs SADE EXPM | Phase D | Done (2 Sep) |
| Numerical eig/amplification | Phase E | Done (2 Sep) |
| Policy + cockpit colors | Phase F | Done (2 Sep) |
| Frozen 100-bar pricing_001 + SHA-256 | Phase G exit | Done (2 Sep) semantic PASS 15/30/55 |
| Host join, infer pause, no second mailbox | Phase H | Done (2 Sep) |
| `StreamPriceEvents` + viewer cards/boards | Phase I | Done (2 Sep) |
| `go test ./...` | Before merge | Green (2 Sep) |
| `go test -race ./...` | Before merge | Blocked here if cgo unavailable |
| Live IEX PriceEvents | Manual; after H/I | In progress 2 Sep (warm-up + gap pause observed; first color not yet claimed) |
| `time_term` reject; unknown `QUANTRAM_PRICING` | A / H | Done (2 Sep) |

**Do not start:** P-05, paper orders, RK45, BUY/SELL+color join.

## 9. Suggested first coding session — **done (2 Sep)**

Phases A–I landed. Host and viewer are wired. Next work is live IEX color observation (not a code increment) or P-05 only after that design exists.

## 10. Change log

| Date | Change |
| :--- | :--- |
| September 2, 2026 | Implementation increment written from SADE `pricing_pipeline` + P-03 host rules. Code not started. |
| September 2, 2026 | Phases A–G landed: `internal/domain/price.go`, `internal/pricing` (active-index quadratic/F4, EXPM cover, policy, cockpit). Pricing Unit Run 001 semantic PASS 15/30/55. Manifest `internal/pricing/testdata/p04_equivalence_manifest.json`. Host (H) not started. |
| September 2, 2026 | Phase H: same keyed worker, `QUANTRAM_PRICING=off|expm`, transactional prepare/commit, rollback/replay, `GetHealth` pricing component. No proto / dashboard. |
| September 2, 2026 | Phase I: `StreamPriceEvents`, dashboard Price Engine **stage** + output card, Departures / Arrivals airport boards. |
| September 2, 2026 | Live IEX: documented `INPUT_GAP` / `STATE_DISCONTINUOUS` (missing adjacent minute, not end-of-data). Restart = cold start (15 Adaptive + 45 Price Engine consecutive eligible minutes). |
| September 2, 2026 | False adjacency latch removed. Irregular provider intervals accepted on the same bar by P-03 and P-04. Pricing science unchanged. |
