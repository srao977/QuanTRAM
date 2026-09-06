# QuanTRAM Volume Engine — Scientific and Architectural Design

**Title:** QuanTRAM Volume Engine — Scientific and Architectural Design  
**Date:** 2026-09-06  
**Status:** IMPLEMENTED AND SCIENTIFICALLY VALIDATED THROUGH PHASE G. This document is the scientific and architectural design authority. Coding chronology lives in the implementation record. StageTransition Volume publication and Process Model file reconciliation remain **DEFERRED / NOT YET AUTHORIZED**.  
**Purpose:** Define P-04V Volume Engine: validated Volume mathematics, QuanTRAM realtime consume/state/emit architecture, inputs, outputs, bounded per-entity state, timing, invariants, and boundaries.  
**Scope:** Scientific and architectural design. Mathematics in this document are frozen. Implementation details, A–G chronology, and host/RPC wiring are recorded in [P-04V implementation record](../implementations/QuanTRAM_P04V_VOLUME_ENGINE_IMPLEMENTATION_090526.md). This pass does not change Go, proto, StageTransition, or the Process Model file.  
**Parents:** [Process Model](QuanTRAM_PROCESS_MODEL_V1_082926.md) (filename as of this reconciliation; **contents not edited**), [P-03 Adaptive Model Host](QuanTRAM_P03_ADAPTIVE_MODEL_HOST_083126.md), [P-04 Price Engine](QuanTRAM_P04_PRICE_ENGINE_090226.md), [Stage Transition Publication V1.1](QuanTRAM_STAGE_TRANSITION_PUBLICATION_V1_2026-09-04.md)  
**Implementation record:** [QuanTRAM_P04V_VOLUME_ENGINE_IMPLEMENTATION_090526.md](../implementations/QuanTRAM_P04V_VOLUME_ENGINE_IMPLEMENTATION_090526.md)  
**Forensic authority:** [APTF Volume Engine Go Refactorability Investigation, 2026-09-05](../investigations/QuanTRAM_APTF_VOLUME_ENGINE_GO_REFACTORABILITY_INVESTIGATION_2026-09-05.md)  
**Historical executable authority:** APTF commit `ae0dacb2e02c5b80c82f6662d1a3c6863f4b989a`  
**QuanTRAM baseline:** `07417d9c85799949cd3b173067795905513af173` (`quantram-stage-transition-v1.1-validated-2026-09-04`)

## Executive Summary

**P-04V Volume Engine** is a standalone scientific interpretation of accepted eligible `Bar.Volume`. It is a sibling of P-03 Adaptive and P-04 Price Engine in the decision-model region. The “V” identifies Volume. P-04V is **not** a child of P-04 and does **not** consume Price output.

P-01–P-10 numbering is unchanged. P-05 remains OMS and Risk.

P-04V is **not** a port of an APTF batch application. It is:

```text
THE VALIDATED APTF VOLUME MATHEMATICS
        implemented inside
THE EXISTING QUANTRAM REALTIME
CONSUME → STATE → PROCESS → EMIT
PIPELINE MODEL
```

P-04V is **one Volume Engine**. It owns `VolumeState` and performs both scientific responsibilities inside that engine. There is no separate QuanTRAM Volume Policy component, service, layer, or process.

The two internal scientific responsibilities (not architectural separations) are:

1. **Volume Feature Mathematics** — produce `V_RAW`, `V_N`, `V1`, `V2`, `interval_mean_vn`, and `predicted_next_V_N`.
2. **Volume Interpretation Mathematics** — apply the frozen APTF interpretation configuration `V_INTERVAL_B10_C2` and emit Volume Output plus the next Volume Interpretation State (inside `VolumeState`).

The historical APTF class named `VolumeEngine` implemented only interpretation. QuanTRAM P-04V includes both responsibilities.

P-03 already uses raw `Bar.Volume` through D01 `updateVolumeInfluence`. That Adaptive path remains and is not P-04V.

P-04V follows the established QuanTRAM realtime model used by Adaptive and Price: consume the existing accepted eligible Bar, own bounded per-entity scientific state, prepare/commit without partial mutation, emit a first-class Volume outcome. It does not create another market subscription.

APTF 015 BUY/HOLD/SELL, P/V fusion, color-age as a required output, Snapshot, and StageTransition Volume publication remain outside P-04V V1 science. StageTransition V1.1 remains frozen. The Volume protobuf contract and ModelHost join are implemented (see Current Implementation Status); they are not scientific dependencies.

## Current Implementation Status

This design is **implemented**. Coding chronology is not duplicated here.

| Item | Status as of 2026-09-06 |
|---|---|
| Go Volume science (`internal/domain/volume.go`, `internal/volume`) | Implemented |
| Frozen feature equivalence (009V / 010) | PASS |
| Phase F interpretation mismatch | Historical FAIL (confirmation state-write) |
| Phase F-R frozen confirmation repair | PASS — session-sliced 014C Indicator / transition 55,199 / 55,199 |
| Test010 → Test014C scientific reconciliation | Outcome A; discrete `G_V` accepted |
| Canonical protobuf (`VolumeEvent` family on `ModelService`) | Implemented |
| `ModelService.StreamVolumeEvents` | Implemented |
| Phase G ModelHost realtime join | Implemented |
| StageTransition Volume publication | **DEFERRED / NOT YET AUTHORIZED** |
| Process Model file reconciliation | **DEFERRED / NOT YET AUTHORIZED** (separate pass) |

There is **no** `QUANTRAM_VOLUME` flag. Volume is present whenever Adaptive Host exists (`QUANTRAM_MODEL=adaptive`). Price remains independently `QUANTRAM_PRICING`.

## Governing principle

APTF historical artifacts define the Volume **scientific transformation** and frozen **migration-equivalence** targets. They do **not** automatically define QuanTRAM runtime lifecycle, storage, session, buffering, state-management, or batch-processing semantics.

Where historical APTF runtime/batch mechanics differ from the established QuanTRAM realtime architecture, **preserve the validated Volume mathematics** and **use the established QuanTRAM realtime architecture** unless the Volume mathematics themselves demonstrably require otherwise.

Every factual statement below is labeled as one of:

| Label | Meaning |
|---|---|
| **APTF FORENSIC BEHAVIOR** | What the historical batch/replay path did |
| **FROZEN SCIENTIFIC AUTHORITY** | Volume Feature Science and Volume Interpretation Science that P-04V must reproduce |
| **QUANTRAM REALTIME BEHAVIOR** | How P-04V operates in the live pipeline |

The reader must not infer the category.

## QuanTRAM terminology

**Canonical QuanTRAM:** P-04V Volume Engine · `VolumeState` · Volume Feature State · Volume Interpretation State · Volume Mathematics · Volume Output · **Indicator** (confirmation-controlled activity category) · `raw_color` (pre-confirmation activity band)

**Historical / provenance only:** APTF Volume Policy · `V_INTERVAL_B10_C2` · `V_EMISSION_V0_1` · historical `VolumePolicyState` · historical APTF `cockpit_color` (same scientific quantity as Indicator)

Historical names remain where forensic traceability or equivalence testing requires them. They are not QuanTRAM architectural ontology. P-04V does not introduce a Volume Policy Service, layer, or process.

## Module / System Overview

### Process identity

| ID | Process | Notes |
|---|---|---|
| P-03 | Adaptive Model Host | Sibling; already live |
| P-04 | Price Engine | Sibling; already live |
| **P-04V** | **Volume Engine** | Inserted scientific sibling; does not renumber P-05–P-10 |
| P-05 | OMS and Risk | Unchanged; not redesigned here |

Approved sequence remains: P-01, P-02, P-03, P-04, **P-04V**, P-05, P-06, P-07, P-08, P-09, P-10, C-01.

### QuanTRAM realtime architecture

```text
                         P-01 MARKET FEED
                                |
                                v
                    P-02 INGESTION / DATA QUALITY
                                |
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
       Adaptive State       Price State       Volume State
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

This is **not** `P-03 → P-04 → P-04V`. P-04V does not consume Price output.

**QUANTRAM REALTIME BEHAVIOR (Phase G, 2026-09-06):** publication remains `Pipeline.fanoutModel` → one `SubscribeModelBars` → keyed worker. After common host gates, Volume is offered B_t in an isolated `processVolume` block, then Adaptive `PrepareStep`, then Price `PrepareStep`, then the existing Adaptive+Price joint commit. Volume commit is independent. Physical order is **not** scientific precedence; Volume runs first so an Adaptive/Price panic cannot deny an already-published B_t. `worker.lastAccepted` remains the Adaptive+Price joint scientific-commit cursor. Volume `accepted_sequence` is the successful Volume-commit count. There is no second mailbox or Volume worker goroutine.

Architectural symmetry with P-04:

```text
P-04 PRICE ENGINE                   P-04V VOLUME ENGINE

Accepted Bar                        Accepted Bar
     |                                   |
   CONSUME                             CONSUME
     |                                   |
  PriceState                         VolumeState
     |                                   |
Price Mathematics                  Volume Mathematics
     |                                   |
    EMIT                                EMIT
     |                                   |
Price Output                        Volume Output
```

P-04V may internally distinguish feature mathematics from interpretation mathematics because Volume science requires both. That is not a second process.

### P-04V consume / state / emit

```text
                         P-04V VOLUME ENGINE

                         Accepted Eligible Bar
                                  |
                               CONSUME
                                  |
                                  v
                            VolumeState
                                  |
                 +----------------+----------------+
                 |                                 |
                 v                                 v
        Volume Feature                     Volume Interpretation
          Mathematics                         Mathematics
                 |                                 |
                 +----------------+----------------+
                                  |
                                  v
                         Candidate Output
                         + Candidate State
                                  |
                                COMMIT
                                  |
                                  v
                         Authoritative State
                                  |
                                EMIT
                                  |
                                  v
                           Volume Output
```

This is **one engine**. Volume Interpretation is not a separate service or process.

**QUANTRAM REALTIME BEHAVIOR:** The incoming Bar is immutable. P-04V scientific state is private to P-04V. Candidate calculations must not partially mutate committed scientific state. A failed or non-actionable computation must not leave partially advanced state. Implemented prepare/commit is `volume.Engine.PrepareStep` / `Commit`. Volume commit is independent of Adaptive+Price `commitA && commitP`. API names and host order are in the [implementation record](../implementations/QuanTRAM_P04V_VOLUME_ENGINE_IMPLEMENTATION_090526.md).

### Volume internal scientific path

**FROZEN SCIENTIFIC AUTHORITY:**

```text
accepted eligible Bar.Volume
        ↓
     V_RAW
        ↓
 ROLLING_MEDIAN_RATIO_15
        ↓
      V_N
        ↓
 causal quadratic window 3          trailing-15 mean(V_N)
        ↓                                    ↓
     V1 / V2                         interval_mean_vn
        ↓
 predicted_next_V_N = V_N
        ↓
 Volume Interpretation Mathematics
 (historical APTF configuration V_INTERVAL_B10_C2)
        ↓
 Volume Output + Volume Interpretation State
```

### Raw-volume dual use

```text
                         Bar.Volume
                              |
                +-------------+-------------+
                |                           |
                v                           v
       P-03 Adaptive influence        P-04V Volume path
       updateVolumeInfluence          V_N / V1 / V2
       → effective mass               interval_mean_vn
                                      → Volume Interpretation
                                      → Volume Output
```

These paths share only the originating observation.

P-04V uses P-04 / Adaptive as its **realtime architectural reference** (same Bar ingress, per-entity owned state, bounded causal state, incremental processing, prepare/commit, first-class emit, no second subscription). It does **not** copy P-04 mathematics or share P-04 scientific state. V1 copies the gonum lstsq algorithm locally and does **not** import `internal/pricing`. P-04’s 15-observation price derivative window must not be reused as Volume’s 3-observation window.

## Inputs

**QUANTRAM REALTIME BEHAVIOR:** the same accepted eligible canonical `domain.Bar` already supplied to P-03 and P-04. No second market subscription. No second `SubscribeModelBars` mailbox.

| Field | Role |
|---|---|
| `Symbol` | Entity key. No named ticker is hard-coded. |
| `Volume` | `V_RAW` source (`uint64`; observed raw participation) |
| `IntervalStart` | Elapsed-minute coordinate for V1/V2, matching P-04 `Observation.Minutes` (`UnixMilli / 60_000`) |
| `SourceTimestamp` | Correlation / payload timestamp |
| `MarketSnapshotID` | Existing QuanTRAM correlation identity |
| `QualityStatus`, `IsFinal` | Eligibility already decided by P-02 |

Additional consume input: the current committed per-entity `VolumeState` (feature windows plus Volume Interpretation State).

**APTF FORENSIC BEHAVIOR:** batch runners read CSV series and JSON arrays. Those files are equivalence authorities, not P-04V runtime inputs.

## Outputs

First-class Volume outcome. Canonical wire names are in `api/proto/quantram/v1/quantram.proto` (`VolumeEvent` / `VolumeEmission`). Field numbers are implemented; this table is scientific meaning, not a proto redesign.

| Output | Meaning |
|---|---|
| `v_raw` | Copy of `V_RAW` |
| `v` / `v_n` | `V_N` |
| `v1`, `v2` | Causal quadratic derivatives of `V_N` |
| `projected_v` / `predicted_next_v_n` | `predicted_next_V_N` = `V_N` (VOLUME_POINT / accepted `G_V`) |
| `projected_v1`, `projected_v2` | Historically unsupported; remain unset / not fabricated |
| `activity_state_value` | Frozen interpretation input: `interval_mean_vn` |
| `raw_color` | Threshold classification before confirmation |
| **Indicator** | Confirmation-controlled activity category (GREEN / AMBER / RED). Historical APTF field: `cockpit_color`. |
| `phase` | V1/V2 activity-phase label |
| `transition_state` | STABLE / PENDING_* / CONFIRMED_* |
| `confidence_state` | HIGH under frozen `INTERVAL_MEAN_V_N` |
| `domain_state` | Historical constant `CAUSAL_LOCAL_VOLUME` |
| `reason_codes` | Activity + phase + confirmation tags |
| next Volume Interpretation State | Confirmation-machine fields after commit (inside `VolumeState`) |

`raw_color` and Indicator share the GREEN/AMBER/RED value domain but are scientifically distinct. Color is **activity band**, not price direction and not BUY/SELL/HOLD.

**QUANTRAM REALTIME BEHAVIOR:** expected insufficient causal state produces typed **MATURING**, not INVALID. INVALID is reserved for genuinely unusable scientific input once the corresponding calculation should otherwise be available. Wire statuses: `MATURING`, `AVAILABLE`, `INVALID`, `ENGINE_ERROR`. Unavailable quantities are absent on the wire; available zero is present and zero. Volume has no independent EffectiveTime. Lineage is `symbol`, `market_snapshot_id`, `interval_start`, `interval_end`, `source_timestamp`.

## Parameters / Configuration

**FROZEN SCIENTIFIC AUTHORITY.** Not arbitrary runtime knobs.

### Volume Feature Science

| Identity / parameter | Frozen value |
|---|---|
| Normalization | `ROLLING_MEDIAN_RATIO_15` |
| Raw-volume window | 15 observations |
| Derivative window | 3 observations |
| Interval-mean window | 15 observations |
| Projection | `VOLUME_POINT` (`predicted_next_V_N = V_N`) |
| Derivative time coordinate | actual `IntervalStart` elapsed minutes |

### Volume Interpretation Science

| Identity / parameter | Frozen value |
|---|---|
| Historical APTF frozen interpretation configuration | `V_INTERVAL_B10_C2` |
| Historical freeze label | `V_EMISSION_V0_1` |
| `state_source` | `INTERVAL_MEAN_V_N` |
| `lower_threshold` | `0.9` |
| `upper_threshold` | `1.1` |
| `confirmation_observations` | `2` |
| `epsilon` | `1e-12` |

`V_INTERVAL_B10_C2` is historical/frozen scientific identity. It is **not** a QuanTRAM Volume Policy Service, layer, or process.

**Canonical QuanTRAM terms:** P-04V Volume Engine, `VolumeState`, Volume Feature State, Volume Interpretation State, Volume Mathematics, Volume Output, Indicator.

**Historical / provenance terms only:** APTF Volume Policy, `V_INTERVAL_B10_C2`, `V_EMISSION_V0_1`, historical `VolumePolicyState` (maps conceptually to Volume Interpretation State), historical APTF `cockpit_color`. Implemented Go names include `volume.Engine`, `volume.State`, `InterpretationState`.

D01 Adaptive constants (`reference_alpha = 0.05`, influence bounds `[0, 3]`) belong to P-03 and are **not** P-04V parameters.

## Assumptions

- Forensic investigation 2026-09-05 is the scientific source. This design does not re-select candidates.
- Executable APTF mathematics and frozen hashes win over prose when they conflict.
- APTF batch/session/replay mechanics do not automatically become QuanTRAM runtime.
- Observation-count windows are not elapsed-minute windows.
- QuanTRAM elastic/nonlinear interval concepts are not retrofitted onto this science.
- Historical entity names in APTF artifacts are evidence labels only.
- StageTransition V1.1 remains frozen.

## Explicit Exclusions

Outside this Volume V1 increment (not rejected forever):

- 009V candidate re-selection
- 010 rejected forecast models, including 60-row volume regression
- SciPy, Python runtime, matplotlib
- P/V mathematical fusion
- APTF 015 BUY/HOLD/SELL interpretation
- APTF 016 paper behavior
- Redesign of P-05 OMS / Risk, Execution, ledger
- DNA / Quantram_transaction, Dynamic Meaning Matrix, Decision Engine / Forum
- Snapshot, Persistence, MongoDB
- StageTransition Volume contract (**DEFERRED / NOT YET AUTHORIZED**)
- Renumbering P-01–P-10
- Treating APTF batch files as live inputs
- A `QUANTRAM_VOLUME` operator flag (not implemented; Volume follows Adaptive Host)

The Volume protobuf contract and ModelHost join are **implemented**. They are no longer exclusions of existence. Process Model file text may still lag; that file is reconciled in a separate pass.

---

## 1. What P-04V means

P-04V answers:

**Relative to a causal local participation baseline, is current normalized activity above, near, or below baseline, and has that band been confirmed?**

It does **not** answer whether Adaptive should BUY/SELL/HOLD, what Price color is, or what order to send.

`Bar.Volume` is raw market participation. Volume **Indicator** is a confirmed activity-band interpretation of prepared volume features (historical APTF name: `cockpit_color`). They are not interchangeable.

---

## 2. Two internal scientific responsibilities

Both occur **inside** P-04V Volume Engine. Neither is a separate process.

### A. Volume Feature Mathematics

```text
accepted eligible Bar.Volume
        ↓
     V_RAW
        ↓
 ROLLING_MEDIAN_RATIO_15
        ↓
      V_N
        ↓
 causal quadratic window 3
        ↓
     V1 / V2
        ↓
 trailing-15 mean(V_N)
        ↓
 interval_mean_vn

predicted_next_V_N = V_N
```

This responsibility owns bounded rolling observation state, causality, maturation/readiness, and the feature numerical methods. The historical APTF interpretation class did **none** of this.

### B. Volume Interpretation Mathematics

```text
V_RAW, V_N, V1, V2, predicted_next_V_N, interval_mean_vn
        ↓
 Volume Interpretation Mathematics
 (historical APTF configuration V_INTERVAL_B10_C2)
        ↓
 Volume Output + next Volume Interpretation State
```

This responsibility owns thresholds, hysteresis, phase labels, and INVALID. It does not normalize, differentiate, integrate, or forecast.

Both together are P-04V. Either half alone is incomplete.

---

## 3. Exact scientific definitions

**FROZEN SCIENTIFIC AUTHORITY** unless a subsection is labeled otherwise.

### 3.1 Raw volume

\[
V_{RAW}(t) = \text{accepted Bar.Volume}(t)
\]

Unchanged units. No clipping, winsorizing, imputation, or entity-specific scale. No named instrument is part of the definition.

An observed raw zero is raw market information. Do **not** replace it with previous volume, invent a denominator epsilon, treat it automatically as missing, or invent synthetic volume.

**APTF FORENSIC BEHAVIOR:** the 007 corpus had `zero_count = 0` and `missing_count = 0`. All-zero / missing live-volume behavior was not validated. That limitation remains.

### 3.2 Normalized volume

Frozen method: `ROLLING_MEDIAN_RATIO_15`.

\[
V_N(t) = \frac{V_{RAW}(t)}{\operatorname{median}(\text{last 15 accepted } V_{RAW} \text{ observations including current})}
\]

The window:

- contains **15 observations**
- includes the current observation
- is **observation-count** based
- is **not** 15 elapsed minutes

Baseline must be finite and strictly greater than 0; otherwise `V_N` is unavailable.

Do not call this “15-minute volume.”

### 3.3 Window validity — sequential observations, not finite-value filtering

**FROZEN SCIENTIFIC AUTHORITY**, settled by executable 009V/010 code. These are **not** equivalent:

| Rule | Meaning |
|---|---|
| **Required (frozen)** | Last N observations in the accepted sequence. Computation proceeds only if **all N values are finite** (and, for `V_N` baseline, median `> 0`). |
| **Not authorized** | Collect the last N *finite* values by silently skipping invalid observations. |

Evidence:

- 009V `rolling_baseline` takes `values[index-window+1:index+1]` positionally.
- 009V `causal_quadratic` skips the index unless `np.all(np.isfinite(current))` on that positional window.
- 010 `interval_mean_vn` uses `vn[index-k+1:index+1]` and `continue`s unless `np.all(np.isfinite(vn_window))`.

P-04V must not compact away nonfinite members of the accepted-observation window. If any required positional member is unavailable, that derived quantity is unavailable for this observation.

### 3.4 Volume derivatives

Trailing **three applicable `V_N` observations** means the last three **positional** accepted observations in the normalized-volume window, all finite, using actual elapsed minutes \(T\):

\[
\tau_i = T_{t-2+i} - T_t, \qquad
V_N(\tau) = a\tau^2 + b\tau + c
\]

\[
V1 = b, \qquad V2 = 2a
\]

Design matrix \([ \tau^2,\ \tau,\ 1 ]\). Historical solver: `numpy.linalg.lstsq(..., rcond=None)` with rank 3 and finite coefficients. Failed fit → unavailable, not a fabricated derivative.

- **3 observations, not 3 minutes**
- actual elapsed time is the coordinate, so irregular accepted spacing **does** change \(V1/V2\)
- method is analogous to P-04 causal quadratic fitting
- series is `V_N`, not close
- window is 3, not P-04’s 15

Do not convert rolling observation windows into clock-time windows. Do not feed `V_N` into PriceEngine.

Units: normalized-volume / minute and / minute².

**APTF FORENSIC BEHAVIOR:** `raw_V1 = ΔV_RAW / Δt` is a historical reference only. It is not `V1`.

### 3.5 Interval activity

\[
\text{interval\_mean\_vn}(t) = \operatorname{mean}(\text{last 15 positional } V_N \text{ observations})
\]

All 15 must be finite. **15 observations ≠ 15 minutes.**

This is the feature activity window. It is not the 014C color-age interval.

**APTF FORENSIC BEHAVIOR:** 014C `EmissionIntervalizer` required exact 60.0-second continuity for color age. That is batch/diagnostic machinery. **CLOSED FOR P-04V V1 — DEFERRED:** color-age is out of scope for P-04V V1.

Other 010 JSON fields (`persist_baseline`, elevated/extreme counts, raw std, max/median) are not Volume Interpretation inputs and are not required P-04V outputs.

### 3.6 Volume point projection

\[
\text{predicted\_next\_V\_N}(t) = V_N(t)
\]

**VOLUME_POINT** is the accepted Test010 discrete Volume evolution / state update:

```text
G_V:
V_N_hat(n+1) = V_N(n)
```

`predicted_next_V_N = V_N` is therefore **not** an unfinished placeholder. Test010 accepted discrete `G_V` and explicitly concluded Volume evolution is **not** an ODE.

Historical chronology (do not invert):

- Test010 accepted discrete `G_V` for Volume.
- Later RK45 experimentation included Price and Volume.
- RK45 was supported/useful for Price, not Volume.
- QuanTRAM P-04 Price uses validated analytic EXPM, not RK45.
- QuanTRAM P-04V remains discrete `G_V`.
- P-04V uses neither RK45 nor EXPM.

P-04V does **not** need a future continuous evolution model to become complete.

The historical engine required this field finite, copied it to `projected_v`, and did **not** use it for color or confirmation. P-04V preserves that contract.

`projected_v1` and `projected_v2` remain unsupported and must not be fabricated.

---

## 4. Frozen Volume Interpretation configuration

Historical APTF frozen interpretation configuration: `V_INTERVAL_B10_C2` (`V_EMISSION_V0_1` freeze label). This is scientific identity, not a QuanTRAM architectural policy layer.

| Parameter | Value |
|---|---|
| `state_source` | `INTERVAL_MEAN_V_N` |
| `lower_threshold` | `0.9` |
| `upper_threshold` | `1.1` |
| `confirmation_observations` | `2` |
| `epsilon` | `1e-12` |

Constructor constraints: `state_source ∈ {V_N, INTERVAL_MEAN_V_N}`; `0 < lower < upper`; `confirmation_observations >= 1`. Frozen production uses only `INTERVAL_MEAN_V_N`.

### 4.1 Activity and raw color

```text
activity = interval_mean_vn

if activity >= 1.1 → raw_color = GREEN,  ACTIVITY_ABOVE_BASELINE
if activity <= 0.9 → raw_color = RED,    ACTIVITY_BELOW_BASELINE
else               → raw_color = AMBER,  ACTIVITY_NEAR_BASELINE
```

This is not Price direction.

### 4.2 Confirmation / hysteresis

Volume Interpretation State is **active causal runtime state** inside `VolumeState`. Historical APTF name: `VolumePolicyState`. Implemented Go name: `InterpretationState`.

| Field | Role |
|---|---|
| `color` | Last committed **Indicator** (or unset / INVALID). Historical APTF: `VolumePolicyState.color` |
| `pending_color` | Candidate raw color awaiting confirmation |
| `pending_count` | Consecutive observations of that candidate |

**FROZEN SCIENTIFIC AUTHORITY** after Phase F-R (frozen executable confirmation state machine):

- `state.Color` **is** the emitted Indicator.
- While confirmation is pending, Indicator is AMBER and `state.Color` becomes AMBER.
- `PendingColor` retains the candidate raw color.
- `PendingCount` advances.
- On confirmation, `PendingColor` clears.
- `PendingCount` remains at the confirming increment, matching frozen executable behavior.

**FROZEN SCIENTIFIC AUTHORITY** (engine observe):

1. **First committed evaluation** (`color` unset): accept `raw_color` immediately. `transition_state = STABLE`. No pending.
2. **Same raw color as committed `color`**: Indicator = `raw_color`, clear pending, `STABLE`.
3. **Raw color differs**:
   - increment `pending_count` if `pending_color` already equals this `raw_color`; otherwise start at 1
   - if `pending_count < 2`: force **Indicator = AMBER**, keep `pending_color = raw_color`, `PENDING_{raw_color}`, `STATE_CONFIRMATION_PENDING`
   - if `pending_count >= 2`: accept Indicator = `raw_color`, `CONFIRMED_{raw_color}`, `STATE_CHANGE_CONFIRMED`
4. **Genuinely nonfinite required inputs when evaluation is otherwise due**: INVALID emission; next state `color = INVALID`.

While pending, Indicator is AMBER even if raw is GREEN or RED. Historical APTF wrote the same value as `cockpit_color`.

Phase F found a Go vs frozen `state.color` write mismatch. Phase F-R corrected production `confirmColor` to the frozen rule above. Do not reintroduce the pre-F-R “keep last confirmed color while pending” semantics.

**APTF FORENSIC BEHAVIOR:** the 014C batch caller reset historical `VolumePolicyState` at `date:session`. The engine itself has no calendar. That caller mechanic is **not** QuanTRAM runtime. P-04V V1 has **no** independent interpretation-state reset. See [Realtime lifecycle](#7-realtime-lifecycle).

---

## 5. Volume phase and emission dimensions

**FROZEN SCIENTIFIC AUTHORITY** — do not invent new phase categories:

| Condition | Phase |
|---|---|
| \(\lvert V1\rvert \le \varepsilon\) | `ACTIVITY_STATIONARY` |
| \(V1 > 0\) and \(V2 > \varepsilon\) | `ACTIVITY_INCREASING_ACCELERATING` |
| \(V1 > 0\) and \(V2 \le \varepsilon\) | `ACTIVITY_INCREASING_DECELERATING` |
| \(V1 < 0\) and \(V2 < -\varepsilon\) | `ACTIVITY_DECREASING_ACCELERATING` |
| \(V1 < 0\) and \(V2 \ge -\varepsilon\) | `ACTIVITY_DECREASING_DECELERATING` |

| Dimension | Source | Role |
|---|---|---|
| `raw_color` | activity vs 0.9/1.1 | pre-hysteresis band |
| Indicator | raw + confirmation | published activity category (historical APTF: `cockpit_color`) |
| `phase` | V1/V2 | derivative category |
| `transition_state` | confirmation machine | STABLE / PENDING_* / CONFIRMED_* |
| `confidence_state` | `state_source` | HIGH under frozen `INTERVAL_MEAN_V_N` |
| `domain_state` | constant | `CAUSAL_LOCAL_VOLUME` |
| `reason_codes` | activity + phase + confirmation | diagnostic tags |

These are not all future StageTransition equality candidates.

---

## 6. Scientific authority and generalization boundary

**FROZEN SCIENTIFIC AUTHORITY** (reproduce before any later recalibration):

- `ROLLING_MEDIAN_RATIO_15`
- derivative window 3
- interval mean window 15
- `VOLUME_POINT`
- 0.9 / 1.1 bands
- confirmation 2
- epsilon `1e-12`
- historical APTF interpretation configuration `V_INTERVAL_B10_C2`

Also record:

- Values were selected on **one historical entity corpus**.
- The freeze is authoritative for **migration equivalence**, not universal optimality.
- P-04V must reproduce the freeze first.
- Do not silently re-optimize windows, bands, or confirmation.
- These are not live user-tuning parameters.

**APTF FORENSIC BEHAVIOR:** 014C development evaluated four candidates; only `V_INTERVAL_B10_C2` met occupancy/interval gates.

---

## 7. Realtime lifecycle

### 7.1 APTF forensic behavior (not QuanTRAM runtime)

| State | APTF batch session boundary |
|---|---|
| Feature windows | Did **not** reset |
| historical `VolumePolicyState` | **Did** reset (`date:session` from a Price-aligned row) |

The mathematics themselves contain **no calendar/session model**. P-04V does **not** inherit `date:session` reset. This design does **not** invent a QuanTRAM date:session replacement.

### 7.2 Bounded realtime VolumeState

**QUANTRAM REALTIME BEHAVIOR.** Conceptual architecture, not frozen Go structs:

```text
VolumeState
    |
    +-- Volume Feature State
    |       |
    |       +-- RawVolumeWindow
    |       +-- NormalizedVolumeWindow
    |       +-- TimeCoordinateWindow
    |       +-- readiness / maturation state as required
    |
    +-- Volume Interpretation State
            |
            +-- color
            +-- pending_color
            +-- pending_count
```

Historical APTF `VolumePolicyState` maps conceptually to Volume Interpretation State. Implemented Go type: `InterpretationState`.

This state exists only because the mathematics require preceding causal observations.

It is **not** a historical database, Snapshot, Persistence, MongoDB, or a replay series. Full historical storage is unnecessary for P-04V realtime operation.

P-04’s pricing rings already store raw volume. P-04V must **not** share that scientific state. Reusing a numerical helper (lstsq) is acceptable.

### 7.3 QuanTRAM lifecycle events

| Event | QUANTRAM REALTIME BEHAVIOR |
|---|---|
| Normal accepted observation | Consume; prepare/compute; commit only on success; emit. Observation-count windows advance by one accepted Bar. Elapsed-time gaps affect V1/V2 coordinates only. Do not manufacture missing observations. |
| Explicit `ResetSymbol` / scientific reset | Clear entire `VolumeState` (Volume Feature State **and** Volume Interpretation State). Maturation begins again. |
| Cold process restart | `VolumeState` begins empty, including empty interpretation state. Maturation begins again. Persisted restoration is outside Volume V1. |
| Infer disabled | Do **not** invent a scientific reset solely because inference is administratively disabled. Do not silently destroy VolumeState. |
| Infer restored | Continue existing state unless QuanTRAM has separately declared an explicit scientific reset/discontinuity. |
| Source / provider transition | Provider identity alone is not a Volume scientific reset. Continuity/discontinuity classification belongs upstream. |
| Elapsed-time gap | Does **not** convert observation-count windows into clock windows. Actual elapsed time is reflected in the V1/V2 coordinate. |
| Market / session boundary | Not an automatic P-04V reset. Any later explicit lifecycle event must be justified by QuanTRAM realtime semantics, not APTF batch caller behavior. |

**D1 CLOSED FOR P-04V V1:** there is **no** independently defined interpretation-state reset event. Volume Interpretation State is owned by `VolumeState`. Explicit scientific reset / `ResetSymbol` and cold restart clear the entire `VolumeState`. Normal runtime does not independently reset interpretation state. If future science needs an independent reset, that must be separately designed and validated. Do not inherit APTF `date:session`.

---

## 8. Time semantics

| Concept | P-04V use |
|---|---|
| Observation count | `V_N` window, derivative **cardinality**, interval-mean window, confirmation observations |
| Actual elapsed time | V1/V2 quadratic coordinate only |
| `IntervalStart` | **QUANTRAM REALTIME BEHAVIOR:** established accepted-observation time basis, same as P-04 minutes |
| `SourceTimestamp` | Correlation / payload |
| Session identity | **APTF FORENSIC BEHAVIOR** only; not a P-04V runtime key |

A larger elapsed interval does not change how many observations enter `V_N` or `interval_mean_vn`.

Duplicate or out-of-order bars: P-02/host continuity already classifies them. P-04V is not a second ingress clock. A bar the host rejects is never consumed.

---

## 9. Maturation / readiness

**QUANTRAM REALTIME BEHAVIOR:** readiness arises from available scientific state, not a dataframe index and not `if observation_number == 29`.

| Milestone | Causal requirement |
|---|---|
| Raw volume | Current accepted observation |
| `V_N` | 15-observation raw-volume window; baseline finite and `> 0` |
| `predicted_next_V_N` | Same as `V_N` |
| `V1` / `V2` | Last 3 positional `V_N` all finite, plus their `IntervalStart` minutes; rank-3 finite fit |
| `interval_mean_vn` | Last 15 positional `V_N` all finite |
| Full Volume Interpretation evaluation | All mandatory scientific inputs valid |

Expected insufficient state → typed maturation / non-actionable outcome.  
INVALID is **not** “not yet enough causal state.”

**APTF FORENSIC BEHAVIOR / equivalence only:** on a complete contiguous series with no nonfinite members after first `V_N`, earliest full interpretation eligibility corresponds to **observation 29** (zero-based index 28). 009V `ACTIONABLE_START = 15` is selection-scoring, not runtime. 014C `INVALID_count = 0` because Price alignment excluded early rows. Those facts are test targets, not live counters.

---

## 10. Invalid / failure / maturation semantics

| Situation | APTF FORENSIC BEHAVIOR | QUANTRAM REALTIME BEHAVIOR |
|---|---|---|
| Missing required engine field | Exception | Preparation owns the contract; must not occur |
| Nonfinite required float when evaluation is due | INVALID emission + state INVALID | INVALID |
| Insufficient causal state | Features NaN; observe → INVALID if called | Typed maturation / non-actionable; do not use INVALID for this |
| Zero raw volume | Formula: `V_N = 0` if baseline `> 0`; corpus never saw 0 | Preserve observed 0; do not impute. Limitation: freeze did not validate this |
| Zero / non-positive rolling baseline | `V_N` unavailable | Quantity unavailable; then maturation or INVALID per whether the calculation is due |
| “Missing” volume | Empty CSV → NaN | Accepted `Bar.Volume` is `uint64`; absence is a P-02 quality question |
| Duplicate / out-of-order time | Batch RuntimeError | Host does not deliver the bar |
| Irregular elapsed time | V1/V2 use actual Δt | Same mathematics |
| Invalid frozen interpretation configuration | `ValueError` at init | Fail closed at construction |
| Color-age Δt ≠ 60.0 | Intervalizer split | **OUT OF SCOPE** for P-04V V1 |

---

## 11. Input / output contract (conceptual)

**Input:** one accepted eligible `domain.Bar` plus committed per-entity `VolumeState`.

**Output:** one Volume outcome (emission or typed maturation/non-actionable) and, on commit, next `VolumeState`.

Correlation: same `Symbol`, `IntervalStart`, `MarketSnapshotID` as Adaptive and Price for that bar.

No second feed. No PriceEvent input. No DecisionEvent input.

Implemented proto: `ModelService.StreamVolumeEvents` → `VolumeEvent`. No dedicated Volume microservice. No independent Volume EffectiveTime. `accepted_sequence` is the successful Volume-commit count for that worker/engine lifetime, **not** `worker.lastAccepted` (Adaptive+Price joint scientific-commit cursor).

---

## 12. Relationship to Adaptive and Price

P-03, P-04, and P-04V are sibling consumers of the same accepted eligible Bar.

```text
P-03 ──×──▶ P-04V
P-04 ──×──▶ P-04V
P-04V ──×──▶ P-03
P-04V ──×──▶ P-04
```

| | P-03 Adaptive | P-04V Volume Engine |
|---|---|---|
| Input | raw volume scalar | prepared V features |
| Math | EMA + log1p relative + log1p absolute / 10, clamped `[0,3]` | 15-median ratio, window-3 quadratic, 15-mean, 0.9/1.1 + confirm 2 |
| Output | `v*` inside Adaptive | Volume outcome |
| QuanTRAM today | Implemented | Implemented (science + Phase G host join) |

Do not replace P-03 influence. Do not make either depend on the other scientifically.

No Adaptive / Price / Volume mathematical fusion is authorized.

---

## 13. Historical P/V convergence (provenance only)

**APTF FORENSIC BEHAVIOR:**

| Stage | What happened | What it is not |
|---|---|---|
| Before 014C | Independent scientific engines | Volume did not consume Price output |
| 014C | Timestamp alignment / joint observation; `P_V_fusion = false` | Not mathematical fusion |
| 015 | Downstream BUY/HOLD/SELL from P/V colors and ages | Not Volume Engine |

P-04V must **not** implement 015.

---

## 14. Future StageTransition input (deferred / non-blocking)

**D7 DEFERRED — OUTSIDE P-04V V1. NON-BLOCKING FOR IMPLEMENTATION DESIGN.**

Design input only. StageTransition V1.1 remains frozen. No Volume StageTransition contract is approved. **DEFERRED / NOT YET AUTHORIZED.** This does not reopen Volume science.

Candidate categorical equality dimensions: Indicator (historical APTF `cockpit_color`); possibly `transition_state`; possibly a typed maturation kind.

Candidate facts (not equality): `V_RAW`, `V_N`, `V1`, `V2`, `interval_mean_vn`, `projected_v`, `phase`, `reason_codes`, timestamps, IDs, `InitiatingBar` = the same accepted Bar.

`phase` would publish densely if used for equality.

---

## 15. Validation authority

APTF batch files are **equivalence authorities**, not P-04V runtime inputs.

### Feature equivalence corpus

| Artifact | SHA256 (investigation) | Notes |
|---|---|---|
| `APTF_TEST_009V_VOLUME_SELECTION_V0_1.json` | `4dbc78a1…` | 101,221 `selected_V_N` / `V1` / `V2` |
| `APTF_TEST_010_VOLUME_ENGINE_EMISSIONS_V0_1.csv` | `0d9134f3…` | 101,205 rows |

### Interpretation equivalence corpus

| Artifact | SHA256 | Notes |
|---|---|---|
| `APTF_TEST_014C_SPY_V_EMISSION_POLICY_V0_1.json` | `f719134f…` | `V_INTERVAL_B10_C2` |
| `APTF_TEST_014C_SPY_V_ENGINE_EMISSIONS_V0_1.csv` | `ecd94653…` | 55,199 aligned observations |

### Optional historical color-age corpus (out of scope for P-04V V1)

| Artifact | SHA256 | Notes |
|---|---|---|
| `APTF_TEST_014C_SPY_V_INTERVALS_V0_1.csv` | `8bdccaa5…` | 5,713 intervals; provenance only |

014C rows are a Price-overlapped subset. Feature tests use 010; interpretation tests use 014C. Observation 29 is an equivalence milestone on a clean contiguous series, not a runtime index.

---

## 16. Risks and design-decision status

| ID | Topic | Status |
|---|---|---|
| D1 | Independent interpretation-state reset | **CLOSED FOR P-04V V1** — none; resets with `VolumeState` |
| D4 | Color-age / `EmissionIntervalizer` | **CLOSED FOR P-04V V1 — DEFERRED** |
| D7 | Future Volume StageTransition equality | **DEFERRED / NON-BLOCKING** — outside P-04V V1; V1.1 frozen |
| R01 | Window-3 lstsq vs NumPy `rcond=None` | Migration risk; may reuse P-04 lstsq primitive, not P-04 state |
| R02 | Categorical sensitivity near 0.9 / 1.1 | Do not retune bands to hide drift |
| R03 | Single-entity calibration provenance | Reproduce freeze; do not re-rank |
| R04 | Observation-count vs elapsed-time after gaps | Follow formulas |
| R05 | Multi-entity operation | Required; freeze corpus is one entity |
| R06 | Named-entity hard-coding | Forbidden in production architecture |
| R07 | Zero / all-zero baseline unvalidated in freeze | Preserve raw 0; do not invent extra rules |
| R08 | Positional window vs finite-skipping | **Settled:** positional, all-finite required |

Closed by earlier reconciliation and this Phase 1 closure (no longer open as design questions):

- Process identity is **P-04V** (not P-05; P-01–P-10 not renumbered).
- Time coordinate is **`IntervalStart` minutes**, matching P-04, unless a later scientific incompatibility is demonstrated.
- Warm-up is **causal maturation**, not INVALID and not `observation_number == 29`.
- Explicit `ResetSymbol` / scientific reset and cold restart **clear entire `VolumeState`**, including Volume Feature State and Volume Interpretation State.
- There is **no** independent Volume Interpretation State reset in P-04V V1.
- Infer/provider changes **do not** silently reset Volume science.
- APTF `date:session` is forensic only.
- Zero raw volume is **preserved**, not imputed.
- Color-age / `EmissionIntervalizer` is **out of scope** for P-04V V1.
- Future StageTransition Volume publication is **deferred / non-blocking**; V1.1 is unchanged.

---

## Human design decisions

No unresolved human scientific decisions block P-04V Phase 1.

| ID | Status | Resolution |
|---|---|---|
| D1 | **CLOSED FOR P-04V V1** | No independent Volume Interpretation State reset. It resets with `VolumeState` (`ResetSymbol` / scientific reset / cold restart). |
| D4 | **CLOSED FOR P-04V V1** | Color-age deferred and out of scope. |
| D7 | **DEFERRED / NON-BLOCKING** | Future StageTransition Volume contract. V1.1 unchanged. Does not block implementation design. |

Prepare/commit API names and proto enums are implemented (`volume.Engine`, `VolumeEvent` family). They are not open scientific questions.

No new unresolved scientific questions were discovered during this 2026-09-06 documentation reconciliation.

---

## Consume / emit invariants

1. P-04V consumes the same accepted eligible Bar supplied to the model scientific siblings.
2. P-04V does not subscribe independently to market data.
3. P-04V owns bounded per-entity VolumeState.
4. P-04V does not consume P-03 Adaptive outputs.
5. P-04V does not consume P-04 Price outputs.
6. P-03 and P-04 do not consume P-04V outputs as part of their scientific computation.
7. P-04V emits a first-class Volume outcome.
8. P-04V does not emit BUY/SELL/HOLD or an order.
9. P-04V does not perform P/V fusion.
10. APTF batch files and historical series are equivalence authorities, not P-04V runtime inputs.
11. P-04V runtime readiness derives from bounded scientific state, not a batch row/index counter.
12. Observation-count windows remain observation-count windows.
13. Actual elapsed time affects the derivative coordinate only where the mathematics specify it.
14. Incoming Bars are immutable.
15. Candidate scientific computation must not partially mutate committed state.
16. Explicit scientific reset clears entire `VolumeState`, including Volume Feature State and Volume Interpretation State. There is no independent interpretation-state reset in P-04V V1.
17. Administrative/provider changes do not silently redefine Volume science.
18. Feature preparation is part of P-04V, not an optional preload.
19. `predicted_next_V_N = V_N`; no RK45.
20. Frozen Volume Feature Science and Volume Interpretation Science constants are not live tuning knobs.
21. No named entity is part of the production architecture.
22. D01 `updateVolumeInfluence` remains P-03-only.
23. StageTransition V1.1 is unchanged by this design.
24. Last-N windows are positional accepted observations; they do not skip invalids to harvest N finite values.

---

## Change log

| Date | Change |
|---|---|
| 2026-09-05 | Initial proposed QuanTRAM Volume Engine design derived from completed APTF Volume forensic investigation. No implementation authorized. |
| 2026-09-05 | Reconciliation: assigned process identity **P-04V** without renumbering P-01–P-10; adopted QuanTRAM realtime consume/state/emit architecture; separated APTF batch mechanics from frozen scientific authority; clarified bounded per-entity realtime VolumeState; reframed lifecycle for QuanTRAM (no inherited `date:session`); reframed warm-up as causal maturation/readiness; reserved INVALID for genuine invalidity; settled positional all-finite window semantics; `IntervalStart` time coordinate; zero volume preserved as observed. No implementation authorized. |
| 2026-09-05 | Final Phase 1 terminology/design closure: aligned P-04V terminology with established QuanTRAM Price/Adaptive engine architecture; retained APTF “policy” terminology only as historical/equivalence provenance; consolidated realtime state under VolumeState with feature and interpretation responsibilities; separated Volume Feature Science from Volume Interpretation Science; closed independent interpretation-state reset for V1; deferred color-age; classified future StageTransition integration as non-blocking/deferred. No mathematics changed. No implementation authorized. |
| 2026-09-06 | Documentation reconciliation: status updated to implemented/validated through Phase G; added Current Implementation Status; canonical P-04V term Indicator (historical APTF `cockpit_color` retained as provenance); Test010 discrete `G_V` stated as accepted authority, not a placeholder; Phase F-R confirmation state-write documented; proto/host existence noted without changing mathematics. Process Model file not edited. |
