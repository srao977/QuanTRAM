# QuanTRAM P-04V Volume Engine — Implementation Design and Implementation Record

**Title:** QuanTRAM P-04V Volume Engine — Implementation Design and Implementation Record  
**Date:** 2026-09-06  
**Status:** IMPLEMENTED AND VALIDATED THROUGH PHASE G. This file is the implementation design **and** the A–G implementation record. StageTransition Volume publication and Process Model file reconciliation remain **DEFERRED / NOT YET AUTHORIZED**.  
**Purpose:** Record how P-04V was designed and then implemented in Go: package layout, bounded `VolumeState`, consume/prepare/commit/emit, frozen Volume Feature and Volume Interpretation mathematics, protobuf contract, ModelHost join, tests, and remaining deferred work.  
**Scope:** Implementation design plus completed A–G chronology. Earlier proposed sections are retained for engineering provenance. Where a proposal conflicts with [Current Validated Implementation](#current-validated-implementation), the current validated implementation is authoritative. This document does not change Volume/Price/Adaptive science, StageTransition, or the Process Model file.  
**Parents:** [Volume Engine scientific/architectural design](../design/QuanTRAM_VOLUME_ENGINE_090526.md), [Process Model](../design/QuanTRAM_PROCESS_MODEL_V1_082926.md) (filename as of this reconciliation; **contents not edited**), [APTF Volume forensic investigation](../investigations/QuanTRAM_APTF_VOLUME_ENGINE_GO_REFACTORABILITY_INVESTIGATION_2026-09-05.md)  
**Implementation precedents:** [P-03 Adaptive Model Host](../design/QuanTRAM_P03_ADAPTIVE_MODEL_HOST_083126.md), [P-03 Implementation](../design/QuanTRAM_P03_IMPLEMENTATION_083126.md), [P-04 Price Engine](../design/QuanTRAM_P04_PRICE_ENGINE_090226.md), [P-04 Implementation](../design/QuanTRAM_P04_IMPLEMENTATION_090226.md)  
**Filename:** `docs/implementations/QuanTRAM_P04V_VOLUME_ENGINE_IMPLEMENTATION_090526.md` (renamed 2026-09-06 from `QuanTRAM_P04V_VOLUME_ENGINE_090526.md` so it is not confused with the scientific design document).  
**QuanTRAM baseline inspected:** `07417d9c85799949cd3b173067795905513af173` (`quantram-stage-transition-v1.1-validated-2026-09-04`)  
**Historical executable authority:** APTF commit `ae0dacb2e02c5b80c82f6662d1a3c6863f4b989a`

## Current Validated Implementation

As of **2026-09-06**, P-04V is no longer a proposal. The following is the authoritative runtime result through Phase G.

| Item | Result |
|---|---|
| `internal/domain/volume.go` | Exists — domain Volume types and publication envelope (`EventID`, `AcceptedSequence`) |
| `internal/volume` | Exists — science + Engine |
| Bounded per-entity `VolumeState` | Implemented |
| Feature mathematics | Implemented (`V_RAW`, `V_N`, windows, `interval_mean_vn`, `G_V`) |
| Derivative mathematics | Implemented (positional 3, actual `IntervalStart` minutes) |
| Interpretation | Implemented (`V_INTERVAL_B10_C2`; Indicator + `raw_color`) |
| `Engine.PrepareStep` / `Commit` | Implemented |
| Phase F frozen equivalence | Historical FAIL on Indicator / transition (309 / 55,199) |
| Phase F-R confirmation repair | PASS — session-sliced 014C Indicator / transition **55,199 / 55,199** |
| Test010 → Test014C reconciliation | Outcome A; scientific readiness established |
| Protobuf Volume contract | Implemented on `ModelService` |
| `ModelService.StreamVolumeEvents` | Implemented; streams Host-produced events |
| Phase G ModelHost join | Implemented |
| Volume Engine ownership | One `volume.Engine` per keyed symbol worker |
| Second `SubscribeModelBars` | None |
| Second Volume mailbox / science goroutine | None |
| Same published eligible Bar after common gates | Yes |
| Volume commit vs A+P `commitA && commitP` | Independent |
| Volume failure isolation | `volDisc` + nested recover; A+P not rolled back |
| `ResetSymbol` | Rebuilds Volume engine; clears `volSeq` / `volDisc` |
| Automatic date/session reset | None |
| `QUANTRAM_VOLUME` | **Does not exist.** SUPERSEDED PRE-IMPLEMENTATION PROPOSAL |
| When Volume is present | Whenever Adaptive Host exists (`QUANTRAM_MODEL=adaptive`) |
| Price configuration | Independent `QUANTRAM_PRICING` |
| RK45 / Volume EXPM / P/V fusion / BUY/SELL/HOLD | None |

**Phase G topology (actual):**

```text
Pipeline.fanoutModel
    → one SubscribeModelBars
    → keyed worker inbox
    → common gates
    → isolated processVolume (independent Volume commit)
    → Adaptive PrepareStep
    → Price PrepareStep
    → existing Adaptive+Price joint commit
```

`worker.lastAccepted` remains the Adaptive+Price joint scientific-commit cursor. Volume `accepted_sequence` is the 1-based successful Volume-commit count for that worker/engine lifetime.

**DEFERRED / NOT YET AUTHORIZED:** StageTransition Volume publication; Process Model file reconciliation; Snapshot/Persistence/Mongo/Aperture; P/V fusion; trading verbs.

## Executive Summary

P-04V is **one collocated Go Volume Engine**. It is a scientific sibling of P-03 Adaptive and P-04 Price Engine. It consumes the **same accepted eligible `domain.Bar`** already delivered through P-01/P-02 and the existing `SubscribeModelBars` keyed worker. It does not create a second market subscription, mailbox, or feed.

P-04V owns bounded per-entity **`VolumeState`** and performs both scientific responsibilities inside that engine:

1. **Volume Feature Mathematics** — `V_RAW`, `V_N` (`ROLLING_MEDIAN_RATIO_15`), `V1`/`V2` (positional window 3, actual `IntervalStart` minutes), `interval_mean_vn` (positional mean 15), `predicted_next_V_N = V_N` (`VOLUME_POINT`).
2. **Volume Interpretation Mathematics** — frozen historical APTF configuration `V_INTERVAL_B10_C2` (thresholds 0.9/1.1, confirmation 2, epsilon `1e-12`), confirmation/hysteresis, phase, INVALID.

There is no QuanTRAM Volume Policy service, layer, or process. Historical APTF “policy” names remain provenance only.

**Prepare/commit recommendation (from inspected host facts, corrected 2026-09-05):** P-04V must **not** join the existing Adaptive+Price both-or-neither transaction. Volume failure must not roll back P-03/P-04. Volume’s processing opportunity must **not** depend on Adaptive or Price scientific commit success. After common host gates, the same published eligible Bar is offered to Volume independently. Adaptive+Price `lastAccepted` is a **scientific-commit** cursor, not the P-02 publication boundary.

Current physical execution is **not** concurrent Adaptive/Price science. It is: ingestion publication decoupled from science, then **deterministic sequential** scientific execution inside one keyed worker. Phase G joined P-04V to that worker after common gates and **before** Adaptive/Price prepare, solely so an A/P panic cannot deny Volume its opportunity. Do not add a second mailbox merely to claim asynchrony.

Earlier sections describing proposed implementation choices are retained for engineering provenance. Where a proposal conflicts with [Current Validated Implementation](#current-validated-implementation), the current validated implementation is authoritative.

## Module / System Overview

```text
                         P-01 MARKET FEED
                                |
                                v
                    P-02 INGESTION / DATA QUALITY
                                |
                      Accepted Eligible Bar
                                |
              +-----------------+-----------------+
              |                 |                 |
              v                 v                 v
       P-03 ADAPTIVE        P-04 PRICE       P-04V VOLUME
       MODEL HOST           ENGINE           ENGINE
              |                 |                 |
           CONSUME           CONSUME           CONSUME
              |                 |                 |
              v                 v                 v
       Adaptive State       Price State       VolumeState
              |                 |                 |
              v                 v                 v
       Adaptive Math        Price Math        Volume Math
              |                 |                 |
              v                 v                 v
            EMIT              EMIT              EMIT
              |                 |                 |
              v                 v                 v
       Adaptive Output      Price Output      Volume Output
```

This is **not** `P-03 → P-04 → P-04V`. P-04V does not consume Price Output or Adaptive Output.

Inspected live join point: `internal/modelhost/host.go` `Host.handle`. One keyed worker per symbol. After common gates, Phase G invokes isolated `processVolume`, then Adaptive `PrepareStep`, then Price `PrepareStep`, then existing `commitA && commitP`. Volume does **not** join the Adaptive+Price commit gate. No second `SubscribeModelBars`.

See [Accepted-Bar Publication and Scientific Consumption Invariant](#accepted-bar-publication-and-scientific-consumption-invariant).

Production package: `internal/volume` exists (2026-09-05 Phases A–E). `internal/adaptive/volume.go` is D01 `updateVolumeInfluence` and **must remain P-03-only**.

## Inputs

| Input | Source | Notes |
|---|---|---|
| Published eligible `domain.Bar` | Same P-02 model-consumer path (`SubscribeModelBars`) already used by P-03/P-04 | `Symbol`, `Volume` (`uint64`), `IntervalStart`, `SourceTimestamp`, `MarketSnapshotID`. Publication is `Pipeline.fanoutModel`, not Adaptive+Price commit. |
| Committed per-entity `VolumeState` | Owned by `volume.Engine` on that worker | Feature + interpretation substate |

P-04V does **not** consume `DecisionEvent`, `PriceEvent`, CSV/filesystem realtime input, APTF batch files, or a Volume mailbox.

Common host gates (infer off, not eligible, duplicate/regression vs host cursor, proven missing, pre-prepare timeout) apply to **all** siblings before prepare. Those are not Adaptive/Price scientific success. A Price or Adaptive prepare/commit failure is **not** a reason to withhold B_t from Volume.

## Outputs

First-class **Volume Output** as `domain.VolumeEvent`, published on `ModelService.StreamVolumeEvents`. No P-05 consumption. Color / Indicator is a Volume **activity band**, not price direction and not BUY/SELL/HOLD.

**HISTORICAL (2026-09-05 pre-contract):** this section originally said “no proto / no StreamVolumeEvents.” That is SUPERSEDED. The contract and Phase G stream are implemented.

## Parameters / Configuration

### Scientific constants (frozen; not env knobs)

| Identity | Frozen value | Class |
|---|---|---|
| Normalization | `ROLLING_MEDIAN_RATIO_15` | Volume Feature Science |
| Raw-volume window | 15 observations | Volume Feature Science |
| Derivative window | 3 observations | Volume Feature Science |
| Interval-mean window | 15 observations | Volume Feature Science |
| Projection | `VOLUME_POINT` (`predicted_next_V_N = V_N`) | Volume Feature Science |
| Time coordinate | `IntervalStart` unix ms / 60_000 | Volume Feature Science |
| Historical interpretation identity | `V_INTERVAL_B10_C2` | Volume Interpretation Science |
| Historical freeze label | `V_EMISSION_V0_1` | provenance |
| `state_source` | `INTERVAL_MEAN_V_N` | Volume Interpretation Science |
| `lower_threshold` | `0.9` | Volume Interpretation Science |
| `upper_threshold` | `1.1` | Volume Interpretation Science |
| `confirmation_observations` | `2` | Volume Interpretation Science |
| `epsilon` | `1e-12` | Volume Interpretation Science |

Do **not** expose these as `QUANTRAM_*` environment variables.

### Operational configuration (CURRENT, Phase G)

Inspected conventions: `QUANTRAM_MODEL=off|adaptive`, `QUANTRAM_PRICING=off|expm`, unknown values fail startup, pricing requires adaptive.

| Variable | Current meaning |
|---|---|
| `QUANTRAM_MODEL` | `adaptive` constructs Adaptive Host. Volume engines are allocated with that Host. `off` → no Host → Volume not wired. |
| `QUANTRAM_PRICING` | Independent Price opt-in (`off` / `expm`). **Not** required for Volume. |
| `QUANTRAM_MODEL_DEADLINE` | Shared worker deadline; Volume prepare is in the same handle, not a second mailbox. |
| `QUANTRAM_VOLUME` | **Does not exist.** |

Volume is on whenever Adaptive Host exists. There is no Volume-only env flag.

#### SUPERSEDED PRE-IMPLEMENTATION PROPOSAL — `QUANTRAM_VOLUME`

The 2026-09-05 implementation design proposed `QUANTRAM_VOLUME=off|on` (unknown fail startup; `on` requires `adaptive`). Phase G did **not** implement that flag. Do not treat the old table as operator guidance.

## Assumptions

- Phase 1 Volume design and Phase 2 Process Model win over historical APTF runtime/batch mechanics.
- Inspected host `PrepareStep`/`Commit` and keyed-worker ownership are the QuanTRAM realtime precedent.
- “Accepted Bar” is **not** one concept in current code. P-02 window accept, P-02 model-path publication, and host `lastAccepted` (Adaptive+Price commit) are distinct. P-04V consumption is keyed to **published eligible B_t**, not to Adaptive+Price commit.
- `domain.Bar.Volume` is `uint64`; conversion to scientific float is `float64(bar.Volume)`, matching `internal/adaptive/mapper.go`.
- `IntervalStart` minutes match P-04: `float64(t.UTC().UnixMilli()) / 60_000.0` (`internal/pricing/mapper.go`). Inspection found no incompatibility.
- APTF frozen artifacts are equivalence authorities, not QuanTRAM runtime inputs. They are **not** currently vendored in this repository.
- StageTransition V1.1 remains frozen. Future P-04V publication is deferred / non-blocking / outside V1.
- P-05 join of Volume Output remains undesigned.

## Exclusions

Outside remaining P-04V V1 work (not rejected forever):

- StageTransition Volume StageID / equality / publication (**DEFERRED / NOT YET AUTHORIZED**)
- Process Model file reconciliation (**DEFERRED / NOT YET AUTHORIZED**)
- Snapshot, Persistence, MongoDB, Aperture
- P/V fusion, APTF 015 BUY/SELL/HOLD, APTF 016
- Decision Neural Network, Forum, Meaning Matrix, voting/quorum
- Color-age / `EmissionIntervalizer`
- Independent Volume Interpretation State reset
- APTF `date:session` reset
- RK45, SciPy, Python runtime
- Candidate re-selection or threshold retuning
- Named-entity production logic
- Moving or replacing P-03 D01 `updateVolumeInfluence`
- Sharing P-04 Price scientific state or Price derivative window
- Downstream order/execution changes
- Adding a `QUANTRAM_VOLUME` flag (proposal SUPERSEDED)

**No longer exclusions of existence:** Go Volume science, proto Volume contract, `StreamVolumeEvents`, ModelHost Phase G join.

---

## 1. Authoritative source hierarchy

| Rank | Authority | Wins for |
|---|---|---|
| 1 | [QuanTRAM_VOLUME_ENGINE_090526.md](../design/QuanTRAM_VOLUME_ENGINE_090526.md) | Frozen Volume mathematics, lifecycle, maturation vs INVALID, ontology |
| 2 | [QuanTRAM_PROCESS_MODEL_V1_082926.md](../design/QuanTRAM_PROCESS_MODEL_V1_082926.md) | Process identity P-04V, sibling topology, same accepted Bar. File contents not reconciled in this pass. |
| 3 | Inspected QuanTRAM Go (`internal/modelhost`, `internal/pricing`, `internal/adaptive`, `internal/domain`, `internal/config`) | Host join, prepare/commit, keyed ownership, config style |
| 4 | Frozen APTF artifacts + [investigation](../investigations/QuanTRAM_APTF_VOLUME_ENGINE_GO_REFACTORABILITY_INVESTIGATION_2026-09-05.md) | Equivalence oracles and historical observe semantics |
| 5 | P-03/P-04 design and implementation documents | Precedents only; do not copy Price/Adaptive science |

Where APTF batch/session/replay mechanics differ from QuanTRAM realtime architecture: **preserve the Volume mathematics** and **use QuanTRAM realtime consume/state/prepare/commit/emit**.

## 2. Process identity and topology

Approved identity: **P-04V — Volume Engine**. P-05 through P-10 are not renumbered.

P-04V is not a child of P-04, not a consumer of Price Output, and not a Volume Policy process.

Canonical terms: P-04V Volume Engine, `VolumeState`, Volume Feature State, Volume Interpretation State, Volume Mathematics, Volume Feature Mathematics, Volume Interpretation Mathematics, Volume Output.

Historical/provenance only: APTF Volume Policy, `V_INTERVAL_B10_C2`, `V_EMISSION_V0_1`, historical `VolumePolicyState`.

## 3. Go package / file inventory

**CURRENT (2026-09-06):** `internal/volume` and `internal/domain/volume.go` exist. Phase G added `internal/modelhost/volume.go` and `internal/modelhost/volume_host_test.go`. Proto Volume RPC is live on `internal/server/volume.go`.

The original 2026-09-05 list below is **HISTORICAL IMPLEMENTATION PLAN**. Compare it to what was actually created.

Recommended package: `internal/volume`.

Domain types live in `internal/domain` so host/tests can name Volume Output without importing proto. Domain must not import `gen/`. `internal/volume` must **not** import `internal/pricing` or `internal/adaptive` (except that host may call all three). Numerical primitive reuse is by **algorithm copy** in V1, not by importing Price science.

```text
internal/domain/volume.go
internal/volume/config.go
internal/volume/mapper.go
internal/volume/windows.go
internal/volume/median.go
internal/volume/features.go
internal/volume/linalg.go
internal/volume/derivatives.go
internal/volume/phase.go
internal/volume/interpretation.go
internal/volume/engine.go
internal/volume/fixture.go          (test helper only)
internal/volume/*_test.go
internal/volume/testdata/          (vendored later; not now)

Host/RPC wiring (implemented Phase G; no VolumeMode env):
internal/modelhost/host.go         worker.volume join
internal/modelhost/volume.go       processVolume, emit, terminal diagnostics
internal/modelhost/volume_host_test.go
internal/server/volume.go          StreamVolumeEvents
internal/server/server.go          volumeEvents wiring
```

**Actual production science files created:** `internal/domain/volume.go`, `config.go`, `mapper.go`, `median.go`, `features.go`, `linalg.go`, `derivatives.go`, `phase.go`, `interpretation.go`, `engine.go`, `state.go`. Proposed `windows.go` / `fixture.go` were **not** created as those names (`state.go` holds rings). `internal/volume/testdata/` exists for equivalence fixtures.

**Not created (and not current work):** `internal/config` `VolumeMode` / `QUANTRAM_VOLUME`.

### 3.1 Production file responsibilities

| Proposed file | Purpose | Owns | Inputs | Outputs | State | Dependencies | Non-responsibilities |
|---|---|---|---|---|---|---|---|
| `internal/domain/volume.go` | Internal Volume Output types | `VolumeEvent`, emission, skip, color/phase/transition constants | none | types only | none | `time` | proto, P-05, StageTransition |
| `config.go` | Frozen scientific constants + construction validation | unexported consts; `Config` | entity id | validated `Config` | none | none | env knobs for windows/thresholds |
| `mapper.go` | `domain.Bar` → Volume observation | `Observation` | `domain.Bar` | `V_RAW`, minutes, ids | none | `domain` | quality gating, continuity |
| `windows.go` | Bounded positional rings | `rawWindow`, `vnWindow`, `minutesWindow` | scalars | last-N slices | O(1) rings cap 15 | none | Price rings, finite-skipping harvest |
| `median.go` | Deterministic median of exactly 15 finite values | `rollingMedian15` | `[15]float64` | median | none | `sort` | even-n averaging (n is 15) |
| `features.go` | `V_N`, interval mean, VOLUME_POINT | feature functions | windows | quantities or unavailable | none | `median.go` | interpretation |
| `linalg.go` | gonum SVD lstsq matching NumPy `rcond=None` | `lstsq`, `finite` | design, y | coeff, rank, ok | none | `gonum` v0.17.0 | Price F4/EXPM |
| `derivatives.go` | Window-3 causal quadratic on `V_N` | `v1V2AtCurrent` | last 3 VN + minutes | V1=b, V2=2a or unavailable | none | `linalg.go` | Price window 15 |
| `phase.go` | Frozen phase labels | `classifyPhase` | V1, V2, ε | phase string | none | `math` | Price trajectory phase |
| `interpretation.go` | Confirmation machine | `InterpretationState`, `observe` | activity + prior state | colors, transition, next state | candidate only | none | feature math, color-age |
| `engine.go` | One engine: clone/prepare/commit/emit | `Engine` / `VolumeState` | Bar + committed state | `VolumeEvent`, working engine, commit flag | committed `VolumeState` | all above | Adaptive/Price, proto, persistence |

### 3.2 Proposed test files

| File | Role |
|---|---|
| `median_test.go` | Odd-15 median; refuse nonfinite windows |
| `features_test.go` | `V_N`, zero baseline, zero raw, interval mean, VOLUME_POINT, positional rule |
| `derivatives_test.go` | Regular and irregular Δt; rank failure; window 3 not 15 |
| `phase_test.go` | Five frozen phase categories; ε inclusivity |
| `interpretation_test.go` | Full confirmation table; first accept; pending AMBER; INVALID vs maturation |
| `windows_test.go` | Bound 15; no growth; no finite-skipping |
| `engine_test.go` | Prepare/commit atomicity; reset; entity isolation; cold start |
| `equivalence_test.go` | Frozen 009V/010/014C oracles after fixtures are vendored |
| `genericity_test.go` | Production types have no SPY/AAPL hard-code |
| `internal/modelhost/volume_host_test.go` | **IMPLEMENTED (Phase G):** same-bar correlation; no second mailbox; infer off does not consume Volume; ResetSymbol clears Volume; failure isolation |

### 3.3 Required code-documentation standard

Every **new** manually authored Go file created during A–G must begin with module-level documentation covering: purpose, inputs, outputs, parameters/configuration, ownership, lifecycle, concurrency, failure behavior, invariants, and explicit non-responsibilities.

Non-trivial functions must comment scientific or runtime responsibility. Do not hand-edit generated files to add comments.

## 4. VolumeState exact implementation design

Canonical architecture: **`VolumeState`** owns Volume Feature State and Volume Interpretation State.

Implemented Go identifiers (implementation names, not a second ontology):

```text
volume.Engine                 // per-entity engine; owns committed VolumeState
volume.State                  // implemented Go name for VolumeState
  Feature                     // Volume Feature State
    Raw                       // last ≤15 V_RAW
    VN                        // last ≤15 positional V_N (NaN if unavailable)
    Minutes                   // corresponding IntervalStart minutes
    AcceptedCount             // causal accepted-bar count (not a readiness predicate)
  Interpretation              // Volume Interpretation State
    Color                     // last committed Indicator (historical APTF cockpit_color); empty = unset
    PendingColor
    PendingCount
```

Do **not** require the Go type to be named `VolumeInterpretationState`. `volume.InterpretationState` is sufficient. Do **not** name production types `VolumePolicyState`.

### 4.1 Volume Feature State

Minimum bounded state for streaming equivalence:

| Ring | Capacity | Contents |
|---|---|---|
| Raw volume | 15 | `float64(V_RAW)` of last accepted observations |
| Normalized volume | 15 | positional `V_N` or `NaN` if that observation’s `V_N` was unavailable |
| Time coordinates | 15 | `IntervalStart` minutes aligned 1:1 with the raw/VN rings |

`AcceptedCount` may exist for diagnostics/equivalence. Readiness **must not** be `AcceptedCount == 29`.

### 4.2 Volume Interpretation State

Maps historically to APTF `VolumePolicyState`:

| Field | Meaning |
|---|---|
| `Color` | Carried historical Indicator (frozen APTF `state.color` / `cockpit_color`). Last emitted Indicator adopted on commit — **not** an independent memory of the last raw that completed confirmation. While pending, this is AMBER. Unset empty, or `INVALID` after a scientific-invalid observe. |
| `PendingColor` | Candidate raw color awaiting confirmation; empty if none |
| `PendingCount` | Frozen increment of that candidate. On `CONFIRMED_*` the executable leaves this at the confirming increment and clears `PendingColor`. |

No color-age fields. No session key. No date.

### 4.3 Ownership, lifetime, bounds, concurrency

| Topic | Design |
|---|---|
| Ownership | `volume.Engine` on the keyed symbol worker. Host map `workers[symbol].volume`. |
| Lifetime | Created when Volume is enabled and the worker is constructed; replaced on `ResetSymbol`; empty on cold start. No persistence restore in V1. |
| Initialization | Empty rings, `Color=""`, `PendingColor=""`, `PendingCount=0`. |
| Copy/candidate | `PrepareStep` clones Engine/state (same pattern as `pricing.Engine.clone`). Compute on the clone. |
| Commit | `Commit(working)` adopts the clone (`pricing.Engine.adopt` precedent). |
| Reset | `ResetSymbol` constructs a new `volume.Engine` for that symbol (entire VolumeState). |
| Memory | Three rings of 15 float64 + three interpretation scalars + small observation copy. O(1) vs runtime length. |
| Isolation | One Engine per entity. No shared VolumeState across symbols. |
| Concurrency | Keyed worker already serializes `handle` per symbol. No mutex on VolumeState. No concurrent `Step` for one entity. |

Do not store unbounded history. Do not use files, MongoDB, Snapshot, or Persistence as working state.

## Accepted-Bar Publication and Scientific Consumption Invariant

This section distinguishes **architectural intent**, **current physical implementation** (HEAD `07417d9c`), and the **required P-04V invariant**. It does not change production code.

### 1. Authoritative acceptance boundary

There are **three** current “accept” ideas. They must not be collapsed.

| Layer | Exact location | What it means |
|---|---|---|
| **P-02 window accept** | `Pipeline.accept` → `WindowStore.Add` (`internal/ingestion/pipeline.go`, `window.go`) | Bar entered/replaced the bounded ingest window. Same-generation `DedupKey` returns `false` and **does not** fan out. This is **not** yet model-path publication. |
| **P-02 model-path publication** | `Pipeline.fanoutModel` (`internal/ingestion/model_path.go`) | Bar is `ModelEligible()` (`IsFinal && QualityComplete && !IsBackfilled`), symbol is not `modelDisc`, continuity vs `modelLast` is first/normal/irregular, and the bar is **successfully enqueued** to every `SubscribeModelBars` channel. Only then is `modelLast[symbol]` updated. **This is the authoritative scientific-observation publication boundary.** |
| **Host scientific-commit cursor** | `worker.lastAccepted` / `hasAccepted` (`internal/modelhost/host.go`) | Updated **only** when `commitA && commitP` after Adaptive+Price prepare. This is **Adaptive+Price scientific commit success**, not P-02 acceptance. |

`ModelEligible` is `internal/domain/quality.go`. Irregular `IntervalStart` (skipped provider minute) is published; it is not automatically `INPUT_GAP`.

**Terminology fact:** current host comments and `lastAccepted` accidentally conflate **ingestion/model publication** with **scientific commit success**. P-04V must not inherit that conflation as its consume gate.

### 2. Publisher

The model-path publisher is **`ingestion.Pipeline.fanoutModel`**, called from `Pipeline.accept` (live) and recovery inject. It is in-process code on the Pipeline goroutine, not a network publisher.

Observe `Pipeline.fanout` / `Subscribe` / `SubscribeFinalized` is a **different**, lossy drop-oldest path. P-03/P-04/P-04V must not use it. Tests: `TestSubscribeFinalizedStillDropOldest` vs `TestSubscribeModelBarsNoSilentDrop`.

### 3. Transport

```text
P-01 live.Run  --chan domain.Bar, cap SubscriberQueue=16-->  Pipeline.Run
        |
        v
Pipeline.accept / WindowStore.Add
        |
        +--> fanout          (lossy observe subscribers, cap typically 16)
        |
        +--> fanoutModel     (modelSubs map[id]chan Bar)
                    |
                    v
         SubscribeModelBars channel
         Host uses cap modelSubscribeBuffer = WindowLimit = 64
                    |
                    v
         Host.Run  (one goroutine)
                    |
                    v
         Host.dispatch --> worker.inbox  (cap WindowLimit = 64, non-blocking)
                    |
                    v
         runWorker goroutine per symbol --> handle
```

| Link | Type | Cap (host path) | Send |
|---|---|---|---|
| P-01 → P-02 | `chan domain.Bar` | 16 | P-01 producer |
| Model path | `chan domain.Bar` per `SubscribeModelBars` | 64 (`Host.Run`) | non-blocking `select`/`default` |
| Worker inbox | `chan domain.Bar` per symbol | 64 | non-blocking `select`/`default` |

Ownership: Pipeline owns `modelSubs`. Host owns worker inboxes. Host is the sole production `SubscribeModelBars` caller.

### 4. Subscriber / consumer topology

One Host subscription. `Host.Run` demuxes by `bar.Symbol` into keyed workers. Each worker is one goroutine (`runWorker`) that calls `handle` sequentially.

P-03 and P-04 do **not** have independent queues. They are invoked inside the same `handle` on the same `domain.Bar` value.

### 5. Ordering

Per-symbol causal order is preserved on the model path while the symbol remains healthy: overflow keeps the in-order prefix and does not substitute a newer bar for an older one (`TestSubscribeModelBarsNoSilentDrop`). Host inbox overflow likewise does not replace queued bars (`select`/`default` refuses the new send).

No cross-symbol ordering.

### 6. Asynchronous vs sequential reality

| Claim | Current code |
|---|---|
| Ingestion can continue while science runs | **Yes.** `Pipeline.accept`/`fanoutModel` does not wait for `PrepareStep`. Publication is decoupled from calculation. |
| P-03 and P-04 consume via independent mailboxes | **No.** |
| P-03 and P-04 run on independent goroutines | **No.** |
| P-03 and P-04 are sequential in one keyed worker | **Yes.** `handle` calls `adaptive.PrepareStep` then `pricing.PrepareStep`. |
| They share prepare/commit | **Yes.** Commit both or neither (`commitA && commitP`). Confirmed by `TestPricingFailRollsBackAdaptiveAndReplays`. |

Accurate description: **asynchronous decoupling from ingestion, with deterministic collocated/sequential scientific execution.**

Independent scientific **responsibility** ≠ independent **delivery** ≠ **concurrent** execution.

### 7–8. Backpressure, overflow, drop

| Path | Behavior | Silent? |
|---|---|---|
| Observe `fanout` | Drop-oldest, then may drop newest; log `drop bar for slow subscriber` | Yes (observe only) |
| `fanoutModel` full | Do **not** enqueue overflowing bar; latch `modelDisc=QUEUE_OVERFLOW`; log; stop later publication for that symbol until `ResetModelPath` | No. Explicit latch. Prefix retained. |
| Host `dispatch` inbox full | Do not enqueue; `markDisc(QUEUE_OVERFLOW)`; emit `DecisionEvent` skip | No. Explicit skip. |
| Host unknown symbol | `dispatch` returns; no skip | **Potential silent omit** if Pipeline symbols ⊄ Host symbols. Inherited config assumption: same list. |
| Worker already `disc` | Emit `STATE_DISCONTINUOUS`; do not enqueue (if disc at dispatch) or do not prepare (if disc at handle) | No. Explicit skip. |

Publication does **not** block the Pipeline on a full model channel. It latches instead.

### 9. Processing-opportunity guarantee (what the code actually supports)

Strongest statement supported by current code:

**While a symbol’s model path and worker remain healthy, each successfully `fanoutModel`-published Bar is delivered once, in causal order, to the Host and then to that symbol’s worker inbox, and `handle` is invoked once for that delivery. There is no scientific retry loop. This is ordered one-opportunity delivery, not distributed exactly-once.**

Duplicates of the same `IntervalStart` are rejected at `fanoutModel` (vs `modelLast`) and again at host continuity (vs `lastAccepted`). Live ingress does not re-offer a failed bar unless a test/harness pushes it again. After Price-fail rollback, `lastAccepted` is unchanged, so a **re-pushed identical** bar can be prepared again (`TestPricingFailRollsBackAdaptiveAndReplays`).

### 10. Explicit failure / skip semantics

See the omission audit table below. Categories 2–4 emit `DecisionEvent` and/or latch `modelDisc` / worker `disc`. Category 5 (silent omission) is forbidden for P-04V after model-path publication.

### 11. No-silent-omission invariant (required for P-04V)

**Once `Pipeline.fanoutModel` has successfully published an eligible Bar B_t onto the model-consumer stream, every enabled scientific sibling must be offered an independent processing opportunity for that same B_t. A sibling must not be denied B_t merely because another sibling’s science failed to commit. If a sibling cannot consume or commit B_t, the runtime must emit an explicit skip, delivery-failure, scientific-failure, or discontinuity record. It must not replace B_t with B_(t+1) as if the omitted observation never existed. P-04V positional windows must not advance across an unconsumed published B_t while pretending `VolumeState` is causally continuous.**

Recovery/latch policy after such a hole is **not** invented here beyond: explicit record required; silent continuity forbidden; `ResetSymbol` remains the approved full scientific reset.

### 12. Implications for P-04V positional windows

Volume windows are positional last-15 / last-3 / last-15. A silently missing published B_t permanently falsifies later `V_N`, V1/V2, and `interval_mean_vn`. Therefore Volume consume/commit must be offered on published B_t even when Adaptive or Price do not commit, and any Volume non-consume of published B_t must be explicit.

### Omission audit (P-02 publication through P-03/P-04)

| Location | What happens | Class |
|---|---|---|
| `WindowStore.Add` same-generation | No fanout | 1 not a new scientific bar |
| Partial / reconstructed / backfill | `fanoutModel` returns | 1 not model-eligible |
| `modelDisc` already set | Logged skip; not published | 2 explicit (prior overflow/gap) |
| Duplicate/regression/unaligned vs `modelLast` | Logged; not published | 1 / 2 host-path continuity |
| `fanoutModel` channel full | Overflowing bar not sent; `QUEUE_OVERFLOW` latch | 3 delivery failure, **not silent** |
| No `modelSubs` | Return; `modelLast` not updated | Host not running |
| `dispatch` unknown symbol | Return, no event | 5 if symbol sets diverge; else N/A |
| Worker inbox full | Skip `QUEUE_OVERFLOW` + latch | 3, explicit |
| Worker `disc` | Skip `STATE_DISCONTINUOUS` | 2 |
| `infer=false` | Skip `INFER_OFF`; no reset | 2 (pre-prepare gate) |
| Not eligible at handle | Skip `NOT_MODEL_ELIGIBLE` | 1/2 (should be rare after fanoutModel) |
| Duplicate vs `lastAccepted` | Skip `DUPLICATE_OR_REGRESSION` | 2 |
| Proven missing irregular | Skip `INPUT_GAP` + latch | 2 |
| Deadline before prepare | Skip `TIMEOUT`; no commit | 2 |
| Deadline after A+P prepare | Skip `TIMEOUT`; no A+P commit; `lastAccepted` unchanged | 2 / 4 |
| Adaptive prepare fail | No A+P commit; emit Adaptive skip | 4 |
| Price prepare fail | No A+P commit; Adaptive rewritten `ENGINE_ERROR` | 4; **couples Adaptive commit** |
| Worker panic | Skip `ENGINE_PANIC` + latch | 2/3 explicit |
| Shutdown | Inboxes closed; in-flight handle may finish | drain, not silent live omit |

### Inherited P-03/P-04 coupling (do not change in P-04V)

Confirmed again from `handle` and `TestPricingFailRollsBackAdaptiveAndReplays`:

- Price prepare failure ⇒ Adaptive is **not** committed.
- Adaptive prepare failure ⇒ Price is **not** committed (Price may have been prepared).
- `lastAccepted` advances only on joint success.

This **violates** a stronger “independent scientific commit” principle, but it is **existing** behavior. P-04V must **not** become a third member of that transaction and must **not** repair the Adaptive↔Price coupling. Flag for separate human review.

**Conflict:** if Volume were gated on `commitA && commitP`, Price failure would erase B_t from Volume’s input sequence. That gating is **rejected**.

**Residual inherited limit:** `handle` is sequential and a single `recover` wraps the whole function. An Adaptive **panic** aborts later siblings on that bar. The bar is not silent (`ENGINE_PANIC`). Giving Volume an opportunity even then requires Volume to run in a nested recover **before** Adaptive, or equivalent. That is a Phase G host-join choice, not a P-03/P-04 science change.

## 5. Accepted-Bar integration

Inspected facts (`internal/ingestion/pipeline.go`, `model_path.go`, `internal/modelhost/host.go`):

- P-02 publishes model-eligible bars via `fanoutModel` onto `SubscribeModelBars` (cap 64 on the Host).
- `Host.Run` (one goroutine) dispatches into per-symbol `inbox` (cap 64, non-blocking).
- Each symbol has one worker goroutine; `handle` applies **common** gates, then sequential Adaptive and Price prepare.
- Adaptive+Price commit is both-or-neither. `lastAccepted` updates only on that joint commit. **Volume must not use that cursor as its publication/consume gate.**
- `ResetSymbol` rebuilds Adaptive and Price engines and clears `lastAccepted`; Volume must be rebuilt the same way when enabled.
- `infer=false` emits skip and does **not** reset Adaptive/Price/Volume.
- Worker panic recover currently marks the worker discontinuous — Volume work in the same `handle` must use a **nested recover** so Volume panic does not latch Adaptive/Price.

**Integration point:** same keyed worker, same published bar, no new mailbox. After **common host gates** (not after A+P commit), offer Volume an independent `PrepareStep`/`Commit`.

P-04V is not invoked on bars that never crossed `fanoutModel`, or that fail common host gates (infer off, not eligible, duplicate/regression, proven missing, pre-prepare timeout). Those are not “Adaptive/Price failed.”

## 6. Prepare / candidate / commit semantics

```text
Accepted Eligible Bar
        |
        v
     CONSUME
        |
        v
Current committed VolumeState
        |
        v
   PREPARE (clone)
        |
        v
Candidate Volume Output
+ candidate next VolumeState
        |
        v
      COMMIT?  --no--> committed VolumeState unchanged
        |
       yes
        v
Authoritative VolumeState
        |
        v
       EMIT
        |
        v
   Volume Output
```

### 6.1 Repository fact: shared Adaptive+Price transaction

P-03 and P-04 already share a transactional pair. Injected Price prepare failure prevents Adaptive commit. That is **existing** P-03/P-04 semantics. This design must not silently extend that pair to Volume.

### 6.2 Recommendation: independent Volume processing opportunity and commit

**Selected:** P-04V is offered B_t and commits **independently** of the Adaptive+Price pair.

Volume consume is gated on **published eligible B_t + common host gates**, not on `commitA && commitP`, not on `lastAccepted` advancing.

| Rule | Behavior |
|---|---|
| Bar never published (`fanoutModel` did not enqueue) | Volume not invoked |
| Common host gate reject (infer off, ineligible, duplicate/regression, proven missing, pre-prepare timeout) | Volume not invoked; existing explicit skip |
| Published B_t passed common gates; Volume prepare succeeded | Commit Volume independently; emit Volume Output — **even if Adaptive and/or Price do not commit** |
| Published B_t passed common gates; Volume prepare hard-failed or panicked | VolumeState unchanged; explicit Volume failure record; Adaptive/Price commit path **unchanged** |
| Volume maturation / unavailable features | **Success commit** of candidate state (windows advanced; typed maturation output). Not a failed prepare |
| Candidate compute | Must not mutate committed VolumeState |
| Incoming `domain.Bar` | Immutable |

Rationale: gating Volume on Adaptive+Price commit would make a Price failure erase B_t from Volume’s positional sequence. Joining the both-or-neither pair would make Volume failure roll back Adaptive and Price. Neither is allowed.

### 6.3 Host sequence — IMPLEMENTED (Phase G, 2026-09-06)

The 2026-09-05 text below this heading was the recommended join. Phase G implemented it as follows. Do **not** add a second mailbox. Keep one keyed worker.

1. Existing **common** gates unchanged (infer, eligibility, host continuity, proven missing, pre-prepare deadline).
2. Isolated `processVolume`: nested recover; `volume.PrepareStep`; independent `volume.Commit` on success. Volume-only latch `volDisc` on ENGINE_ERROR / panic / injected fail.
3. Existing Adaptive prepare; existing Price prepare if enabled.
4. Existing shared deadline check for Adaptive/Price. If exceeded: Adaptive/Price commit none; `lastAccepted` unchanged. Volume already independently committed or explicitly failed.
5. If `commitA && commitP`: commit Adaptive and Price; set `lastAccepted` (**existing only**; not a Volume gate).
6. If Volume hard-fails: leave VolumeState unchanged; emit explicit Volume `ENGINE_ERROR`; do not rewrite Adaptive/Price events.

Volume runs **before** Adaptive/Price so an Adaptive panic cannot deny Volume its opportunity on that B_t. Volume panic does not mark worker `disc` and does not deny A+P. That is host-join structure, not a change to Adaptive/Price science or both-or-neither commit.

Do not invent a third shared transaction.

### 6.4 What “failed prepare” means

Failed prepare (no Volume commit):

- entity mismatch
- panic
- host pre-prepare timeout / common-gate reject (Volume not invoked)
- detected state corruption (rings desynchronized)

Not failed prepare (commit candidate):

- insufficient causal state (maturation)
- `V_N` unavailable (non-positive median)
- V1/V2 lstsq failure (quantity unavailable)
- full interpretation INVALID when evaluation is due (commit interpretation `Color=INVALID`)

If Volume cannot consume a **published** B_t, that observation is missing from Volume windows. That must be an **explicit** Volume skip/failure/discontinuity. Do **not** later append B_(t+1) as if `VolumeState` were continuous. Do not invent a production Volume `INPUT_GAP` or auto-reset in this pass; `ResetSymbol` remains the approved full reset. Engine `apply` must keep hard-fail rare (panic / entity mismatch).

## 7. Scientific numerical implementation

Do not change the mathematics.

### 7.1 Raw volume

```text
V_RAW = float64(accepted Bar.Volume)
```

Preserve the observed quantity. No clipping, winsorizing, imputation, previous-value substitution, synthetic volume, or entity-specific scale.

`uint64` → `float64` is exact for integers ≤ 2^53. Historical freeze volumes are in that range. Do not invent a BigInt path in V1. Record the mantissa limitation as a known limit.

### 7.2 Rolling median 15 / `V_N`

Window = last 15 **positional** accepted `V_RAW`, including current.

Algorithm (n=15 is odd; matches NumPy `median` for odd length):

1. If `len(raw) < 15`: `V_N` unavailable.
2. Take the positional slice of exactly 15. If any member is nonfinite: unavailable (do not skip).
3. Copy, sort, median = `sorted[7]`.
4. If median is nonfinite or `<= 0`: `V_N` unavailable.
5. Else `V_N = V_RAW_current / median`.

Do not search backward for 15 finite values.

### 7.3 Interval mean

`interval_mean_vn = mean(last 15 positional V_N)`.

All 15 must be finite. If any is `NaN`/Inf: unavailable. Do not skip.

### 7.4 VOLUME_POINT

`predicted_next_V_N = V_N` when `V_N` is available; otherwise unavailable. Not RK45. Do not fabricate `projected_v1` / `projected_v2`.

### 7.5 V1 / V2 — reuse analysis

Inspected P-04 primitive: unexported `causalQuadraticAtIndex` + `lstsq` in `internal/pricing/derivatives.go` and `internal/pricing/linalg.go`.

P-04 contract (must be copied, not imported):

- Relative coordinates `τ_i = T_i - T_current`
- Design `[τ², τ, 1]`
- `lstsq` = gonum SVD; rank cutoff `eps * max(M,N) * max(S)` with `eps = 2.220446049250313e-16` (NumPy `rcond=None`)
- Require `rank == 3` and finite coefficients
- `V1 = b = coeff[1]`, `V2 = 2a = 2*coeff[0]`

**Allowed:** reuse this **numerical algorithm**.  
**Forbidden:** import `internal/pricing`; reuse Price rings; reuse Price window 15; run EXPM/F4/RK45.

V1 recommendation: copy `lstsq`/`finite` into `internal/volume/linalg.go` citing the P-04 file as precedent. Optional later extraction to a shared `internal/numutil` is **not** required for V1 and must not change P-04 science.

Volume window is **3**, not 15. Series is `V_N`, not close.

Do **not** claim bitwise identity with NumPy or with P-04 Price derivatives. Claim: same lstsq contract; categorical exactness for colors/phase/transition; numerical tolerances for floats (see §16).

### 7.6 Finite and threshold comparisons

| Check | Rule |
|---|---|
| Finite | `!NaN && !Inf` (P-04 `finite`) |
| Median `> 0` | `median > 0` after finite |
| GREEN | `activity >= 1.1` |
| RED | `activity <= 0.9` |
| AMBER | otherwise (`0.9 < activity < 1.1`) |
| Phase stationary | `abs(V1) <= 1e-12` |
| Phase accel | `V2 > 1e-12` or `V2 < -1e-12` as specified |

Preserve inclusivity of 0.9 / 1.1. Do not retune.

## 8. Time-coordinate implementation

| Topic | Design |
|---|---|
| Units of `T` | Minutes = `IntervalStart.UTC().UnixMilli() / 60000.0` (P-04) |
| `τ` | `T_i - T_current` for the 3-point window |
| Observation order | Host-accepted causal order. Windows advance by one accepted bar |
| Irregular Δt | Does **not** reset. Changes V1/V2 only |
| Missing provider minute | Not an automatic input gap (host already treats irregular as accept unless proven missing) |
| Duplicate / non-monotonic | Host rejects; P-04V never consumes |
| Manufacture / interpolate | Forbidden |
| Session / calendar | Not a Volume runtime key |

Inspection found no concrete incompatibility with using `IntervalStart` as the P-04 minutes basis.

## 9. Maturation / readiness vs INVALID

Warm-up is **causal maturation**, not `observation_number == 29`, and not automatically INVALID.

| Milestone | Ready when |
|---|---|
| Raw | Current accepted bar consumed |
| `V_N` / `predicted_next_V_N` | 15 raw observations; all finite; median `> 0` |
| `V1`/`V2` | Last 3 positional `V_N` all finite + minutes; rank-3 finite fit |
| `interval_mean_vn` | Last 15 positional `V_N` all finite |
| Full interpretation | All mandatory interpretation inputs valid (`V_RAW`, `V_N`, `V1`, `V2`, `predicted_next_V_N`, `interval_mean_vn`) |

Proposed internal status (domain strings; **not** proto enums):

| Status | Meaning |
|---|---|
| `MATURING_FEATURES` | Windows not yet scientifically sufficient |
| `FEATURES_PARTIAL` | Some features available; interpretation not due |
| `EMITTED` | Full interpretation produced |
| `INVALID` | Required inputs nonfinite **when evaluation is due** |
| `ENGINE_ERROR` | Hard failure (no commit) |

INVALID is **not** “not yet enough causal state.”

Equivalence fact only: on a clean contiguous series with no nonfinite `V_N` after first `V_N`, first full interpretation is observation **29** (0-based index 28). Use as a test target, not a runtime `if`.

## 10. Zero / undefined / nonfinite

| Case | Quantity | Output class |
|---|---|---|
| Observed raw 0, median `> 0` | `V_N = 0` (finite) | Proceed; freeze did not validate live zeros |
| Median `<= 0` or nonfinite | `V_N` unavailable | Maturation if interpretation not due; INVALID if interpretation is otherwise due and this leaves a required input nonfinite |
| lstsq fail / rank ≠ 3 | V1/V2 unavailable | Same distinction |
| Accepted `Bar.Volume` | Always a `uint64` | Absence is P-02; do not invent missing-volume science |
| Nonfinite required float when interpretation due | — | INVALID emission; next `Color=INVALID` |

Do not impute, replace with previous volume, add a denominator epsilon, or treat 0 as missing.

## 11. Confirmation / hysteresis state machine

Frozen `confirmation_observations = 2`. Historical observe semantics preserved.

Proposed Go strings (historical labels, not proto):

**Colors:** `GREEN`, `AMBER`, `RED`, `INVALID`  
**Transitions:** `STABLE`, `PENDING_GREEN`, `PENDING_RED`, `PENDING_AMBER`, `CONFIRMED_GREEN`, `CONFIRMED_RED`, `CONFIRMED_AMBER`  
**Confidence:** `HIGH` when `state_source=INTERVAL_MEAN_V_N`  
**Domain:** `CAUSAL_LOCAL_VOLUME`

`activity = interval_mean_vn`

```text
if activity >= 1.1 → raw_color = GREEN
if activity <= 0.9 → raw_color = RED
else               → raw_color = AMBER
```

| Current `Color` | Raw | Next Indicator (cockpit) | Next Color | Pending | Transition |
|---|---|---|---|---|---|
| unset (`""`) | any valid raw | raw | raw | clear | `STABLE` |
| `C` | `C` | `C` | `C` | clear | `STABLE` |
| `C` | `R ≠ C`, pending was not `R` | **AMBER** | **AMBER** | `R`, count=1 | `PENDING_R` |
| `C` | `R ≠ C`, pending already `R`, increment to `count < 2` | **AMBER** | **AMBER** | `R`, incremented count | `PENDING_R` |
| `C` | `R ≠ C`, pending already `R`, increment to `count >= 2` | `R` | `R` | `pending_color` empty; `pending_count` left at the confirming increment | `CONFIRMED_R` |
| any | required inputs nonfinite and interpretation due | `INVALID` | `INVALID` | clear | (INVALID emission) |

`next.Color` **is** the emitted Indicator. Frozen APTF writes `VolumePolicyState.color = cockpit_color`. While pending, that value is AMBER even if raw is GREEN or RED. Do not retain a prior confirmed raw in `Color` during pending.

After `Color=INVALID`, the next valid raw is `≠ INVALID`, so confirmation starts (cockpit AMBER until 2). Do not invent a special INVALID-recovery rule.

This is a stateful machine. Do not collapse it to a stateless threshold.

## 12. Phase mathematics

`ε = 1e-12`

| Condition | Proposed Go string |
|---|---|
| `abs(V1) <= ε` | `ACTIVITY_STATIONARY` |
| `V1 > 0` and `V2 > ε` | `ACTIVITY_INCREASING_ACCELERATING` |
| `V1 > 0` and `V2 <= ε` | `ACTIVITY_INCREASING_DECELERATING` |
| `V1 < 0` and `V2 < -ε` | `ACTIVITY_DECREASING_ACCELERATING` |
| `V1 < 0` and `V2 >= -ε` | `ACTIVITY_DECREASING_DECELERATING` |

Do not redesign labels. Do not use P-04 trajectory-phase names.

## 13. Lifecycle / reset

Preserve Phase 1 closures (D1 closed for V1).

| Event | Behavior |
|---|---|
| Normal published eligible observation (common gates passed) | Volume prepare/commit independently of Adaptive+Price commit; windows advance by one |
| Published B_t Volume cannot consume | Explicit Volume skip/failure; do **not** treat later bars as continuous VolumeState. Full reset remains `ResetSymbol`. |
| `Host.ResetSymbol` | New Adaptive, Price (if on), **and** Volume engines. Entire VolumeState cleared. Maturation restarts |
| Cold process restart | Empty VolumeState |
| Persisted restore | Outside V1 |
| Infer disabled | Do not consume; **do not** reset VolumeState |
| Infer restored | Continue existing VolumeState unless explicit scientific reset / proven host discontinuity |
| Provider/source identity change | Not a Volume reset |
| Large Δt | Not a reset |
| Market/session boundary | Not a reset. Do **not** inherit APTF `date:session` |
| Independent interpretation reset | **None** in V1 |

## 14. Failure containment

| Failure | Committed VolumeState | Adaptive/Price | Host |
|---|---|---|---|
| Numerical unavailable feature | Commit windows; quantity NaN; maturation or INVALID per due-ness | Unchanged | Continue |
| Interpretation INVALID | Commit `Color=INVALID` | Unchanged | Continue |
| Entity mismatch | Unchanged | Unchanged | Volume error; no Adaptive disc latch |
| Volume panic | Unchanged (nested recover) | Unchanged | Volume error counter; **must not** mark worker discontinuous |
| Host pre-prepare timeout / common-gate reject | Unchanged (Volume not invoked) | Unchanged (existing) | Existing skip |
| Adaptive and/or Price prepare/commit fail | Volume still offered B_t; commit independently if Volume prepare succeeded | Existing both-or-neither pair **unchanged** | Existing Adaptive/Price skips |
| Published B_t Volume cannot commit | Unchanged; **explicit** Volume failure; do not silently continue windows | Unchanged | Volume error/skip recorded |
| Duplicate/non-monotonic | Never called | Existing skip | Existing |
| Host shutdown | Drop in-memory VolumeState | Existing | Existing |
| Cold restart | Empty | Empty | Empty |

Incoming bars remain immutable. Candidate clones only.

## 15. Output / internal domain model

Implemented `domain.VolumeEvent` (internal) plus proto `quantram.v1.VolumeEvent`. **HISTORICAL:** this table originally said “no proto.” SUPERSEDED.

| Field | Purpose | Historical map |
|---|---|---|
| `EventID`, `AcceptedSequence` | Host publication envelope (Phase G) | not science |
| `Lineage.Symbol` | Entity | 014C payload symbol (production is generic) |
| `Lineage.IntervalStart` | Initiating interval (not a Volume EffectiveTime) | — |
| `Lineage.SourceTimestamp` | Correlation | 014C timestamp string where applicable |
| `Lineage.MarketSnapshotID` | Initiating-bar identity | — |
| `Status` | `MATURING` / `AVAILABLE` / `INVALID` / `ENGINE_ERROR` | — |
| `VRaw` | `V_RAW` | `v_raw` |
| `VN` | `V_N` | `v` |
| `V1`, `V2` | derivatives | `v1`, `v2` |
| `IntervalMeanVN` | activity value | `activity_state_value` / `interval_mean_vn` |
| `PredictedNextVN` | VOLUME_POINT / `G_V` | `predicted_next_V_N` / `projected_v` |
| `RawColor` | pre-hysteresis band | `raw_color` |
| `Indicator` | confirmation-controlled category | historical APTF `cockpit_color` |
| `Phase` | frozen phase | `phase` |
| `TransitionState` | STABLE / PENDING_* / CONFIRMED_* | `transition_state` |
| `ConfidenceState` | `HIGH` | `confidence_state` |
| `DomainState` | `CAUSAL_LOCAL_VOLUME` | `domain_state` |
| `ReasonCodes` | activity + phase + confirmation tags | `reason_codes` |
| `MaturationReason` | why not yet actionable | — |
| `Skip` | hard/non-consume reasons | — |

Do not emit BUY/SELL/HOLD or an order. Do not emit color-age.

## 16. Equivalence harness

`internal/volume/testdata/` **exists**. It holds the equivalence fixture material currently used by Volume tests. Production `volume.Engine` does **not** read testdata. There is no runtime dependency on APTF file paths.

Present in that directory (as of 2026-09-06 inspection): `provenance.json`, `APTF_TEST_014C_SPY_V_EMISSION_POLICY_V0_1.json` (exact full copy), and deterministic Level-1 subsets `level1_source_prefix.csv`, `level1_010_prefix.csv`, `level1_014c_first_session.csv`.

Full frozen APTF corpora remain externally hash-verified in the APTF repository (`pre08242026_docs/`). They are not vendored here (86MB+). Full-corpus tests read them only when `QUANTRAM_P04V_FROZEN_DIR` is set.

Full hashes verified from APTF freeze inventories (`pre08242026_docs/APTF_TEST_00{9V,10,14C}_ARTIFACT_HASHES_V0_1.json`), matching investigation prefixes:

| Artifact | SHA256 | Role |
|---|---|---|
| `APTF_TEST_009V_VOLUME_SELECTION_V0_1.json` | `4dbc78a1e213715577a9b68eca8cae186137fcaa2959afeb9d6bac308f430dd9` | `selected_V_N` / `V1` / `V2` (101,221) |
| `APTF_TEST_010_VOLUME_ENGINE_EMISSIONS_V0_1.csv` | `0d9134f3a1996d83dd43257264ddd6a43b5e02215b61c94ec034c3e1ee152d3c` | Primary **feature** corpus (101,205) |
| `APTF_TEST_014C_SPY_V_EMISSION_POLICY_V0_1.json` | `f719134f241b00888099e237c02f237a2db4b59f02b25ea5498c51006991bcd8` | Frozen `V_INTERVAL_B10_C2` |
| `APTF_TEST_014C_SPY_V_ENGINE_EMISSIONS_V0_1.csv` | `ecd946532e32a8c5167aab72e8c56d3d3389ab00705a75d4cb91cf3031fd451e` | Primary **interpretation** corpus (55,199) |
| `APTF_TEST_014C_SUMMARY_V0_1.json` | `100f0b4807831f6eebd2e44fe8ab7b2c9597113916243b635b75d819fe80044b` | 139/139 PASS summary |
| `APTF_TEST_014C_V_POLICY_FREEZE_V0_1.json` | `029d7bac58a1c1ba8635ec3fb46879b892c3034807e847260ee782a9d6ff8740` | Policy freeze record |
| `APTF_TEST_014C_SPY_V_INTERVALS_V0_1.csv` | `8bdccaa5529a5c5bdac4a86bfef1f0dd1226a30ec69049e3ad8b1a28bbc7c8bc` | Color-age provenance only; **out of scope** |

014C summary claim: **139/139 PASS**. Feature tests use 010; interpretation tests use 014C. 014C is a Price-overlapped subset (early rows excluded; `INVALID_count=0`). Observation 29 is a clean-series milestone, not a runtime index.

Equivalence classes:

1. **Numerical** — `V_N`, V1/V2, `interval_mean_vn` with documented abs/ulp tolerances (follow P-04 pricing equivalence style; do not claim bitwise NumPy identity).
2. **Categorical** — `raw_color`, Indicator (historical APTF `cockpit_color`), phase, transition, INVALID **exact**.
3. **State-transition** — `Color` / pending fields after each observe.
4. **Sequence** — contiguous replay vs 010 / 014C row order.
5. **Maturation alignment** — first full interpretation on a clean series at observation 29.

Do not retune thresholds to pass. Historical symbol `SPY` may appear **only** in fixtures.

**Note:** The 014C emission corpus includes APTF `date:session` interpretation-state resets. QuanTRAM V1 must **not** implement that reset. Equivalence tests must either (a) replay session-sliced 014C segments as separate engine lifetimes, or (b) inject an explicit test-only reset between sessions **without** putting session reset in production. Production `ResetSymbol` is the only reset. Record this as a required harness design, not a science change.

## 17. Unit-test matrix

| ID | Topic | Pass criterion |
|---|---|---|
| U01 | Rolling median 15 | Middle of sorted 15; matches fixture windows |
| U02 | Normalization | `V_N = V_RAW / median`; inclusive current |
| U03 | Positional window | Nonfinite member → unavailable; no skip-back |
| U04 | Derivative regular Δt | V1=b, V2=2a; rank 3 |
| U05 | Derivative irregular Δt | Result changes with actual minutes; no reset |
| U06 | Derivative failure | Rank/nonfinite → unavailable, not fabricated |
| U07 | Interval mean 15 | Mean of positional VN; all-finite required |
| U08 | VOLUME_POINT | Equals `V_N`; no RK45 |
| U09 | Threshold inclusivity | 0.9 / 1.1 inclusive as specified |
| U10 | Phase | Five frozen labels |
| U11 | First interpretation | Immediate accept; STABLE |
| U12 | Confirmation | Pending AMBER; confirm at 2 |
| U13 | Same color | Clears pending; STABLE |
| U14 | Maturation | Status not INVALID before due |
| U15 | Zero raw | Preserved; `V_N=0` if median `> 0` |
| U16 | Zero/non-positive median | `V_N` unavailable |
| U17 | Reset | Full VolumeState empty |
| U18 | Cold start | Empty interpretation + empty rings |
| U19 | Ring bound | Length never exceeds 15 after long runs |

## 18. State / atomicity and integration-test matrix

| ID | Topic | Pass criterion |
|---|---|---|
| A01 | Prepare clone | Committed rings/interpretation unchanged if not committed |
| A02 | Failed prepare | State hash identical |
| A03 | Commit once | Second adopt of same working is idempotent or rejected; one advance per accept |
| A04 | Reset | Feature + interpretation cleared |
| A05 | Isolation | Entity A cannot mutate entity B |
| A06 | Bound | Memory/rings O(1) over 10k steps |
| I01 | Same published Bar | Volume `IntervalStart` / `MarketSnapshotID` match the B_t offered to Adaptive/Price in that handle |
| I02 | No second subscription | Host still has one `SubscribeModelBars` |
| I03 | Irregular interval | Published irregular B_t ⇒ Volume opportunity; no Volume reset |
| I04 | Infer off/on | VolumeState preserved across infer pause |
| I05 | Provider identity | Source string change alone does not reset Volume |
| I06 | `ResetSymbol` | Volume empty; Adaptive/Price existing reset still occurs |
| I07 | No session reset | Synthetic date/session change does not clear interpretation |
| I08 | Pricing off, Volume on | Volume still offered published B_t |
| I09 | Volume hard-fail | Adaptive/Price committed state unchanged |
| I10 | Volume panic | Worker not latched `STATE_DISCONTINUOUS` |
| I11 | Price prepare fail | Volume still offered B_t; Adaptive+Price both-or-neither **unchanged** |
| I12 | Adaptive prepare fail (non-panic) | Volume still offered B_t |
| G01 | No SPY/AAPL in production source | `rg` over `internal/volume` excluding testdata |

### Delivery invariants and Phase G validation

Invariants are unchanged. Disposition is from Phase G Host/server tests and inherited Host overflow behavior. This is **not** a claim that every DELIVERY item has a newly invented Volume-only test.

| ID | Requirement | Phase G disposition |
|---|---|---|
| DELIVERY-01 | Every `fanoutModel`-published B_t is offered once, in causal order, to each enabled sibling under healthy operation | **VALIDATED on Host path** (`TestG07`, `TestG03`). Inherited `Pipeline.fanoutModel` semantics unchanged. Not distributed exactly-once. |
| DELIVERY-02 | P-04 failure does not suppress P-04V’s opportunity for B_t | **VALIDATED** (`TestG08`, `TestG16`). A+P joint commit unchanged. |
| DELIVERY-03 | P-03 failure does not silently suppress P-04V’s opportunity for B_t | **VALIDATED.** Non-panic Adaptive prepare fail: `TestG09`. Adaptive panic: **closed** — Volume nested recover runs first (`TestG11`). |
| DELIVERY-04 | P-04V failure does not change P-03/P-04 outcomes for B_t | **VALIDATED** (`TestG10`, `TestG12`). |
| DELIVERY-05 | A slow scientific consumer cannot silently replace B_t with B_(t+1) | **INHERITED HOST:** inbox overflow latches, does not drop-oldest. Phase G `TestG36` proves a slow **RPC Volume subscriber** does not alter science; that is publication backpressure, not a new overflow policy. |
| DELIVERY-06 | Queue/mailbox overflow is surfaced and counted; no silent omission | **INHERITED HOST:** `QUEUE_OVERFLOW` + skip/log. Phase G did not add a dedicated Volume overflow test. |
| DELIVERY-07 | Per-entity causal ordering preserved while healthy | **VALIDATED** (`TestG07`). |
| DELIVERY-08 | No duplicate scientific opportunity for one published Bar under healthy operation | **VALIDATED under healthy Host path** (one Volume event per published bar in `TestG07`). Not distributed exactly-once. |
| DELIVERY-09 | Received-but-not-committed is distinguishable from never-received | **VALIDATED.** Common-gate: no Volume event (`TestG15`). Post-gate Volume fail: explicit `ENGINE_ERROR` without commit increment (`TestG10`). |
| DELIVERY-10 | P-04V windows advance only for observations that satisfy the P-04V consume/commit contract | **VALIDATED.** Infer-off does not commit (`TestG15`). Injected Volume fail does not increment `volSeq` (`TestG10`). |
| DELIVERY-11 | If a published positional observation is lost, P-04V does not silently continue as continuous | **VALIDATED Volume-specific:** `volDisc` + `VOLUME_DISCONTINUOUS` (`TestG10`). Restore via `ResetSymbol`. Shared worker `disc` (overflow/proven gap/Adaptive panic) still blocks all siblings. |
| DELIVERY-12 | Infer-disabled / ineligible / pre-publish rejection is distinguishable from post-publish delivery/scientific failure | **VALIDATED.** Common gate: no Volume invoke (`TestG15`). Post-publication Volume failure: explicit Volume event (`TestG10` / `TestG12`). |

## 19. Runtime validation and deferred live-market validation

This section distinguishes **runtime integration validation** (done in Phase G) from **external / live-market validation** (not complete). Local CSV replay is **not** a live market.

### 19.1 Phase G runtime validation — COMPLETED (2026-09-06)

Deterministic ModelHost / server evidence (`internal/modelhost/volume_host_test.go`, `TestStreamVolumeEventsLive`, frozen `./internal/volume`):

- same-Bar lineage (`MarketSnapshotID`, `IntervalStart`)
- one VolumeState / Volume Engine per symbol; multi-symbol isolation
- causal order B1 → B2 → B3 through the Host path
- independent Volume commit; `lastAccepted` remains A+P-only
- Adaptive prepare failure / Adaptive panic do not deny or roll back Volume
- Price prepare failure does not roll back Volume; A+P joint commit unchanged
- Volume failure / Volume panic do not roll back A+P; Volume hole is explicit
- `ResetSymbol` clears Volume and restarts maturation; no automatic session reset
- one `SubscribeModelBars`; no Volume mailbox; no Volume science goroutine
- `StreamVolumeEvents` streams Host-produced events; slow/cancelled client does not control science
- terminal diagnostics (`volume engine initialized`, `volume maturing` / `emit` / `invalid` / `error`)
- scientific noninterference: Volume/Adaptive/Price mathematics packages unchanged by host join

Local CSV QuanTRAM server run (no provider credentials): `QUANTRAM_SOURCE=csv`, `QUANTRAM_MODEL=adaptive`, **`QUANTRAM_PRICING=off`**. Observed Volume startup and MATURING lines on the same process as Adaptive `model skip` / `model decision`. That run overflowed the inherited worker inbox (`STATE_DISCONTINUOUS`) before AVAILABLE. It was **not** a live-market run and did **not** observe Price `pricing emit`.

Success is scientific and architectural evidence, not “the server ran.”

### 19.2 Full live / external-market validation — DEFERRED

Not claimed complete:

- external provider realtime run with Price **and** Volume both active
- sustained RTH behavior
- production-scale queue behavior
- longer-run operational observability
- dashboard Volume viewer evidence
- StageTransition Volume publication evidence

`QUANTRAM_VOLUME=off` was never implemented. Volume is present whenever Adaptive Host exists. `QUANTRAM_MODEL=off` → no Host → Volume not wired.

## 20. Performance / complexity

| Work | Cost |
|---|---|
| Append + trim rings | O(1) amortized, cap 15 |
| Median 15 | O(15 log 15) copy-sort |
| Mean 15 | O(15) |
| 3×3 lstsq | O(1) small dense SVD |
| Interpretation | O(1) |
| Per-entity memory | O(1) vs runtime length |

Forbidden: full-history dataframes, O(n²) replay, unbounded slices, recomputing all bars each step.

CPU: one small SVD plus tiny sorts per published bar per entity. Memory: a few hundred bytes per entity plus the Engine struct. Latency: Volume runs in the same keyed-worker handle (sequential with Adaptive/Price, not a parallel goroutine). It must fit in `QUANTRAM_MODEL_DEADLINE` without adding a second mailbox. Physical concurrency is not required for scientific independence.

## 21. Observability

Observational only. Not a second science path. There is **no** independent Volume off mode and **no** `QUANTRAM_VOLUME` switch.

Current Phase G surfaces:

- Volume engine initialized / active (`volume engine initialized symbols=N mode=discrete_G_V`) when Adaptive Host exists
- Adaptive Host absent (`QUANTRAM_MODEL=off`) → Volume not wired (`StreamVolumeEvents` `FailedPrecondition`)
- scientific status: `MATURING` / `AVAILABLE` / `INVALID` / `ENGINE_ERROR`
- per-symbol Volume event / last-event catch-up (`LastVolumeEvents`)
- Volume resets (`ResetSymbol` rebuilds the engine)
- Volume failures / panics / `volDisc` discontinuity (`VOLUME_DISCONTINUOUS`)
- accepted_sequence / event publication (non-blocking; drop + log on full subscriber buffer)
- terminal diagnostics (`volume emit` / `maturing` / `invalid` / `error`)

No filesystem log as scientific correctness. No Snapshot/StageTransition dependency.

## 22. Relationship to P-03

P-03 D01 `updateVolumeInfluence` (`internal/adaptive/volume.go`) remains unchanged. Do not move it, call P-04V from D01, feed Volume Output into Adaptive, or share VolumeState.

Shared fact only: originating `Bar.Volume`.

## 23. Relationship to P-04

Reuse as **architectural precedent**: collocated Go, same accepted Bar, per-entity owned state, bounded rings, prepare/commit, first-class emit, no second subscription, gonum lstsq algorithm.

Do not consume `PriceEvent`/Price color, share `PriceState`/history, reuse window 15, run EXPM/F4/RK45, or fuse P/V.

## 24. StageTransition exclusion

StageTransition V1.1 is frozen. P-04V V1 must not add a StageID, equality field, publisher path, diagnostic, or proto change. Future Volume StageTransition remains **deferred / outside V1 / non-blocking**. Do not design it here.

## 25. HISTORICAL IMPLEMENTATION PLAN (A–G executed)

Science first, host second — same discipline as P-03 A–C then D, and P-04 A–G then H. This table was the 2026-09-05 plan. **Do not read it as unimplemented.**

| Phase | Planned work | Result |
|---|---|---|
| **A** | `domain.VolumeEvent` + frozen `Config` + mapper | **IMPLEMENTED** 2026-09-05 |
| **B** | Windows, median, `V_N`, interval mean, VOLUME_POINT | **IMPLEMENTED** 2026-09-05 |
| **C** | Copied lstsq + window-3 V1/V2 | **IMPLEMENTED** 2026-09-05 |
| **D** | Interpretation machine + phase | **IMPLEMENTED** 2026-09-05 |
| **E** | `Engine` prepare/commit/reset/maturation | **IMPLEMENTED** 2026-09-05 (proto came after E, not “no proto forever”) |
| **F** | Frozen fixtures + equivalence harness | **EXECUTED** 2026-09-05 — interpretation FAIL (see Phase F) |
| **F-R** | Frozen confirmation state-write | **IMPLEMENTED / PASS** 2026-09-05 |
| **Proto** | `VolumeEvent` family + `StreamVolumeEvents` | **IMPLEMENTED** 2026-09-05 |
| **G** | Host join + isolation + `ResetSymbol` + delivery tests | **IMPLEMENTED** 2026-09-06. Plan’s `QUANTRAM_VOLUME` flag was **not** used. |
| **H** | Broader live/dashboard validation | **DEFERRED / NOT YET AUTHORIZED** |

Dashboard Volume viewers and StageTransition Volume publication remain after V1 unless separately authorized.

## 26. Acceptance criteria (original coding increment — now used as completed-work checklist)

A–G were required to demonstrate:

- what state exists, who owns it, how it is bounded
- how each accepted Bar changes **candidate** state
- when commit happens and when it does not
- exact feature and interpretation calculations
- maturation ≠ INVALID
- reset/time/irregular/zero/undefined/confirmation behavior
- Volume Output contents
- frozen vs operational configuration
- what may be reused from P-04 (lstsq algorithm) and what must not (Price state/windows/EXPM)
- equivalence and genericity
- realtime non-interference
- explicit V1 exclusions

## 27. Known limitations

- Frozen corpora did not validate live zero/all-zero/missing volume.
- 014C interpretation corpus is session-reset in APTF; QuanTRAM V1 is not. Harness must slice or inject test resets.
- `uint64`→`float64` mantissa limit.
- Independent Volume commit can leave a published B_t out of Volume windows if Volume hard-fails; that hole must be explicit, not silent.
- Inherited: Adaptive+Price both-or-neither and shared `lastAccepted` cursor. P-04V must not use that cursor as its consume gate and must not repair the coupling.
- Inherited: sequential `handle` + one recover — **closed by Phase G:** Volume nested recover runs before Adaptive.
- No bitwise NumPy claim.
- Color-age out of scope.
- Full 009V/010/014C corpora remain hash-verified in APTF; Level-1 subsets live under `internal/volume/testdata/`.

## 28. Risks

| ID | Risk | Mitigation |
|---|---|---|
| R01 | Window-3 lstsq vs NumPy | Copy P-04 lstsq contract; tolerances; do not retune bands |
| R02 | Threshold sensitivity 0.9/1.1 | Exact inclusivity tests |
| R03 | Accidental shared A+P+V transaction **or** gating Volume on A+P commit | DELIVERY-02/04; Volume fail does not roll back Adaptive; Price fail does not suppress Volume |
| R09 | Silent Volume continuity after lost published B_t | DELIVERY-11; overflow latch; explicit Volume skip |
| R10 | Treating `lastAccepted` as P-02 publication | Design forbids Volume consume gate on A+P commit |
| R04 | Volume panic latching worker disc | Nested recover required in Phase G |
| R05 | Session-reset mismatch vs 014C | Session-sliced harness |
| R06 | Importing `internal/pricing` | Forbidden; copy lstsq |
| R07 | Named-entity hard-code | Genericity test |
| R08 | Treating 29 as runtime | Readiness from windows only |

## 29. Implementation questions — disposition after Phase G

No new scientific questions.

1. **Exact float tolerances** for 010 V1/V2 vs gonum — **closed in Phase F** (measured max abs; see Phase F-R).
2. **Volume vs Adaptive order inside `handle`** — **closed in Phase G:** isolated `processVolume` before Adaptive/Price.
3. **Internal Volume fan-out API** — **closed in Phase G / proto:** `SubscribeVolumeEvents` mirrors Price; `StreamVolumeEvents` is live.
4. **Optional later `internal/numutil` extraction** — still not required for V1.
5. **Volume continuity after explicit non-consume of published B_t** — **closed in Phase G** with Volume-only `volDisc` (explicit `ENGINE_ERROR` / `VOLUME_DISCONTINUOUS`; restore via `ResetSymbol`; A+P unaffected). Shared worker `disc` still blocks all siblings.
6. **Inherited Adaptive↔Price both-or-neither** — documented; not repaired by P-04V.

## Terminology — Indicator (Phase E)

Canonical P-04V Volume Output field: **`Indicator`**.

Historical APTF / Phase D internal name: `cockpit_color` / `CockpitColor`.

Mapping: `VolumeEvent.Indicator` is the confirmation-controlled category. Phase D internals historically used `CockpitColor` as the interpretation-result field name. P-04V proto `VolumeIndicator` / `indicator` is canonical. P-04 Price `CockpitColor` / `cockpit_color` was **not** renamed in this pass.

**DEFERRED / NOT YET AUTHORIZED:** a controlled QuanTRAM-wide `cockpit` → `Indicator` audit across P-04 Price, StageTransition, Process Model, and dashboard. Do not treat that as part of P-04V V1.

## Phase F — Frozen APTF equivalence (2026-09-05) — HISTORICAL FAIL

**This is not current status.** Phase F found the confirmation-state mismatch. Phase F-R is the authoritative frozen-equivalence result.

**Status at Phase F close:** FAIL for clean APTF interpretation equivalence. Feature / derivative / raw-color / phase / confidence / domain PASS. Production science was **not** changed in Phase F itself.

**Validation document:** [QuanTRAM_P04V_VOLUME_FROZEN_EQUIVALENCE_VALIDATION_2026-09-05.md](../investigations/QuanTRAM_P04V_VOLUME_FROZEN_EQUIVALENCE_VALIDATION_2026-09-05.md)

**Fixture provenance:** `internal/volume/testdata/` (policy exact copy + deterministic Level-1 subsets). Full 009V/010/014C corpora remain hash-verified in APTF `pre08242026_docs/` and are read only by tests when present. Production Engine does not read testdata.

**Exact frozen result (hash-verified 010 / 014C):**

- `V_RAW`, `V_N`, `predicted_next_V_N`: 101,205 / 101,205 exact via `volume.Engine`
- `interval_mean_vn`, `V1`, `V2`: 101,205 / 101,205 within 1e-12 / 1e-9 (measured max abs 5.7e-14 / 7.8e-12 / 2.7e-12)
- `raw_color`, `phase`, `confidence`, `domain_state`: 55,199 / 55,199 exact
- `Indicator` / historical `cockpit_color`: 54,890 / 55,199 (309 mismatch)
- `transition_state`: 52,393 / 55,199

**Unresolved discrepancy at Phase F close (preserved):** frozen HEAD `VolumeEngine.observe` writes emitted cockpit color into `VolumePolicyState.color` while pending. Phase D / production `confirmColor` then kept the last confirmed color. Indicator 309 / 55,199 mismatch. Human review chose frozen APTF.

## Phase F-R — Frozen confirmation state-write repair (2026-09-05) — CURRENT AUTHORITY

Phase F found the confirmation-state mismatch. Phase F-R corrected the state-write behavior to frozen executable authority. Frozen session-sliced 014C interpretation equivalence then passed exactly.

**Human decision:** preserve the frozen APTF confirmation state machine.

**Exact correction:** `confirmColor` now sets `next.Color` to the emitted Indicator (`VolumePolicyState.color = cockpit_color`). While pending, `Color` becomes AMBER. Feature, derivative, threshold, phase, confirmation count, and Engine Prepare/Commit architecture were not changed. Production automatic session reset was not added.

**Revalidation (session-sliced 014C, hash-verified):**

- `raw_color`, `Indicator` / `cockpit_color`, `transition_state`, `phase`, `confidence`, `domain_state`: **55,199 / 55,199**
- `V_RAW`, `V_N`, `predicted_next_V_N`: 101,205 exact
- `V1` / `V2` / `interval_mean_vn`: unchanged tolerances (max abs 7.84e-12 / 2.73e-12 / 5.68e-14; outside_tol=0)

Continuous Engine vs 014C `Indicator` still differs at historical session cuts (**41** mismatches). That is harness lifecycle, not repaired.

First Phase F mismatch `2023-03-30T11:16:00Z` now matches: raw GREEN, Indicator AMBER, `PENDING_GREEN`.

## P-04V Protobuf Contract Implementation

**Purpose:** Add the already-validated P-04V Volume Engine to the canonical QuanTRAM protobuf contract. Science is unchanged. **HISTORICAL (contract phase):** ModelHost was not yet wired. Phase G (2026-09-06) wired `StreamVolumeEvents` to Host-produced events.

**Source proto path:** `api/proto/quantram/v1/quantram.proto`  
**Package / go_package:** `quantram.v1` / `quantram/gen/quantram/v1;quantramv1`

**P-04 structural precedent:** P-04 Price is not a dedicated microservice. It is exposed as `ModelService.StreamPriceEvents` (server streaming) with `PriceEvent` / `PriceEmission` / `PricingSkip` / `PricingStatus`. P-04V mirrors that **structure** on the same service: `StreamVolumeEvents` → `VolumeEvent` / `VolumeEmission` / `VolumeSkip` / `VolumeStatus`. Price science (EXPM, `rk_success`, `PriceCockpit`) is not copied.

**Temporary terminology asymmetry (intentional):** P-04 keeps historical `PriceCockpit` / `cockpit_color`. New P-04V fields use canonical **Indicator**. Historical APTF `cockpit_color` remains provenance-only.

### New messages

| Message | Role |
|---|---|
| `VolumeQuantity` | One scientific scalar + readiness; `optional double value` only when AVAILABLE |
| `VolumeEmission` | Validated quantities + raw color + Indicator + transition + phase + confidence + domain |
| `VolumeSkip` | Non-AVAILABLE outcome (`MATURING` / `INVALID` / `ENGINE_ERROR`) |
| `VolumeEvent` | Publication envelope + lineage + status + emission + skip |
| `StreamVolumeEventsRequest` | Same shape as `StreamPriceEventsRequest` (`symbols`, `max_events`) |

### New enums

| Enum | Values (number) |
|---|---|
| `VolumeStatus` | UNSPECIFIED=0, MATURING=1, AVAILABLE=2, INVALID=3, ENGINE_ERROR=4 |
| `VolumeQuantityStatus` | UNSPECIFIED=0, INSUFFICIENT=1, AVAILABLE=2, UNDEFINED=3 |
| `VolumeIndicator` | UNSPECIFIED=0, GREEN=1, AMBER=2, RED=3 (not BUY/SELL/HOLD; shared by raw color and Indicator) |
| `VolumeTransition` | UNSPECIFIED=0, STABLE=1, PENDING_GREEN=2, PENDING_AMBER=3, PENDING_RED=4, CONFIRMED_GREEN=5, CONFIRMED_AMBER=6, CONFIRMED_RED=7 |
| `VolumePhase` | UNSPECIFIED=0, ACTIVITY_STATIONARY=1, INCREASING_ACCELERATING=2, INCREASING_DECELERATING=3, DECREASING_ACCELERATING=4, DECREASING_DECELERATING=5 |
| `VolumeConfidence` | UNSPECIFIED=0, HIGH=1 (thin historical metadata; no invented score) |
| `VolumeDomainState` | UNSPECIFIED=0, CAUSAL_LOCAL_VOLUME=1 |
| `VolumeSkipReason` | UNSPECIFIED=0, MATURING=1, INVALID=2, ENGINE_ERROR=3 |

**Status semantics:** MATURING ≠ ERROR. INVALID ≠ ENGINE_ERROR. AVAILABLE means required scientific output is available. Maturation is expected (V_N first possible at raw observation 15; V1/V2 at 17; fully finite `interval_mean_vn` at 29).

**Indicator semantics:** confirmation-controlled categorical state. Distinct from `raw_color`. Not a trading instruction.

**Presence semantics:** unavailable quantities omit `VolumeQuantity.value`. Available zero sets `value = 0` with status AVAILABLE. NaN is not a wire presence mechanism.

**V_RAW wire type:** `double` inside `VolumeQuantity`. Canonical scientific output is `float64(Bar.Volume)`. The Bar integer is not duplicated.

**G_V:** `predicted_next_v_n` only. No trajectory, horizon, solver, RK, or EXPM fields.

**Lineage:** `symbol`, `market_snapshot_id`, `interval_start_unix_ms`, `interval_end_unix_ms`, `source_timestamp`. Same-Bar future correlation uses `market_snapshot_id` + initiating `interval_start`. `source_timestamp` remains provider provenance.

**EffectiveTime:** not present. Volume has no independent effective-time concept; initiating-Bar lineage is sufficient.

**Internal VolumeState not serialized:** raw/VN/time rings, pending confirmation color/count, engine generation.

**Service exposure:** additive RPC on existing `ModelService`:

```text
rpc StreamVolumeEvents(StreamVolumeEventsRequest) returns (stream VolumeEvent);
```

No dedicated Volume protobuf service. `SemanticService` unchanged (read-only vocabulary, not science). Adapter `toProtoVolumeEvent` translates domain → proto without mathematics. Phase G wires `StreamVolumeEvents` to Host-produced events.

**Generated artifacts:** `gen/quantram/v1/quantram.pb.go`, `gen/quantram/v1/quantram_grpc.pb.go` via `buf generate`. Not manually edited.

**Tests:** `internal/server/volume_contract_test.go` (C01–C25 plus mapper, isolation, unwired RPC).

**Compatibility:** additive only. Existing P-03/P-04 field numbers, enum values, and service methods unchanged.

**Contract-phase exclusions (historical):** at contract landing, ModelHost was not yet wired. StageTransition, Process Model, Snapshot/Persistence/Mongo/Aperture, P/V fusion, trading verbs, Volume RK/EXPM, and scientific change to `internal/volume` remained out of that increment. Phase G later wired ModelHost; the other exclusions still hold.

**Known limitations (contract phase, superseded by Phase G for RPC/host):** Semantic catalog does not yet document P-04V terms. Dashboard proto copy is not updated in this phase.

**Next phase after contract:** Phase G ModelHost realtime integration (landed 2026-09-06).

## Phase G — ModelHost Realtime Runtime Integration

**Purpose:** Attach the validated P-04V Engine to the existing keyed ModelHost worker, publish canonical VolumeEvents on `ModelService.StreamVolumeEvents`, and emit Price-style terminal diagnostics. Science and proto are unchanged.

**Scope:** `internal/modelhost` host join, `internal/server` stream wiring, publication-envelope fields on `domain.VolumeEvent`, Phase G tests, this section. No Volume/Price/Adaptive mathematics. No proto redesign. No StageTransition. No Process Model.

**Runtime topology:**

```text
Pipeline.fanoutModel
    → one SubscribeModelBars
    → keyed worker inbox
    → common gates
    → isolated processVolume (independent commit)
    → Adaptive PrepareStep
    → Price PrepareStep
    → existing commitA && commitP
```

No second subscription, mailbox, or Volume worker goroutine.

**Input:** the same published eligible `domain.Bar` after infer / eligibility / continuity / proven-missing / pre-prepare timeout gates.

**Output:** `domain.VolumeEvent` on Host volume subscribers + last-event catch-up; proto via existing mapper; `log.Printf` lines.

**Configuration:** Volume is on whenever Adaptive Host exists. No `QUANTRAM_VOLUME` flag. Price remains `QUANTRAM_PRICING`. Logging uses the standard library logger with no extra env (same as `pricing emit` / `model skip`).

**Ownership:** one `volume.Engine` per symbol worker. Keyed worker serializes mutation.

**Lifecycle:** constructed in `Host.New`; `ResetSymbol` rebuilds a cold engine and clears `volSeq` / `volDisc`. No automatic session/date/source reset.

**Common gates:** worker `disc`, infer off, not final/not eligible, duplicate/regression/unaligned, proven missing INPUT_GAP, pre-prepare timeout. These deny all siblings, including Volume.

**Processing opportunity:** after those gates, Volume is always offered B_t even if Adaptive or Price later fails or panics.

**Independent Volume commit:** `volume.Commit` is not part of `commitA && commitP`. Price/Adaptive failure cannot roll it back. Volume failure cannot roll back A+P.

**A+P joint commit:** unchanged. `worker.lastAccepted` still advances only on Adaptive+Price joint success.

**accepted_sequence semantics:** 1-based count of successful Volume commits on that worker for this engine lifetime (reset on `ResetSymbol`). It is **not** `worker.lastAccepted` and is not the Host event-id counter. Analogous to Price `AcceptedSequence` (engine observation index), not the A+P cursor.

**Lineage:** `VolumeEvent` copies initiating Bar `MarketSnapshotID`, `IntervalStart`, `IntervalEnd`, `SourceTimestamp`. No invented EffectiveTime.

**Reset:** `ResetSymbol` replaces Adaptive, Price (if enabled), and Volume engines; clears Volume latch/sequence. Other symbols unaffected.

**Failure isolation:** Volume prepare/panic uses `volDisc` (Volume-only). Subsequent Volume observations emit explicit `ENGINE_ERROR` / `VOLUME_DISCONTINUOUS` and do not advance Volume state. A+P continue. Adaptive injected panic still uses the outer recover after Volume has run.

**Positional discontinuity:** Volume ENGINE_ERROR or panic latches Volume only. Common worker `disc` (overflow, proven gap, Adaptive panic) still blocks all siblings.

**RPC publication:** `SubscribeVolumeEvents` / `LastVolumeEvents` / `UnsubscribeVolumeEvents` mirror Price. Non-blocking send; full buffer drops + log. `StreamVolumeEvents` does not ingest Bars. Off Host → `FailedPrecondition` (`volume is not wired`).

**Terminal diagnostics:** `log.Printf` in the Host, same logger as Price.

- Startup: `volume engine initialized symbols=N mode=discrete_G_V`
- AVAILABLE: `volume emit symbol=... status=AVAILABLE indicator=... raw=... vn=...`
- MATURING: `volume maturing ... vn=INSUFFICIENT|value ... indicator=UNAVAILABLE`
- INVALID: `volume invalid ...`
- ENGINE_ERROR: `volume error ...`

Unavailable quantities print `INSUFFICIENT` / `UNDEFINED` / `UNAVAILABLE`, never a fake `0`. Observational only.

**Backpressure:** drop on full subscriber buffer; science continues (same as Price).

**Tests:** `internal/modelhost/volume_host_test.go`, `internal/server` live `StreamVolumeEvents`.

**Known limitations:** StageTransition still P-03/P-04 only. Semantic catalog still lacks P-04V terms. Dashboard proto copy not updated. Volume is Adaptive-host-scoped (Host is nil when `QUANTRAM_MODEL=off`).

**Exclusions:** StageTransition, Process Model, Snapshot/Persistence/Mongo/Aperture, P/V fusion, trading, Volume RK/EXPM, proto redesign.

**Next phase:** separately authorized StageTransition / Process Model reconciliation. Not started.

## Change log

| Date | Change |
|---|---|
| 2026-09-05 | Initial proposed P-04V implementation design. Independent Volume prepare/commit; `internal/volume`; frozen science unchanged; StageTransition/proto/Process Model/Phase 1 design untouched. Implementation not authorized. |
| 2026-09-05 | Phase 3A: verified current P-02 `fanoutModel` / `SubscribeModelBars` / keyed-worker delivery; documented three distinct “accept” meanings; corrected Volume so it is offered published B_t independently of Adaptive+Price commit; added delivery invariant, omission audit, and DELIVERY-01–12. No production code changed. |
| 2026-09-05 | Phase A foundation landed (`internal/domain/volume.go`, `internal/volume` config/mapper/state). Time representation: retain canonical `IntervalStart` (`time.Time`); do not precompute unix-ms/60_000 minutes in Phase A. Feature mathematics and host join not started. |
| 2026-09-05 | Phase B feature mathematics landed (`median.go`, `features.go`). Rolling median-15 = copied `sorted[7]`; `V_N = V_RAW_current / median` when median `> 0` and finite; unavailable `V_N` stored as positional NaN; `interval_mean_vn` requires 15 finite positional `V_N` (no nanmean); `VOLUME_POINT` is `predicted_next_V_N = V_N`. FeatureStatus distinguishes maturation (`FeatureInsufficient`) from mathematically undefined (`FeatureUndefined`). `AdvanceFeatures` is a candidate-state helper, not Engine PrepareStep/Commit. V1/V2, interpretation, host join not started. |
| 2026-09-05 | Phase C derivative mathematics landed (`linalg.go`, `derivatives.go`). Copied P-04 gonum SVD `lstsq` (NumPy `rcond=None` cutoff, `gonum.org/v1/gonum v0.17.0`) locally; do not import `internal/pricing`. Window-3 positional `V_N` + actual `IntervalStart` elapsed minutes (`τ_i = (t_i − t_current).Minutes()`, `τ_current = 0`). `V_N(τ)=aτ²+bτ+c`, `V1=b`, `V2=2a`. Duplicate/non-increasing times and positional NaN/Inf are `FeatureUndefined`. Interpretation, engine lifecycle, and host join not started. |
| 2026-09-05 | Phase D interpretation mathematics landed (`interpretation.go`, `phase.go`). `Interpret` is a pure prior→candidate transform; committed `InterpretationState` is not written. Raw color from `interval_mean_vn` with inclusive 0.9/1.1; first valid color confirms immediately; pending forces cockpit AMBER; confirm=2. Phase uses frozen `ACTIVITY_*` labels and ε=1e-12. Confidence `HIGH` and domain `CAUSAL_LOCAL_VOLUME` only on valid color interpretation. Representation labels live in `internal/domain/volume.go`; machine stays in `internal/volume`. Color-age, session reset, final INVALID emission, and Engine PrepareStep/Commit not started. |
| 2026-09-05 | Phase E engine lifecycle landed (`engine.go`, `domain.VolumeEvent`). Per-entity `Engine` owns `VolumeState`. `PrepareStep` clones (generation-tagged) and never mutates committed rings; `Commit` adopts only a same-generation candidate. MATURING candidates commit so windows can mature. INVALID records the causal NaN `V_N` position and does not advance `InterpretationState`. `VolumeEvent.Indicator` is the canonical categorical output (`cockpit_color` historical). No modelhost/proto/StageTransition/Process Model changes. |
| 2026-09-05 | Phase F frozen APTF equivalence validation. Hash-verified 009V/010/014C. Feature/derivative Engine replay PASS. Indicator/transition FAIL vs APTF `state.color=cockpit` write (309/55,199). Volume mathematics not changed. Production session reset not added. Host integration not started. |
| 2026-09-05 | Phase F-R: `confirmColor` writes `next.Color =` emitted Indicator (frozen APTF). Session-sliced 014C Indicator/transition 55,199/55,199. No other Volume mathematics changed. No production session reset. Phase G not started. |
| 2026-09-05 | P-04V protobuf contract: additive `VolumeEvent` family + `ModelService.StreamVolumeEvents`. Adapter mapper only. Science, ModelHost, StageTransition, and Process Model unchanged. |
| 2026-09-06 | Phase G: per-worker Volume Engine on ModelHost; independent commit; `StreamVolumeEvents` live; Price-style terminal diagnostics. Science, proto numbering, StageTransition, and Process Model unchanged. |
| 2026-09-06 | Documentation reconciliation: renamed this file to `QuanTRAM_P04V_VOLUME_ENGINE_IMPLEMENTATION_090526.md`; header/status reconciled; Current Validated Implementation added; stale no-Go/no-proto/no-host claims marked historical; `QUANTRAM_VOLUME` marked SUPERSEDED PRE-IMPLEMENTATION PROPOSAL; Phase F FAIL preserved as history and F-R PASS stated as current; Phase G marked implemented; P-04V Indicator terminology aligned (historical `cockpit_color` retained as provenance). No science/code/proto/Process Model changes in this documentation pass. |
| 2026-09-06 | Final post-alignment cleanup: reconciled equivalence-fixture status, converted future delivery-test wording to completed Phase G validation, separated completed runtime validation from deferred external/live-market validation, and removed stale implication of an independent Volume off mode. No science/code/proto/Process Model changes. |
