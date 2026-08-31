# QuanTRAM P-03 Adaptive Model Host — Design

**Date:** August 31, 2026  
**Status:** Scientific port (Phases A–C) complete — Go adaptive box matches Unit Run 001 offline. Live host integration (Phase D) is **not** complete until finalized delivery, startup, deadline/atomicity, and DecisionEvent contracts below are implemented.  
**Scope:** Collocated Go adaptive host that consumes P-02 finalized bars.  
**Parents:** [Process Model](QuanTRAM_PROCESS_MODEL_082926.md), [P-02 Data Quality](QuanTRAM_INGESTION_P02_DATA_QUALITY_083126.md)  
**Review:** [P03_feedback_083126.md](../../P03_feedback_083126.md) (incorporated 31 Aug)  
**Scientific source:** `C:\Users\chino\SADE` (Python Adaptive Pipeline D01 → D02 → D04 → emitter)  
**Language evidence:** [SADE Go refactorability investigation, 2026-08-27](file:///C:/Users/chino/SADE/docs/investigations/SADE_GO_REFACTORABILITY_AND_SCALED_RUNTIME_INVESTIGATION_2026-08-27.md)  
**Implementation plan:** [P-03 Implementation](QuanTRAM_P03_IMPLEMENTATION_083126.md)

## 1. Purpose

P-03 is the next QuanTRAM process after the P-02 data gate. It turns a **finalized, quality-eligible 1-minute bar** into exactly one **DecisionEvent**: a BUY/SELL/HOLD decision or a typed skip. It never sends orders. D01 is recursive, so a silently dropped bar is a permanently wrong state trajectory, not a missed tick.

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
| Live input | **P-02 model-consumer path only** (not the lossy observe `SubscribeFinalized`). Eligible: finalized, `COMPLETE`, not backfilled, per-symbol adjacency, and the Phase 0 `infer` policy in §5.4. |
| SDX / SADE `sdx_client` | **Do not use** on the live path. SDX remains an offline CSV tool in another repo. |
| Adaptive mathematics | **Go black box** in this process (`internal/adaptive`). Stdlib scalar math; no NumPy/SciPy on this path. |
| Python adaptive wrapper / `ModelInferenceService.Predict` | **Not required** for adaptive. A stateful Python wrapper is strictly worse (affinity + serialize-every-bar). Amends process-model P-04 for this increment. |
| RK45 / pricing | **Deferred as code, not cancelled.** Target: PriceEngine decides on RK45 trajectories, joined to the adaptive model. Transport: Go holds bars/F4; Python RK45 is a **called** solver (or later Go equivalent proved against it), not a gRPC client of P-02. |
| Persistence of bars | Still not required. P-03 holds **per-symbol scientific state** only (D01 runtime + 15-context). |
| CSV in QuanTRAM | Allowed only as an **offline equivalence harness** that maps rows into `domain.Bar` and calls the same Go engine. |
| Orders / risk | Out of scope. `DecisionEvent` is the stop line. Emitter LONG/SHORT is `emitter_position_state` (scientific context), not an account position. |

Process-model amendment (this increment): P-03 **owns** the proved adaptive path in Go. P-04 is reserved for the unfinished RK45 / PriceEngine join (Python solver called by Go). Do not implement `ModelInferenceService` until that increment. Do not treat Finding 001 `expm` as the final PriceEngine producer until RK45 migration and equivalence are closed.

## 4. Runtime topology (local Phase 0)

```text
P-01 Alpaca ──► P-02 pipeline
                    │
                    │ FinalizedBarConsumer (model path; no silent drop)
                    │ observe/dashboard stream remains lossy and separate
                    ▼
              P-03 Model Host
                    │  one keyed worker per symbol (no concurrent Step)
                    │  AdaptiveEngine transactional Step
                    ▼
              DecisionEvent  (decision | typed skip)
                    │
                    ✗  not connected to P-05 / P-06 in this increment
```

Collocation rule: P-03 calls P-02 through a **Go interface**, not loopback gRPC. Proto types stay in `internal/server` and are added only after domain events are frozen.

Symbols are independent. One owner/worker per symbol: no concurrent `Step` on the same symbol; a slow symbol must not consume another symbol’s deadline. Do not share D01 state across symbols.

## 5. Input contract (P-02 → P-03)

P-03 may **commit** an engine step only when **all** of these hold:

1. The bar arrived on the **model** finalized path (`is_final=true`).
2. The Phase 0 `infer` policy in §5.4 allows evaluation.
3. `bar.ModelEligible()` — `COMPLETE`, not backfilled.
4. Per-symbol `IntervalStart` is **exactly one minute after** the last accepted step (adjacency), except the first accepted bar after a documented cold start.

Otherwise emit a typed skip and leave scientific state unchanged. Do not reuse a previous decision as if it were new (OP-05).

The Next.js viewer remains on the **observe** stream. It is not the P-03 input.

### 5.1 Finalized delivery guarantees (critical)

`Pipeline.SubscribeFinalized` today is a depth-2 **drop-oldest** fan-out. Clearing `infer` only happens if the *second* send also fails. That is acceptable for a viewer. It is **not** acceptable for recursive D01: evaluating N+2 without N+1 changes every later state.

Phase 0 P-03 must use a distinct consumer (names follow repo style):

```text
FinalizedBarConsumer
  SubscribeModelBars(buffer)  // model contract, not observe
  Unsubscribe
  ReadinessFor(symbol)        // Phase 0 may still consult global infer; see §5.4
  Window(symbol, limit)
```

Required behavior:

- Never **silently** drop an eligible finalized bar destined for P-03.
- Each accepted step implies a per-symbol expected next `IntervalStart` (last + 1 minute).
- Overflow, a missing minute, or a non-adjacent interval marks that symbol **discontinuous**.
- While discontinuous, P-03 must not evaluate later bars for that symbol until an explicit reset (Phase 0) or a later documented rebuild.
- A small blocking mailbox at 1-minute cadence is allowed if it is isolated from the WebSocket reader and bounded by the 200 ms host deadline.
- The observe/dashboard stream may stay best-effort.

Acceptance: stall the P-03 consumer, publish three finalized bars into a depth-2 transport, and prove either all three are processed in order **or** the symbol enters a named discontinuous/gated state before any later bar is evaluated.

P-02 observe `Subscribe` / `SubscribeFinalized` may remain lossy. Do not wire the live host to that path.

### 5.2 Startup and rebuild (Phase 0: cold start)

`SubscribeFinalized` does not replay the current window. **Phase 0 mode is cold start:**

- Subscribe first; ignore historical window contents.
- Initialize only from newly finalized eligible bars.
- Report `INITIALIZING n/16` (15 context bars, first ACTIONABLE on the 16th accepted eligible step).
- Every process restart resets all model state and repeats warm-up.
- Host restart is **not** treated as a new market session (no calendar; see §8.1).

Deterministic rebuild (subscribe, watermark, finalized window, overlap dedup, then live) is **deferred**. Do not seed from reconstructed bars for live evaluation. Do not leave the mode implicit: health must show `cold`.

### 5.3 Field map: `domain.Bar` → observation

D01’s scientific inputs are **close, volume, and model event time**. Open/high/low stay on provenance; they do not enter the D01 step.

**Canonical event time is `Bar.IntervalStart`** (already UTC, used by P-02 for order, continuity, and dedup). Do not reparse `SourceTimestamp` for D01. Keep `SourceTimestamp` on the DecisionEvent for provenance only. The offline fixture adapter builds `IntervalStart` from the frozen CSV timestamp before the same mapper.

| SADE field | QuanTRAM source | Notes |
| :--- | :--- | :--- |
| `entity_id` | `Bar.Symbol` | Uppercase ticker. |
| `event_time` | `Bar.IntervalStart` as UTC unix seconds | Not `SourceTimestamp`. |
| `receive_time` | `Bar.ReceiptTime` | Operational only. Must not enter D01 equations. |
| `sequence_id` | per-symbol count of **accepted** committed steps, starting at 0 | Replaces SDX `source_row_index`. |
| `price` | `Bar.Close` | Same as `SourceRowNormalizer`. |
| `volume` | `Bar.Volume` as float64 | Reject if conversion is non-finite. |
| `session` | `"UNKNOWN"` | Placeholder until DI-04. |
| `source_quality` | `1.0` when model-eligible | P-02 already gated. |
| `bid` / `ask` | nil | Unchanged. |

`data_valid=true` in SADE was an input assumption. P-02 eligibility replaces it.

### 5.4 Per-symbol vs global `infer` (Phase 0: global)

P-02 `Readiness().Infer` is **global**: every configured symbol must have a contiguous complete live head. One stale name pauses AAPL, MSFT, and NVDA together.

**Phase 0:** keep that global gate. Document it: one symbol failing continuity **pauses evaluation for all engines**; scientific state is **not** reset. Add a test that proves pause-not-reset.

**Intended evolution:** `ReadinessFor(symbol)` so engines fail and recover independently. Do not implement per-symbol infer in P-02 as a silent change without tests.

### 5.5 Engine input/output validation

At the P-03 boundary, after P-02 checks, still reject and skip (`INVALID_INPUT` or `ENGINE_ERROR`) with **no commit** if:

- close/volume/derived values are non-finite
- `IntervalStart` does not advance, or `dt` is `<= 0` after the first step
- half-life or H / Q_G / Q_S / Q_R / C is non-finite or outside its documented domain
- confidence is outside `[0, 1]`

Do not invent a maximum stock price. Validate mathematical domains and float safety only.

### 5.6 `physical_row` and `source_row_index`

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
| `adaptive_emitter` | 15-deque, decision predicate, `emitter_position_state` LONG/SHORT | `CONTEXT_LENGTH = 15`. First 15 committed steps: `INITIALIZING` skip. |
| `scientific_baseline.py` | Rule / code fingerprints | Copy constants as model-version identity. |
| `adaptive_pipeline`, `sdx_client`, CLI, CSV writers | Orchestration / SDX | **Do not port.** P-03 host replaces them. |

Do **not** port into the hot path:

- Unbounded `emissions`, `adaptation_audit`, `feedback_audit`, `trace_records`, `_rows` (SADE already has a retain_diagnostics opt-in; Go production keeps counters only).
- Pricing `pipeline.py` full-history refits (O(n²)). If pricing is added later, compute **only the current index**.
- JSON+SHA-256 of the entire emission four times per bar as a requirement. Keep `market_snapshot_id` from P-02; hash a small identity payload if an `observation_id` is still needed.

Warm-up: after 15 accepted finalized bars the 16th can be `ACTIONABLE`. That is ~16 minutes of eligible IEX prints, independent of the 64-bar observe window.

## 7. Output contract — `DecisionEvent`

Every finalized input that the host considers produces **exactly one** terminal `DecisionEvent`. Prefer a decision-or-skip union. Do not put `skipped=true` on a vector that also has `side=BUY`.

```text
DecisionEvent
  event_id              one per terminal outcome (immutable)
  signal_id             one per evaluated model step (INITIALIZING included)
  decision_id           only for BUY / SELL / HOLD
  symbol, interval_start, market_snapshot_id
  accepted_sequence
  received_at, completed_at, latency
  model_version, schema_version
  pre_state_hash, post_state_hash
  outcome = decision | skip
```

**Decision** (model status `ACTIONABLE`):

| Field | Type / range | Required |
| :--- | :--- | :--- |
| `side` | BUY, SELL, HOLD | yes |
| `confidence` / capturability `C` | `[0, 1]` | yes |
| `H` | 0 or 1 | yes |
| `Q_G`, `Q_S`, `Q_R` | `[0, 1]` | yes |
| `path_direction` | UPWARD, DOWNWARD, FLAT | yes |
| `model_status` | ACTIONABLE | yes |
| `emitter_position_state` | FLAT, LONG, SHORT | diagnostic only |

HOLD is a **decision**. `emitter_position_state` is frozen SADE context, not P-05/P-08 position.

**Skip** (no `decision_id`):

| Reason | Typical cause |
| :--- | :--- |
| `INFER_OFF` | Global Phase 0 infer false |
| `NOT_MODEL_ELIGIBLE` | Partial, reconstructed, not complete |
| `INITIALIZING` | Accepted step, context not yet 15 |
| `DUPLICATE_OR_REGRESSION` | `IntervalStart` ≤ last |
| `INPUT_GAP` | Non-adjacent minute |
| `QUEUE_OVERFLOW` | Model mailbox would drop |
| `TIMEOUT` | Step exceeded deadline; state not committed |
| `INVALID_INPUT` | Non-finite / domain check |
| `ENGINE_ERROR` | Scientific failure |
| `ENGINE_PANIC` | Recovered panic |
| `STATE_DISCONTINUOUS` | Symbol paused after gap/overflow |

`INITIALIZING` is an evaluated non-actionable outcome (has `signal_id`, no `decision_id`). Gate skips may omit `signal_id` only if the engine never ran; still require `event_id`.

ID invariants: unique within the process instance. A duplicate input yields a new skip event (`DUPLICATE_OR_REGRESSION`), not a replay of the prior decision. Upstream IDs on a decision must survive later P-05/P-06 unchanged.

Duplicate/contradictory proto (`side` + `skipped`) is forbidden. Freeze these domain types **before** Phase E proto.

### 7.1 State transaction and deadline

`AdaptiveEngine.Step` is a transaction:

```text
validate bar and time
copy / read immutable pre-state
calculate D01 → D02 → D04 → emitter
validate outputs
if wall time > deadline: discard work, emit TIMEOUT, do not commit
else atomically commit post-state
emit one DecisionEvent
```

- Commit is all-or-nothing. Timeout, error, and panic must not leave a half-updated engine.
- `context.WithTimeout` around a synchronous `Step` does **not** stop the CPU. Compute on a copy (or commit only after success). Do not leave a goroutine mutating committed state after TIMEOUT.
- No second `Step` for the same symbol concurrently.
- Deadline: **200 ms** from accept to outcome. A breach is a **model-health defect**, not routine shedding. Do not use a lossy queue as flow control.

Acceptance: a slow step that mutates after the deadline must yield exactly one `TIMEOUT`, no Decision for that interval, unchanged committed state hash.

### 7.2 Model version

`model_version` is a printable SHA-256 over a canonical JSON object (sorted keys) containing:

- schema version string (e.g. `quantram.adaptive.v1`)
- SADE baseline rule fingerprint and implementation fingerprint
- every D01/D02/D04/emitter constant that can change a decision (config structs, `CONTEXT_LENGTH`, clamps, decision predicate)
- operation-order identity (named step list)
- Go module path + version; git commit if available
- if the working tree is dirty or commit is unknown: suffix `+dirty` or `+unknown` — still valid, but live evidence must show it

Change the fingerprint when an equation, order, clamp, threshold, or warm-up rule changes. Do not include build-host, timestamp, or race-detector flags.

## 8. State, health, session

Per symbol, retain only:

- D01 `RuntimeState`
- deque of 15 completed context records
- `emitter_position_state`, `previous_decision`, `completed_count`, `last_interval_start`
- counters, not histories

Optional: bounded diagnostic ring **only** when explicitly enabled for local validation.

### 8.1 Session reset (Phase 0)

DI-04 is open. Until a calendar exists:

- process restart resets all model state (cold start)
- `infer=false` **pauses** without reset
- **no** automatic daily or “market open” reset from the local clock
- any operator reset is explicit and must emit an audit/skip event (`STATE_DISCONTINUOUS` or a dedicated reset reason)
- extended-hours bars are whatever P-02/Alpaca deliver; P-03 does not invent RTH filters

### 8.2 Model health

Per-symbol status: `off` | `cold` | `initializing` | `ready` | `paused` | `discontinuous` | `error`.

Also expose: accepted-step count, warm-up `n/16`, last received/accepted interval, last DecisionEvent time and outcome, last skip reason, consecutive errors/timeouts, mailbox depth, model version, pre/post hashes when diagnostics are on.

Aggregate:

| Component | When |
| :--- | :--- |
| **Healthy** | Host enabled; no discontinuous/error symbol; last eligible input finished within deadline |
| **Degraded** | Any symbol initializing, paused, lagging, or recently skipped for operational reasons |
| **Unavailable** | Host failed to start, baseline/config invalid, or every enabled engine is terminally failed |

Initialization is not an error; it must be visible. Panic/error on one symbol must not stop other symbols or P-02.

### 8.3 Provenance and replay (Phase 0 boundary)

`market_snapshot_id` + `model_version` are necessary and **not** sufficient for DI-07. D01 depends on prior state and the 15-context.

Phase 0 **can** reproduce: frozen Unit Run 001 fixtures (input SHA-256 + expected output + SADE revision), plus per-event `accepted_sequence` and pre/post state hashes.

Phase 0 **cannot** claim production replay of a live IEX session after process restart. Durable decision-input recording belongs to the later recording plane. Do not advertise MV-01 closed for live trading.

## 9. Equivalence (MV-01 for this increment)

A live IEX decision is not “the SADE model” until the Go box matches Python on a **frozen OHLCV sequence**.

Required fixtures (checked in):

- input CSV/JSON with origin note and SHA-256
- expected output with SADE revision / fingerprints
- per-field tolerance table (not only “small ulp”)
- exact match for `model_status`, `path_direction`, `side` / `position_decision`, and decision counts
- first-divergence report: symbol, sequence, field, expected, actual, pre-step hash
- deterministic repeated runs; `go test -race` on the engine

If a 1-ulp `C` vs median(C₁₅) flips `side`, treat it as a blocker; do not widen tolerance.

Do **not** require live IEX prices to match Unit Run 001.

## 9.1 Definition of done (adaptive increment)

- Frozen Unit Run 001 matches under the tolerance matrix.
- Every considered finalized input produces exactly one DecisionEvent.
- Partial, reconstructed head, duplicate, regressive, or unknown-quality bars do not mutate committed state.
- A missing eligible bar cannot pass silently; the symbol pauses or is discontinuous.
- Per-symbol work is ordered, isolated, race-free, and bounded.
- Timeout / error / panic / shutdown cannot partially commit state.
- Cold-start warm-up is deterministic and visible (`INITIALIZING n/16`).
- Global `infer` Phase 0 policy is explicit and tested (pause, not reset).
- Model version and `market_snapshot_id` accompany every evaluated outcome.
- Health distinguishes off / initializing / ready / paused / discontinuous / failed.
- `QUANTRAM_MODEL=off` leaves ingestion unchanged.
- `go test ./...` and `go test -race ./...` pass.
- A regular-hours live run crosses warm-up and records decisions/skips with latency — pass on domain fields (`model_status`, `side` or `skip_reason`), not a log substring.
- No P-03 output is connected to orders.

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
| August 31, 2026 | Incorporated `P03_feedback_083126.md`: model delivery (no silent drop), cold start, global infer Phase 0, transactional Step, DecisionEvent, IntervalStart event time, health/replay/DoD. |
| August 31, 2026 | Phases A–C landed: collocated Go D01→D02→D04→emitter, `DecisionEvent` domain types, Unit Run 001 equivalence. Phase D0 / host / proto still open. |
