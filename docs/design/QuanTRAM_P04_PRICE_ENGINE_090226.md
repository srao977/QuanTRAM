# QuanTRAM P-04 Price Engine — Design

**Date:** September 2, 2026  
**Last updated:** September 2, 2026  
**Status:** Governing summary accepted. Phases A–I landed (2 Sep), including viewer Price Engine card and airport boards.  
**Scope:** Collocated Go PriceEngine that consumes the same accepted eligible 1-minute bars as P-03 and emits `PriceEvent` (PriceEmission / typed pricing skip). Never sends orders.  
**Parents:** [Process Model](QuanTRAM_PROCESS_MODEL_082926.md), [P-03 Adaptive Model Host](QuanTRAM_P03_ADAPTIVE_MODEL_HOST_083126.md), [P-03 Implementation](QuanTRAM_P03_IMPLEMENTATION_083126.md), [P-02 Data Quality](QuanTRAM_INGESTION_P02_DATA_QUALITY_083126.md)  
**Scientific source:** `C:\Users\chino\SADE` (`sade/pricing_pipeline/` → derivatives → F4 → EXPM `solve_cover` → numerical → PriceEngine → optional cockpit)  
**Language evidence:** [SADE Go refactorability investigation, 2026-08-27](file:///C:/Users/chino/SADE/docs/investigations/SADE_GO_REFACTORABILITY_AND_SCALED_RUNTIME_INVESTIGATION_2026-08-27.md) Part F  
**Implementation plan:** [P-04 Implementation](QuanTRAM_P04_IMPLEMENTATION_090226.md)  
**Python reference (do not import at runtime):** `C:\Users\chino\SADE`

## 0. Governing implementation summary (accepted 2 Sep)

P-04 is the QuanTRAM Price Engine scientific migration increment.

The objective is narrow: faithfully port the already validated SADE production pricing mathematics into collocated Go and prove scientific/semantic equivalence **before** connecting it to the P-03 runtime host.

P-04 is **not** a Decision Engine, risk engine, execution engine, or confirmation layer for P-03.

### Scientific position

P-03 Adaptive and P-04 Pricing are independent scientific siblings operating on the same accepted eligible Bar.

```text
                         accepted Bar
                              |
                    keyed symbol worker
                              |
                 +------------+------------+
                 |                         |
                 v                         v
          P-03 Adaptive              P-04 Pricing
                 |                         |
          DecisionEvent                PriceEvent
```

P-04 must **not** consume P-03 BUY/SELL/HOLD as a scientific input.  
P-03 must **not** consume P-04 GREEN/AMBER/RED as a scientific input.

Any future reasoning, alignment, judgment, Decision Engine, OMS/risk joining, or execution-event generation is explicitly outside P-04.

Do not attempt to determine how DecisionEvent and PriceEvent eventually combine to produce an executable action. Their later downstream join remains **provisional** and is not part of this increment.

### Scientific pipeline to port

```text
Bar / OHLCV history
    -> causal_quadratic_at_index
    -> p1 / p2 / jp
    -> fit_f4_at_index
    -> physical affine coefficients
    -> analytic EXPM solve_cover
    -> numerical interpretation
    -> EmissionPolicy / PriceEngine
    -> optional cockpit
    -> PriceEvent
```

Preserve the accepted mathematics. Do **not** improve, reformulate, recalibrate, optimize scientifically, or substitute different mathematical conventions.

- Production derivative and F4 computation must remain active-index only.
- Do not reintroduce full-history O(n²) fitting.
- F4 population standard deviation is `ddof=0`.
- Preserve ridge `lambda=1` and `diag([0,1,1,1])`.
- Preserve the physical coefficient conversion.
- Preserve condition-number semantics.
- Preserve existing policy constants and medians.
- Do not recalibrate thresholds to compensate for Go/gonum numerical drift.
- Preserve `active_index = current - 1`.
- Preserve bounded scientific history of 31 rows.
- Preserve the one-minute projection horizon and 11-point grid.
- Preserve domain-exit, first-exit, exit-dimension, stability, and `D_local_maximum` semantics.

### Solver decision is closed

Go production uses analytic EXPM only (`gonum` `Dense.Exp`, `time_term=false`). Do not port RK45, call Python RK45, create a Python pricing sidecar, or add `solve_ivp`. Frozen SADE RK45 remains an offline Gold Nugget. Finding 001 already closed EXPM vs RK45; P-04 must not reopen it. `time_term=true` is unsupported and must be rejected explicitly.

### Numerical migration risk

Treat F4 and downstream numerical classification as the highest-risk migration area, not EXPM. Small continuous differences in condition number, eigenvalues, or perturbation amplification can cross policy thresholds.

Validation must distinguish:

1. continuous numerical agreement within explicitly recorded tolerances; and
2. discrete scientific/semantic equivalence.

The latter is the **governing acceptance criterion**. At minimum preserve equivalence for pricing status, emitted/not emitted, projection success, domain exit, first exit / exit dimension where applicable, color, trajectory phase, confidence state, and cockpit output. Never modify policy thresholds merely to make these outputs agree.

### Implementation order is a hard boundary

Phases A–G (mapper through frozen 100-bar replay) must finish and go green before Phase H (host) or Phase I (proto). Do not mix mathematical debugging with host/runtime debugging.

Phase G expected 100-bar behavior: `WARMUP_DERIVATIVE=15`, `WARMUP_F4=30`, `EMITTED=55`. Fixture SHA-256 recorded at check-in. Produce a machine-readable P-04 equivalence manifest (fixture hash, revision, gonum version, counts, max numerical deltas, semantic PASS/FAIL).

### State integrity

Pricing Step must use copy-compute-commit. A failed, timed-out, panicking, non-finite, or otherwise rejected computation must not partially mutate committed pricing history or policy state.

When Phase H is reached: one keyed owner per symbol; no second model mailbox; one transactional prepare/commit for P-03 and P-04 (commit both or neither), plus an explicit rollback/replay test.

### Scope stop line

P-04 ends at `PriceEvent`. Viewer Price Engine cards and airport boards landed 2 Sep in `quantram-dashboard`. Do not implement a Decision Engine, alignment logic, P-05/P-06, paper trading, order generation, BUY/SELL + color combination, Volume Engine, RK45, or Python runtime bridges.

**Do not improve the mathematics.** Port the accepted mathematics, prove equivalence, freeze the scientific checkpoint, and only then integrate with the runtime host.

## 1. Purpose

P-03 turns an accepted eligible bar into a scientific **DecisionEvent** (BUY / SELL / HOLD or typed skip). That is not the product decision. The locked destination is:

**Adaptive model + PriceEngine, with the PriceEngine decision on analytic EXPM trajectories.**

P-04 is that PriceEngine join. It ports SADE’s production pricing path into Go inside QuanTRAM. It does **not** stand up a Python sidecar, does **not** call `ModelInferenceService.Predict`, does **not** port RK45, and does **not** connect to P-05 / P-06.

PriceEngine **does not emit BUY / SELL / HOLD**. Adaptive already owns that. PriceEngine emits trajectory phase, turning tendency, domain/stability/confidence, and color (GREEN / AMBER / RED / INVALID). Combining those two scientific surfaces into an order intent is a later process (P-05), not this increment.

## 2. What SADE pricing is today

SADE Pricing Unit Run 001 streams the same 100 AAPL vectors as Adaptive Unit Run 001, then:

```text
AdaptivePipeline.process_vector
        │  adaptive output row (OHLCV + lineage)
        ▼
PricingPipeline.process
        │
        ├─ causal quadratic at active_index (p1, p2); jp = Δp2
        ├─ F4 ridge fit at active_index (physical affine coefficients)
        ├─ solve_cover(..., time_term=false)   production EXPM, 11-point [0,1] cover
        ├─ build_numerical_row                 eigvals, expm(A) amplification
        ├─ PriceEngine.observe → PriceEmission
        └─ optional PriceCockpitInterpreter
```

Validated offline baseline: `SADE/output/unit_runs/pricing_001/` (100 AAPL bars). Observed counts: 15 `WARMUP_DERIVATIVE`, 30 `WARMUP_F4`, 55 `EMITTED` (55 EXPM successes, 0 projection failures, 18 domain exits). Adaptive on the same stream: 15 `INITIALIZING`, then 8 BUY / 10 SELL / 67 HOLD. Pricing does **not** wait for adaptive ACTIONABLE; it steps on every adaptive record, including initializing rows.

Required adaptive fields into pricing are OHLCV + `entity_id` + `source_row_index` + `source_timestamp`. `position_decision` is recorded beside pricing in the CSV; it is **not** an input to F4, EXPM, or policy.

Production projection is `sade/pricing_pipeline/projection.py::solve_cover` → `analytic_affine_trajectory` (`scipy.linalg.expm`). Frozen RK45 is `solve_cover_rk45_reference` (APTF `diagnostics/run_test_013b_qqq_validation.py::solve_cover` lineage). Finding 001: 55 solves / 605 points; downstream PriceEmission / PolicyState / cockpit matched. That gate is closed in SADE. QuanTRAM must not reopen it by putting RK45 in Go.

## 3. Decisions for QuanTRAM

| Topic | Decision |
| :--- | :--- |
| Process ID | **P-04** is PriceEngine / Go EXPM. The August 29 process-model P-04 (Python adaptive `Predict` worker) is **superseded**. Adaptive math stays in P-03. |
| Live input | Same **accepted eligible** bars as P-03 (`SubscribeModelBars` / `ModelEligible()`, adjacency, Phase 0 `infer`). Not the lossy observe stream. Not REST backfill. |
| Join | **Sibling on the bar**, not a consumer of `Decision.side`. After the host accepts a bar for the symbol, P-03 and P-04 both step, including valid irregular intervals. Adaptive `INITIALIZING` still feeds pricing. A host-gate skip (`QUEUE_OVERFLOW`, proven `INPUT_GAP`, `NOT_MODEL_ELIGIBLE`, …) feeds **neither**. |
| Time basis | Live: `Bar.IntervalStart` as minutes (`unix_ms / 60_000`), same event-time lock as P-03. Offline fixture: parse `source_timestamp` into `IntervalStart` the same way P-03 Unit Run 001 tests do. Consecutive 1-minute bars make the quadratic `x = t - t_index` independent of wall timezone. |
| Active index | SADE evaluates derivatives / F4 / cover at `active_index = current - 1` (newest close is pending). Preserve that lag. Do not fit F4 on the newest row (`fit_f4_at_index` requires `index < len(p)-1`). |
| Solver | Go production = **4×4 affine EXPM** (`time_term == false` only). Reject `true` with `ANALYTIC_TIME_TERM_UNSUPPORTED`. Do not port `solve_cover_rk45_reference`. Offline Gold Nugget remains SADE Python RK45; QuanTRAM tests compare to **checked-in SADE EXPM outputs**, not a live Python process. |
| Linear algebra | **gonum** (`gonum.org/v1/gonum`, pin **v0.17.0** — already in the local module cache per the investigation). Allowed for lstsq / ridge solve / `Cond` / eigenvalues / `Dense.Exp` only. Do not add NumPy/SciPy, Python cgo, or a homegrown Padé EXPM. Adaptive P-03 stays stdlib scalar math. |
| History | Bounded deque `max(derivative_window, f4_window)+1` (default **31**). Production uses `causal_quadratic_at_index` and `fit_f4_at_index` only. Do not reintroduce full-history O(n²) refits on the live path (Finding 002). |
| SDX / APTF | **Do not use** on the live path or in CI. No APTF checkout. |
| Orders / risk | Out of scope. `PriceEvent` is the stop line, parallel to P-03 `DecisionEvent`. |
| Proto | Domain types first. `ModelInferenceService` is **not** added. Northbound `ModelService.StreamPriceEvents` landed 2 Sep (Phase I). Off → `FailedPrecondition`; unavailable → `Unavailable`. Last-per-symbol catch-up, no durable history. |
| Dashboard | Adaptive Pipeline Price Engine **stage** minicard, output card, Departures (Adaptive) / Arrivals (Price Engine) airport boards landed 2 Sep. Do not change that two-band layout unless asked. |
| Default | `QUANTRAM_PRICING=off`. Live adaptive stays unchanged until pricing is opted in. |

### 3.1 Config

| Variable | Default | Meaning |
| :--- | :--- | :--- |
| `QUANTRAM_MODEL` | `off` | Unchanged. `adaptive` required before pricing may run. |
| `QUANTRAM_PRICING` | `off` | `off` or `expm`. Unknown values: **fail startup**. |
| `QUANTRAM_MODEL_DEADLINE` | `200ms` | Shared host deadline; pricing step is inside the same worker, not a second mailbox. |

If `QUANTRAM_PRICING=expm` and `QUANTRAM_MODEL` is not `adaptive`: **fail startup**. Do not silently run pricing off the observe path. If pricing is requested but fingerprints/config fail: pricing component **unavailable**, ingestion and P-03 stay up.

## 4. Runtime topology (local Phase 0)

```text
P-01 Alpaca ──► P-02 pipeline
                    │
                    │ FinalizedBarConsumer (model path; no silent drop)
                    ▼
              P-03 Model Host  (keyed worker per symbol)
                    │
                    ├─ AdaptiveEngine.Step  → DecisionEvent
                    └─ PricingEngine.Step   → PriceEvent     (this increment, when expm)
                    │
                    ✗  neither connected to P-05 / P-06
```

Collocation: P-04 is **in-process Go**, called through a Go interface from the same keyed worker as P-03. No loopback gRPC. No Python worker. One owner per symbol: no concurrent Step on adaptive or pricing state for that symbol.

P-04 must not get a second `SubscribeModelBars` mailbox. Two consumers of the model path would race on overflow semantics. The host that already accepted the bar is the only caller.

## 5. Scientific contract (must not drift)

### 5.1 Causal quadratic (`derivatives.py`)

- `p` = close.
- Trailing window default **15**.
- Design `[x², x, 1]` with `x = t - t_index` in minutes.
- `p1 = b`, `p2 = 2a` from `y ≈ a x² + b x + c`.
- Production: `causal_quadratic_at_index` (one index). Match `numpy.linalg.lstsq(..., rcond=None)` rank/finite checks; failed fit → NaN, not a panic.
- `jp[i] = p2[i] - p2[i-1]` when previous p2 is finite.

### 5.2 F4 (`dynamics.py`)

- Window default **30**, `ridge_lambda = 1.0`.
- Population std **`ddof=0`**.
- Ridge `diag([0, 1, 1, 1])`.
- Target `jp`; standardized design `[1, (p,p1,p2 - means)/scales]`.
- Physical `[b, a_p, a_p1, a_p2]`: intercept adjusted by `beta[0] - slopes·means`.
- Store min/max of the window for domain-exit tests; `condition = cond(design)`.
- `fit_f4_at_index` returns none when scales ≤ 0, non-finite, or `index` is the newest row.

### 5.3 EXPM cover (`projection.py::solve_cover`)

- Grid `linspace(0, 1, 11)` — **projection horizon is one minute**, distinct from source timestamp gaps.
- State `[p, p1, p2]`; jerk `b + a_p p + a_p1 p1 + a_p2 p2`.
- Affine lift: 4×4 `[[A, c], [0, 0]]` with `A = [[0,1,0],[0,0,1],[a_p,a_p1,a_p2]]`, `c = [0,0,b]`, initial `[p,p1,p2,1]`. Trajectory is `(expm(M t) v)[:3]`.
- Post-solve (unchanged vs RK45 packaging): finite check; `|y_end - y0| > 1e6 * scales` → `NUMERICALLY_UNSTABLE`; `D_local_maximum`; envelope exit vs window min/max; `first_exit_time`; `exit_dimension` in `{P,P1,P2}` joined by `|`.
- Production payload uses `solver_method=ANALYTIC_EXPM`. Legacy SADE field `rk_success` remains the boolean “projection succeeded” for policy/CSV equivalence.

### 5.4 Numerical + policy (`numerical.py`, `price_engine/`)

- Companion `A` 3×3 from physical slopes; `max_real_eigenvalue = max Re(λ(A))`; `perturbation_amplification = max_j ‖expm(A)[:,j]‖₂` (one-minute growth, **3×3 expm**, not the 4×4 affine lift).
- `PriceEngine.observe` is coherence only (symbol + timestamp). Mathematics is `EmissionPolicy.emit`.
- Policy does **not** produce BUY/SELL/HOLD. Colors GREEN/AMBER/RED; INVALID on projection failure / non-finite.
- Frozen `PolicyConfig` medians/q95 copied from `PricingPipeline` defaults (`P_EMISSION_V0_1`, epsilon `0.0035332071428566536`, …). Do not “recalibrate” in this increment.
- Cockpit is **in scope for equivalence** (`enable_cockpit=true` in Unit Run pricing_001: 55 cockpit outputs). It is not an order gate.

### 5.5 Warm-up (not a mystery skip)

| Status | When | Count on pricing_001 |
| :--- | :--- | :--- |
| `WARMUP_DERIVATIVE` | no previous close, or p1/p2 non-finite | 15 |
| `WARMUP_F4` | `global_active_index < f4_window` or jp window not all finite | 30 |
| `F4_FIT_UNAVAILABLE` | `fit_f4_at_index` returns none | 0 on fixture |
| `EMITTED` | numerical row + PriceEngine.observe | 55 |
| `RK45_FAILURE` | SADE name when `rk_success` is false; Go may spell `PROJECTION_FAILURE` internally and map the CSV name in tests | 0 on fixture |

First PriceEmission is later than first adaptive ACTIONABLE (16th vs ~46th accepted minute). The viewer already treats adaptive warm-up as initialization; pricing warm-up is the same idea with a longer board.

## 6. Domain output

Freeze in `internal/domain` **before** proto (same rule as P-03 `DecisionEvent`):

```text
PriceEvent
  oneof outcome { PriceEmission, PricingSkip }
  symbol, interval_start, market_snapshot_id, accepted_sequence, latency
  hashes / versions when diagnostics on
```

`PricingSkip` reasons at least: `WARMUP_DERIVATIVE`, `WARMUP_F4`, `F4_FIT_UNAVAILABLE`, `PROJECTION_FAILURE`, `NUMERICALLY_UNSTABLE`, `INVALID_INPUT`, `TIMEOUT`, `ENGINE_ERROR`. Do not overload P-03 `SkipReason` with these unless a review explicitly merges the enums.

HOLD remains a P-03 decision. GREEN/AMBER/RED is a P-04 emission. Do not invent a single “trade color” in this increment.

## 7. What this increment will not do

- P-05 risk, P-06 paper/live orders, ledger, benchmark
- Python `Predict` worker / `ModelInferenceService`
- RK45 in Go, calling Python RK45, APTF repo dependency
- `time_term == true` EXPM
- Full-history `causal_quadratic` / `fit_f4` on the live path
- Second model-path mailbox
- Joining adaptive side + price color into an `OrderIntent`

## 8. Definition of done (after implementation, not this morning)

- Frozen Pricing Unit Run 001 compare: status counts 15 / 30 / 55; color/phase/confidence/`rk_success`/`domain_exit` within a stated tolerance table (eigenvalue / cond / amplification are MEDIUM-HIGH drift risk — investigation F.2.5).
- Fixture SHA-256 recorded at check-in (do not reuse the August 25 SADE report hashes blindly; re-hash the copied file).
- `time_term true` rejected; RK45 not imported.
- Host: pricing off by default; `expm` requires `adaptive`; overflow / proven loss still pause **both**; a skipped provider minute does not. Infer pause-not-reset applies to pricing state too.
- `go test ./...` green. `StreamPriceEvents` and viewer cards landed 2 Sep. Live IEX color is not claimed until a regular-hours emitted `PriceEvent` is accepted. A skipped provider minute no longer pauses pricing.
- No P-04 output connected to orders.

## 9. Change log

| Date | Change |
| :--- | :--- |
| September 2, 2026 | Initial P-04 design: Go PriceEngine on EXPM, sibling of P-03 on accepted eligible bars, gonum v0.17.0, no Python sidecar, no RK45, no orders. |
| September 2, 2026 | Governing implementation summary accepted. Phases A–G green; host not wired. |
| September 2, 2026 | Phase H host join: collocated EXPM on the same accepted bar as P-03; commit both or neither; pricing default off. |
| September 2, 2026 | Phase I: `StreamPriceEvents`; dashboard Price Engine stage + output card; Departures / Arrivals airport boards. |
| September 2, 2026 | Live IEX: `INPUT_GAP` / `STATE_DISCONTINUOUS` is a missing adjacent minute (not end-of-data). Restart is a cold start (15 Adaptive + 45 Price Engine consecutive accepted minutes). |
| September 2, 2026 | Incoming bar spacing is not required to be one minute. EXPM horizon remains one minute. Irregular accepted bars step pricing with actual timestamps. |
