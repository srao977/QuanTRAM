# QuanTRAM P-04V Volume Frozen APTF Equivalence Validation

**Title:** QuanTRAM P-04V Volume Frozen APTF Equivalence Validation  
**Date:** 2026-09-05  
**Status:** PHASE F FAIL PRESERVED — PHASE F-R REVALIDATION PASS — HUMAN REVIEW BEFORE PHASE G  
**Purpose:** Determine whether the Go P-04V implementation from Phases A–E reproduces the frozen APTF Volume scientific transformation and interpretation on the authoritative historical validation corpus.  
**Scope:** Hash-verified frozen 009V / 010 / 014C artifacts; Level-1 checked-in fixtures; Level-2 full-corpus Engine replay and session-sliced interpretation. No modelhost wiring. No science change. No Phase G.  
**Documentation path:** `docs/investigations/` (established post-2026-08-24 investigation location).

## Executive Summary

Go P-04V **reproduces frozen APTF Volume Feature and Derivative mathematics** on the full 010 corpus (101,205 aligned emissions) through `volume.Engine`. `V_RAW`, `V_N`, and `predicted_next_V_N` match exactly. `interval_mean_vn` and `V1`/`V2` stay inside tight double-precision tolerance with **zero values outside tolerance** and **zero phase flips** at ε = 1e-12.

Go P-04V **does not reproduce frozen APTF cockpit_color / transition_state**. Production `confirmColor` keeps the last confirmed color in `InterpretationState.Color` while pending (approved QuanTRAM Phase D design). Frozen HEAD `spy_volume_engine.observe` writes the **emitted cockpit color** into `VolumePolicyState.color` (AMBER while pending). That single bookkeeping difference is sufficient to diverge later Indicator values.

Evidence that this is the only interpretation defect:

- `raw_color`, `phase`, `confidence`, `domain_state`: **55,199 / 55,199** exact on session-sliced 014C.
- A test-only APTF state-write oracle restores **55,199 / 55,199** `cockpit_color` and `transition_state`.
- Production mathematics were **not** changed to force a PASS.

**Overall equivalence verdict after initial Phase F: FAIL.**

This was not a clean scientific PASS. It was also not an alignment, warm-up, or session-harness failure. Feature/derivative equivalence stood. Indicator equivalence failed until humans authorized changing QuanTRAM confirmation state-write to the frozen APTF rule.

**Phase F-R (authorized 2026-09-05):** production `confirmColor` now writes `next.Color` = emitted Indicator. Session-sliced 014C revalidation: **55,199 / 55,199** on raw_color, Indicator, transition_state, phase, confidence, and domain_state. Feature/derivative numbers unchanged. Continuous Engine vs 014C still differs at session cuts (41 Indicator mismatches) and was not repaired.

**Revalidation verdict: PASS** (session-sliced scientific equivalence).

Phase G / live delivery was not started.

## Module / System Overview

```text
frozen source V_RAW + timestamp
        ↓
volume.Engine.PrepareStep / Commit     (production composition A–E)
        ↓
V_N, V1, V2, interval_mean_vn, predicted_next_V_N
        ↓
Interpret(interval_mean_vn, V1, V2, prior)
        ↓
raw_color, Indicator (historical cockpit_color), phase, confidence, domain
```

014C historical replay additionally reset **interpretation state only** at `date:session`. Production `Engine` does **not** do that. The harness isolates that reset; `engine.go` was not changed.

## Inputs

| Item | Value |
|---|---|
| QuanTRAM branch | `main` |
| QuanTRAM HEAD | `07417d9c85799949cd3b173067795905513af173` |
| APTF commit inspected | `ae0dacb2e02c5b80c82f6662d1a3c6863f4b989a` |
| APTF working tree | Dirty: freeze files relocated to `pre08242026_docs/`; HEAD still `ae0dacb`. Read-only. |
| Design | `docs/design/QuanTRAM_VOLUME_ENGINE_090526.md` |
| Implementation | `docs/implementations/QuanTRAM_P04V_VOLUME_ENGINE_090526.md` |
| Forensic parent | `docs/investigations/QuanTRAM_APTF_VOLUME_ENGINE_GO_REFACTORABILITY_INVESTIGATION_2026-09-05.md` |
| Frozen executable observe | `git show ae0dacb:spy_volume_engine/engine.py` |
| Frozen 014C harness reset | `diagnostics/test014c_common.py` `replay` (`date:session` → `VolumePolicyState()`) |

## Outputs

- This validation document
- Level-1 fixtures under `internal/volume/testdata/`
- Test/diagnostic code only: `equivalence_test.go`, `frozen_full_test.go`
- Changelog-only update to the P-04V implementation document

No production Volume mathematics, Engine lifecycle, modelhost, ingestion, P-03, P-04, proto, StageTransition, or Process Model change.

## Parameters / Configuration

Frozen constants compared to vendored 014C policy `V_INTERVAL_B10_C2` (hash-identical copy):

| Parameter | Frozen / Go |
|---|---|
| Normalization | `ROLLING_MEDIAN_RATIO_15` / window 15 |
| Derivative window | 3 positional `V_N`, elapsed minutes |
| Interval mean | 15 positional `V_N` (no nanmean) |
| Projection | `VOLUME_POINT` = `predicted_next_V_N = V_N` |
| State source | `INTERVAL_MEAN_V_N` |
| Thresholds | 0.9 inclusive RED, 1.1 inclusive GREEN |
| Confirmation | 2 |
| Epsilon | 1e-12 (phase only) |
| Price inputs | false |

## Assumptions

- Hash authority overrides path convenience. Artifacts were used only after full SHA-256 match.
- 010 is the feature-emission authority. 009V selection JSON was hash-verified as provenance, not separately replayed; 010 `V_N`/`V1`/`V2` already embed that selection.
- 014C is a Price-timestamp inner join of 010, not a prefix. Alignment is by timestamp, not dataframe index.
- `MarketSnapshotID` is synthetic in this harness. The frozen corpus predates that field. Equivalence target is Volume science, not snapshot identity.
- All source `V_RAW` values in this corpus convert exactly through `uint64` (`float64(uint64(v)) == v`). Replay stops if that fails.
- Historical `cockpit_color` is terminology-identical to QuanTRAM `VolumeEvent.Indicator`. Frozen CSV columns were not renamed.

## Exclusions

- Live runtime delivery, modelhost join, proto, StageTransition, Process Model alignment
- Price/Volume fusion, BUY/SELL/HOLD, profitability, generalization across instruments
- Global cockpit→Indicator migration
- Changing confirmation mathematics without human authorization
- Automatic production session/date reset

## Repository / commit identities

**Before Phase F (verified):**

| Item | Value |
|---|---|
| QuanTRAM branch | `main` |
| QuanTRAM HEAD | `07417d9c85799949cd3b173067795905513af173` |
| Pre-existing dirty (preserved, not cleaned) | `docs/design/QuanTRAM_PROCESS_MODEL_082926.md`; `internal/semantics/loader_test.go`; `internal/semantics/tooling/tooling_test.go`; `internal/server/semantics_test.go` |
| Pre-existing untracked P-04V work | Volume design/implementation/investigation docs; `internal/domain/volume.go`; `internal/volume/` Phases A–E; `tools/__pycache__/` |

**After Phase F:** HEAD unchanged. No commit. Added Phase F tests, testdata, and this document. Process Model and semantics tests remain dirty and untouched.

**APTF:** commit `ae0dacb2e02c5b80c82f6662d1a3c6863f4b989a`. Working tree dirty; authoritative bytes live at `C:\Users\chino\APTF\pre08242026_docs\`. APTF was not modified, checked out, cleaned, or regenerated.

## Frozen artifact inventory

| Artifact | Path | Bytes | Records | SHA-256 | Hash |
|---|---|---|---|---|---|
| 009V selection | `C:\Users\chino\APTF\pre08242026_docs\APTF_TEST_009V_VOLUME_SELECTION_V0_1.json` | 9,718,988 | 101,221 | `4dbc78a1e213715577a9b68eca8cae186137fcaa2959afeb9d6bac308f430dd9` | PASS |
| 009V observations (source) | `C:\Users\chino\APTF\pre08242026_docs\APTF_TEST_009V_PRICE_VOLUME_OBSERVATIONS_V0_1.csv` | 28,247,146 | 101,221 | `71432eb6e19aa65df5a8fadbef120c519f89e88a16dbbb2f6d6a6f9237122fa9` | PASS |
| 010 Volume emissions | `C:\Users\chino\APTF\pre08242026_docs\APTF_TEST_010_VOLUME_ENGINE_EMISSIONS_V0_1.csv` | 42,812,387 | 101,205 | `0d9134f3a1996d83dd43257264ddd6a43b5e02215b61c94ec034c3e1ee152d3c` | PASS |
| 014C policy | `C:\Users\chino\APTF\pre08242026_docs\APTF_TEST_014C_SPY_V_EMISSION_POLICY_V0_1.json` | 1,485 | 1 | `f719134f241b00888099e237c02f237a2db4b59f02b25ea5498c51006991bcd8` | PASS |
| 014C V emissions | `C:\Users\chino\APTF\pre08242026_docs\APTF_TEST_014C_SPY_V_ENGINE_EMISSIONS_V0_1.csv` | 16,384,270 | 55,199 | `ecd946532e32a8c5167aab72e8c56d3d3389ab00705a75d4cb91cf3031fd451e` | PASS |
| 014C summary | `C:\Users\chino\APTF\pre08242026_docs\APTF_TEST_014C_SUMMARY_V0_1.json` | 4,160 | 1 | `100f0b4807831f6eebd2e44fe8ab7b2c9597113916243b635b75d819fe80044b` | PASS |

014C summary reports historical APTF self-validation `139/139 PASS` and `INVALID_count=0`. That is not the Go equivalence result.

## Fixture inventory

Preferred location: `internal/volume/testdata/`. Production `engine.go` does not read these files (`TestF08`).

| Vendored file | Kind | Source SHA-256 | Fixture SHA-256 | Selection |
|---|---|---|---|---|
| `APTF_TEST_014C_SPY_V_EMISSION_POLICY_V0_1.json` | exact full copy | `f719134f…991bcd8` | same | entire 1,485-byte policy |
| `level1_source_prefix.csv` | deterministic subset | `71432eb6…7122fa9` | `a041cd823e3371c76c3be9d21f039f60a883a796f0790c90faa7259ed89b14a3` | first 80 source rows; columns `source_observation_index,timestamp,V_RAW` |
| `level1_010_prefix.csv` | deterministic subset | `0d9134f3…e152d3c` | `5537bd3923ab40305d469fa9b3f2d0f8e629dc998ff08857f05f76f488f60319` | 010 rows with `observation_index<=80` |
| `level1_014c_first_session.csv` | deterministic subset | `ecd94653…fd451e` | `7cd38ab82d3116dd1a6c73e248c2cd1a294feae23c56fafbb7f1afc930c29e58` | first `date:session` = `2023-03-30:PREMARKET` (82 rows) |
| `provenance.json` | metadata | n/a | `457c678768898f7307bb15b9531d8b214f5fe61da4d2c6a6f6fda3a3a229b28f` | documents the above |

Level-1 source prefix is sufficient for feature/derivative maturation (first `V_N` at obs 15, first `V1` at 17, first interval mean at 29, plus the 08:14→08:16 gap). It does **not** reach 014C (first 014C timestamp 11:07). Level-1 interpretation therefore feeds 014C feature columns into `Interpret` with a harness-only session-fresh prior.

Full 86MB+ corpus is not vendored. Level 2 discovers `QUANTRAM_P04V_FROZEN_DIR` or sibling `../APTF/pre08242026_docs` and re-verifies hashes before use.

## Replay methodology

### Level 1 — `go test ./internal/volume/`

Always-on. Hash-guards fixtures. Replays 80 source bars through `Engine`. Compares to 010 prefix. Session-slices first 014C session through `Interpret`.

### Level 2 — full corpus

Command:

```text
go test ./internal/volume/ -count=1 -timeout 180s -run "TestFFull|TestF0"
```

or, if the APTF tree is not beside QuanTRAM:

```text
set QUANTRAM_P04V_FROZEN_DIR=C:\Users\chino\APTF\pre08242026_docs
go test ./internal/volume/ -count=1 -timeout 180s -run TestFFull
```

For each source row: construct `domain.Bar` → `PrepareStep` → inspect `VolumeEvent` → `Commit` if committable. Entity is test-only `VOL1`. `MarketSnapshotID` is `frozen-synthetic-{index}`.

014C interpretation: for each `date:session`, `InterpretationState = {}` then `Interpret`. Production `Engine.Reset()` is **not** used at session boundaries because it would also clear Feature windows (historical APTF did not reset features).

A second diagnostic path (`aptfConfirm`) exists **in tests only** to prove 014C cockpit identity if and only if next `state.color` is the emitted cockpit color.

## Row alignment

010 emission loop (frozen diagnostic, not copied into production):

```text
for index in range(15, len(rows)-1):
    emit if selected[index] and vn[index+1] are finite
    observation_index = index+1   # 1-based
```

101,221 − 15 warmup − 1 trailing row = **101,205**. Compare Engine event at 1-based source index `N` to 010 `observation_index=N`, and require equal timestamps.

Forensic readiness on this corpus (derived from state, not magic counters):

| Quantity | First due (1-based) | Evidence |
|---|---|---|
| `V_N` | 15 | Engine obs 15 available; 010 starts at 16 |
| `V1`/`V2` | 17 | obs 16 undefined (only two finite `V_N`); obs 17 available |
| `interval_mean_vn` | 29 | 010 `mean_vn` first finite at observation 29 |

010 does not emit observation 15, so first finite `V_N` is proven by Engine readiness plus exact `V_N` match from observation 16 onward.

014C rows align by `timestamp` to the continuous source/Engine replay. Missing timestamps: **0 / 55,199**.

Irregular time is preserved. Example: 1-based 15 = `2023-03-30T08:14:00Z`, 16 = `2023-03-30T08:16:00Z` (skipped 08:15). Existing Phase C synthetic irregular-time tests remain as protection against false 1-minute inference.

## Source / input columns

Volume science reconstructed from:

| Column | Artifact | Use |
|---|---|---|
| `source_observation_index` | 009V observations | 1-based alignment |
| `timestamp` | 009V observations | `IntervalStart`; V1/V2 τ |
| `V_RAW` | 009V observations | `Bar.Volume` |

014C also supplies `session` for harness-only `date:session` interpretation reset, plus expected feature/categorical columns.

No Price column is consumed by Go Volume science.

## Session / reset treatment

**Historical APTF 014C:** `test014c_common.replay` resets `VolumePolicyState()` when `timestamp[:10] + ':' + session` changes. Feature windows do not reset.

**Production P-04V:** `Engine.Reset()` clears Feature **and** Interpretation. There is no automatic date/session/provider reset. Confirmed by `TestF07` and by reading `engine.go`.

**Harness:** interpretation-only reset at `date:session`. 014C emission file has **356** distinct `date:session` keys.

Continuous Engine `Indicator` vs 014C `cockpit_color` mismatches = **340**. Session-sliced production `Interpret` mismatches = **309**. The extra ~31 are category **E** (continuous interpretation across historical session cuts) on top of the confirmation bookkeeping difference.

## Tolerance methodology

| Field | Tolerance | Justification |
|---|---|---|
| `V_RAW` | exact (0) | integer source through `uint64` |
| `V_N`, `predicted_next_V_N` | 1e-12 abs | same ratio of integer median |
| `interval_mean_vn` | 1e-12 abs | mean of those ratios |
| `V1`, `V2` | 1e-9 abs | NumPy `lstsq` vs gonum thin SVD; measured max abs ≪ 1e-9 |
| Categorical | exact | no softening |

`max_rel` is reported but **not** used as the pass gate. Near-zero frozen derivatives inflate relative error while absolute error remains ~1e-12. Pass/fail is absolute tolerance plus exact categoricals.

## Results by field (full 010 Engine replay)

| Field / State | Compared | Match / within tol | Mismatch | max_abs | max_rel | mean_abs | outside_tol |
|---|---|---|---|---|---|---|---|
| `V_RAW` | 101,205 | 101,205 exact | 0 | 0 | 0 | 0 | 0 |
| `V_N` | 101,205 | 101,205 exact | 0 | 0 | 0 | 0 | 0 |
| `V1` | 101,205 | 101,205 within 1e-9 (14,704 exact) | 0 | 7.8426e-12 | 1.0 (near-zero denom) | 1.6921e-15 | 0 |
| `V2` | 101,205 | 101,205 within 1e-9 (7,308 exact) | 0 | 2.7285e-12 | 7.74 (near-zero denom) | 1.7185e-15 | 0 |
| `interval_mean_vn` | 101,205 | 101,205 within 1e-12 (82,238 exact) | 0 | 5.6843e-14 | 6.15e-16 | 7.9371e-17 | 0 |
| `predicted_next_V_N` | 101,205 | 101,205 exact | 0 | 0 | 0 | 0 | 0 |

Largest `|ΔV1|` at `2023-09-05T08:00:00Z`: frozen `8.4556327782932765` vs Go `8.4556327782854339`.  
Largest `|ΔV2|` at `2023-08-28T21:24:00Z`: frozen `-5206.1688008014062` vs Go `-5206.1688008014089`.

014C-aligned Engine feature compare (55,199 timestamps): same conclusion; all `outside_tol=0`; missing timestamps=0.

Level-1 prefix (65 010 rows): same pattern; `V1`/`V2` max abs ~1e-14.

## Categorical results (014C session-sliced)

| Field / State | Compared | Production `Interpret` | APTF-oracle (test-only) |
|---|---|---|---|
| `raw_color` | 55,199 | **55,199 / 55,199** | 55,199 / 55,199 |
| `phase` | 55,199 | **55,199 / 55,199** | 55,199 / 55,199 |
| `confidence` | 55,199 | **55,199 / 55,199** | 55,199 / 55,199 |
| `domain_state` | 55,199 | **55,199 / 55,199** | 55,199 / 55,199 |
| `Indicator` / `cockpit_color` | 55,199 | **54,890 / 55,199** (309 mismatch) | **55,199 / 55,199** |
| `transition_state` | 55,199 | **52,393 / 55,199** (2,806 mismatch) | **55,199 / 55,199** |

Level-1 first session (82 rows): raw/phase/confidence/domain 82/82; production Indicator 3 mismatches; APTF-oracle 82/82.

## Threshold-sensitivity audit

Inclusive 0.9 / 1.1 raw-color rules match frozen raw_color on every 014C row, including neighborhood rows (`|activity−0.9| < 0.02` or `|activity−1.1| < 0.02`). Examples:

| Timestamp | activity | frozen raw | Go raw | frozen cockpit | Go Indicator |
|---|---|---|---|---|---|
| 2023-03-30T11:16:00Z | 1.1057924296925286 | GREEN | GREEN | AMBER | GREEN |
| 2023-03-30T11:20:00Z | 0.9028070138573532 | AMBER | AMBER | AMBER | AMBER |
| 2023-03-30T14:22:00Z | 1.1172259868457308 | GREEN | GREEN | AMBER | GREEN |
| 2023-03-30T14:58:00Z | 0.8864476371052057 | RED | RED | AMBER | AMBER |

The 11:16 and 14:22 Indicator disagreements are **not** threshold flips. Raw color agrees. The lamp differs because confirmation state already diverged.

Engine-vs-frozen derivative phase audit: **127** rows with `|V1|` or `|V2|` ≤ 1e-9; **0 phase flips**.

No float error in this corpus flips a 0.9 / 1.1 raw color.

## Warm-up / readiness

PASS. Semantic alignment by 1-based observation index + timestamp. First 010 row is observation 16 (`08:16`, `V1=nan`). Engine reports `V1` unavailable there and available at observation 17. Interval mean becomes available at observation 29, matching 010 `mean_vn`.

## Engine-level replay

PASS for features. Full source (101,221 bars) through Phase E `Engine`. `ENGINE_ERROR` count = 0. Scientific `INVALID` count = 0. Feature compare matches helper-level 010 results.

Engine `Indicator` vs 014C `cockpit_color` is **not** a feature-replay failure. It mixes (1) confirmation bookkeeping and (2) absence of production session reset.

## Scientific-invalid and zero-volume rows

| Question | Result |
|---|---|
| Frozen scientific-INVALID positions | none (`INVALID_count=0`; Engine INVALID=0) |
| Zero `V_RAW` in 009V/010 source | **0** |
| All-zero median denominator | not present |
| Rank-deficient V1/V2 | not represented in frozen finite 010/014C rows; remains covered by Phase C unit tests |

## Discrepancy table

| ID | Where | Expected | Actual | Class | Notes |
|---|---|---|---|---|---|
| D1 | 014C `2023-03-30T11:16:00Z` (first) | `cockpit_color=AMBER` `PENDING_GREEN` | `Indicator=GREEN` `STABLE` | **F** confirmation-state bookkeeping (also design-vs-frozen-Python) | activity=1.10579 raw GREEN. After 11:15 GREEN→AMBER pending, APTF stored `state.color=AMBER`; QuanTRAM kept confirmed GREEN, so return-to-GREEN is STABLE. Window: 014C rows at 11:15–11:16. Artifact `ecd94653…fd451e`. Function: `confirmColor` vs HEAD `VolumeEngine.observe`. **Science not changed.** |
| D2 | 014C full corpus | 55,199 Indicator | 309 mismatches | **F** (same root as D1) | 2,806 transition mismatches; same cause |
| D3 | Engine `Indicator` vs 014C without session reset | 014C cockpit | 340 mismatches | **E** + **F** | 309 from D2 plus ~31 session-boundary extras. Production must not gain automatic session reset. |
| D4 | `max_rel` on V1/V2 | n/a | up to 1.0 / 7.74 | **C** (not a fail) | Absolute errors ≤ 7.8e-12; relative inflation near zero. No categorical consequence. |

No A/B/G discrepancies on the hash-verified corpus. No hidden row omission.

## Feature / derivative / interpretation coverage

| ID | Claim | Result |
|---|---|---|
| F01 | raw `V_RAW` mapping | PASS exact |
| F02 | 15-position rolling median | PASS (implied by exact `V_N`) |
| F03 | `V_N` | PASS exact |
| F04 | no finite-value harvesting | PASS (positional NaN; 010 `V1=nan` at obs 16) |
| F05 | `interval_mean_vn` | PASS within 1e-12 |
| F06 | `predicted_next_V_N = V_N` | PASS exact |
| F07 | maturation alignment | PASS |
| F08 | zero-volume | **not in corpus** |
| F09 | last-three positional `V_N` | PASS |
| F10 | actual elapsed-time τ | PASS (irregular 08:14→08:16 preserved; Phase C irregular tests remain) |
| F11 | `V1` | PASS within 1e-9 (measured max 7.8e-12) |
| F12 | `V2` | PASS within 1e-9 (measured max 2.7e-12) |
| F13 | rank/readiness | warmup/NaN represented; rank-deficient only in unit tests |
| F14 | no future data | PASS (causal Engine replay) |
| F15 | activity = `interval_mean_vn` | PASS |
| F16 | raw color | PASS exact |
| F17/F18 | 0.9 / 1.1 inclusive | PASS exact; neighborhood inspected |
| F19–F22 | confirmation / Indicator | Phase F **FAIL**; Phase F-R **PASS** 55,199/55,199 |
| F23 | phase | PASS exact on frozen V1/V2; 0 Engine-derivative flips |
| F24 | confidence HIGH | PASS |
| F25 | domain `CAUSAL_LOCAL_VOLUME` | PASS |

## Limitations

Confirmed from this run and the forensic investigation:

- One historical entity corpus (SPY) selected the 0.9 / 1.1 bands.
- Zero / missing / all-zero-denominator Volume is not frozen-validated.
- Irregular-time **is** present in this corpus (gaps exist) and was replayed with actual timestamps; additional synthetic Phase C cases remain.
- Production QuanTRAM does not inherit historical session-reset mechanics.
- Phase F proves **migration fidelity of features/derivatives** and **identifies** an interpretation-state difference. It does not prove profitability, generalization, or live delivery.

## Conclusion

**Verdict: FAIL.**

Required clean-PASS gates:

1. Hash-verified artifacts — **yes**
2. Semantically aligned rows — **yes**
3. All required categoricals exact — **no** (`Indicator`, `transition_state`)
4. Numerics within tight justified tolerance — **yes**
5. No hidden omission — **yes**
6. No production science changed to force PASS — **yes**
7. Session/harness isolated from production lifecycle — **yes**
8. Engine replay consistent with helpers — **yes** (features)

Because (3) failed, **initial Phase F was not a clean scientific PASS.** That finding is retained.

Human review then authorized option 1: adopt frozen APTF `state.color = cockpit` write. That is Phase F-R below.

## Phase F-R revalidation (2026-09-05)

Authorized correction: `internal/volume/interpretation.go` `confirmColor` now matches HEAD `spy_volume_engine.observe`:

```text
next.Color = emitted Indicator
# while pending, emitted Indicator is AMBER
# CONFIRMED_* leaves pending_count at the confirming increment
```

No feature, derivative, threshold, phase, confirmation-count, or Engine lifecycle change. No production session reset.

Measured session-sliced 014C after repair:

| Field | Compared | Match |
|---|---|---|
| `raw_color` | 55,199 | 55,199 |
| `Indicator` / `cockpit_color` | 55,199 | 55,199 |
| `transition_state` | 55,199 | 55,199 |
| `phase` | 55,199 | 55,199 |
| `confidence` | 55,199 | 55,199 |
| `domain_state` | 55,199 | 55,199 |

010 Engine feature replay unchanged (101,205; all `outside_tol=0`).

First Phase F mismatch `2023-03-30T11:16:00Z`: production now emits Indicator AMBER / `PENDING_GREEN` (frozen).

Continuous Engine vs 014C Indicator: **41** mismatches, first at `2023-04-03T13:19:00Z` (want AMBER, got RED). Class **E** session/harness lifecycle. Not repaired.

**Revalidation verdict: PASS.**

Clean-PASS gate (3) now holds for session-sliced interpretation. Gate (6) is restated: the only science change was the authorized confirmation state-write.

## go test / go vet

| Command | Result |
|---|---|
| `go test ./internal/volume/ ./internal/domain/ -count=1` | PASS (`volume` 2.299s including Level 2) |
| `go test ./internal/volume/ -count=1 -timeout 180s -run "TestF0"` | PASS (Level 1) |
| `go test ./internal/volume/ -count=1 -timeout 180s -run "TestFFull"` | PASS (Level 2; documents Indicator divergence without forcing production to match) |
| `go test ./internal/ingestion/ -count=1` | PASS |
| `go test ./internal/adaptive/ -count=1` | PASS |
| `go test ./internal/pricing/ -count=1` | PASS |
| `go test ./internal/modelhost/ -count=1` | PASS this run |
| `go vet ./internal/volume/ ./internal/domain/` | PASS |
| `go vet ./...` | PASS |

`TestResetSymbolReplayMatchesUninterrupted` is a pre-existing modelhost flake (failed in Phase E, including isolation). This Phase F run of `./internal/modelhost/` passed. The test and implementation were not modified. Treat as unrelated to Volume.

## Change log

| Date | Change |
|---|---|
| 2026-09-05 | Phase F frozen-equivalence validation. Features/derivatives PASS. Indicator/transition FAIL vs APTF `state.color=cockpit` write. No production science change. |
| 2026-09-05 | Phase F-R authorized confirmation state-write repair. Session-sliced 014C Indicator/transition 55,199/55,199. Feature/derivative numbers unchanged. Continuous Engine session difference (41) left as lifecycle. |
