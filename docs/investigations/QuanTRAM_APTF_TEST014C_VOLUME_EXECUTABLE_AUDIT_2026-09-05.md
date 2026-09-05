# QuanTRAM — APTF Test 014C Volume Executable-Source Audit

**Title:** QuanTRAM — APTF Test 014C Volume Executable-Source and Completion-Status Forensic Audit  
**Date:** 2026-09-05  
**Status:** INVESTIGATION COMPLETE — HUMAN REVIEW REQUIRED  
**Purpose:** Reconstruct from executable Python what Test 014C actually ran and validated as Volume science, what “139 / 139 PASS” means, and whether that era is a completed baseline or an intermediate freeze.  
**Scope:** Read-only APTF Python/git forensics starting at Test 014C, then only the 010/009V/007 dependencies 014C actually consumes. Read-only comparison to current QuanTRAM P-04V. No production change. No existing P-04V doc rewrite. No Phase G.  
**Documentation path:** `docs/investigations/QuanTRAM_APTF_TEST014C_VOLUME_EXECUTABLE_AUDIT_2026-09-05.md`

## Executive Summary

Test 014C at APTF HEAD `ae0dacb2e02c5b80c82f6662d1a3c6863f4b989a` is **not** a Volume feature calculator. It is a **policy-selection + interpretation + Price/Volume alignment** harness.

Executable facts:

1. **014C does not compute `V_RAW`, `V_N`, `V1`, `V2`, `interval_mean_vn`, or `predicted_next_V_N`.** It loads them from `APTF_TEST_010_VOLUME_ENGINE_EMISSIONS_V0_1.csv` and inner-joins Price timestamps from Test 014B.
2. **014C does compute** raw color, confirmation, `cockpit_color`, `transition_state`, phase, confidence, and domain via `spy_volume_engine.VolumeEngine.observe`.
3. **Feature science lives in 009V** (normalization + causal quadratic) **and 010** (interval mean + `VOLUME_POINT` copy-through).
4. **`139 / 139 PASS` is not 139 independent Volume-math assertions.** `build_acceptance_gates` has **20 unique checks** and repeats them with `index % len(checks)` until 139 slots are filled. Those checks are mostly occupancy, interval-assignment, hash/immutability, and “no fusion / no trading” flags. None compare `V_N`/`V1`/`V2` to an independent oracle.
5. **Confirmation state-write** in HEAD `observe` is: emitted `cockpit_color` is written into next `VolumePolicyState.color`. Current QuanTRAM F-R `confirmColor` matches that executable machine. That is historical-fidelity evidence, not proof the machine is complete science.
6. **No committed Volume scientific change exists after the 014C commit.** HEAD *is* the 014C completion commit. Later `volume_engine/` rename and Tests 015/016 exist only in the dirty working tree.

**Completion classification: B — IMPLEMENTED AND VALIDATED, BUT NOT DEMONSTRABLY COMPLETE.**  
**Confidence: MEDIUM.**

014C froze a selected interpretation policy (`V_INTERVAL_B10_C2`) and demonstrated occupancy/interval/determinism gates. It did not unit-validate the upstream feature pipeline, did not use `predicted_next_V_N` for color, left `projected_V1/V2` explicitly unsupported, assigned confidence/domain as constants, and padded its acceptance ledger.

## Module / System Overview

```text
Test 007 observation map  (volume column)
        ↓  [009V selection + multivariate]
V_RAW, V_N, V1, V2  in 009V artifacts
        ↓  [010 Volume diagnostic]
interval_mean_vn, predicted_next_V_N=V_N (VOLUME_POINT)
        ↓  CSV
014C load_common_rows  inner-join 010 × 014B Price timestamps
        ↓
014C replay: session-reset VolumePolicyState
        ↓
spy_volume_engine.VolumeEngine.observe
        ↓
VolumeEmission (raw/cockpit/phase/transition/confidence/domain)
        ↓
EmissionIntervalizer (color-age; 60s continuity)
        ↓
occupancy / interval invariants / 20 checks padded to 139
        ↓  [working-tree only, not at HEAD]
015 consumes V_color + V_interval_age  (does not recompute Volume)
```

014C Price role: **timestamp alignment and descriptive joint intervals only.** `observe` uses no Price field. Summary sets `P_V_fusion: false`.

## Inputs

| Item | Value |
|---|---|
| APTF path | `C:\Users\chino\APTF` |
| APTF branch | `main` |
| APTF HEAD | `ae0dacb2e02c5b80c82f6662d1a3c6863f4b989a` — “Test 014C completed with Pand V Engine emission charts” |
| APTF working tree | Dirty (freeze files relocated to `pre08242026_docs/`; uncommitted `volume_engine/` rename; uncommitted 015/016). **Authority = HEAD blobs via `git show`.** |
| 014C Volume input | HEAD `APTF_TEST_010_VOLUME_ENGINE_EMISSIONS_V0_1.csv` |
| 014C Price input | `APTF_TEST_014B_SPY_P_ENGINE_EMISSIONS_V0_2.csv` |
| 014C policy | `APTF_TEST_014C_SPY_V_EMISSION_POLICY_V0_1.json` |
| QuanTRAM compared (read-only) | `internal/volume/*`, `internal/domain/volume.go` at uncommitted P-04V F-R |

## Outputs

This investigation document only.

## Parameters / Configuration

Frozen 014C selected policy, from development candidate `V_INTERVAL_B10_C2` written into `parameters`:

| Parameter | Executable value |
|---|---|
| `state_source` | `INTERVAL_MEAN_V_N` |
| `lower_threshold` | `0.9` inclusive RED |
| `upper_threshold` | `1.1` inclusive GREEN |
| `confirmation_observations` | `2` |
| `epsilon` | `1e-12` (phase only) |
| `policy_id` (wrapper) | `V_EMISSION_V0_1` |

Four development candidates existed. Selection rule in `run_test_014c_v_development.py`: minimum `changes_per_session` among those with GREEN/RED occupancy ≥ 7.5%, AMBER ≤ 50%, median interval ≥ 3.

## Assumptions

- HEAD `ae0dacb` is the appropriate 014C authority (it *is* the 014C completion commit; no later commits exist).
- Working-tree `volume_engine` is a later uncommitted packaging rename, not HEAD authority. Observe mathematics were compared and match except `symbol` source.
- Frozen CSVs corroborate lineage; they are not used as the formula source.

## Exclusions

- No APTF mutation, checkout, or artifact regeneration
- No QuanTRAM production or existing P-04V doc edits
- No Phase G
- Docs read only after code conclusions, for a discrepancy table

## Methodology

Code first: locate 014C Python → follow imports → 010 CSV producer → 009V arrays → 007 `volume`. Then frozen artifacts as corroboration. Then optional doc cross-check.

## Authority Hierarchy

1. HEAD Python (`git show ae0dacb:…`)
2. Frozen artifacts produced by that Python
3. Working-tree later files (015/016, `volume_engine`) labeled as **post-HEAD / uncommitted**
4. Markdown last

## Test014C Python Inventory

HEAD files that participate in 014C:

| File | Role |
|---|---|
| `diagnostics/run_test_014c_v_development.py` | `main` — four-candidate policy selection on DEVELOPMENT partition; writes policy + freeze hash |
| `diagnostics/run_test_014c_validation.py` | `main` — validation replay, intervals, charts, summary, **139-gate builder** |
| `diagnostics/test014c_common.py` | `load_common_rows`, `replay`, `intervalize`, `score`, `config_from_policy` |
| `diagnostics/finalize_test_014c_evidence.py` | Re-emits gates/hashes from existing summary; does **not** replay |
| `spy_volume_engine/engine.py` | `VolumeEngine.observe` |
| `spy_volume_engine/__init__.py` | package export |
| `emission_intervals.py` | `EmissionIntervalizer` color-age |

HEAD imports: `from spy_volume_engine import VolumeEngine, …`  
Working-tree `test014c_common.py` currently imports `volume_engine` (uncommitted rename). Not HEAD.

## Executable Call Graph

```text
run_test_014c_v_development.main
    load_common_rows()
        010 CSV + 014B P CSV timestamp join
        json.loads(interval_state_json) → interval_mean_vn
    for each of 4 VolumePolicyConfig:
        replay(development_rows, config)
            VolumeEngine(config)
            on date:session change: VolumePolicyState()
            engine.observe(row, state)
    score(...) occupancy / median interval
    min(eligible) → write POLICY + FREEZE

run_test_014c_validation.main
    sha256(POLICY) must equal freeze
    config_from_policy(POLICY)
    load_common_rows()
    replay(all common rows) twice; hashes must match
    validation = rows where partition==VALIDATION
    score(validation) → V_validation
    intervalize(P p_color, V cockpit_color)
    joint_intervals(p_color_cockpit_color)
    interval_invariants → must PASS or raise
    write emissions / intervals / charts
    verify_immutability of 014/014B hash inventories
    build_acceptance_gates(summary)  # 20 checks × pad to 139
    print ACCEPTANCE: 139/139 PASS

VolumeEngine.observe  [spy_volume_engine/engine.py]
    read V_RAW, V_N, V1, V2, predicted_next_V_N, interval_mean_vn
    if any nonfinite → INVALID emission, state.color=INVALID
    activity = interval_mean if state_source==INTERVAL_MEAN_V_N else V_N
    raw_color from activity vs 0.9/1.1
    confirmation vs prior state.color
    phase from V1/V2
    confidence HIGH or MEDIUM
    domain_state = "CAUSAL_LOCAL_VOLUME"
    return emission, VolumePolicyState(color=cockpit, pending_*)
```

Exact classes/functions:

- `test014c_common.load_common_rows`, `.replay`, `.intervalize`, `.score`, `.config_from_policy`
- `VolumeEngine.observe`, `._phase`, `._build`
- `VolumePolicyConfig`, `VolumePolicyState`, `VolumeEmission`
- `EmissionIntervalizer.observe`, `.complete`
- `run_test_014c_validation.build_acceptance_gates`, `.interval_invariants`, `.joint_intervals`

## Upstream 010 / 009V Dependencies

**010** (`diagnostics/run_test_010_volume_engine.py`):

- Input: `APTF_TEST_009V_PRICE_VOLUME_OBSERVATIONS_V0_1.csv` plus `APTF_TEST_009V_VOLUME_SELECTION_V0_1.json` (asserts `primary_V_N_candidate == "ROLLING_MEDIAN_RATIO_15"`).
- Does **not** recompute `V_N`/`V1`/`V2`; reads CSV columns.
- Computes interval descriptors for k ∈ {3,5,8,15}; `mean_vn = np.mean(vn_window)` requiring all-finite window.
- Compares 10 forecast models; **selected primary = `VOLUME_POINT`** (`predictions["VOLUME_POINT"] = vn.copy()`).
- Writes 010 emissions: `predicted_next_V_N = selected[index]` (= `V_N` for VOLUME_POINT), `interval_state_json` including `mean_vn`.
- Emission loop: `for index in range(15, len(rows)-1)` if selected and next `V_N` finite → 101,205 rows.

**009V** (`diagnostics/run_test_009v_volume_selection.py`):

- Input: `APTF_TEST_007_OBSERVATION_EPISODE_MAP_V0_1.csv` field `volume`.
- `rolling_baseline` median/mean windows 15/30/60, current included.
- `normalize`: `V_N = volume / baseline` when baseline finite and `> 0`.
- Candidate selected by a ranking key → frozen name `ROLLING_MEDIAN_RATIO_15`.
- `causal_quadratic`: last `window` positional values, `x = times[i]-times[current]`, `lstsq` of `[τ², τ, 1]`, `V1=b`, `V2=2a`, require rank 3.
- Derivative window selected from {3,5,8,15} → frozen primary **3**.

**009V multivariate** (`run_test_009v_multivariate_analysis.py`):

- `V_RAW = source_row["volume"]` (string/float copy of 007 `volume`).
- Writes the observations CSV 010 later reads.

Lineage **is** 007 → 009V → 010 → 014C. Proven by file paths and array copies, not by docs.

## Source-Derived Volume Mathematics

### V_RAW

- **009V:** `float(row["volume"])` from Test 007; empty → NaN; zeros counted, not imputed.
- **Multivariate:** `V_RAW = source_row["volume"]`.
- **010/014C:** copy-through `float(row["V_RAW"])`.
- **Engine:** copy-through; required finite for a valid emission.
- Positional: one value per observation row; no harvesting.

### V_N

Executable (`rolling_baseline` + `normalize`):

```text
baseline[t] = median(volume[t-14 : t+1])   # window 15, includes current
if baseline finite and baseline > 0:
    V_N[t] = volume[t] / baseline[t]
else:
    V_N[t] = NaN
```

Verified in 009V, not assumed from the name. 010 asserts the frozen selected candidate string is `ROLLING_MEDIAN_RATIO_15`.

### V1 / V2

`causal_quadratic(times, values, window)` with selected window **3**:

- Last 3 positional finite `V_N`
- `τ_i = t_i - t_current` in minutes (`datetime.timestamp()/60`)
- `design = [τ², τ, 1]`
- `np.linalg.lstsq(..., rcond=None)`
- `V1 = coefficients[1]`, `V2 = 2 * coefficients[0]`
- Skip / fail if any nonfinite in window, rank ≠ 3, or nonfinite coefficients

Actual elapsed minutes, not fixed 1.0 cadence.

### interval_mean_vn

Computed only in **010**:

```text
vn_window = vn[index-k+1 : index+1]   # k=15 for selected interval
if not all finite: skip (no nanmean)
mean_vn = np.mean(vn_window)
```

014C parses `interval_state_json["mean_vn"]`. It does not recompute the mean.

### predicted_next_V_N

010: `predictions["VOLUME_POINT"] = vn.copy()` then `predicted_next_V_N = selected[index]`. Frozen selection JSON: `"primary_model_id": "VOLUME_POINT"`.

So for the frozen 010 artifact consumed by 014C: **`predicted_next_V_N = V_N`**.

014C engine **requires it finite** and copies to `projected_v`. Color uses `interval_mean_vn`, not the prediction. `projected_v1`/`projected_v2` are always `None`. Development policy writes `"projected_V1_V2": "UNSUPPORTED_NOT_FABRICATED"`. 010 also records `"recommended_evolution": "G_V_DISCRETE_STATE_UPDATE"` and ODE suitability **WEAK**.

**Assessment:** this is a **selected persistence/no-change predictor (B)** that 010 ranked against other models, **and** a **thin/provisional trajectory surface (C/D)** for 014C: unused for color, derivatives unsupported, evolution recommended. Not a proven final projection model (not A).

## Source-Derived Interpretation State Machine

HEAD `spy_volume_engine.VolumeEngine.observe` statement order:

1. Parse six floats; if any nonfinite → INVALID emission, `VolumePolicyState(color="INVALID")`.
2. `activity = V_N` or `interval_mean` per `state_source`. Frozen: interval mean.
3. Raw color: `>= upper` GREEN; `<= lower` RED; else AMBER. Inclusive.
4. `color = raw_color`; `transition = "STABLE"`; pending cleared to `None`/`0`.
5. If `state.color is not None` and `raw != state.color`:
   - `pending_count = state.pending_count+1` if same pending candidate else `1`
   - if `pending_count < confirmation`: `color="AMBER"`, `pending_color=raw`, `PENDING_{raw}`
   - else: `CONFIRMED_{raw}` (`color` remains raw)
6. `confidence = "HIGH"` if `INTERVAL_MEAN_V_N` else `"MEDIUM"`
7. `_build(..., color)` → `cockpit_color=color`
8. **Return `VolumePolicyState(color=color, pending_color=pending_color, pending_count=pending_count)`**

Therefore **`state.color` is the emitted cockpit**, every valid observe. While pending, both cockpit and next `state.color` are AMBER.

First valid (`state.color is None`): immediate raw, STABLE.  
Same raw as `state.color`: STABLE, pending cleared.  
GREEN→AMBER pending: next `state.color=AMBER`; a following GREEN is `PENDING_GREEN`, not return-to-GREEN.  
GREEN→RED pending then second RED: `CONFIRMED_RED`; frozen leaves `pending_count=2` and `pending_color=None`.

014C `replay` resets **only** `VolumePolicyState()` when `timestamp[:10]+':'+session` changes. `VolumeEngine` has no session method. Feature windows are not present in the engine (features come precomputed). Price is not reset by Volume replay.

## Reset / Session Ownership

| Question | Executable answer |
|---|---|
| Does `VolumeEngine` reset on session? | **No.** No reset method. |
| Who resets interpretation? | **014C harness** `test014c_common.replay` |
| Do feature windows reset? | **N/A in 014C** — features are CSV rows, not live windows |
| Does 010 reset on session? | **No** — full-array positional windows |
| Is session reset scientific Volume math? | **Harness/lifecycle** for 014C validation occupancy |

## What 014C Computes vs Inherits

| Value | Classification |
|---|---|
| `V_RAW` | **C/D** loaded from 010; 010 copied 009V; 009V from 007 `volume` |
| `V_N` | **D** 009V; 010/014C copy |
| `V1`/`V2` | **D** 009V `causal_quadratic`; 010/014C copy |
| `interval_mean_vn` | **C** 010 `np.mean`; 014C parses JSON |
| `predicted_next_V_N` | **C** 010 `VOLUME_POINT=vn`; 014C copy-through |
| raw color | **B** `VolumeEngine.observe` |
| cockpit / pending / transition | **B** `observe` |
| phase | **B** `observe._phase` |
| confidence | **B** constant HIGH/MEDIUM |
| domain | **B** constant `CAUSAL_LOCAL_VOLUME` |
| session reset | **A** 014C `replay` only |
| P/V join | **A** 014C timestamp inner join |

## 139-Assertion Inventory

`run_test_014c_validation.build_acceptance_gates`:

```python
checks = [  # length 20
    P_AUTHORITY, V_FREEZE, V_NOT_MODIFIED,
    V_VALIDATION_ROWS (==17312), V_VALIDATION_SESSIONS (==39),
    V_INVALID_ZERO, V_GREEN_OCCUPANCY (>=0.075),
    V_AMBER_OCCUPANCY (<=0.60), V_RED_OCCUPANCY (>=0.075),
    V_PERSISTENCE (median_interval >= 3),
    P_INTERVAL_ASSIGNMENT, V_INTERVAL_ASSIGNMENT, PV_INTERVAL_ASSIGNMENT,
    DETERMINISTIC_REPLAY, BOUNDED_STATE, FIVE_CHARTS,
    IMMUTABILITY, NO_FUSION, NO_EXECUTION, NO_TRADING_PNL_BROKER,
]
for index in range(139):
    name, passed, value, artifact = checks[index % len(checks)]
    gates[f"G{index+1:03d}"] = ...
```

**Unique scientific/operational checks: 20.**  
**Independent Volume-feature-math checks: 0.**

| Category | Unique checks | What they prove |
|---|---|---|
| Volume feature math | 0 | nothing |
| Volume interpretation values | 0 (no per-row cockpit oracle) | nothing bitwise |
| Occupancy / persistence of cockpit on VALIDATION partition | 5 | selected lamp is not collapsed to one color |
| INVALID count | 1 | all validation rows were finite inputs |
| Interval assignment (P, V, PV) | 3 | contiguous 60s intervalizer bookkeeping |
| Hash / freeze / immutability | 4 | files did not change |
| Determinism / bounded state / charts | 3 | replay hash + 5 PNGs |
| No fusion / no execution / no PnL | 3 | scope flags |
| Price policy hash | 1 | 014B P policy unchanged |

`V_validation.observations == 17312` is the **VALIDATION partition** after the 014 split, not the 55,199 aligned emission rows.

**What 139/139 proves:** the 20 unique gates were true, then duplicated to fill a 139-slot ledger. It does **not** prove complete Volume-feature validation, universal calibration, or profitability.

Also enforced by `raise` (not in the 139 list): policy hash vs freeze; P/V join length; deterministic double replay; interval invariant status.

## Price / Volume Relationship

- Join key: `timestamp` only.
- 014C requires every 014B Price row to have a 010 Volume row.
- `observe(row, state)` never reads `price` or `p_color`.
- Joint state is string concatenation `p_color + "_" + cockpit_color` for intervalizer/charts/lead-lag.
- `causality_claim: False` in transition-relationship summary.
- `P_V_fusion: false`. No BUY/SELL in 014C.

014C is an **integration/alignment + V-policy validation** test, not a pure Volume-math unit test and not a fusion test.

## Post-014C Volume Usage

`git log ae0dacb..HEAD` is empty. **No later commit exists.**

`spy_volume_engine/engine.py` history: introduced in `ae0dacb` only.

Working-tree (uncommitted, not HEAD):

- `volume_engine/engine.py`: same observe math; `symbol=str(numerical["symbol"])` instead of hardcoded `"SPY"`.
- `run_test_015_validation.py`: reads `APTF_TEST_014C_SPY_PV_ALIGNED_EMISSIONS_V0_1.csv`; uses **`V_color` and `V_interval_age` only**; does not import VolumeEngine; does not recompute features.
- 016*: consumes 015 decisions / frozen P+V colors and ages for controller/paper paths.

015/016 treat 014C Volume **output lamps/ages as frozen upstream**. They do not continue Volume-feature experimentation. They also **do not exist at the 014C commit**.

## Completion / Incompletion Evidence

**Toward a freeze/baseline**

- Policy hashed and checked before validation reveal
- `V_policy_modified_after_validation: False`
- Classification `SPY_V_ENGINE_COCKPIT_READY` when occupancy gates pass
- No later **committed** Volume science
- Uncommitted 015 consumes V color/age unchanged

**Against “complete science”**

- 139 gates are padded repeats of 20 non-feature checks
- Feature pipeline is inherited, not re-proven in 014C
- Four experimental policies; winner is an occupancy heuristic on DEVELOPMENT
- `predicted_next_V_N` unused for color; `projected_V1_V2` unsupported
- 010 `recommended_evolution` still points at further discrete-state work
- Confidence and domain are constants
- Engine hard-codes `symbol="SPY"`
- Session reset is harness-only
- Zero/missing volume not exercised (009V audit `zero_count` used for reporting; 014C INVALID_count=0)
- No TODO/FIXME in 009V/010/014C Python; incompletion is structural (`UNSUPPORTED_NOT_FABRICATED`, constant metadata, padded gates)

## Component Maturity Matrix

| Component | Executable implementation | Validation in 014C | Completion evidence | Status |
|---|---|---|---|---|
| V_RAW | 007 `volume` copy | finite required | corpus has no zeros | VALIDATED on this corpus |
| Normalization / V_N | 009V median-15 ratio | 010 asserts candidate name only | selected among 9 candidates | VALIDATED / selected |
| V1/V2 | 009V quadratic window 3 | copy-through + phase uses them | selected among 4 windows | VALIDATED / selected |
| interval_mean_vn | 010 mean-15 all-finite | used as activity | not independently asserted | VALIDATED as input |
| predicted_next_V_N | 010 VOLUME_POINT = V_N | finite required; unused for color | recommended_evolution remains | PLACEHOLDER / baseline predictor |
| raw color | observe thresholds | occupancy only | 4 candidates tried | VALIDATED selected policy |
| confirmation / Indicator | observe state machine | occupancy + INVALID=0 | no per-row oracle in 014C | VALIDATED selected policy |
| transition_state | observe labels | written to CSV; not gated | diagnostic | VALIDATED (emitted) |
| phase | observe ε inequalities | written; not gated | diagnostic | VALIDATED (emitted) |
| confidence | constant HIGH | not gated | metadata | EXPERIMENTAL / metadata |
| domain state | constant | not gated | metadata | EXPERIMENTAL / metadata |
| session reset | 014C replay only | occupancy computed after reset | harness | HARNESS |
| P/V alignment | timestamp join + intervalizer | 3 interval invariants + charts | descriptive | VALIDATED alignment |
| downstream | 015 uncommitted | n/a at HEAD | later color consumer | UNCLEAR at HEAD; later UNCHANGED consume |

## APTF Python vs current QuanTRAM P-04V

| Concept | APTF Python (HEAD 014C path) | QuanTRAM P-04V (F-R, read-only) | Match? | Maturity concern |
|---|---|---|---|---|
| V_RAW | `float(volume)` | `float64(Bar.Volume)` | Yes on integer corpus | uint64 mantissa limit |
| V_N | median-15 inclusive / current | same | Yes | — |
| V1/V2 | 3-row τ minutes, lstsq, b / 2a | gonum lstsq same model | Yes (F measured) | solver ulps |
| interval_mean_vn | np.mean 15 finite | same | Yes | — |
| predicted_next_V_N | VOLUME_POINT = V_N | `predicted = V_N` | Yes | APTF left richer projection unsupported |
| raw color | interval_mean, 0.9/1.1 inclusive | same | Yes | bands from one-entity development |
| confirmation | observe machine | F-R `confirmColor` | **Yes after F-R** | historical machine, not proven universal |
| state.Color | emitted cockpit | next.Color = Indicator | Yes after F-R | — |
| pending | pending_color/count; confirm leaves count=2 | same | Yes | — |
| Indicator / cockpit | `cockpit_color` | `VolumeEvent.Indicator` | Yes (term map) | — |
| transition | STABLE / PENDING_* / CONFIRMED_* | same | Yes | diagnostic |
| phase | ACTIVITY_* , ε=1e-12 | same | Yes | diagnostic |
| confidence | HIGH if interval source | HIGH on valid color | Yes | constant |
| domain | CAUSAL_LOCAL_VOLUME | same | Yes | constant |
| session reset | harness `replay` | production Engine does not | **Intentional arch. diff.** | do not import harness into production |

## F-R Assessment

**Does F-R reproduce executable 014C confirmation?**  
**Yes.** HEAD `return VolumePolicyState(color=color, …)` with `color` equal to emitted cockpit is what F-R implemented. Pending AMBER write, return-to-prior-raw as a new pending candidate, and leftover `pending_count=2` on confirm all match.

**Is that machine mature/final Volume science?**  
**Not demonstrably.** 014C validated occupancy of the selected policy, not a theory of confirmation. The machine is the frozen historical interpreter. Treat F-R as **migration fidelity**, not a completeness certificate.

Frozen-output corroboration (prior QuanTRAM F-R measurements, not re-run here): 010 features and session-sliced 014C categoricals match. That corroborates the reconstruction; it does not upgrade 014C into complete science.

## Documentation Cross-Check (after code)

| Documentation / prior-migration claim | Executable evidence | Match? | Significance |
|---|---|---|---|
| “139/139 PASS” = full Volume validation | 20 unique gates padded to 139 | **No** | Earlier work can over-read the scoreboard |
| 014C computes Volume features | 014C loads 010 CSV | **No** | Scientific authority is 009V/010 |
| 014C is the mature Volume Engine | First reusable `VolumeEngine`; features upstream; policy selected experimentally | Partial | Engine is the interpreter, not the whole science |
| `predicted_next_V_N = V_N` is the Volume model | 010 selected VOLUME_POINT among 10 models; unused for 014C color | Partial | Persistence baseline, not a proven forecast |
| Session reset is Volume science | Only `replay` in diagnostics | **No** | Harness lifecycle |
| Confidence HIGH is computed quality | Constant from `state_source` | **No** | Metadata |
| Later 015 continues Volume R&D | Uncommitted 015 consumes V_color/age only | No at HEAD | Downstream lamp consumer, not a new Volume model |

Do not rewrite the code-derived classification to match those docs.

## Conclusion

**Question 1 — What Volume science did 014C execute?**  
Interpretation only: activity-band color, confirmation, phase labels, constant confidence/domain, plus intervalizer ages. Features were inputs.

**Question 2 — What was inherited?**  
`V_RAW`/`V_N`/`V1`/`V2` from 009V (via 010 CSV). `interval_mean_vn` and `predicted_next_V_N` from 010.

**Question 3 — What is 139/139?**  
Twenty unique acceptance predicates, mostly occupancy, interval bookkeeping, hashes, and scope flags, repeated to 139 IDs. Not a 139-point Volume-math exam.

**Question 4 — Completed engine or intermediate?**  
A **frozen selected interpreter** on top of earlier feature experiments. Implemented and gate-validated; **not demonstrably complete** Volume science.

## Limitations

- Working tree is dirty; HEAD blobs were used as authority.
- 015/016 inspected only as post-HEAD uncommitted consumers.
- Docs cross-check is illustrative, not a full APTF markdown corpus review.
- QuanTRAM comparison is read-only against current F-R sources; this audit did not re-run `go test`.

## Change Log

| Date | Change |
|---|---|
| 2026-09-05 | Executable-source 014C audit. 139/139 decoded as padded 20-check ledger. Completion class B / MEDIUM. No production or prior P-04V doc edits. |
