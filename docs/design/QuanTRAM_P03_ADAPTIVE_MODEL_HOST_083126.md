# QuanTRAM P-03 Adaptive Model Host — Design

**Date:** August 31, 2026  
**Status:** Design for implementation. No P-03 code in this repository yet.  
**Scope:** Collocated Go adaptive host that consumes P-02 finalized bars.  
**Parents:** [Process Model](QuanTRAM_PROCESS_MODEL_082926.md), [P-02 Data Quality](QuanTRAM_INGESTION_P02_DATA_QUALITY_083126.md)  
**Scientific source:** `C:\Users\chino\SADE` (Python Adaptive Pipeline D01 → D02 → D04 → emitter)  
**Language evidence:** [SADE Go refactorability investigation, 2026-08-27](file:///C:/Users/chino/SADE/docs/investigations/SADE_GO_REFACTORABILITY_AND_SCALED_RUNTIME_INVESTIGATION_2026-08-27.md)  
**Implementation plan:** [P-03 Implementation](QuanTRAM_P03_IMPLEMENTATION_083126.md)

## 1. Purpose

P-03 is the next QuanTRAM process after the P-02 data gate. It turns a **finalized, quality-eligible 1-minute bar** into a **DecisionVector or an explicit skip**. It never sends orders.

This increment implements the **adaptive** scientific path in Go inside QuanTRAM. It does **not** reconnect SADE to SDX, and it does **not** stand up a Python sidecar for the adaptive model.

## 2. What SADE is today (authoritative for science, not for live ingress)

SADE V0.1 is a Python package. The live product path is:

```text
SDX gRPC StreamVectors (CSV MarketVector)
        │
        ▼
sade/input/sdx_client.py
        │
        ▼
AdaptivePipeline.process_vector
        │  source_row dict + physical_row = source_row_index + 2
        ▼
AdaptiveEmitter.process
        │
        ├─ SourceRowNormalizer → NormalizedObservation (close, volume, event_time)
        ├─ D01V02Model.step          Markovian recursive state
        ├─ D02 build_return_shape
        ├─ D04 CapturabilityModelV0_2
        └─ rolling-15 context → BUY / SELL / HOLD or INITIALIZING
```

Validated offline baseline: SADE Unit Run 001, AAPL, 100 vectors, outputs under `SADE/output/unit_runs/001/`.

### 2.1 Scientific lineage (roadmap, not “two finished products”)

| Site | Role |
| :--- | :--- |
| **APTF** | Experimental site. Equations, experiments, and the original Python were developed there. |
| **SADE** | Take **proved** APTF code into a product runtime. The lift is **not complete**. |
| **QuanTRAM P-03+** | Same proved science, live bars from P-02, Go-dominant runtime. |

The intended product decision is **not** adaptive BUY/SELL/HOLD alone. The roadmap is:

**Adaptive model + PriceEngine, with the PriceEngine decision on RK45 trajectories.**

SADE V0.1 shipped the adaptive path and a pricing pipeline, but did **not** finish joining them, and did **not** finish migrating the proved Python RK45 path from APTF. The 27 Aug Go refactorability study therefore described a split system and a later `expm` production switch (Finding 001) **before** that RK45 migration was complete. Those are snapshots of incomplete SADE, not the destination.

**RK45 is the scientific authority for that pricing decision** — the golden nugget from APTF. The 11-point `[0,1]` minute cover (`projected_*`, `domain_exit`, `rk_success` → GREEN/RED/AMBER) is not a disposable implementation detail. Completing the proved Python RK45 migration, then joining it to adaptive state, is the pricing increment.

Analytic `expm` is only a candidate **accelerator** of the same cover. It may be used in production only after it matches RK45 on the frozen trajectory and downstream PriceEngine colors. It does not replace RK45 as the definition of the decision.

This P-03 increment ports only what SADE **did** prove and lift: the adaptive emitter. It must not treat “adaptive and pricing are separate forever” as design intent.

## 3. Decisions for QuanTRAM

| Topic | Decision |
| :--- | :--- |
| Live input | **P-02 finalized bar stream only.** `SubscribeFinalized` plus `infer=true` and `Bar.ModelEligible()`. |
| SDX / SADE `sdx_client` | **Do not use** on the live path. SDX remains an offline CSV tool in another repo. |
| Adaptive mathematics | **Go black box** in this process (`internal/adaptive`). Stdlib scalar math; no NumPy/SciPy on this path. |
| Python adaptive wrapper / `ModelInferenceService.Predict` | **Not required** for adaptive. A stateful Python wrapper is strictly worse (affinity + serialize-every-bar). Amends process-model P-04 for this increment. |
| RK45 / pricing | **Deferred as code, not cancelled.** Target: PriceEngine decides on RK45 trajectories, joined to the adaptive model. Transport: Go holds bars/F4; Python RK45 is a **called** solver (or later Go equivalent proved against it), not a gRPC client of P-02. |
| Persistence of bars | Still not required. P-03 holds **per-symbol scientific state** only (D01 runtime + 15-context). |
| CSV in QuanTRAM | Allowed only as an **offline equivalence harness** that maps rows into `domain.Bar` and calls the same Go engine. |
| Orders / risk | Out of scope. `DecisionVector` is the stop line. |

Process-model amendment (this increment): P-03 **owns** the proved adaptive path in Go. P-04 is reserved for the unfinished RK45 / PriceEngine join (Python solver called by Go). Do not implement `ModelInferenceService` until that increment. Do not treat Finding 001 `expm` as the final PriceEngine producer until RK45 migration and equivalence are closed.

## 4. Runtime topology (local Phase 0)

```text
P-01 Alpaca ──► P-02 pipeline
                    │
                    │ SubscribeFinalized (in-process, depth 2)
                    │ infer / observe from GetReadiness
                    ▼
              P-03 Model Host
                    │  per-symbol AdaptiveEngine
                    │  skip if !infer or !ModelEligible or INITIALIZING
                    ▼
              DecisionVector | Skip
                    │
                    ✗  not connected to P-05 / P-06 in this increment

Northbound (optional same increment or immediately after):
  ModelService.StreamDecisions / GetModelInfo  (proto edge only)
```

Collocation rule from the process model stands: P-03 calls P-02 through a **Go interface**, not loopback gRPC. Proto types stay in `internal/server`.

One owner per symbol: causal order is **along** a symbol. Symbols are independent. Do not share D01 state across symbols.

## 5. Input contract (P-02 → P-03)

P-03 may step the engine only when **all** of these hold:

1. The bar arrived on the finalized path (`is_final=true`).
2. `pipeline.Readiness().Infer` is true (contiguous complete live head, feed healthy).
3. `bar.ModelEligible()` — `COMPLETE`, not backfilled.
4. Per-symbol `interval_start` is strictly after the last accepted step (same causality SADE enforces on `event_time`).

Otherwise emit **skip** and leave scientific state unchanged. Do not reuse the previous `DecisionVector` as if it were new (OP-05).

The Next.js viewer remains on the **observe** stream. It is not the P-03 input.

### 5.1 Field map: `domain.Bar` → SADE `NormalizedObservation`

D01’s scientific inputs are **close, volume, and source event time**. Open/high/low are kept on the emission record and `observation_id` payload; they do not enter the D01 step.

| SADE field | QuanTRAM source | Notes |
| :--- | :--- | :--- |
| `entity_id` | `Bar.Symbol` | Uppercase ticker. |
| `event_time` | parse `Bar.SourceTimestamp` to UTC unix seconds | Verbatim provider text; P-03 assigns no session semantics. |
| `receive_time` | `Bar.ReceiptTime` | Operational only. Must not enter D01 equations. |
| `sequence_id` | per-symbol count of **accepted** finalized steps, starting at 0 | Replaces SDX `source_row_index`. |
| `price` | `Bar.Close` | Same as `SourceRowNormalizer`. |
| `volume` | `Bar.Volume` | |
| `session` | `"UNKNOWN"` | SADE already treated this as a placeholder, not SDX truth. |
| `source_quality` | `1.0` when model-eligible | Do not invent a second quality scale; P-02 already gated. |
| `bid` / `ask` | nil | Unchanged. |

`data_valid=true` in SADE was an input assumption, not source data. P-02 eligibility replaces that assumption.

### 5.2 `physical_row` and `source_row_index`

SADE hashes `physical_row` into `observation_id` and passes `physical_row - 2` as `sequence_id`. That `+2` is a **CSV / frozen-emitter compatibility shim**, not market physics.

| Mode | Rule |
| :--- | :--- |
| Live P-02 | `sequence_id` = accepted-step index. Identity for provenance is `market_snapshot_id` plus symbol and `interval_start`. Do not require CSV row numbers. |
| Offline equivalence vs Unit Run 001 | Apply `physical_row = source_row_index + 2` **only** so SHA-256 `observation_id` can be compared to the frozen Python run. |

## 6. Scientific box (what to port)

Port the **equations and operation order**, not the Python package layout or the unbounded diagnostics.

| SADE module | Role | Go notes |
| :--- | :--- | :--- |
| `d01/v02/*` (23 files) | Markovian state update | No window. One `RuntimeState` per symbol. |
| `d02/v02` | Return shape from DMO/FMO | 8 forward samples. |
| `d04` capturability | H, Q_G, Q_S, Q_R, C | `structural_quality` must be `Pow(x, 1.0/3.0)`, **not** `Cbrt`. |
| `adaptive_emitter` | 15-deque, decision predicate, position LONG/SHORT | `CONTEXT_LENGTH = 15`. First 15 steps: `INITIALIZING`, no decision. |
| `scientific_baseline.py` | Rule / code fingerprints | Copy constants as model-version identity. |
| `adaptive_pipeline`, `sdx_client`, CLI, CSV writers | Orchestration / SDX | **Do not port.** P-03 host replaces them. |

Do **not** port into the hot path:

- Unbounded `emissions`, `adaptation_audit`, `feedback_audit`, `trace_records`, `_rows` (SADE already has a retain_diagnostics opt-in; Go production keeps counters only).
- Pricing `pipeline.py` full-history refits (O(n²)). If pricing is added later, compute **only the current index**.
- JSON+SHA-256 of the entire emission four times per bar as a requirement. Keep `market_snapshot_id` from P-02; hash a small identity payload if an `observation_id` is still needed.

Warm-up: after 15 accepted finalized bars the 16th can be `ACTIONABLE`. That is ~16 minutes of eligible IEX prints, independent of the 64-bar observe window.

## 7. Output contract (P-03 → later P-05)

P-03 produces either a skip or a versioned decision. Suggested mapping from an ACTIONABLE emission:

| DecisionVector field | Source |
| :--- | :--- |
| `side` | BUY → buy/long, SELL → sell/short, HOLD → hold/flat |
| `confidence` | D04 `C` (capturability) |
| `quality` / regime | H, Q_G, Q_S, Q_R, path_direction, status |
| `market_snapshot_id` | P-02 bar |
| `signal_id` | New ID per accepted step (even INITIALIZING may record a skip id) |
| `decision_id` | New ID only when a DecisionVector is emitted (ACTIONABLE) |
| `model_version` | Rule + implementation fingerprints + Go module version |
| `bar_interval_start` | `Bar.IntervalStart` |

HOLD is a decision, not a skip. Skip means “P-03 did not evaluate” (gate, timeout, INITIALIZING, error).

Deadline (process model OP-05, 1-minute bars): **200 ms** from finalized-bar accept to DecisionVector or skip. On timeout, skip; do not enqueue a backlog of stale bars (depth 1–2).

## 8. State, scale, and failure

Per symbol, retain only:

- D01 `RuntimeState` (level, velocity, acceleration, half-life, parameters)
- deque of 15 completed context records
- `position_state`, `previous_decision`, `completed_count`, `last_interval_start`
- diagnostic **counters**, not histories

That is the 10–20 KB/channel bound from the investigation. Reset the engine on process start and on an explicit session reset later (DI-04). A mid-day P-02 gap that clears `infer` does **not** rewind D01; it only stops new evaluations until continuity returns. Document that as “pause, don’t reset,” unless a later calendar rule says otherwise.

Failure domain:

- Feed `infer=false` → skip, host stays up.
- Engine panic/error on one symbol → skip that bar, isolate that symbol, do not stop other symbols or P-02.
- Host reports `model` component health separately from `marketfeed` / `ingestion`.

## 9. Equivalence (MV-01 for this increment)

A live IEX decision is not “the SADE model” until the Go box matches Python on a **frozen OHLCV sequence**.

Harness (no SDX):

1. Read the same 100 AAPL rows Unit Run 001 used (or `SADE/output/unit_runs/001` inputs reconstructed as bars).
2. Map each row to `domain.Bar` (final, COMPLETE, not backfilled).
3. Step Python `AdaptiveEmitter` and Go `AdaptiveEngine` on that sequence.
4. Compare per observation: `H`, `Q_G`, `Q_S`, `Q_R`, `C`, `path_direction`, `position_decision`, `status`.

Tolerances: bitwise for `sqrt` and integer/median-of-15; small ulp band for `exp` / `log1p` / `pow` (libm vs Go). `position_decision` must match exactly on the frozen run; if a 1-ulp `C` vs median(C₁₅) flip appears, treat it as a blocker and inspect the predicate, do not paper over it.

Do **not** require live IEX prices to match Unit Run 001. That run is CSV history.

## 10. Explicitly out of increment

- P-05 risk, P-06 paper orders, ledger, benchmark
- Completing APTF→SADE RK45 migration, joining adaptive output to PriceEngine, or treating `expm` as the destination producer
- Databento, SIP, tick aggregation
- Azure / AKS sharding
- Dashboard decision views until `ModelService` is registered (optional northbound can follow the first green equivalence test)

## 11. Change log

| Date | Change |
| :--- | :--- |
| August 31, 2026 | Initial P-03 design: Go adaptive box, P-02 finalized input, no SDX, no adaptive Python sidecar. |
| August 31, 2026 | Recorded APTF→SADE incomplete lift: intended decision is adaptive + PriceEngine on RK45 trajectories; Go analysis predates finished RK45 Python migration. |
| August 31, 2026 | RK45 recorded as the scientific authority (golden nugget) for the PriceEngine decision; expm is an optional match to that cover, not a substitute definition. |
