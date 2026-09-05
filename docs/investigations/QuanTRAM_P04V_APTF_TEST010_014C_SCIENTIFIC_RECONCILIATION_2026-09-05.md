# QuanTRAM — P-04V APTF Test 010 → Test 014C Scientific Authority Reconciliation

**Title:** QuanTRAM — P-04V APTF Test 010 → Test 014C Scientific Authority Reconciliation  
**Date:** 2026-09-05  
**Status:** RECONCILIATION COMPLETE — HUMAN REVIEW REQUIRED — PHASE G NOT STARTED  
**Purpose:** Reconcile the accepted APTF Test 010 Volume scientific model and the Test 014C reusable interpretation layer against the current QuanTRAM P-04V implementation (Phases A–F-R), after correcting the earlier incompleteness reading of discrete G_V persistence.  
**Scope:** Read-only review of the authoritative Test 010 PASS result, frozen 010/014C artifacts already hashed in Phase F, existing F/F-R equivalence evidence, the existing 014C executable audit (interpretation only; document not rewritten), and current P-04V sources. No production change. No Process Model, proto, or StageTransition change. No Phase G.  
**Documentation path:** `docs/investigations/QuanTRAM_P04V_APTF_TEST010_014C_SCIENTIFIC_RECONCILIATION_2026-09-05.md`

## Executive Summary

**Yes. Current QuanTRAM P-04V faithfully implements the accepted APTF Test 010 → Test 014C Volume scientific model.**

The earlier working concern — that `predicted_next_V_N = V_N` marked unfinished Volume science because Price later entered RK45 experiments — is not supported by the authoritative Test 010 PASS result.

Test 010 explicitly:

- **Accepted** Volume as a **discrete G_V observer / state update**, not an ODE.
- **Selected** `G_V: V_N_hat(n+1) = V_N(n)` by a frozen coverage-first comparison of ten one-step representations.
- Set **Volume RK readiness = NO**.
- Set **Test 011 readiness = CONDITIONAL, PRICE-ONLY RK PROPAGATION**.
- Recommended that Volume remain an observer while Price uses RK.

That is a scientific selection, not a failed or unfinished Volume test. Simple is not incomplete.

Test 014C then consumed that accepted upstream representation and added the reusable interpretation/policy layer (`V_INTERVAL_B10_C2`, activity = `interval_mean_vn`, confirmation, historical `cockpit_color` → QuanTRAM `Indicator`, phase, thin confidence/domain metadata) plus Price/Volume temporal alignment.

Current P-04V (Phases A–F-R) matches that accepted lineage on every required scientific element. Frozen feature and session-sliced interpretation equivalence remain valid. Production does not auto-reset at session boundary. No Volume RK/ODE path exists. No P/V fusion or trading verbs exist.

**Final reconciliation outcome: A — ACCEPTED APTF VOLUME SCIENCE FULLY RECONCILED.**

**PHASE G SCIENTIFIC READINESS = READY.**

Phase G is **not** started. Human review is required before any runtime integration.

This document **supersedes the earlier incompleteness interpretation** of Test 010 / 014C Volume. It does **not** rewrite or delete:

- `APTF_TEST_010_RESULT_V0_1.md`
- `docs/investigations/QuanTRAM_APTF_TEST014C_VOLUME_EXECUTABLE_AUDIT_2026-09-05.md`
- Phase F / F-R validation
- P-04V design or implementation documents
- the Process Model

## Module / System Overview

Accepted historical lineage:

```text
Test 007 observation map field `volume`
        ↓
009V     V_RAW, V_N, V1, V2
        ↓
Test 010  interval_mean_vn
          selected G_V: V_N_hat(n+1) = V_N(n)
          predicted_next_V_N = V_N   (VOLUME_POINT)
          X_V = {V_N, IntervalState_15, Observer(V1,V2,sign,persistence)}
        ↓
Test 014C VolumeEngine.observe
          activity interpretation (interval_mean_vn)
          raw color / confirmation / cockpit_color
          phase / confidence / domain
          Volume intervals + Price/Volume temporal alignment
```

Current QuanTRAM P-04V (package `internal/volume`, types in `internal/domain/volume.go`) implements the same scientific stack as a per-entity Engine with PrepareStep/Commit. It is not yet wired into modelhost (Phase G).

Scientific independence intended by Test 010:

```text
SAME MARKET OBSERVATION
    ├── Price scientific lens   (P-04; RK authorized only later, Price-only)
    └── Volume scientific lens  (P-04V; discrete G_V observer; no RK)
```

No scalar mix. No Volume-causes-Price claim. Scientific independence is not transport independence.

## Inputs

| Item | Value |
|---|---|
| QuanTRAM HEAD | `07417d9c85799949cd3b173067795905513af173` — “Freeze validated StageTransition V1.1 baseline” |
| QuanTRAM working tree | Dirty before this phase (unrelated Process Model / semantics tests + uncommitted P-04V A–F-R). **Not cleaned.** |
| Primary Test 010 authority | `C:\Users\chino\APTF\pre08242026_docs\APTF_TEST_010_RESULT_V0_1.md` (identical at APTF HEAD `ae0dacb2e02c5b80c82f6662d1a3c6863f4b989a`) |
| APTF HEAD | `ae0dacb2e02c5b80c82f6662d1a3c6863f4b989a` |
| Frozen 010/014C artifacts | Previously hash-verified in Phase F (`pre08242026_docs/`) |
| Existing 014C executable audit | `docs/investigations/QuanTRAM_APTF_TEST014C_VOLUME_EXECUTABLE_AUDIT_2026-09-05.md` (read-only; not rewritten) |
| Existing F / F-R validation | `docs/investigations/QuanTRAM_P04V_VOLUME_FROZEN_EQUIVALENCE_VALIDATION_2026-09-05.md` (read-only) |
| Current P-04V sources | `internal/domain/volume.go`, `internal/volume/*.go` (read-only) |

## Outputs

This reconciliation document only.

## Parameters / Configuration

Frozen P-04V scientific constants (`internal/volume/config.go`), matching the accepted 009V → 010 → 014C lineage:

| Constant | Value | Authority |
|---|---|---|
| `NormalizationID` | `ROLLING_MEDIAN_RATIO_15` | 009V selection; 010 asserts |
| `RawWindow` | 15 | 009V / 010 V_N |
| `DerivativeWindow` | 3 | 009V selected V1/V2 window |
| `IntervalMeanWindow` | 15 | 010 selected interval |
| `ProjectionID` | `VOLUME_POINT` | 010 selected G_V |
| `InterpretationID` | `V_INTERVAL_B10_C2` | 014C selected policy |
| `StateSource` | `INTERVAL_MEAN_V_N` | 014C activity |
| `LowerThreshold` | 0.9 inclusive RED | 014C |
| `UpperThreshold` | 1.1 inclusive GREEN | 014C |
| `ConfirmationCount` | 2 | 014C |
| `Epsilon` | 1e-12 | 014C phase only |

These are not environment knobs.

## Assumptions

- The authoritative Test 010 PASS result is the scientific disposition of Volume, not later informal “Volume looks unfinished because Price did RK” reasoning.
- `recommended_evolution: G_V_DISCRETE_STATE_UPDATE` names the architecture Test 010 accepted, not an unimplemented replacement. Executable evidence does not require reintroducing an incompletion assumption.
- Interval descriptors that Test 010 computed for selection/diagnostics, but that Test 014C `VolumeEngine.observe` does not consume, are not required P-04V runtime science.
- Existing F / F-R corpus numbers remain valid because production Volume science has not changed since F-R. Full-corpus replay was not rerun.
- Historical APTF `cockpit_color` maps to canonical QuanTRAM `Indicator`. Historical artifact names are not globally renamed.

## Exclusions

- No production code change
- No Phase G / modelhost / proto / StageTransition / Process Model / Snapshot / Persistence / Aperture change
- No rewrite of historical APTF documents or prior QuanTRAM investigations
- No invention of richer Volume science because Price is more complex
- No automatic production session reset to chase 014C harness session-sliced counts
- No addition of unused Test 010 interval diagnostics to P-04V

## Historical Authority

Authority hierarchy used here:

1. Frozen APTF executable Python (already reconstructed in the 014C executable audit)
2. Authoritative Test 010 PASS result (`APTF_TEST_010_RESULT_V0_1.md`)
3. Frozen Test 010 / 014C outputs (hash-verified in Phase F)
4. Executable Test 014C validation (014C audit + F-R session-sliced equivalence)
5. Existing QuanTRAM F / F-R equivalence results
6. Historical APTF documentation as supporting context

The purpose of this phase is migration fidelity, not rediscovery of the model.

## Test 010 PASS Interpretation

Direct quotations / exact fields from `APTF_TEST_010_RESULT_V0_1.md`:

| Field | Exact value |
|---|---|
| Status | **PASS** |
| Acceptance | **120/120 PASS** |
| Test 011 readiness | **CONDITIONAL, PRICE-ONLY RK PROPAGATION** |
| Price | continuous local dynamics; `X_P = [P, P1, P2]` |
| Price RK readiness | **CONDITIONAL** (one elapsed minute of `INTRASESSION_CONTINUOUS` only) |
| Volume | observed participation/result channel |
| Volume evolution | **discrete G_V observer update**, not an ODE |
| Volume RK readiness | **NO** |
| Selected G_V | `V_N_hat(n+1) = V_N(n)` |
| Exact Volume state X_V | `{V_N, IntervalState_15, Observer(V1,V2,sign,persistence)}` |
| Exact Volume interface | discrete G_V point update plus interval-15 state and V1/V2 observer events; **no RK integration** |
| Session close | **resets neither engine** |
| Mathematical state | spans premarket, regular, and after-hours observations **continuously** |
| Independent engines | **Yes** |
| Scalar P/V mix | **none / NO** |
| Volume-causes-Price | **not asserted**; causal lead/lag claim **NO** |
| New trading actions | **0** |
| Color thresholds created in Test 010 | **NO** |
| BUY/HOLD/SELL changed | **NO** |
| Can Volume remain observer while Price uses RK? | **Yes; this is the recommended dual-engine design** |

Test 010 compared ten one-step Volume representations. Coverage-first selection chose persistence G_V. The 15-row interval median had lower MAE (1.744 vs 2.207) but 13 fewer forecasts; it was retained as the primary **condition descriptor**, not as a replacement G_V. Pointwise derivative Taylor was unusable (MAE 5,011.60). Volume was therefore **not** advanced into continuous ODE/RK. That is a selection, not a failed test.

**Test 010 Volume was ACCEPTED.**

Test 011 readiness is Price-only. It does not mean Volume was conditionally accepted. It means Price was authorized for a later RK experiment under strict conditions, and Volume was **not** authorized for RK.

## Accepted Dual-Engine Architecture

Test 010 establishes two independent scientific lenses on the same market observation:

- Price: continuous local dynamics; later conditionally eligible for one-minute RK.
- Volume: discrete G_V observer update plus interval-15 state plus V1/V2 observer events.

Control rows recorded joint persistence/transition labels. Those labels are dual-engine **description**, not fusion mathematics. No scalar mix exists. No new trading actions were created.

Current QuanTRAM can preserve this relationship without separate market subscriptions, processes, goroutines, or mailboxes. Scientific independence is not transport independence. This phase does not redesign runtime architecture.

## Accepted Volume State X_V

Test 010 Q12:

```text
X_V = {V_N, IntervalState_15, Observer(V1,V2,sign,persistence)}
```

Semantic meaning, from the same result:

- `V_N` is the accepted normalized Volume observation.
- `IntervalState_15` is the accepted 15-observation interval condition, of which `interval_mean_vn` is the quantity that later became the 014C activity source. Test 010 also computed many diagnostic interval descriptors (see IntervalState_15 section).
- `Observer(V1,V2,sign,persistence)` is observer-event state. V1/V2 sign changes are “frequent observer events, not primary dynamics.” Persistence in the 010 emission sense is interval/event persistence used during selection and cockpit-candidate listing. Test 014C later implemented reusable confirmation persistence on color, which is the accepted runtime persistence machine.

## Accepted G_V

Exact selected law:

```text
G_V: V_N_hat(n+1) = V_N(n)
```

Executable 010 identity: `VOLUME_POINT`, `predicted_next_V_N = V_N`.

`recommended_evolution: G_V_DISCRETE_STATE_UPDATE` is the name of this accepted architecture. It is not an unimplemented future replacement. The Test 010 result states Volume evolution **is** a discrete G_V observer update.

Distinguish:

- **SIMPLE** — persistence was selected among ten candidates and accepted.
- **INCOMPLETE** — not supported by the PASS result.

## Test 011 Price-Only RK Boundary

| Item | Disposition |
|---|---|
| Test 010 | PASS 120/120 — dual-engine local-dynamics foundation accepted |
| Test 011 readiness | CONDITIONAL, PRICE-ONLY RK PROPAGATION |
| Evaluate RK45 / DOP853 | Yes, in Test 011 **only**; no RK solver was executed in Test 010 |
| Volume suitable for RK | **NO** |
| “Not authorized for RK” | **≠** “failed Volume model” |

Price alone proceeded into later RK experimentation. Volume deliberately remained a discrete observer/update model.

## Test 014C Interpretation Layer

The existing 014C executable audit established, and this reconciliation accepts without rewriting that document:

- 014C does **not** recompute V_RAW / V_N / V1 / V2 / interval_mean_vn / predicted_next_V_N. It loads Test 010 emissions and inner-joins 014B Price timestamps.
- 014C **does** compute interpretation via `spy_volume_engine.VolumeEngine.observe`.
- Selected policy: `V_INTERVAL_B10_C2`, activity = `INTERVAL_MEAN_V_N`, thresholds 0.9 / 1.1, confirmation = 2, ε = 1e-12 for phase.
- Historical `cockpit_color` is the emitted confirmed/pending color. Canonical QuanTRAM name: `Indicator`.
- Session `date:session` reset belongs to the 014C **replay/test harness**, not to the Volume scientific engine.
- 014C `139/139 PASS` is occupancy/interval/immutability/no-fusion gating, not an independent V_N/V1 numeric oracle. Feature mathematics authority remains 009V/010 plus QuanTRAM F replay.

What 014C added, relative to accepted Test 010 Volume:

- reusable activity interpretation
- confirmation / Indicator
- phase labels
- thin confidence / domain metadata
- Price/Volume temporal alignment (timestamp join + color-age intervalizer)

What 014C did **not** do:

- replace G_V
- authorize Volume RK
- assert Volume-causes-Price
- create BUY/HOLD/SELL

## Current QuanTRAM P-04V Inventory

Verified present (read-only; not modified):

| Path | Role |
|---|---|
| `internal/domain/volume.go` | Lineage, representation labels, `VolumeEvent` |
| `internal/volume/config.go` | Frozen scientific constants |
| `internal/volume/mapper.go` | `V_RAW = float64(Bar.Volume)` |
| `internal/volume/state.go` | Feature rings + InterpretationState |
| `internal/volume/median.go` | Positional median-15 |
| `internal/volume/features.go` | V_N, interval_mean_vn, VOLUME_POINT |
| `internal/volume/linalg.go` | Copied SVD lstsq; no `internal/pricing` import |
| `internal/volume/derivatives.go` | Causal quadratic V1/V2 |
| `internal/volume/phase.go` | ε = 1e-12 ACTIVITY_* inequalities |
| `internal/volume/interpretation.go` | Activity color + confirmation |
| `internal/volume/engine.go` | PrepareStep / Commit / explicit Reset |
| Phase A–E tests, `equivalence_test.go`, `frozen_full_test.go`, `import_guard_test.go` | Equivalence and isolation |
| `internal/volume/testdata/*` | Frozen policy copy and Level-1 prefixes |

Production comments and import-guard tests forbid Adaptive/Price imports, modelhost import at this phase, P/V fusion, and automatic session reset.

## X_V Reconciliation

| Test 010 X_V element | QuanTRAM representation | Match? | Notes |
|---|---|---|---|
| `V_N` | `FeatureResult.VN`, `VolumeEvent.VN`, positional `FeatureState.VN` | **YES** | Median-15 current-inclusive ratio; NaN kept in position |
| `IntervalState_15` | `interval_mean_vn` (`FeatureResult.IntervalMean`, `VolumeEvent.IntervalMeanVN`) computed from last 15 positional V_N | **YES** for the accepted reusable lineage | See IntervalState_15 section. 010 diagnostic JSON fields are not stored |
| `V1` | `FeatureResult.V1`, `VolumeEvent.V1` | **YES** | Observer quantity; not an ODE state for integration |
| `V2` | `FeatureResult.V2`, `VolumeEvent.V2` | **YES** | Observer quantity |
| sign / phase | `ACTIVITY_*` via `classifyPhase` | **YES** | Interpretation/observer information only |
| persistence / confirmation | `InterpretationState.{Color,PendingColor,PendingCount}`; confirmation = 2 | **YES** for 014C reusable persistence | 010 `persist_baseline` / burst persistence were selection/diagnostic, not 014C observe inputs |

Identical data structures are not required. Semantic/scientific equivalence holds.

## Feature Mathematics Reconciliation

### V_RAW — PASS

Current: `ObservationFromBar` sets `VRaw = float64(bar.Volume)`.

Verified:

- no imputation
- no fabricated Volume
- zero remains zero (`float64(0)`)
- no Price input
- no provider-specific scientific dependency (mapper reads canonical `domain.Bar` only)

### V_N — PASS

Current (`normalizeFromRaw` / `rollingMedian15`):

```text
V_N = V_RAW_current / median(last 15 V_RAW observations including current)
```

when the median is finite and `> 0`; otherwise undefined (NaN), not imputed. Zero raw with positive median yields `V_N = 0`.

Verified:

- positional 15-observation window
- current-inclusive
- no time-duration substitution
- no invalid-value skipping that changes positional semantics (unavailable V_N occupies its index)
- no Price dependency

### V1 / V2 — PASS

Current (`derivatives.go`):

- last 3 positional V_N
- actual `IntervalStart` elapsed-minute coordinates `τ_i = (t_i − t_current).Minutes()`
- causal quadratic `V_N(τ) = aτ² + bτ + c`
- `V1 = b`, `V2 = 2a`
- rank == 3 required; no imputation

These remain observer-state quantities. They feed phase labels only. They are **not** used to create a Volume ODE/RK propagation path.

### IntervalState_15

Test 010 computed, for interval families 3/5/8/15: mean, median, min, max, range, std, CV, burst ratios/counts/fractions, persistence, plus V1/V2 observer state. Total interval rows: 404,801.

Classification of those descriptors:

| Historical quantity | Class | Survived into accepted 014C reusable engine? |
|---|---|---|
| `interval_mean_vn` (`mean` of last 15 V_N) | **A** — required runtime condition / 014C activity | **Yes** — `state_source = INTERVAL_MEAN_V_N` |
| interval median (15-row) | **B** — Test 010 selection/condition descriptor (lower MAE, 13 fewer forecasts) | **No** as G_V; not 014C activity |
| min / max / range / std / CV | **B** — Test 010 diagnostic / future color *candidates* | **No**. 014C observe does not use them |
| burst ratios / counts / fractions | **B** — Test 010 diagnostic / cockpit candidates | **No** |
| 010 persist_baseline / event persistence | **B** — Test 010 diagnostic; listed as *future* Volume color quantity | **C** — superseded by 014C color confirmation, not migrated as burst persistence |
| V1/V2 inside interval JSON | observer copy-through | **Yes** as first-class V1/V2, not as JSON |

Test 010 itself created **no color thresholds**. “Future Volume color quantities” in the Test 010 Q&A are candidates, not an accepted runtime interface. Test 014C later selected `interval_mean_vn` and did **not** adopt dispersion/burst as activity.

P-04V therefore correctly represents IntervalState_15 as the 15-position V_N window plus `interval_mean_vn`. It does **not** need every Test 010 diagnostic in runtime state.

**Required IntervalState_15 semantics: PASS.**

## G_V Reconciliation

Current P-04V (`volumePoint`):

```text
predicted_next_V_N = V_N
```

when V_N is available; otherwise the same unavailability status. Projection identity is frozen `VOLUME_POINT`.

This is exact equivalence to Test 010 selected `G_V: V_N_hat(n+1) = V_N(n)`.

**Does current P-04V preserve the accepted discrete G_V state-update semantics? PASS.**

RK, ODE, Taylor projection, or a more complex predictor is **not** required. Persistence is **not** a defect.

## Volume Is Not an ODE Engine — PASS

Current `internal/volume` contains:

- no Volume RK45
- no DOP853
- no continuous Volume integration
- no Price-style EXPM propagation
- no derivative Taylor multi-step Volume trajectory
- no fabricated multi-step Volume path

`linalg.go` mentions EXPM only as a non-responsibility. Import-guard tests forbid `internal/pricing` and `internal/adaptive`. V1/V2 are observer derivatives only.

## Interpretation Reconciliation

### Activity — PASS

Activity source is `interval_mean_vn` only (`Interpret` uses `features.IntervalMean`; comments and design forbid using `predicted_next_V_N` for color).

Exact thresholds:

| Condition | Color |
|---|---|
| `activity >= 1.1` | GREEN |
| `activity <= 0.9` | RED |
| otherwise | AMBER |

Inclusive bounds. Exact match to frozen 014C policy.

### Confirmation / Indicator — PASS

F-R repaired `confirmColor` so `next.Color` is the emitted Indicator (historical `cockpit_color`), matching HEAD `spy_volume_engine.observe`.

Current machine:

- first valid raw color accepted immediately (`STABLE`)
- same raw as carried `state.Color` clears pending (`STABLE`)
- differing raw with `pending_count < 2` emits Indicator **AMBER**, writes `next.Color = AMBER`, `PENDING_{raw}`
- second consecutive same pending raw confirms (`CONFIRMED_{raw}`, Color = raw)
- on confirm, frozen leaves `pending_count = 2` and `pending_color` empty
- changed pending candidate resets count to 1

Session-sliced 014C F-R results (unchanged since F-R; science not modified):

| Field | Match |
|---|---|
| `raw_color` | 55,199 / 55,199 |
| `Indicator` / `cockpit_color` | 55,199 / 55,199 |
| `transition_state` | 55,199 / 55,199 |
| `phase` | 55,199 / 55,199 |
| `confidence` | 55,199 / 55,199 |
| `domain_state` | 55,199 / 55,199 |

These results remain valid. Continuous Engine vs 014C still differs at harness session cuts (41 Indicator mismatches) and must **not** be “repaired” by a production session reset.

### Phase — PASS

`Epsilon = 1e-12`. Exact accepted inequalities in `classifyPhase`:

```text
|V1| <= ε                 → ACTIVITY_STATIONARY
V1 > 0  and V2 >  ε       → ACTIVITY_INCREASING_ACCELERATING
V1 > 0  and V2 <= ε       → ACTIVITY_INCREASING_DECELERATING
V1 < 0  and V2 <  -ε      → ACTIVITY_DECREASING_ACCELERATING
V1 < 0  and V2 >= -ε      → ACTIVITY_DECREASING_DECELERATING
```

Phase is interpretation/observer information. It does not create Volume continuous propagation. F-R: 0 phase flips from derivative tolerance.

### Confidence and domain state — PASS (historical equivalence)

Historical 014C behavior, preserved:

- `confidence = HIGH` when a valid `INTERVAL_MEAN_V_N` color interpretation is produced; otherwise unset / not a richer model
- `domain_state = CAUSAL_LOCAL_VOLUME` on that same valid interpretation

These are thin/static historical metadata. P-04V does not invent richer confidence or domain behavior. Session-sliced equivalence is 55,199 / 55,199.

## Session / Lifecycle Reconciliation — PASS

Test 010: session close resets neither engine; mathematical state spans premarket, regular, and after-hours continuously.

014C executable audit: `date:session` reset belongs to replay/test harness.

Current production P-04V:

- `Engine.Reset` exists as an **explicit** operator
- PrepareStep / AdvanceFeatures / Interpret perform **no** automatic session, date, provider, or gap reset
- comments and Phase E tests treat PREMARKET / `date:session` reset as forbidden production behavior

This is the expected correct production behavior.

014C session reset remains correctly classified as **harness-only**.

## Price / Volume Independence — PASS

Verified on current P-04V:

- does not consume P-04 Price output
- does not consume P-03 Adaptive output
- does not modify Price mathematics
- does not assert Volume-causes-Price
- does not scalar-mix Price and Volume
- does not produce BUY / HOLD / SELL
- does not produce orders
- does not perform P/V fusion

Import-guard tests assert no production import of `internal/pricing` or `internal/adaptive`. `VolumeEvent` carries Volume quantities and Indicator only. Dual-engine scientific independence is architecturally preservable without transport redesign.

## Frozen Equivalence Evidence

Full-corpus Engine replay was **not** rerun. Reason: P-04V production science is unchanged since Phase F-R. Rerun is not required to verify the current working tree.

Existing F / F-R numbers remain the authoritative migration measurements:

Feature / derivative (010, 101,205 rows):

| Field | Result |
|---|---|
| `V_RAW` | 101,205 / 101,205 exact |
| `V_N` | 101,205 / 101,205 exact |
| `predicted_next_V_N` | 101,205 / 101,205 exact |
| `V1` | all within 1e-9; max abs ≈ 7.84e-12 |
| `V2` | all within 1e-9; max abs ≈ 2.73e-12 |
| `interval_mean_vn` | all within 1e-12; max abs ≈ 5.68e-14 |
| phase flips from derivative tolerance | 0 |

Interpretation (014C session-sliced after F-R): 55,199 / 55,199 on raw_color, Indicator, transition_state, phase, confidence, domain_state.

## Prior-Investigation Reassessment

The 014C executable audit classified Test014C-era Volume as:

```text
B — IMPLEMENTED AND VALIDATED, BUT NOT DEMONSTRABLY COMPLETE
Confidence: MEDIUM
```

That conclusion was reasonable given the evidence then available. In particular, `predicted_next_V_N = V_N` and `recommended_evolution: G_V_DISCRETE_STATE_UPDATE` were read as a placeholder / further-work pointer, and 014C’s padded occupancy gates did not independently re-prove feature mathematics.

This document does **not** delete or rewrite that audit.

The authoritative Test 010 PASS result supplies the missing disposition:

- VOLUME_POINT persistence **is** the selected, accepted G_V.
- Discrete G_V **is** the accepted Volume evolution.
- Volume RK **NO** is an acceptance boundary, not an incompleteness finding.
- `recommended_evolution: G_V_DISCRETE_STATE_UPDATE` describes that accepted architecture.

014C remains a reusable interpretation/alignment layer on top of accepted Test 010 Volume, not a demonstration that Volume science was left unfinished.

Therefore the **combined** evidence now supports a **stronger** conclusion for QuanTRAM migration fidelity: the accepted APTF Volume science is fully reconciled in P-04V. Residual 014C-audit observations (padded 139 gates, constant confidence/domain, harness session reset, unused projected_V1/V2) do not reopen Volume as incomplete, and they do not describe missing P-04V science.

## Remaining Limitations

None that constitute a material accepted-science gap or extra invented science.

Documented non-scientific / out-of-scope facts (do not block Phase G scientific readiness):

1. **Phase G is not implemented.** This phase only determines scientific readiness.
2. **014C color-age intervalizer** is Price/Volume alignment/reporting, not Volume feature or G_V science. P-04V correctly omits it from the Volume Engine.
3. **Continuous production vs 014C session-sliced replay** still differs at session cuts (41 Indicator). Correct production behavior is continuous. Do not add automatic session reset.
4. **014C 139/139** remains occupancy/immutability gating. Feature equivalence is proven by QuanTRAM F replay against frozen 010 emissions, not by those gates.
5. **Calibration corpus** is one-entity (SPY) historical FirstRateData. That is the accepted APTF freeze, not a P-04V invention.
6. **Confidence / domain** remain thin historical constants. Not a defect.
7. **Test 010 interval diagnostic JSON** (std, burst, persist_baseline, …) is not in P-04V runtime. Correct 014C reduction.

No material accepted APTF Volume science is absent. No material extra QuanTRAM Volume science has been invented.

## Phase G Readiness Decision

| # | Criterion | Result |
|---|---|---|
| 1 | V_RAW matches | PASS |
| 2 | V_N matches | PASS |
| 3 | V1/V2 matches | PASS |
| 4 | IntervalState_15 behavior required by the accepted reusable lineage is represented | PASS |
| 5 | Discrete G_V semantics match | PASS |
| 6 | No Volume RK/ODE propagation exists | PASS |
| 7 | Test 014C activity interpretation matches | PASS |
| 8 | Confirmation / Indicator matches | PASS |
| 9 | Phase matches | PASS |
| 10 | Session lifecycle is correctly continuous in production | PASS |
| 11 | P/V mathematical independence is preserved | PASS |
| 12 | Frozen corpus equivalence remains valid | PASS |
| 13 | No material accepted APTF Volume science is absent | PASS |
| 14 | No material extra QuanTRAM Volume science has been invented | PASS |

**PHASE G SCIENTIFIC READINESS = READY.**

Reason: current P-04V faithfully implements the accepted Test 010 discrete G_V Volume model and the Test 014C reusable interpretation layer. All fourteen readiness criteria pass. Remaining differences are harness, alignment, or correctly excluded diagnostics.

**Phase G is not started.** Human review is required before runtime integration.

## Repository Hygiene

QuanTRAM HEAD (unchanged): `07417d9c85799949cd3b173067795905513af173`

Git status **before** this investigation (working tree not cleaned):

```text
 M docs/design/QuanTRAM_PROCESS_MODEL_082926.md
 M internal/semantics/loader_test.go
 M internal/semantics/tooling/tooling_test.go
 M internal/server/semantics_test.go
?? docs/design/QuanTRAM_VOLUME_ENGINE_090526.md
?? docs/implementations/
?? docs/investigations/QuanTRAM_APTF_TEST014C_VOLUME_EXECUTABLE_AUDIT_2026-09-05.md
?? docs/investigations/QuanTRAM_APTF_VOLUME_ENGINE_GO_REFACTORABILITY_INVESTIGATION_2026-09-05.md
?? docs/investigations/QuanTRAM_P04V_VOLUME_FROZEN_EQUIVALENCE_VALIDATION_2026-09-05.md
?? internal/domain/volume.go
?? internal/volume/
?? tools/__pycache__/
```

Only this new investigation document is created by this phase. No clean, reset, stage, commit, or revert.

## Change Log

| Date | Change |
|---|---|
| 2026-09-05 | Created scientific reconciliation of accepted Test 010 → Test 014C Volume against QuanTRAM P-04V A–F-R. Outcome A. Phase G scientific readiness READY. No production change. Supersedes prior incompleteness *interpretation* of G_V persistence without rewriting historical documents. |
