# QuanTRAM — APTF Volume Engine Full Repository Forensic Investigation and Go Refactorability Assessment

**Title:** QuanTRAM — APTF Volume Engine Full Repository Forensic Investigation and Go Refactorability Assessment  
**Date:** 2026-09-05  
**Status:** INVESTIGATION COMPLETE — HUMAN REVIEW REQUIRED  
**Purpose:** Reconstruct the complete validated APTF Volume scientific/runtime path — from raw market volume through numerical preparation, stateful VolumeEngine processing, VolumeEmission, validation, and first downstream convergence — and assess which parts can be faithfully refactored into Go for QuanTRAM.  
**Scope:** Read-only executable and documentary forensics of the historical APTF repository, plus contextual reading of QuanTRAM P-01–P-04 / StageTransition V1.1 and the SADE Go-refactorability investigation. No implementation. No scientific change. No process-model, proto, or StageTransition change.  
**Documentation path:** This repository’s established investigation path is `docs/investigations/` (used by `QuanTRAM_STAGE_TRANSITION_LIVE_RUN_AUDIT_2026-09-04.md`). This document is stored there.

## Executive Summary

The mature APTF Volume path is **not** the `VolumeEngine` package alone. `VolumeEngine` is a thin, Price-independent policy interpreter. The scientific pipeline that produces its inputs lives in Tests **009V** and **010**. Test **014C** is the first reusable runtime plus the first evidenced P/V timestamp alignment. Test **015** is the first *semantic* consumer of both colors; it is downstream of Volume, not part of Volume.

Exact mature lineage, proven by executable call chains and frozen artifacts:

```text
Test 007 observation map field `volume`   (raw market volume, FirstRateData 1-min)
        ↓
Test 009V  V_RAW = source volume (unchanged)
        ↓
Test 009V  V_N = V_RAW / median(trailing 15 V_RAW including current)
           primary candidate: ROLLING_MEDIAN_RATIO_15
        ↓
Test 009V  V1, V2 = causal 3-row quadratic on V_N vs elapsed minutes
        ↓
Test 010   interval descriptors (k ∈ {3,5,8,15}); selected interval k=15
           interval_mean_vn = mean(trailing 15 V_N)
           interval_state_json
        ↓
Test 010   predicted_next_V_N = V_N(t)     [VOLUME_POINT persistence; not RK45]
        ↓
014C       VolumeEngine.observe(numerical, VolumePolicyState)
           frozen policy V_INTERVAL_B10_C2
           activity = interval_mean_vn; bands 0.90 / 1.10; confirm 2
        ↓
014C       VolumeEmission (cockpit_color GREEN/AMBER/RED)
           next VolumePolicyState
        ↓
014C       EmissionIntervalizer (V color age; requires exact 60.0 s continuity)
        ↓
014C       timestamp-aligned P/V artifacts
        ↓
015        joint color/age interpretation → BUY/HOLD/SELL   [DEFERRED]
```

**D01 `update_volume_influence` is a different path.** It consumes raw volume inside Adaptive effective-mass and is already present in QuanTRAM P-03. It does not produce `V_N`, `V1`, `V2`, `VolumeEmission`, or cockpit color. SADE deferred the standalone Volume Pipeline; that deferral is not a scientific rejection.

**Go refactorability:** the complete production path appears Go-refactorable. No Volume step has a justified reason to remain in Python. The numerically sensitive step (causal quadratic `lstsq`) is the same method QuanTRAM P-04 already implemented in gonum. The policy shell is scalar threshold/hysteresis logic. The missing work is not a Python residue — it is the **upstream feature pipeline** (normalization, derivatives, interval mean) plus an equivalence corpus against frozen 009V/010/014C artifacts.

Highest scientific risks: (1) feature windows do **not** reset on session while policy state **does**; (2) windows are observation-count, not elastic time; (3) 014C thresholds were selected on one historical entity corpus; (4) zero/missing volume were not exercised in the frozen 007 series; (5) `predicted_next_V_N` is required and copied through but does not drive color.

## Module / System Overview

Two APTF engine packages exist. They must not be collapsed.

| Authority | Path | Status | Role |
|---|---|---|---|
| Committed HEAD `ae0dacb` | `spy_volume_engine/engine.py` | Frozen 014C runtime | Hard-codes emission `symbol="SPY"` |
| Working tree (uncommitted, 2026-08-25) | `volume_engine/engine.py` | Generic rename + docstring pass | Reads `numerical["symbol"]`; same policy math |

This investigation treats **HEAD `spy_volume_engine` + frozen 009V/010/014C artifacts** as the validated scientific authority. The working-tree `volume_engine` package is a later packaging refactor, not a new scientific freeze.

Upstream producers are diagnostics, not the engine package:

| Stage | Producer | What it freezes |
|---|---|---|
| Raw audit + V_N + V1/V2 | `diagnostics/run_test_009v_volume_selection.py` | `ROLLING_MEDIAN_RATIO_15`, window 3 |
| Joint observation CSV | `diagnostics/run_test_009v_multivariate_analysis.py` | `V_RAW`/`V_N`/`V1`/`V2` beside Price fields |
| Intervals + VOLUME_POINT | `diagnostics/run_test_010_volume_engine.py` | `interval_state_json`, `predicted_next_V_N` |
| Policy runtime | `diagnostics/test014c_common.py` + `VolumeEngine` | cockpit color, confirmation |
| Color age | `emission_intervals.py::EmissionIntervalizer` | contiguous V intervals |

QuanTRAM today: P-01 → P-02 → collocated P-03 Adaptive and P-04 Price Engine. `domain.Bar.Volume` is already on the accepted eligible bar. P-03 already applies D01 volume *influence*. There is no standalone Volume process and no Volume StageTransition.

## Inputs

Investigated repositories and baselines:

| Item | Value |
|---|---|
| APTF HEAD investigated | `ae0dacb2e02c5b80c82f6662d1a3c6863f4b989a` — “Test 014C completed with Pand V Engine emission charts” |
| APTF working tree | Dirty: `spy_volume_engine/` deleted; `volume_engine/` untracked; many freeze artifacts relocated to `pre08242026_docs/` |
| QuanTRAM HEAD at investigation start | `07417d9c85799949cd3b173067795905513af173` on `main` |
| Prior APTF Volume engine report | `APTF/post08242026_docs/investigations/APTF_PRICE_VOLUME_ENGINE_EXECUTABLE_FORENSIC_ANALYSIS_2026-08-25.md` |
| Prior 007–009V forensics | `APTF/post08242026_docs/investigations/APTF_TEST_007_TO_009V_EXECUTABLE_FORENSIC_ANALYSIS_2026-08-25.md` |
| Prior 010–016 forensics | `APTF/post08242026_docs/investigations/APTF_TEST_010_TO_016_EXECUTABLE_FORENSIC_ANALYSIS_2026-08-25.md` |
| SADE deferral record | `QuanTRAM/SADE_GO_REFACTORABILITY_AND_SCALED_RUNTIME_INVESTIGATION_2026-08-27.md` |
| QuanTRAM process model (read only) | `docs/design/QuanTRAM_PROCESS_MODEL_082926.md` |
| Frozen StageTransition (read only) | V1.1 at QuanTRAM HEAD |

Artifact path note: at APTF HEAD, freeze files lived at repository root. In the current working tree they live under `pre08242026_docs/`. Diagnostics still resolve `ROOT / "APTF_TEST_..."`. Citations below use the **working-tree location** of readable files and state HEAD-root provenance where hashes were captured.

## Outputs

This document only. No code, proto, process-model, StageTransition, Snapshot, Mongo, DNA, or APTF/SADE change.

## Parameters / Configuration

Frozen mature Volume policy (014C), not QuanTRAM runtime config:

| Name | Frozen value | Class |
|---|---|---|
| `primary_V_N_candidate` | `ROLLING_MEDIAN_RATIO_15` | scientific calibration |
| `primary_volume_derivative_window` | `3` | scientific calibration |
| `primary_model_id` | `VOLUME_POINT` | scientific calibration |
| selected interval for JSON | `15` | scientific calibration |
| `state_source` | `INTERVAL_MEAN_V_N` | scientific calibration |
| `lower_threshold` | `0.9` | scientific calibration |
| `upper_threshold` | `1.1` | scientific calibration |
| `confirmation_observations` | `2` | scientific calibration |
| `epsilon` | `1e-12` | scientific / numerical |
| `policy_id` (runtime) | `V_INTERVAL_B10_C2` | scientific identity |
| `policy_id` (artifact) | `V_EMISSION_V0_1` | freeze identity |
| D01 `reference_alpha` | `0.05` | **different path** |
| D01 `influence_bounds` | `[0, 3]` | **different path** |

## Assumptions

- Executable APTF code and frozen hashes win over design prose when they conflict.
- Chronology is taken from import/call graphs and artifact producers, not filenames alone.
- “Mature VolumeEngine” means the 014C reusable policy runtime. “Complete Volume path” means 009V + 010 + 014C.
- QuanTRAM elastic-interval ideas are **not** retrofitted onto APTF.
- Historical entity name `SPY` appears only as APTF evidence. It is not a proposed QuanTRAM hard-code.

## Explicit Exclusions

- No Volume implementation.
- No QuanTRAM master-process update and no P-05 numbering decision.
- No proto, StageTransition, Snapshot, Persistence, Mongo, or DNA / Quantram_transaction work.
- No APTF or SADE modification.
- No scientific recalibration.
- No assumption that Price and Volume are symmetric.
- No assumption that SADE’s deferred Volume Pipeline means Volume was rejected.
- No assumption that `Bar.Volume` equals `VolumeEmission`.
- No assumption that importing only `VolumeEngine` is a complete migration.

---

## Evidence classes used below

| Tag | Meaning |
|---|---|
| **PROVEN BY EXECUTABLE CODE** | Behavior read from a function that would run |
| **PROVEN BY TEST** | Asserted by an automated test or runner gate |
| **PROVEN BY FROZEN ARTIFACT** | Present in a hashed freeze file |
| **DOCUMENTED INTENT ONLY** | Design/method prose without executable enforcement |
| **INFERRED — REQUIRES VALIDATION** | Reasonable from code, not independently replayed here |
| **UNKNOWN** | Not specified by code or freeze |

This investigation did **not** re-execute 009V/010/014C. Counts and hashes are taken from frozen artifacts and source inspection.

---

## PART 1 — Exact lineage reconstruction

### 1.1 Chronology from call chains, not filenames

| Generation | What it actually is | Introduces | Consumes | Does **not** contain |
|---|---|---|---|---|
| Test 007 | Observation/episode map over FirstRateData 1-min bars | Source field `volume` on aligned rows | Market CSV + earlier adaptive/emitter columns | Volume normalization, VolumeEngine |
| Test 009 | Price derivative freeze (sibling, not Volume) | `primary_D1` / `primary_D2` on price | 007 map | V_N |
| Test 009V | Volume normalization + derivative **selection** | `V_RAW`, `V_N`, `V1`, `V2` | 007 `volume` + timestamps | VolumeEngine class, predicted_next, interval JSON |
| 009V multivariate | Joint observation CSV + descriptive P×V analytics | `APTF_TEST_009V_PRICE_VOLUME_OBSERVATIONS_V0_1.csv` | 009V selection JSON + 009 price files | Policy color |
| Test 010 Volume | Interval descriptors + forecast-model **selection** | `interval_state_json`, `predicted_next_V_N`, observer events | 009V observation CSV + 009 crossings (events only) | `VolumeEngine` class; RK45 |
| Test 010 Price | Separate Price local-dynamics / emissions in the same test family | Price emissions | 009 price | Volume policy |
| Test 011 | Control / RK interface using 010 artifacts | Volume observer state tables | 010 Volume + Price projections | VolumeEngine |
| Test 014 / 014B | Price engine + cockpit | P emissions used later for alignment | Price trajectory | VolumeEngine |
| Test 014C | First reusable VolumeEngine + P/V alignment | `VolumeEmission`, frozen V policy, V intervals, PV aligned CSV | 010 V emissions + 014B P emissions | BUY/SELL; Price→Volume math |
| Test 015 | Execution interpreter | BUY/HOLD/SELL from P/V colors and ages | 014C aligned CSV | Volume mathematics |
| Test 016 | Paper account | Orders/ledger | 015 decisions | Volume mathematics |
| 2026-08-25 generic refactor | Package rename (working tree) | `volume_engine`, symbol from payload | Same 014C math | New freeze |

**PROVEN BY EXECUTABLE CODE:** `git log -S "class VolumeEngine"` / HEAD contents show `VolumeEngine` first as a reusable class in the 014C commit. 009V and 010 are diagnostics that write arrays/CSV, not `VolumeEngine.observe`.

**Naming collision:** `diagnostics/run_test_010_volume_engine.py` is **not** the mature `VolumeEngine`. It is the 010 interval/forecast selector. The class appears in `spy_volume_engine/engine.py` (HEAD) / `volume_engine/engine.py` (working tree).

### 1.2 Scientific lineage diagram (actual APTF behavior)

```text
FirstRateData 1-min bar
   └─ Test 007 map.volume                    [V_RAW source]
         │
         ├─────────────────────────────────────────────┐
         │                                             │
         ▼                                             ▼
   009V rolling median ratio 15              D01 v02 update_volume_influence
   V_N = V_RAW / median(V_RAW[t-14:t])       EMA ref + log1p relative+absolute
         │                                   → Adaptive v* / effective_mass
         ▼                                   [SEPARATE PATH — not VolumeEngine]
   009V causal quadratic window 3
   V_N(τ)=aτ²+bτ+c ; V1=b ; V2=2a
         │
         ▼
   010 trailing-k descriptors
   interval_mean_vn = mean(V_N[t-14:t])      [k=15 selected]
   interval_state_json
         │
         ▼
   010 VOLUME_POINT
   predicted_next_V_N(t) = V_N(t)
         │
         ▼
   014C VolumeEngine.observe
   activity = interval_mean_vn
   raw_color by {0.9, 1.1}
   phase from V1,V2
   hysteresis confirmation=2
         │
         ├─ VolumeEmission.cockpit_color
         └─ VolumePolicyState
                │
                ▼
         EmissionIntervalizer (exact 60s + session)
                │
                ▼
         014C timestamp join to 014B P emissions
                │
                ▼
         015 interpreter (P_color, V_color, ages)   [DEFERRED]
```

Price never enters the left column above 014C except as **descriptive/join** columns. **PROVEN BY EXECUTABLE CODE** (009V selection imports no price; 010 Volume forecast uses only V fields; 014C policy `price_inputs_used: false`).

### 1.3 Intermediate tests actually required

Required for the mature production path:

1. **007** — raw `volume` provenance and 101,221-row authority.
2. **009V selection** — frozen V_N / V1 / V2.
3. **009V multivariate** — only because 010 reads `APTF_TEST_009V_PRICE_VOLUME_OBSERVATIONS_V0_1.csv`, not the selection JSON arrays directly.
4. **010 Volume** — interval JSON + VOLUME_POINT + emission CSV that 014C loads.
5. **014B P emissions** — timestamp universe for 014C common rows (alignment only).
6. **014C** — VolumeEngine + frozen policy + validation.

Not required for Volume science (historical / downstream):

- 009 Price (except as the timestamp/price sibling that multivariate joins).
- 010 Price local dynamics.
- 011 control.
- 014 Price policy development (except producing 014B files).
- 015 / 016.
- D01 `aptf_d01.volume.*` legacy modules.
- D01 v02 `volume.py` influence.

---

## PART 2 — Raw input provenance

### 2.1 Field-level provenance table (mature VolumeEngine inputs)

| Field | Type | Source module | Producing function | Mathematical definition | Dependencies | Window | Time dependency | Config | State | First introduced | Frozen/validated | Artifact | Downstream |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `V_RAW` | float (CSV); conceptually source volume | 007 map → 009V multivariate | `run_test_009v_multivariate_analysis.main` L115, L153 | `V_RAW(t) = source_row["volume"]` unchanged | 007 `volume` | none | timestamp copied, not used in value | none | none | 009V | 009V raw audit + selection | `APTF_TEST_009V_PRICE_VOLUME_OBSERVATIONS_V0_1.csv` | 010, 014C, VolumeEngine copy-through |
| `V_N` | float or empty/NaN | 009V selection | `rolling_baseline` L120–125; `normalize` L128–138; `main` L267–278, L324 | `V_RAW / median(V_RAW[t-w+1:t])`, w=15 | `V_RAW` | 15 observations including current | observation index, not clock | `BASELINE_WINDOWS`, selected 15 | none beyond window | 009V | 009V selection JSON; 010 gate L92–93 | `APTF_TEST_009V_VOLUME_SELECTION_V0_1.json` `selected_V_N` | 010, 014C, emission field `v` |
| `V1` | float or NaN | 009V selection | `causal_quadratic` L68–88 | `b` from `V_N(τ)=aτ²+bτ+c` | finite V_N window, elapsed minutes | 3 observations | **yes** — τ in minutes relative to current | `DERIVATIVE_WINDOWS`, selected 3 | none | 009V | 009V selection `selected_V1` | same JSON | 010, 014C phase |
| `V2` | float or NaN | 009V selection | `causal_quadratic` L87 | `2a` | same as V1 | 3 | yes | window 3 | none | 009V | `selected_V2` | same JSON | 010, 014C phase |
| `predicted_next_V_N` | float | 010 Volume | `main` L151, L227 | `VOLUME_POINT`: copy of `V_N(t)` | `V_N` | none | none for selected model | model selection | none | 010 | 010 selection JSON | `APTF_TEST_010_VOLUME_ENGINE_EMISSIONS_V0_1.csv` | VolumeEngine `projected_v` only |
| `interval_mean_vn` | float | 010 Volume → 014C loader | 010 L122 `mean_vn`; 014C `load_common_rows` L58–61 | `mean(V_N[t-k+1:t])`, k=15 | finite V_N window | 15 | observation count | selected interval 15 | none | 010 | 010 emissions JSON blob; 014C policy | `interval_state_json.mean_vn` | VolumeEngine activity when `INTERVAL_MEAN_V_N` |
| `timestamp` | str ISO UTC | 007 / 009 / 010 | copied | metadata + session key + derivative coordinate source | source timestamp | — | parsed to minutes for V1/V2; string prefix for 014C session | — | — | 007 | 007/009V/010/014C | all CSVs | engine payload; session reset |
| `VolumePolicyState` | dataclass | caller | 014C `replay` L79–89 | prior color / pending color / count | previous emission | — | session reset | confirmation=2 | **yes** | 014C | 014C V emissions | policy freeze | next observe |

**PROVEN BY EXECUTABLE CODE** for every row above.

### 2.2 Aliases (do not silently normalize)

| Name | Where | Same as? |
|---|---|---|
| `volume` | 007 CSV column | `V_RAW` after 009V |
| `V_RAW` | 009V+ | source volume |
| `V_N` / `v` | selection / VolumeEmission.v | same value |
| `selected_V_N` | 009V JSON array | per-row V_N |
| `mean_vn` | 010 interval JSON key | `interval_mean_vn` after 014C parse |
| `V_mean_relative_to_baseline` | 010 interval CSV column | same as `mean_vn` (L134) |
| `predicted_next_V_N` | 010 / engine input | VolumeEmission.`projected_v` |
| `projected_v` | emission | copy of predicted_next |
| `projected_v1` / `projected_v2` | emission | always `None` |
| `cockpit_color` | VolumeEmission | 015 `v_color` |
| `raw_V1` | 009V multivariate L124–125 | **not** V1; backward raw-volume difference / Δt |
| `G_V` | 010 recommended evolution | architectural label for discrete state update, not a coded class |
| Test 010 “volume engine emissions” | CSV rows | **not** `VolumeEngine` objects |
| D01 `v_star` / `v*` | Adaptive influence | **not** V_N |

### 2.3 Raw volume origin

007 map header and first rows (**PROVEN BY FROZEN ARTIFACT** `pre08242026_docs/APTF_TEST_007_OBSERVATION_EPISODE_MAP_V0_1.csv` L1–3):

- `entity_id=SPY`
- `volume` float (example `10367.0`, `1124.0`)
- `source_provider=FirstRateData`, `source_dataset=SPY_1min_firstratedata`
- `event_timestamp_utc` ISO Z
- `session_type` in {PREMARKET, REGULAR, AFTERHOURS}

009V reads `row["volume"]` (**PROVEN BY EXECUTABLE CODE** `run_test_009v_volume_selection.py` L168–173). Empty string → NaN and `missing += 1`. Non-empty → `float(raw)`.

009V raw audit (**PROVEN BY FROZEN ARTIFACT** `APTF_TEST_009V_RAW_VOLUME_AUDIT_V0_1.json`): `source_rows=101221`, `valid_count=101221`, `missing_count=0`, `zero_count=0`, `minimum=100.0`, `median=35592.0`, `maximum=4822591.0`, `extremes_removed=0`, `source_modified=false`.

---

## PART 3 — Volume normalization

### 3.1 Authoritative formula

**Primary / frozen:** `ROLLING_MEDIAN_RATIO_15`

\[
V_N(t)=\frac{V_{RAW}(t)}{\operatorname{median}(V_{RAW}[t-14:t])}
\]

Trailing window **includes current**. Baseline must be finite and `> 0` or `V_N` is NaN.

**PROVEN BY EXECUTABLE CODE** `rolling_baseline` L120–125 and `normalize` L128–137.  
**PROVEN BY FROZEN ARTIFACT** `APTF_TEST_009V_VOLUME_SELECTION_V0_1.json` L19–21; `APTF_TEST_014C_V_AUTHORITY_V0_1.json` L3–8.  
**DOCUMENTED INTENT** `APTF_TEST_009V_VOLUME_NORMALIZATION_METHOD_V0_1.md` L11–12.

### 3.2 All candidates (historical; not authoritative)

For `w ∈ {15,30,60}`:

| Candidate | Formula | Valid only if |
|---|---|---|
| `ROLLING_MEDIAN_RATIO_w` | `V_RAW / median(window)` | baseline > 0 |
| `ROLLING_MEAN_RATIO_w` | `V_RAW / mean(window)` | baseline > 0 |
| `LOG_ROLLING_MEDIAN_RELATIVE_w` | `log(V_RAW / median(window))` | baseline > 0 **and** `V_RAW > 0` |

**PROVEN BY EXECUTABLE CODE** L226–235, L133–137.

Time-of-day relative Volume was **evaluated and rejected** as a primary candidate (`time_of_day_normalization_tested: false`). **PROVEN BY FROZEN ARTIFACT** selection JSON L10–11; **DOCUMENTED INTENT** method md L22–24.

### 3.3 Why ROLLING_MEDIAN_RATIO_15 became authority

Lexicographic rank in `main` L267–277:

1. max `valid_actionable`
2. min `numerical_failures`
3. min fixed-15 single-observation V1 reversal percentage
4. min V2 sign-change rate
5. max median V1 persistence
6. min regular-session minute residual IQR
7. max raw-volume Spearman
8. candidate name

No crossings, emitter labels, or P&L (**PROVEN BY EXECUTABLE CODE** L315–316; **PROVEN BY TEST** multivariate L98–99 rejects if `selection_used_pnl`).

From the comparison CSV, all 15-window methods have `valid_actionable=101206` and `numerical_failures=0`. `ROLLING_MEDIAN_RATIO_15` then wins on reversal percentage `19.078` vs mean-ratio `19.174` and log `21.160`. **PROVEN BY FROZEN ARTIFACT** comparison CSV L8 vs L5–L2.

010 **re-asserts** this freeze: `if selection["primary_V_N_candidate"] != "ROLLING_MEDIAN_RATIO_15": raise RuntimeError`. **PROVEN BY TEST** `run_test_010_volume_engine.py` L92–93.

### 3.4 Operational properties

| Property | Behavior | Evidence |
|---|---|---|
| Rolling vs static | Rolling; recomputed from trailing window each row | CODE L122–124 |
| Initialization | Indices `< 14` → NaN; `ACTIONABLE_START=15` for scoring | CODE L20, L121 |
| Window length | 15 observations, not 15 minutes | CODE L18, L227 |
| Session reset | **None.** Window may span PREMARKET/REGULAR/AFTERHOURS and day boundaries | CODE has no session check in `rolling_baseline` |
| Zero volume | Would yield `V_N=0` if baseline > 0; frozen corpus has `zero_count=0` | ARTIFACT raw audit L56; CODE L137 |
| Missing volume | Empty → NaN; not replaced by 0 | CODE L169–171; INTENT method md L5 |
| Nonfinite | Stay NaN; excluded from baseline_valid | CODE L131 |
| Clipping | None. Max V_N in comparison is `5210.16` | ARTIFACT comparison L8 |
| Historical recomputation | Batch over full series; mathematically causal (current+past only) | CODE L122 `index-window+1:index+1` |
| Causal | Yes. `future_observations_used: 0` | ARTIFACT selection L6 |
| Changes over time | Yes — baseline moves. No full-dataset baseline | INTENT method md L20 |

**UNKNOWN:** behavior if an entire 15-window is zeros (baseline 0 → NaN). Not present in frozen SPY series.

---

## PART 4 — Volume derivatives

### 4.1 Authoritative formulas

On frozen `V_N`, for window `w=3`, using elapsed minutes `T`:

\[
x_i = T_{t-w+1+i} - T_t,\qquad
V_N(\tau)=a\tau^2+b\tau+c
\]

\[
V1=b,\qquad V2=2a
\]

Design matrix `[x², x, 1]`. Solve `np.linalg.lstsq(design, current, rcond=None)`. Require `rank==3` and finite coefficients.

**PROVEN BY EXECUTABLE CODE** `causal_quadratic` L72–87.

Units: normalized-volume / minute and / minute². **DOCUMENTED INTENT** `APTF_TEST_009V_VOLUME_DERIVATIVE_METHOD_V0_1.md` L11.

### 4.2 Properties

| Property | Behavior | Evidence |
|---|---|---|
| Numerical method | Linear least squares, polynomial degree 2 | CODE L77–79 |
| Time coordinate | `datetime.timestamp()/60.0` elapsed minutes | CODE L174 |
| Window | 3 observations including current | ARTIFACT selection L22 |
| Causality | Trailing window only; x recentered on current | CODE L76 |
| Irregular time | **Handled** — uses actual Δt, not assumed Δ=1 | CODE L76; INTENT method md L11 |
| Input history | 3 finite V_N values | CODE L73–75 |
| Warm-up | First possible fit at index 2, but V_N NaN until index 14, so first 3-finite V_N window is index **16** | INFERRED from CODE + selection JSON first non-null V1 at array index 16 |
| Session/reset | **None** in the fitter | CODE |
| Failures | LinAlgError, rank≠3, or nonfinite coeffs → leave NaN, increment failures | CODE L80–85 |
| Library | NumPy `lstsq` | CODE L12, L79 |
| Complexity | O(n) fits of 3×3 | CODE loop |

Reference-only `raw_V1 = diff(V_RAW) / diff(seconds/60)` is **not** primary V1. **PROVEN BY EXECUTABLE CODE** multivariate L124–125; **DOCUMENTED INTENT** derivative method md L13–15.

### 4.3 Window selection

Candidates `{3,5,8,15}`. Rank: max valid, min single-obs V1 reversal %, min V2 sign-change rate, max median V1 persistence, then smaller window. **PROVEN BY EXECUTABLE CODE** L293–300. Winner: **3**. **PROVEN BY FROZEN ARTIFACT** selection L22.

### 4.4 Relationship to Price derivatives

**Analogous method, independently calibrated series and window. Not the same path.**

| | Volume V1/V2 | Price P1/P2 (APTF 009 / QuanTRAM P-04) |
|---|---|---|
| Series | `V_N` | close / price |
| Window | **3** | **15** (QuanTRAM `causalQuadraticAtIndex`) |
| Fit | `y = a x² + b x + c`, x = minutes − current | same |
| Outputs | V1=`b`, V2=`2a` | p1=`b`, p2=`2a` |
| Library | `np.linalg.lstsq` | QuanTRAM `pricing.lstsq` / gonum |

**PROVEN BY EXECUTABLE CODE** APTF `causal_quadratic` vs QuanTRAM `internal/pricing/derivatives.go` L5–28.  
Do **not** reuse P-04’s 15-window on volume. Do **not** feed V_N into PriceEngine.

---

## PART 5 — `predicted_next_V_N`

### 5.1 What actually feeds VolumeEngine

**Producer:** `diagnostics/run_test_010_volume_engine.py::main` L151, L213, L227.  
**Selected model:** `VOLUME_POINT`.  
**Mathematics:** `predicted_next_V_N(t) = V_N(t)` — persistence / carry-forward, **not** a solver forecast.

**PROVEN BY EXECUTABLE CODE** L151 `predictions["VOLUME_POINT"] = vn.copy()`.  
**PROVEN BY FROZEN ARTIFACT** `APTF_TEST_010_VOLUME_ENGINE_SELECTION_V0_1.json` L41 `primary_model_id: VOLUME_POINT`; 014C policy L26 `projected_V(t+1)=V_N(t)`.

Classification: **carry-forward of current V_N**, scored as a one-step predictor during 010 selection. Horizon is one next observation. No RK45 (`runge_kutta_used: false`). Warm-up: predictions `[:15] = nan` (L181). Failure: none for VOLUME_POINT (failed_forecasts=0).

### 5.2 Candidates that did **not** become authority

| Model | Formula | Why not primary |
|---|---|---|
| `VOLUME_DERIVATIVE` | `V_N + V1·h + 0.5·V2·h²` with **revealed next** `h=Δt` | Higher complexity / error in lexicographic rank; also uses next interval for scoring — not causal at runtime without knowing next h |
| `VOLUME_INTERVAL_Kk` | current trailing median V_N over k | Worse rank than POINT |
| `VOLUME_INTERVAL_DERIVATIVE_UPDATE_Kk` | 60-row standardized lstsq of `[V_N, mean_k, median_raw, std, max/median, V1, V2]` → next V_N | Development only |

**PROVEN BY EXECUTABLE CODE** L152–177, L209.  
**INFERRED:** VOLUME_DERIVATIVE’s use of `h_next` (L152–153) is a scoring leak if treated as a live forecast. It was not selected.

### 5.3 Use inside VolumeEngine

`predicted_next_V_N` is **required** (KeyError if missing; INVALID if nonfinite) and copied to `projected_v`. It is **not** used for `raw_color`, `phase`, or confirmation.

**PROVEN BY EXECUTABLE CODE** HEAD `spy_volume_engine/engine.py` L67–72, L81–108.

`projected_v1` / `projected_v2` are always `None` (`UNSUPPORTED_NOT_FABRICATED`). **PROVEN BY EXECUTABLE CODE** L143; **PROVEN BY FROZEN ARTIFACT** 014C policy L22.

---

## PART 6 — `interval_mean_vn` / interval semantics

### 6.1 Two different “intervals” exist

Do not conflate them.

| Structure | Owner | What it aggregates | Boundary rule |
|---|---|---|---|
| **Feature interval** (010) | trailing observation window k | V_N / raw-volume statistics | Fixed **count** k; slides every observation |
| **Color interval** (014C `EmissionIntervalizer`) | contiguous same cockpit color | color age / duration | Same session **and** elapsed **exactly 60.0 seconds** |

QuanTRAM elastic/nonlinear interval ideas apply to neither without a future design. APTF feature intervals are count-based. APTF color intervals are fixed-cadence.

### 6.2 Feature interval (feeds VolumeEngine)

- Begin: first index where `k` consecutive V_N are finite (`index >= k-1`).
- End: current observation (inclusive).
- Elastic? **No.** k ∈ {3,5,8,15} observation counts.
- Reset? **No session reset.**
- Selected k for JSON: **15** when primary model is VOLUME_POINT (empty interval → default 15). **PROVEN BY EXECUTABLE CODE** L214.
- `mean_vn = float(np.mean(vn_window))`. **PROVEN BY EXECUTABLE CODE** L122.
- Other JSON keys: `median` (raw), `std` (raw population), `max_median`, `since_max`, `count_elevated`, `count_extreme`, `persist_baseline`. **PROVEN BY EXECUTABLE CODE** L221.
- Serialization: `json.dumps(state, sort_keys=True, separators=(",",":"))`. **PROVEN BY EXECUTABLE CODE** L225.
- 014C parse: `interval_mean_vn = float(interval["mean_vn"])`. **PROVEN BY EXECUTABLE CODE** `test014c_common.py` L58–61.

Regime counts inside the JSON use 009V quantile boundaries (q25=0.63985, q75=1.58333, q95=5.91848). **PROVEN BY FROZEN ARTIFACT** selection JSON L404927–404931. **VolumeEngine does not read those counts.**

`persist_baseline` walks backward while `V_N >= 1` (**PROVEN BY EXECUTABLE CODE** L62–68, L126). That walk is unbounded in the batch implementation. VolumeEngine does not use it.

### 6.3 Color interval (downstream of VolumeEngine)

`EmissionIntervalizer.observe` (`pre08242026_docs/emission_intervals.py` L39–76, HEAD-root equivalent):

- Continues only if same `session_id`, same color, and `elapsed_seconds == 60.0`.
- Else completes prior interval and starts a new one.
- Timestamps must strictly increase (L49–50).
- `duration_minutes = observation_count` (L85) — count, not clock duration.

**PROVEN BY EXECUTABLE CODE.** This is diagnostic/alignment machinery for 014C/015, not an input to `VolumeEngine.observe`.

### 6.4 How VolumeEngine receives interval mean

014C `load_common_rows` inner-joins 010 V emissions to 014B P emissions **by timestamp string**, then injects parsed JSON fields into the row dict. `replay` passes that dict to `engine.observe(row, state)`.

**PROVEN BY EXECUTABLE CODE** `test014c_common.py` L49–91.

If P has no matching timestamp, the V row is dropped. Mismatch of counts raises `RuntimeError` (L72–73). 014C aligned universe is **55,199** observations, not the full 101,205 010 emissions. **PROVEN BY FROZEN ARTIFACT** 014C summary invariants L84.

---

## PART 7 — Mature Volume Engine

### 7.1 Packages

**Committed authority (HEAD `ae0dacb`):**

- `spy_volume_engine/__init__.py` — exports
- `spy_volume_engine/engine.py` — 131 lines; `VolumePolicyConfig`, `VolumePolicyState`, `VolumeEmission`, `VolumeEngine`

**Working-tree packaging (uncommitted):**

- `volume_engine/__init__.py`
- `volume_engine/engine.py` — same observe math; `_build` uses `numerical["symbol"]` (L342) instead of `"SPY"` (HEAD L141)

Scientific observe logic is otherwise identical. **PROVEN BY EXECUTABLE CODE** (side-by-side read).

### 7.2 Constructor contract

```text
VolumeEngine(config: VolumePolicyConfig)
  require state_source ∈ {V_N, INTERVAL_MEAN_V_N}
  require 0 < lower_threshold < upper_threshold
  require confirmation_observations >= 1
```

**PROVEN BY EXECUTABLE CODE** HEAD L53–60.

### 7.3 Observe contract

```text
observe(numerical, state) -> (VolumeEmission, VolumePolicyState)

required numerical keys:
  V_RAW, V_N, V1, V2, predicted_next_V_N, interval_mean_vn, timestamp
  HEAD: symbol ignored (hard-coded SPY)
  working-tree volume_engine: numerical["symbol"] required
```

### 7.4 Persistent state

`VolumePolicyState{color, pending_color, pending_count}` is **external**. Engine is stateless besides `config`. Caller resets on session in 014C `replay` L84–87.

### 7.5 Faithful pseudocode (HEAD)

```text
v_raw, v, v1, v2, projected_v, interval_mean = float fields
if any not finite:
    return INVALID emission + VolumePolicyState(color="INVALID")

activity = v if state_source=="V_N" else interval_mean
if activity >= upper: raw_color=GREEN; reason=ACTIVITY_ABOVE_BASELINE
elif activity <= lower: raw_color=RED;  reason=ACTIVITY_BELOW_BASELINE
else:                    raw_color=AMBER; reason=ACTIVITY_NEAR_BASELINE

phase = ACTIVITY_STATIONARY                         if |v1| <= ε
      | ACTIVITY_INCREASING_ACCELERATING            if v1>0 and v2>ε
      | ACTIVITY_INCREASING_DECELERATING            if v1>0 and v2<=ε
      | ACTIVITY_DECREASING_ACCELERATING            if v1<0 and v2<-ε
      | ACTIVITY_DECREASING_DECELERATING            if v1<0 and v2>=-ε

color = raw_color; pending_color=None; pending_count=0; transition=STABLE
if state.color is not None and raw_color != state.color:
    pending_count = state.pending_count+1 if state.pending_color==raw_color else 1
    if pending_count < confirmation_observations:
        color = AMBER
        pending_color = raw_color
        transition = PENDING_{raw_color}
        reasons += STATE_CONFIRMATION_PENDING
    else:
        transition = CONFIRMED_{raw_color}
        reasons += STATE_CHANGE_CONFIRMED

confidence = HIGH if state_source==INTERVAL_MEAN_V_N else MEDIUM
domain_state = CAUSAL_LOCAL_VOLUME   # constant
return emission(cockpit_color=color, raw_color=raw_color, ...),
       VolumePolicyState(color, pending_color, pending_count)
```

**PROVEN BY EXECUTABLE CODE** HEAD L62–147.

Notes:

- First observation (`state.color is None`) accepts `raw_color` immediately (no pending).
- While pending, **cockpit** is forced AMBER even if raw is GREEN or RED.
- `reason_codes` are de-duplicated via `dict.fromkeys` (HEAD L146).
- Color is **activity band**, not price direction (`color_is_price_direction: false`).

### 7.6 Frozen 014C policy values

From `APTF_TEST_014C_SPY_V_EMISSION_POLICY_V0_1.json` (**PROVEN BY FROZEN ARTIFACT**, sha256 `f719134f…`):

- selected candidate `V_INTERVAL_B10_C2`
- `state_source=INTERVAL_MEAN_V_N`
- thresholds `0.9` / `1.1`
- confirmation `2`
- epsilon `1e-12`
- `price_inputs_used: false`
- `RK45: false`

Development candidates (only one eligible under the selection rule):

| Candidate | Eligible? | Why |
|---|---|---|
| `V_POINT_B10_C1` | No | median interval 1 < 3 |
| `V_POINT_B20_C2` | No | AMBER occupancy 69.8% > 50% |
| `V_INTERVAL_B10_C2` | **Yes** | GREEN 49.9%, RED 9.5%, AMBER 40.6%, median interval 7 |
| `V_INTERVAL_B20_C3` | No | RED occupancy 2.15% < 7.5% |

**PROVEN BY FROZEN ARTIFACT** development scorecard; **PROVEN BY EXECUTABLE CODE** `run_test_014c_v_development.py` L17–23, L56–63.

Selection rule: minimum development changes/session among candidates with GREEN and RED occupancy ≥ 7.5%, AMBER ≤ 50%, median interval ≥ 3. **With only one eligible candidate, the rule did not compare two valid policies.**

### 7.7 Session semantics (caller, not engine)

```text
session_id = timestamp[:10] + ":" + session   # date + 014B session label
if session_id changed: state = VolumePolicyState()
```

**PROVEN BY EXECUTABLE CODE** `test014c_common.replay` L84–87. Engine itself has no calendar.

### 7.8 Invalid / failure branches

| Condition | Behavior |
|---|---|
| Bad config | `ValueError` at init |
| Missing/unconvertible keys | exception (not INVALID emission) |
| Nonfinite required floats | INVALID emission + state color INVALID |
| HEAD missing symbol | none (hard-coded) |
| Working-tree missing symbol | `KeyError` |

**PROVEN BY EXECUTABLE CODE.**

---

## PART 8 — Scientific constants / calibration

| Name | Value | Source | How chosen | Status | Affects categorical output? | Safe as live user config? |
|---|---|---|---|---|---|---|
| baseline window | 15 | 009V | lex rank among {15,30,60} | frozen | Yes (V_N) | No — scientific |
| normalization method | ROLLING_MEDIAN_RATIO | 009V | same rank | frozen | Yes | No |
| derivative window | 3 | 009V | lex rank among {3,5,8,15} | frozen | Yes (phase) | No |
| forecast model | VOLUME_POINT | 010 | lex rank among 10 models | frozen | Only via finite projected_v | No |
| interval k | 15 | 010 default for POINT | default when model has no k | frozen | Yes (activity) | No |
| lower/upper | 0.9 / 1.1 | 014C | occupancy/interval gates on development split | frozen | Yes (color) | No |
| confirmation | 2 | 014C | same | frozen | Yes | No |
| epsilon | 1e-12 | 014C / engine | numerical deadband | frozen | Phase only unless V1≈0 | Engineering, keep fixed |
| ACTIONABLE_START | 15 | 009V L20 | scoring start | frozen | Warm-up / validity | Engineering |
| CONDITION_LIMIT | 1e8 | 010 L20 | rejected models only | historical | No for POINT | X |
| regime q25/q75/q95 | 0.640 / 1.583 / 5.918 | 009V actionable V_N | descriptive 010 JSON | frozen descriptive | **Not** VolumeEngine color | No (entity-specific) |
| lstsq `rcond=None` | NumPy default | 009V/010 | library | frozen numerical | Fit success | Must match P-04 lstsq |
| D01 reference_alpha 0.05, bounds [0,3], `/10.0` | Adaptive | D01 design | **other path** | Yes inside Adaptive | Already QuanTRAM Adaptive config |

Classification:

- **Scientific calibration:** windows, method, thresholds, confirmation, VOLUME_POINT.
- **Engineering:** epsilon, ISO parse, JSON separators, diagnostic paths.
- **Diagnostic:** 009V comparison metrics, 010 MAE, 014C scorecards, charts.

Do not expose frozen bands as arbitrary user knobs. A future config surface, if any, would be a freeze-governed policy identity, not free floats.

---

## PART 9 — State and memory

### 9.1 State table

| Variable | Owner | Initial | Update | Reset | Bound | Scope | Session | Depends on previous obs | Depends on previous emission |
|---|---|---|---|---|---|---|---|---|---|
| last 15 `V_RAW` | feature prep (not engine) | empty | append | **APTF: never** | 15 | entity | no | yes | no |
| last 15 `V_N` | feature prep | empty | append when finite | never in APTF | 15 | entity | no | yes | no |
| last 3 times (minutes) | feature prep | empty | append | never in APTF | 3 | entity | no | yes | no |
| `VolumePolicyState.color` | caller | None | observe | 014C session change | enum | entity | **yes** | no | yes |
| `pending_color` | caller | None | observe | session / confirm | enum | entity | yes | no | yes |
| `pending_count` | caller | 0 | increment/reset | session | ≤ confirmation (2) | entity | yes | no | yes |
| Intervalizer `IntervalState` | 014C caller | None | observe | session/gap/color | 1 record | entity+engine | yes | yes | yes (color) |
| 010 `persist_baseline` walk | 010 batch only | 0 | backward scan | none | **unbounded in batch** | series | no | yes | no |
| D01 `VolumeReference` | Adaptive | 1.0 (QuanTRAM) | EMA | symbol reset | 1 scalar | entity | QuanTRAM worker reset | yes | no |

### 9.2 Minimum bounded state for streaming Go

Per entity:

1. ring of 15 raw volumes  
2. ring of 15 V_N (and 15 elapsed-minute timestamps, or 15 `IntervalStart`s)  
3. `VolumePolicyState` (3 fields)  
4. optional `EmissionIntervalizer` state (5 fields) if color age is in scope  

15 raw + 15 V_N covers V_N, V1/V2 (needs 3), and interval mean (needs 15).

`persist_baseline` can be a counter increment/reset — **equivalent**, not implemented here.

010’s 60-row regression and full-series arrays are **not** required for VOLUME_POINT.

No hidden process-global Volume mutable state in the engine package. 009V/010 runners are batch scripts with local arrays.

### 9.3 What QuanTRAM already stores

P-04 `pricing.history` already appends `obs.Volume` in a bounded deque (**PROVEN BY EXECUTABLE CODE** `internal/pricing/pipeline.go` L54). That volume is **not** V_N and is not a VolumeEngine input. Reuse of the buffer would be a future design choice; it must not alter P-04 science.

---

## PART 10 — Time semantics

| Stage | Timestamp supplied? | Parsed? | Compared? | Used numerically? | Fixed cadence assumed? | Derivative coordinate? | Interval boundary? | Metadata only? | Session key? | Reset trigger? |
|---|---|---|---|---|---|---|---|---|---|---|
| 007 `volume` | yes UTC | no in 007 itself | row order | no | source is 1-min | no | no | value is volume | `session_type` present | no |
| 009V V_N | yes | to minutes for order check | `diff(time)>0` required | order only | no for formula (count window) | no | no | — | session used only in residual-IQR metric | **no** |
| 009V V1/V2 | yes | minutes | no pairwise equal | **yes** τ | no — irregular OK | **yes** | no | — | no | no |
| 010 feature interval | yes | minutes for `h_next` scoring | no | VOLUME_DERIVATIVE only | count window | no | count k | — | no | no |
| 010 VOLUME_POINT | copied | no | no | no | no | no | no | yes | no | no |
| 014C observe | yes string | no inside engine | no | no | no | no | no | payload | via caller prefix | caller session |
| 014C align | yes | equality of strings | **yes** join key | no | implicit 1-min P universe | no | no | — | from P row | — |
| EmissionIntervalizer | yes | datetime | increase; Δt==60.0 | **yes** seconds | **yes, exact 60.0** | no | **yes** | — | `session_id` | session or gap |

### 10.1 Irregular / missing / duplicate / OOO

| Situation | Specified behavior | Evidence class |
|---|---|---|
| Missing source minute | 009V: missing volume → NaN; 009V order check fails if time not strictly increasing | CODE L169–180. A **skipped minute with no row** is just a larger Δt for V1/V2 and a missing observation in the count window. |
| Irregular interval | V1/V2 use actual minutes. V_N window still counts observations, not minutes. Color interval breaks if Δt ≠ 60.0 | CODE |
| Duplicate timestamp | 009V `np.diff(time) <= 0` → RuntimeError. Intervalizer `timestamp <= last` → ValueError | CODE |
| Out-of-order | same as duplicate/non-increasing — rejected in 009V and intervalizer | CODE |
| Session transition | Features continue across session. Policy state cleared. Color interval ends | CODE 009V vs 014C replay vs intervalizer |

**UNKNOWN:** live QuanTRAM `LiveFresh` / gap-fill interaction with a 15-observation Volume window. APTF batch assumed a complete ordered 007 series.

009V **requires** `len(volume)==101221` and strictly increasing times (**PROVEN BY TEST** L179–180). That is corpus-authority, not a general streaming law.

---

## PART 11 — Price / Volume independence

| Question | Answer | Evidence |
|---|---|---|
| Does Volume consume Price output before 014C? | **No** for V_N/V1/V2/interval/VOLUME_POINT. Price columns appear only in 009V multivariate CSV and 010 observer `concurrent_P*` | 009V selection has no price import; 010 L94–100 loads P for events L257–258 only |
| Does Price consume Volume output? | **No** in 014/014B Price engine. `observation.volume` exists on Price `MarketObservation` but PriceEngine does not read it for color (prior forensic). QuanTRAM P-04 stores volume in history but derivatives use close | prior APTF Price forensic; QuanTRAM `derivatives.go` uses prices |
| Are timestamps merely aligned later? | **Yes.** 014C `load_common_rows` inner-joins on timestamp | CODE L51–57 |
| First actual convergence | 014C aligned emissions + joint interval **observation**. Summary: `P_V_fusion: false` | ARTIFACT 014C summary L2 |
| What is compared/combined? | Same-timestamp P `cockpit_color` and V `cockpit_color`; later 015 uses colors + interval ages | 014C validation `joint_state`; 015 interpreter L31–36 |
| Fusion vs interpretation vs alignment | 014C = **alignment + joint observation**. 015 = **semantic interpretation**. No mathematical fusion of V_N into Price ODE | ARTIFACT `P_V_fusion: false`; 015 rules on colors/ages |

014C transition timing explicitly `causality_claim: false`. **PROVEN BY FROZEN ARTIFACT** summary L113–122.

---

## PART 12 — D01 volume influence vs Volume Engine

These paths share **only** the originating market volume observation.

### 12.1 D01 / QuanTRAM Adaptive influence (already live in P-03)

APTF `d01_adaptive_parametric_model/src/d01/v02/volume.py` L8–15  
SADE `sade/d01/v02/volume.py` L8–15  
QuanTRAM `internal/adaptive/volume.go` L5–11

```text
ref' = (1-α)*prev_ref + α*volume
relative = log1p(volume / max(ref', ε))
absolute = log1p(max(volume, 0)) / 10
v* = clamp(relative + absolute, lo, hi)     # default [0, 3]
```

α = 0.05. Output feeds Adaptive effective-mass / coherence, **not** a Volume lamp.

Legacy `aptf_d01.volume.RelativeVolumeEstimator` (window 20, half-life 45s, rolling mean/median options) is an **older D01 Stage-1 module**. It is not 009V and not VolumeEngine. **X — historical.**

### 12.2 Why this is not the VolumeEngine

| | D01 influence | VolumeEngine path |
|---|---|---|
| Input | raw volume scalar | V_RAW, V_N, V1, V2, predicted_next, interval_mean |
| Output | `v*` scalar | GREEN/AMBER/RED + phase + reasons |
| Normalization | EMA reference + log1p | 15-median ratio |
| Derivatives | none | causal quadratic |
| Policy / hysteresis | none | 0.9/1.1 + confirm 2 |
| RK45 | no | no |
| Price | Adaptive loop only | none |
| QuanTRAM today | **Implemented in P-03** | **Absent** |

SADE investigation recorded this distinction and **deferred** Volume Pipeline development — it did not reject the Volume model (`SADE_GO_REFACTORABILITY…` L516–519, L4118).

---

## PART 13 — Validation authority

### 13.1 Strongest frozen corpora

| Purpose | Strongest artifact | SHA256 (freeze inventory) | Rows / notes | Class |
|---|---|---|---|---|
| Normalization selection | `APTF_TEST_009V_VOLUME_SELECTION_V0_1.json` | `4dbc78a1…` | 101,221 `selected_V_N` | validation freeze |
| Normalization comparison | `…NORMALIZATION_COMPARISON_V0_1.csv` | `a074594a…` | 9 candidates | development |
| Raw audit | `…RAW_VOLUME_AUDIT_V0_1.json` | `a69e82c0…` | 101,221 | validation |
| Derivatives | same selection JSON `selected_V1/V2` + window comparison `fa0e1f15…` | arrays length 101,221 | validation |
| Joint row corpus | `…PRICE_VOLUME_OBSERVATIONS_V0_1.csv` | `71432eb6…` | 101,221 + header | validation input to 010 |
| Prediction/interval | `APTF_TEST_010_VOLUME_ENGINE_EMISSIONS_V0_1.csv` | `0d9134f3…` | 101,205 emissions | **primary Go equivalence corpus for features** |
| Model choice | `…VOLUME_ENGINE_SELECTION_V0_1.json` | `94f139db…` | VOLUME_POINT | validation |
| Interval states | `…VOLUME_INTERVAL_STATES_V0_1.csv` | `1de5880c…` | 4× series | development + replay |
| VolumeEngine emission | `APTF_TEST_014C_SPY_V_ENGINE_EMISSIONS_V0_1.csv` | `ecd94653…` | 55,199 + header (aligned) | **primary Go equivalence corpus for policy** |
| Policy | `…SPY_V_EMISSION_POLICY_V0_1.json` | `f719134f…` | V_INTERVAL_B10_C2 | frozen before validation |
| Policy freeze record | `…V_POLICY_FREEZE_V0_1.json` | `029d7bac…` | sha of policy | validation |
| P/V aligned | `…SPY_PV_ALIGNED_EMISSIONS_V0_1.csv` | `0a4053b2…` | 55,199 | alignment / 015 input |
| V intervals | `…SPY_V_INTERVALS_V0_1.csv` | `8bdccaa5…` | 5,713 intervals | optional age corpus |
| 014C summary | `…SUMMARY_V0_1.json` | `100f0b48…` | 139/139 PASS | validation |
| Replay hash | summary `V_replay_sha256=d586e79f…` | — | deterministic claim | validation |

Hash inventories: `APTF_TEST_009V_ARTIFACT_HASHES_V0_1.json`, `APTF_TEST_010_ARTIFACT_HASHES_V0_1.json`, `APTF_TEST_014C_ARTIFACT_HASHES_V0_1.json`.

Determinism: 014C summary `performance.deterministic: true`. Runners write artifacts; they are generated from code + frozen inputs. This investigation did not regenerate them.

**Enough for a Go equivalence corpus?** **Yes, for the mature path**, if 010 emissions (features) and 014C V emissions (policy) are replayed with the same numerical policy. Gaps: no isolated unit corpus for rolling median / window-3 quadratic / interval mean as standalone tables (they are embedded in 009V JSON / 010 CSV). No zero-volume or irregular-gap fixtures.

### 13.2 014C validation occupancy (validation partition)

From summary `V_validation`: 17,312 observations, 39 sessions, GREEN 50.7%, AMBER 40.8%, RED 8.5%, INVALID 0, median interval 7, 1,746 color changes. **PROVEN BY FROZEN ARTIFACT.**

---

## PART 14 — Test inventory

| File | Class | Functions | Assertions / gates | Tolerance | Rows | Authoritative? |
|---|---|---|---|---|---|---|
| `diagnostics/run_test_009v_volume_selection.py` | validation experiment + freeze writer | `causal_quadratic`, `rolling_baseline`, `normalize`, `main` | `len==101221`, strictly increasing time | none (hard fail) | 101,221 | **Yes** for V_N/V1/V2 |
| `diagnostics/run_test_009v_multivariate_analysis.py` | validation / descriptive | `main` | selection frozen; 007/009 alignment | none | 101,221 | Yes as 010 input producer; P×V analytics are X |
| `diagnostics/run_test_010_volume_engine.py` | validation experiment + freeze writer | `state_regime`, `error_metrics`, `main` | 009V candidate still `ROLLING_MEDIAN_RATIO_15`; len 101221 | none | 101,205 emissions | **Yes** for interval + POINT |
| `diagnostics/run_test_010_control_analysis.py` | development / descriptive | control join | concurrence only | — | — | X for Volume science |
| `diagnostics/run_test_011_control.py` | integration / development | observer + RK interface | — | — | — | X |
| `diagnostics/run_test_014c_v_development.py` | development experiment | `candidates`, `main` | occupancy/interval gates | occupancy thresholds | 37,887 development | Yes as policy-selection record; not re-open |
| `diagnostics/run_test_014c_validation.py` | frozen corpus / validation | replay, intervalize, charts | 139 gates; replay sha | none for colors | 17,312 val / 55,199 aligned | **Yes** |
| `diagnostics/test014c_common.py` | integration harness | `load_common_rows`, `replay`, `intervalize`, `score` | P/V timestamp count match | none | — | **Yes** wiring |
| `diagnostics/finalize_test_014c_evidence.py` | evidence packager | hashes | — | — | — | Yes inventory |
| `diagnostics/test_price_volume_generic_refactor.py` | unit test | VolumeEngine observe | pending GREEN → cockpit AMBER | exact | synthetic 1 row | Packaging only; working tree |
| `d01_…/tests/test_volume_math.py` | unit test | D01 relative volume | D01 formulas | exact | synthetic | **D01 path only** |
| 015/016 runners | downstream semantic / paper | interpreter | SPY-only; color domain | — | 014C aligned | **Deferred** |

### 14.1 Gaps a Go migration must close

1. Isolated Golden tests for `ROLLING_MEDIAN_RATIO_15` on a short causal fixture (not only 101k JSON).
2. Isolated Golden tests for window-3 `causal_quadratic` vs `selected_V1/V2` sample rows, including irregular Δt.
3. Isolated Golden tests for 15-mean `interval_mean_vn` vs 010 JSON.
4. VolumeEngine confirmation sequences (first obs, pending, confirmed, session reset, INVALID).
5. Zero volume, missing volume, baseline=0, nonfinite V1 during warm-up.
6. Multi-entity independence (APTF corpus is one entity).
7. Streaming vs batch equivalence on 010 → 014C chain.
8. No pytest currently asserts 009V/010 mathematics independently of the freeze writers.

---

## PART 15 — Go refactorability

| Module / function | Class | Reason | Python deps | Go equivalent already in QuanTRAM | Risk | Required equivalence |
|---|---|---|---|---|---|---|
| `V_RAW` copy from `Bar.Volume` | G1 | integer/float copy | none | `domain.Bar.Volume` uint64 | Low (type width) | bar-level fixtures |
| `rolling_baseline` median/mean | G2 | sliding window stats | numpy median/mean | stdlib / small ring + sort of 15 | Low | 009V selected_V_N |
| `normalize` ratio / log-ratio | G2 | scalar | numpy | stdlib | Low | same |
| Candidate ranking 009V | X | freeze already chosen | scipy spearmanr | — | — | do not re-run selection live |
| `causal_quadratic` w=3 | G3 | lstsq rank/`rcond=None` | numpy lstsq | `pricing.causalQuadraticAtIndex` + `lstsq` (change window/series only) | Medium numerical | 009V V1/V2 sample + 010 rows |
| `raw_V1` | X | reference only | numpy diff | — | — | none |
| 010 interval descriptors used by engine (`mean_vn`) | G2 | mean of 15 | numpy | stdlib | Low | 010 `mean_vn` |
| Other 010 interval fields | X / optional diagnostic | not in observe | numpy | — | — | only if JSON replay required |
| VOLUME_POINT | G1 | assignment | none | assignment | Low | 010 `predicted_next_V_N` |
| VOLUME_DERIVATIVE / interval regression | X | not selected | numpy cond/lstsq | — | — | none |
| `VolumeEngine.observe` | G1/G2 | thresholds + hysteresis; float compare at 0.9/1.1 | math | stdlib | Medium categorical near bands | 014C V emissions |
| `VolumePolicyState` | G1 | 3 fields | dataclasses | struct | Low | session-reset tests |
| `EmissionIntervalizer` | G2 | datetime + exact 60s | stdlib datetime | `time.Time` | Medium (60.0 equality) | 014C V intervals if age in scope |
| 014C timestamp join | G1 | map by string/time | csv | existing bar time | Low | alignment tests |
| 009V multivariate P×V analytics | X | descriptive | scipy pearsonr/spearmanr | — | — | none |
| 015 interpreter | Deferred | not Volume science | — | — | — | later |
| D01 `update_volume_influence` | already G2 in QuanTRAM | already ported | math | `adaptive.updateVolumeInfluence` | n/a | existing Adaptive tests |
| `aptf_d01.volume.*` | X | legacy D01 | custom | — | — | none |
| Charts / matplotlib 014C | X | diagnostic | matplotlib | — | — | none |

**No P1 (retain Python) item** is justified for the production Volume path. NumPy/SciPy use is median, mean, `lstsq`, and selection-time rank correlations. Selection-time SciPy is X.

---

## PART 16 — Numerical library dependencies

| Operation | Library | Where | QuanTRAM equivalent | Add dependency? |
|---|---|---|---|---|
| `np.median` / `np.mean` / `np.std` / `np.quantile` | numpy | 009V, 010, 014C score | stdlib; 15-element sort | No |
| `np.linalg.lstsq(..., rcond=None)` | numpy | 009V quadratic; 010 rejected models | existing `pricing` lstsq / gonum v0.17.0 | No |
| `np.linalg.cond` | numpy | 010 UPDATE models only | gonum `Cond` (already allowed for P-04) | No — X path |
| `scipy.stats.spearmanr` / `pearsonr` | scipy | 009V selection + multivariate | not needed live | No |
| `statistics.median` | stdlib | 009V run lengths | stdlib | No |
| `math.isfinite` / `log` | stdlib | 009V, engine | `math` | No |
| pandas | — | **not used** in Volume diagnostics | — | No |
| JSON number artifacts | stdlib json | interval_state_json, policies | `encoding/json` | No |
| matplotlib | 014C charts | diagnostic | — | No |

gonum is already in QuanTRAM `go.mod` (`gonum.org/v1/gonum v0.17.0`). **Do not add libraries for this investigation.**

---

## PART 17 — Streaming / realtime refactorability

| Question | Answer |
|---|---|
| Can one accepted Bar be processed incrementally? | **Yes**, if 15-raw / 15-V_N rings are kept. 009V/010 batch loops are written as full-history scans but each index uses only a trailing window. |
| Minimum history | 15 raw volumes; 15 V_N + timestamps; 3 for quadratic is subsumed |
| Full-history recompute? | Batch scripts rescan from 0 every run. Live path must **not** copy that. Mathematically unnecessary for the selected methods. |
| Per-entity state? | Yes. APTF happened to run one entity. Engine has no cross-entity memory. |
| Thousands of entities? | Compatible in principle with P-03/P-04 keyed workers. State is tiny. |
| Hidden global mutables? | None in engine. Diagnostics use `ROOT` paths. |
| Reset on symbol reset? | Policy state: yes (mirror 014C session + QuanTRAM `ResetSymbol`). Feature rings: **APTF did not reset on session**; whether QuanTRAM should reset on `ResetSymbol` is a **future design** question (see risks). |
| Persist across observations | rings + VolumePolicyState |
| Bounded? | Yes for the mature path |

Compatibility with P-03/P-04 workers: **conceptually compatible** as another post-accept, per-symbol, prepare/commit-friendly observer. **No P-number assigned.**

---

## PART 18 — Current QuanTRAM compatibility (no modification)

Already available on the accepted eligible bar / worker:

| QuanTRAM surface | Relevance |
|---|---|
| `domain.Bar.Volume uint64` | raw volume analogue of `V_RAW` |
| `IntervalStart` / `SourceTimestamp` | time coordinate |
| `Symbol` | entity key (do not hard-code a name) |
| P-02 quality / infer gate | same eligibility as P-03/P-04 |
| modelhost per-symbol worker + `ResetSymbol` | ownership model |
| P-04 bounded history including `volumes` | **do not reuse for Volume science without a design**; P-04 must stay unchanged |
| `pricing.causalQuadraticAtIndex` | reusable **method** with window=3 on V_N |
| P-03 `updateVolumeInfluence` | **must remain the Adaptive path**; not a substitute |
| prepare/commit | Volume should commit only after the same accepted bar, if added later |
| StageTransition Hub | sideways publish after commit; frozen V1.1 already has `Color`, `DomainState`, `ConfidenceState` fields that could *later* carry Volume — **not authorized now** |
| proto | no VolumeEvent; do not add now |

Additionally required for a future Volume path (design later):

- feature rings and policy state per entity
- frozen policy identity
- session-key definition for policy reset (014C used `date:session` from Price rows)
- decision whether feature windows reset on QuanTRAM discontinuity / `ResetSymbol`
- optional color-age intervalizer
- equivalence harness against 010/014C corpora

Must not change P-03/P-04 science, Adaptive hashes, EXPM, or StageTransition V1.1.

---

## PART 19 — StageTransition compatibility (future design input only)

StageTransition V1.1 is **frozen**. Do not add a Volume stage now. Conceptual notes only:

Candidate **authoritative** Volume StageState dimensions (categorical):

- `Kind` (e.g. future VOLUME — name not assigned)
- `Color` = `cockpit_color` (GREEN/AMBER/RED/INVALID)
- `DomainState` = `CAUSAL_LOCAL_VOLUME` (constant today; low utility)
- `ConfidenceState` = HIGH vs MEDIUM (tied to state_source; constant under frozen policy)
- optional: `transition_state` (STABLE / PENDING_* / CONFIRMED_*) if treated as meaningful
- optional: skip/warmup (V_N or V1 not yet finite)

Candidate **facts** (not in equality): `v_raw`, `v`, `v1`, `v2`, `projected_v`, `activity_state_value`, `phase`, `reason_codes`, timestamps, IDs, `InitiatingBar`.

Meaningful change (analogous to P-04 color): `cockpit_color` and possibly `transition_state`. Phase is derivative-driven and would be dense if included in equality — likely **exclude** phase from equality, same spirit as excluding floats.

`InitiatingBar`: Bar-driven Volume evaluation should carry the same accepted `domain.Bar` as P-03/P-04 on that minute.

Do **not** treat this as a contract. Do **not** add P05.

---

## PART 20 — Migration boundary

Smallest scientifically complete QuanTRAM Volume capability = **upstream features + mature policy**, not the policy shell alone.

### REQUIRED production components

1. `V_RAW` from accepted eligible `Bar.Volume` (entity-keyed, not a named ticker).
2. Causal `ROLLING_MEDIAN_RATIO_15` → `V_N`.
3. Causal quadratic window 3 on `V_N` vs elapsed minutes → `V1`, `V2`.
4. Trailing-15 mean of `V_N` → `interval_mean_vn`.
5. `predicted_next_V_N = V_N` (VOLUME_POINT).
6. `VolumeEngine.observe` with frozen `V_INTERVAL_B10_C2` (0.9/1.1, confirm 2, `INTERVAL_MEAN_V_N`).
7. Per-entity `VolumePolicyState` with an explicit session/reset rule.

### REQUIRED state

15 raw volumes, 15 V_N, 15 timestamps (or 15 + 3), `VolumePolicyState`.

### REQUIRED frozen reference artifacts

- `APTF_TEST_009V_VOLUME_SELECTION_V0_1.json` (or derived row samples)
- `APTF_TEST_010_VOLUME_ENGINE_EMISSIONS_V0_1.csv`
- `APTF_TEST_014C_SPY_V_EMISSION_POLICY_V0_1.json`
- `APTF_TEST_014C_SPY_V_ENGINE_EMISSIONS_V0_1.csv`

### OPTIONAL diagnostics

- `EmissionIntervalizer` / V age
- full `interval_state_json`
- TXT StageTransition (future)
- 014C charts

### HISTORICAL-only

- 9 normalization candidates and Spearman ranking
- 4 derivative-window bake-off
- 10 forecast models including 60-row regression
- 009V multivariate P×V studies
- 010 observer events / lead-lag
- 011 control
- D01 `aptf_d01.volume.*`
- HEAD `symbol="SPY"` hard-code
- matplotlib scorecards

### DEFERRED downstream

- 015 BUY/HOLD/SELL interpreter
- 016 paper
- P/V fusion
- proto VolumeEvent
- process-model process id
- StageTransition Volume stage
- DNA / Quantram_transaction

---

## PART 21 — Open questions / risk register

| ID | Risk | Evidence | Impact | Recommended action |
|---|---|---|---|---|
| R01 | Scientific equivalence of window-3 lstsq vs NumPy `rcond=None` | Same issue P-04 already faced; Volume window is 3 not 15 | Wrong V1/V2 → wrong phase; color usually unaffected | Replay 009V V1/V2 and 014C phase strings |
| R02 | Categorical sensitivity at 0.9 / 1.1 | Activity is a float mean; bands are hard | GREEN/AMBER/RED flips on ulp noise | Bit-exact or tight-tol compare on 014C colors; record near-band rows |
| R03 | Normalization authority looks settled but is SPY-corpus-specific | 009V rank on one 101,221-row entity | Other entities may need different windows if someone re-selects | Treat freeze as authority; do not re-rank live |
| R04 | Feature windows do not reset on session; policy does | 009V `rolling_baseline` vs 014C `replay` | Overnight/PREMARKET raw volume enters REGULAR V_N | Decide explicitly in a later design; do not silently “fix” |
| R05 | Observation-count vs clock time | k=15 bars, not 15 minutes | Gap-fill / missing minutes change the economic window | Document; fixture irregular series before live |
| R06 | Color interval requires Δt == 60.0 | `emission_intervals.py` L60 | QuanTRAM elastic intervals would split/age differently | Keep intervalizer optional; do not assume QuanTRAM elasticity |
| R07 | `predicted_next_V_N` unused for color but required finite | engine L67–72, L81–108 | Warm-up INVALID if projected missing | Implement VOLUME_POINT assignment; test INVALID path |
| R08 | 014C aligned subset ≠ full 010 series | 101,205 vs 55,199 | Equivalence on 014C only proves P-overlapped minutes | Use 010 CSV for feature tests, 014C for policy |
| R09 | Zero/missing volume untested | raw audit zero_count=0, missing=0 | Live IEX can have 0 | Add fixtures; do not invent APTF behavior |
| R10 | Calibration provenance is one development split | only `V_INTERVAL_B10_C2` eligible | Policy is thin-sliced, not a robust multi-candidate winner | Record as freeze fact; do not retune |
| R11 | HEAD SPY hard-code vs generic refactor | HEAD L141 vs working-tree L342; 014C rows lack `symbol` | Working-tree engine can KeyError on 014C replay | Treat HEAD+artifacts as authority; generic symbol is packaging |
| R12 | Artifact path relocation | diagnostics `ROOT / APTF_TEST_*` vs `pre08242026_docs/` | Replay from working tree may fail | Use hashes; restore paths only if re-running APTF |
| R13 | Coupling to one historical entity | 007 `entity_id=SPY`; artifact names `*_SPY_*` | Accidental hard-code in a future port | Forbid named-entity constants in QuanTRAM Volume design |
| R14 | `Bar.Volume` uint64 vs APTF float | domain bar vs CSV `10367.0` | Overflow theoretically; shares are integer | Cast with tests |
| R15 | D01 vs VolumeEngine confusion | both say “volume” | Implementing the wrong math | Keep this document as the distinction authority |
| R16 | Stale docs vs code | lineage md still says `spy_volume_engine` | Wrong import in future work | Executable wins; note working-tree rename |
| R17 | Unbounded `persist_baseline` if someone ports 010 JSON wholesale | 010 L62–68 | Memory on long sessions | Do not port unless needed; use a counter |
| R18 | Concurrency | engine is pure; state is caller-owned | Safe if per-entity serialized like P-03/P-04 | Follow worker model |
| R19 | Performance | 014C median interpreter 3.5 µs; 15-median + 3×3 lstsq | Negligible vs P-04 F4/EXPM | Measure in a later increment |
| R20 | Test coverage gap | no isolated Golden unit tests for features | Silent drift | Close Part 14 gaps before claiming Go equivalence |
| R21 | LiveFresh / discontinuity | QuanTRAM P-02 can drop infer; APTF batch had a complete file | Window contamination after gaps | Future design; UNKNOWN in APTF |
| R22 | Working-tree APTF dirtiness | many deletes/moves | Wrong file chosen as authority | Cite HEAD hash + freeze sha256 |

---

## Complete file inventory (APTF Volume-relevant)

### Production / reusable

- `spy_volume_engine/engine.py`, `spy_volume_engine/__init__.py` — HEAD `ae0dacb` (deleted in working tree)
- `volume_engine/engine.py`, `volume_engine/__init__.py` — working tree uncommitted generic refactor
- `emission_intervals.py` — HEAD root; working tree `pre08242026_docs/emission_intervals.py`
- D01 v02 `d01_adaptive_parametric_model/src/d01/v02/volume.py` — Adaptive influence
- SADE `sade/d01/v02/volume.py` — same influence (context)
- QuanTRAM `internal/adaptive/volume.go` — already-ported influence
- Legacy `d01_adaptive_parametric_model/src/aptf_d01/volume/{relative_volume,volume_density,volume_direction,volume_movement,volume_decay}.py`
- `d01_adaptive_parametric_model/src/aptf_d01/models/volume_state.py`

### Diagnostics / runners

- `diagnostics/run_test_009v_volume_selection.py`
- `diagnostics/run_test_009v_multivariate_analysis.py`
- `diagnostics/run_test_010_volume_engine.py`
- `diagnostics/run_test_010_control_analysis.py`
- `diagnostics/run_test_011_control.py`
- `diagnostics/test014c_common.py`
- `diagnostics/run_test_014c_v_development.py`
- `diagnostics/run_test_014c_validation.py`
- `diagnostics/finalize_test_014c_evidence.py`
- `diagnostics/test_price_volume_generic_refactor.py`

### Frozen artifacts (working tree: `pre08242026_docs/`; HEAD: repo root)

009V: plan, source/test009 authority, raw audit, normalization method+comparison, derivative method+comparison, selection JSON, observations CSV, crossing features, joint frequencies, turning trajectories, episode alignment, time-to-crossing, relationships JSON, trajectory separation, 010 recommendation, hashes, summary, result, acceptance gates, multivariate method, pretest hashes, runtime immutability.

010: volume interval method+states, model comparison, engine emissions, observer events, selection JSON, lead-lag, control observations, session boundary, hashes, summary, plus Price-side 010 files in the same family.

011: `APTF_TEST_011_VOLUME_OBSERVER_STATES_V0_1.csv`, `APTF_TEST_011_VOLUME_CONDITION_METRICS_V0_1.csv`.

014C: V policy, V emissions, V intervals, PV aligned, PV joint intervals, engine intervals, P intervals, transition relationships, development/validation scorecards, authority, policy freeze, acceptance gates, summary, artifact hashes, streaming performance, charts under `output/test014c_charts/`.

### Documents

- `pre08242026_docs/APTF_SPY_VOLUME_ENGINE_LINEAGE_V0_1.md`
- `post08242026_docs/investigations/APTF_PRICE_VOLUME_ENGINE_EXECUTABLE_FORENSIC_ANALYSIS_2026-08-25.md`
- `post08242026_docs/investigations/APTF_TEST_007_TO_009V_EXECUTABLE_FORENSIC_ANALYSIS_2026-08-25.md`
- `post08242026_docs/investigations/APTF_TEST_010_TO_016_EXECUTABLE_FORENSIC_ANALYSIS_2026-08-25.md`
- `post08242026_docs/implementations/APTF_PRICE_VOLUME_ENGINE_GENERIC_REFACTOR_2026-08-25.md`
- D01 `docs/D01_VOLUME_MODEL_V0_1.md` (Adaptive volume model, not VolumeEngine)

### Source data

- `APTF_TEST_007_OBSERVATION_EPISODE_MAP_V0_1.csv` (working tree `pre08242026_docs/`)

---

## Executable call graph

```text
007 CSV.volume
   → run_test_009v_volume_selection.main
        → rolling_baseline / normalize → selected_V_N
        → causal_quadratic(window=3) → selected_V1, selected_V2
        → APTF_TEST_009V_VOLUME_SELECTION_V0_1.json
   → run_test_009v_multivariate_analysis.main
        → writes PRICE_VOLUME_OBSERVATIONS (V_RAW, V_N, V1, V2, price columns)
   → run_test_010_volume_engine.main
        → interval_data[k].mean_vn
        → predictions[VOLUME_POINT] = V_N
        → APTF_TEST_010_VOLUME_ENGINE_EMISSIONS_V0_1.csv
   → test014c_common.load_common_rows
        → join 014B P emissions by timestamp
        → interval_mean_vn from JSON
   → test014c_common.replay
        → VolumeEngine.observe
        → VolumeEmission + VolumePolicyState
   → test014c_common.intervalize
        → EmissionIntervalizer.observe(..., "SPY", ...)
   → run_test_014c_validation
        → PV aligned CSV
   → run_test_015_validation / ExecutionInterpreter
        → BUY/HOLD/SELL from P_color, V_color, ages

D01 update_volume_influence(volume) ──×── no import from the above
```

014C development/validation currently `from volume_engine import …` in the working tree (`test014c_common.py` L15). HEAD used `spy_volume_engine`. **DOCUMENTED** working-tree drift.

---

## Known limitations

- This investigation did not re-run 009V/010/014C; hashes were not recomputed.
- APTF working tree is not clean; authority is HEAD `ae0dacb` plus freeze sha256 inventories.
- No live QuanTRAM Volume behavior exists to compare.
- 014C `intervalize` still passes literal `"SPY"` (`test014c_common.py` L105) even after the generic engine rename. That is diagnostic packaging, not Volume mathematics.
- 009V `VOLUME_SELECTION` JSON embeds full 101,221-length arrays; this investigation sampled structure and the documented primary keys, not every element.
- QuanTRAM `docs/design/QuanTRAM_PROCESS_MODEL_082926.md` numbers P-05 as OMS/Risk. That existing number is **not** a Volume process id and was not changed.

---

## Unanswered questions

1. Should a future QuanTRAM Volume feature window reset on `ResetSymbol`, session change, or proven gap? APTF features do not reset; APTF policy does. **UNKNOWN for QuanTRAM.**
2. How should missing IEX minutes interact with a 15-observation median? APTF never inserted placeholder rows.
3. Are 0.9/1.1 bands scientifically portable across entities, or only a freeze for the 007 corpus? Freeze says portable policy identity; corpus is one entity. **INFERRED — REQUIRES VALIDATION.**
4. Is color-age (`EmissionIntervalizer`) in the first QuanTRAM Volume increment or deferred with 015? Not decided.
5. Should `transition_state` participate in a future StageTransition equality? Not decided; V1.1 must not be edited now.
6. Does anyone still need 010 `interval_state_json` extras (`persist_baseline`, elevated counts) in production? VolumeEngine does not consume them.
7. Working-tree `volume_engine` vs HEAD `spy_volume_engine`: which package name would a later implementation start from? Scientifically equivalent observe math; symbol field differs.

---

## Final findings

1. The complete validated Volume path is **009V (normalize + derivatives) → 010 (interval mean + VOLUME_POINT) → 014C (VolumeEngine policy) → optional intervalizer → 014C P/V alignment**. Importing only `VolumeEngine` is scientifically incomplete.

2. Mature inputs are prepared **before** the engine: `V_RAW`, `V_N=ROLLING_MEDIAN_RATIO_15`, `V1/V2` causal quadratic window 3, `predicted_next_V_N=V_N`, `interval_mean_vn=mean` of 15 `V_N`.

3. `VolumeEngine` is a Price-independent activity-band interpreter with two-observation confirmation. It does not normalize, differentiate, integrate, call PriceEngine, or emit BUY/HOLD/SELL.

4. First downstream P/V convergence is **timestamp alignment and joint observation** in 014C (`P_V_fusion: false`). First semantic combination is 015 (deferred).

5. D01/QuanTRAM `updateVolumeInfluence` is a **separate Adaptive path** already live in P-03. SADE deferred the standalone Volume Pipeline; that is not a rejection of 009V–014C.

6. Strongest freeze authority: 009V selection JSON + 010 Volume emissions + 014C V policy and V emissions (hashes in Part 13).

7. The complete production path appears **Go-refactorable** with gonum/stdlib already present. No Volume step should remain Python for scientific reasons.

8. Highest migration risks: session/window reset mismatch, observation-count vs irregular time, categorical float bands, one-entity calibration, and warm-up INVALID when V1/projected are nonfinite.

9. Recommended first increment = Part 20 REQUIRED set only. No process-model number, proto, or StageTransition change until separately authorized.

---

## Change log

| Date | Change |
|---|---|
| 2026-09-05 | Initial forensic investigation document created. Read-only. No APTF, SADE, QuanTRAM production, proto, process-model, or StageTransition modification. |