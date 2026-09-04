# QuanTRAM StageTransition V1.1 Live-Run Forensic Audit

**Title:** QuanTRAM StageTransition V1.1 Live-Run Forensic Audit  
**Date:** 2026-09-04  
**Status:** AUDIT COMPLETE — PASS WITH OBSERVATIONS  
**Purpose:** Determine whether the StageTransition V1.1 implementation behaved according to its design and implementation contracts during a real live IEX run, using `stage_transitions.txt` as the diagnostic artifact.  
**Scope:** Investigation only. No production-code change. No Snapshot, Persistence, MongoDB, Aperture, proto, or dashboard work. No scientific change to D01/D02/D04, Adaptive, EXPM, or Price Engine.  
**Authoritative references:** `docs/design/QuanTRAM_STAGE_TRANSITION_PUBLICATION_V1_2026-09-04.md` (V1.1), `internal/stagetransition/`, `internal/domain/bar.go`, live `stage_transitions.txt`.  
**Documentation path note:** This QuanTRAM repository previously stored post-2026-08-24 design material under `docs/design/` only. No `docs/investigations/` directory existed. This audit uses the requested investigations path: `docs/investigations/QuanTRAM_STAGE_TRANSITION_LIVE_RUN_AUDIT_2026-09-04.md`.

## Executive Summary

The 2026-09-04 live run published **289** StageTransition events from process start `2026-09-04T17:44:31.8081442Z` to clean shutdown `2026-09-04T20:08:31.8269033Z` (2 hours 24 minutes). The diagnostic footer reports **289 written, 0 dropped, 0 write errors**. Independent parse of the TXT file matches those counts exactly.

StageTransition published only on meaningful categorical state change. There were **zero** adjacent same-entity events with identical authoritative StageState. P-03 did not emit one transition per DecisionEvent. P-04 did not emit one transition per PriceEvent. Sequences were strictly monotonic per `(StageID, EntityID)` from 1 with no gaps, duplicates, or resets. First states published with Previous = `ABSENT`. Reconstruction (previous of N+1 equals current of N) holds for every printed state block.

P-01 stayed `HEALTHY` for the entire run. P-02 published three capability events: `OBSERVE_ONLY` → `OBSERVE_INFER` → `OBSERVE_ONLY`, the last at `20:01:30.8109281Z` while FeedState remained `HEALTHY`. That close-time drop is `LiveFresh` / `MaxFinalLateness` (90s), not a feed failure.

No synthetic path-status P-03 transitions (`Initiating Bar: N/A`) occurred. Every P-03 and P-04 transition carried a printed initiating Bar. 58 same-accepted-Bar P-03/P-04 pairs matched on printed OHLCV, interval, source, quality, and `market_snapshot_id`.

**Primary verdict: PASS WITH OBSERVATIONS.**

The observations are not StageTransition equality defects. The most important is a pre-existing Price Engine **active-row** packaging fact: every `EMITTED` P-04 event has `EffectiveEventTime = PriceEvent.IntervalStart` one minute behind the accepted initiating Bar. StageTransition copied both values faithfully. Warm-up P-04 and all P-03 events agree. Do not change Price Engine science to “fix” this as part of StageTransition.

## System / Module Overview

QuanTRAM realtime flow:

```text
P-01 Market Feed
      |
      v
P-02 Ingestion / Data Quality
      |
      +----------------------+
      |                      |
      v                      v
P-03 Adaptive           P-04 Price Engine
```

P-03 and P-04 are collocated siblings on the same accepted eligible Bar. StageTransition is sideways publication when a stage’s reconstructable StageState changes. It does not subscribe to bars, does not recompute science, and does not control realtime execution.

```text
realtime stage
    |
    | meaningful StageState change
    v
Hub (detector + bounded Publisher)
    |
    v
stage.transitions
    +-- TXT Diagnostic  (this run)
    +-- FUTURE Snapshot Service
```

Governing rule: **BAR CHANGED != STAGE CHANGED**.

## Inputs

- Live diagnostic: `stage_transitions.txt` (repo root; git-ignored; produced by `stagetransition.Diagnostic`).
- Implementation: `internal/stagetransition/{model,detector,hub,bus,diagnostic}.go`.
- Integration: `cmd/quantram-server/main.go`, `internal/ingestion/pipeline.go`, `internal/modelhost/host.go`, `internal/config/config.go`.
- Canonical Bar: `internal/domain/bar.go`.
- Design: `docs/design/QuanTRAM_STAGE_TRANSITION_PUBLICATION_V1_2026-09-04.md`.
- Existing unit/integration tests under `internal/stagetransition`, `internal/modelhost`, `internal/ingestion`, `internal/adaptive`, `internal/pricing`.

## Outputs

- This audit document.
- Temporary parser: `tools/audit_stage_transitions_2026_09_04.py` (investigation only; not on the realtime path).

## Parameters / Configuration

This live run used IEX symbols AAPL, MSFT, NVDA (evidence from P-01 facts). Those symbols are run evidence, not StageTransition configuration.

Expected scientific gates (unchanged, not modified by this audit):

| Clock | Owner | Length |
| :--- | :--- | :--- |
| Adaptive context | `adaptive.ContextLength` | 15 accepted eligible bars |
| Adaptive first actionable | `adaptive.ActionableAfter` | 16th engine step (`AcceptedSequence` field = 15 at first HOLD) |
| Price derivative warm-up | `pricing.Config.DerivativeWindow` | 15 |
| Price F4 warm-up | `pricing.Config.F4Window` | 30 |
| First color | `WarmupBars()` | 45 accepted pricing bars |

There is no 100-bar live session. 100 is the offline SADE Unit Run 001 fixture.

## Assumptions

- The TXT file is a lossy human rendering of in-memory `stagetransition.Event` values. `writeInitiatingBar` prints a subset of `domain.Bar`.
- Footer counters are Diagnostic subscriber counters, independently verified by counting `STAGE TRANSITION` blocks.
- “Proven by live artifact” requires a field actually printed in the TXT.
- Code/tests prove in-memory Event behavior that the TXT cannot show.

## Exclusions

- Snapshot Service, Persistence, MongoDB, Aperture, proto, dashboard.
- Any change to D01/D02/D04/EXPM/policy/warm-up/gap/session handling.
- Commit. Fixes. Redesign.

---

## 1. Run Metadata

| Field | Value | Evidence class |
| :--- | :--- | :--- |
| Process Started | `2026-09-04T17:44:31.8081442Z` | PROVEN BY LIVE ARTIFACT |
| Process Stopped | `2026-09-04T20:08:31.8269033Z` | PROVEN BY LIVE ARTIFACT |
| Duration | 02:24:00 (8640 s) | PROVEN BY LIVE ARTIFACT |
| Contract Version | `1.1` | PROVEN BY LIVE ARTIFACT |
| Footer written | 289 | PROVEN BY LIVE ARTIFACT |
| Independently parsed blocks | 289 | PROVEN BY LIVE ARTIFACT |
| Footer dropped | 0 | PROVEN BY LIVE ARTIFACT |
| Footer write errors | 0 | PROVEN BY LIVE ARTIFACT |
| Footer vs parse | consistent | PROVEN BY LIVE ARTIFACT |
| Shutdown | header + 289 blocks + footer; no truncated block | PROVEN BY LIVE ARTIFACT |
| Entities | `GLOBAL`, `AAPL`, `MSFT`, `NVDA` | PROVEN BY LIVE ARTIFACT |
| Stages | `P01_MARKET_FEED`, `P02_INGESTION`, `P03_ADAPTIVE`, `P04_PRICE_ENGINE` | PROVEN BY LIVE ARTIFACT |
| Feed source | `ALPACA_IEX` | PROVEN BY LIVE ARTIFACT |

Inconsistencies: none in header/footer/count.

## 2. Audit Method

1. Read V1.1 design and `internal/stagetransition` implementation (equality, sequence, Hub mappers, bus, diagnostic).
2. Read integration: `Pipeline.publishTransitions`, `Host.emit` / `emitPrice`, `refreshPathStatus`, `refreshInfer` / `InferReady`.
3. Independently parse `stage_transitions.txt` with `tools/audit_stage_transitions_2026_09_04.py` (block split on `STAGE TRANSITION`, not footer trust).
4. Verify sequence, reconstruction, authoritative adjacency, synthetic no-Bar P-03, printed Bar completeness, same-snapshot P-03/P-04 pairs, warm-up, and close window.
5. Scan Effective Time vs initiating-Bar Interval Start (`Event.BarAgrees` timestamp clause).
6. Run `go test ./...`, `go vet ./...`, `git diff --check`, targeted scientific tests, and attempt `-race`.

The parser is not a realtime subscriber and was not imported by the server.

## 3. Transition Inventory

Independently observed total: **289**.

### 3.1 By StageID

| StageID | Count |
| :--- | ---: |
| `P01_MARKET_FEED` | 1 |
| `P02_INGESTION` | 3 |
| `P03_ADAPTIVE` | 140 |
| `P04_PRICE_ENGINE` | 145 |
| **Total** | **289** |

### 3.2 By StageID + EntityID

| Scope | Count | Sequence first–last |
| :--- | ---: | :--- |
| `P01_MARKET_FEED:GLOBAL` | 1 | 1–1 |
| `P02_INGESTION:GLOBAL` | 3 | 1–3 |
| `P03_ADAPTIVE:AAPL` | 43 | 1–43 |
| `P03_ADAPTIVE:MSFT` | 45 | 1–45 |
| `P03_ADAPTIVE:NVDA` | 52 | 1–52 |
| `P04_PRICE_ENGINE:AAPL` | 48 | 1–48 |
| `P04_PRICE_ENGINE:MSFT` | 48 | 1–48 |
| `P04_PRICE_ENGINE:NVDA` | 49 | 1–49 |

### 3.3 P-03 kind and side

| Kind transition | Count |
| :--- | ---: |
| `ABSENT → SKIP` | 3 |
| `SKIP → SKIP` | 3 |
| `SKIP → DECISION` | 3 |
| `DECISION → DECISION` | 131 |

| Side transition (DECISION → DECISION only) | Count |
| :--- | ---: |
| `HOLD → BUY` | 40 |
| `BUY → HOLD` | 40 |
| `HOLD → SELL` | 26 |
| `SELL → HOLD` | 25 |
| `BUY → SELL` | 0 |
| `SELL → BUY` | 0 |

Current-code occupancy (not “events per minute”): `SKIP:INFER_OFF` 3, `SKIP:INITIALIZING` 3, `DECISION:HOLD` 68, `DECISION:BUY` 40, `DECISION:SELL` 26.

No `HOLD → HOLD` StageTransition exists. Repeated HOLD DecisionEvents were suppressed.

### 3.4 P-04 status and color

| PricingStatus / Code family | Count |
| :--- | ---: |
| `WARMUP_DERIVATIVE` | 3 |
| `WARMUP_F4` | 3 |
| `EMITTED` | 139 |

Color-to-color among events that already had a color:

| Color change | Count | Meaning |
| :--- | ---: | :--- |
| `AMBER → GREEN` | 26 | color changed |
| `GREEN → AMBER` | 26 | color changed |
| `AMBER → RED` | 20 | color changed |
| `RED → AMBER` | 21 | color changed |
| `AMBER → AMBER` | 38 | same color; other authoritative field(s) changed |
| `RED → RED` | 4 | same color; other authoritative field(s) changed |
| `GREEN → GREEN` | 1 | same color; other authoritative field(s) changed |

Authoritative field-change counts (from previous printed state, when present): Color 96, Confidence State 88, Domain State 79, Trajectory Phase 78, Domain Exit 77, Pricing Status 6, Skip Reason 6.

Same-color / other-field example (`P04_PRICE_ENGINE:NVDA:4`, accepted 47, `2026-09-04T18:31:00Z` effective / bar `18:32:00Z`):

```text
Previous: EMITTED:AMBER  Trajectory Phase DOWN_ACCELERATING  Domain IN_DOMAIN  Confidence LOW  Domain Exit false
Current:  EMITTED:AMBER  Trajectory Phase UP_ACCELERATING    Domain IN_DOMAIN  Confidence LOW  Domain Exit false
```

This is valid V1.1 behavior. Design §8’s older sentence “AMBER→AMBER does not publish” is **stale** relative to the V1.1 table and `StageState.Equal`. Live evidence matches the code, not that stale sentence.

## 4. Sequence Integrity

For every `(StageID, EntityID)`:

- first sequence = 1
- last sequence = count
- strictly monotonic +1
- no duplicates
- no missing numbers
- no resets

Transition IDs match `{StageID}:{EntityID}:{Sequence}` (example: `P03_ADAPTIVE:AAPL:3`).

`ResetSymbol` / `ResetEntity` was not observed (sequence never continued across a cleared last-state republish of the same code). Process-lifetime sequences are not claimed across restarts; this was one process.

**Result: PASS. PROVEN BY LIVE ARTIFACT.**

## 5. Reconstruction Invariant

Reducer rule: `latest[StageID|EntityID] = event.Current` in sequence order.

Live check: for every consecutive pair at the same scope, printed Previous of N+1 equals printed Current of N. Mismatches: **0**.

First event at each scope has Previous `ABSENT` (example `P01_MARKET_FEED:GLOBAL:1`, `P03_ADAPTIVE:AAPL:1`).

In-memory reconstruction is also proven by `TestReconstructionMatchesDetector` (`Hub.LatestAll`).

What the TXT cannot prove: latest PathDirection / Strength / Confidence / Uncertainty after later silent bars. That is the documented fact-vs-state split (`AdaptiveFacts` are not equality keys).

| Claim | Class |
| :--- | :--- |
| Printed Current reconstructs latest printed StageState | PROVEN BY LIVE ARTIFACT |
| Detector LatestAll matches a reducer | PROVEN BY TEST |
| Latest continuous scores after silent bars | NOT PROVABLE FROM CURRENT ARTIFACT (and not promised) |

**Result: PASS.**

## 6. P-01 Findings

One event: `P01_MARKET_FEED:GLOBAL:1`.

```text
Previous: ABSENT
Current:  Kind FEED / Code HEALTHY / Feed State HEALTHY
Initiating Bar: N/A
Facts: Source ID ALPACA_IEX; Last Error N/A; Symbols AAPL,MSFT,NVDA
Effective: 2026-09-04T17:44:32.3849184Z
Published: 2026-09-04T17:44:32.8107738Z
```

No later P-01 event. The adapter remained HEALTHY through close and until Ctrl+C. Equality is Kind + FeedState; SourceID / symbols / LastError are facts.

Initiating Bar absent: required. `TestP01HasNoInitiatingBar` agrees.

**Result: PASS.**

## 7. P-02 Findings

Three events, entity `GLOBAL`.

| ID | Time | Previous | Current | Infer | FeedState | Bar |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `P02_INGESTION:GLOBAL:1` | 17:44:32.8107738Z | ABSENT | `OBSERVE_ONLY` | false | HEALTHY | N/A |
| `P02_INGESTION:GLOBAL:2` | 17:46:02.1168753Z | `OBSERVE_ONLY` | `OBSERVE_INFER` | true | HEALTHY | NVDA 17:45:00Z |
| `P02_INGESTION:GLOBAL:3` | 20:01:30.8109281Z | `OBSERVE_INFER` | `OBSERVE_ONLY` | false | HEALTHY | N/A |

`GLOBAL:2` is the accept-caused infer-on transition. `Pipeline.accept` calls `refreshInfer` then `publishTransitions(bar)`. The bar that completed `InferReady` for all configured symbols was NVDA `2026-09-04T17:45:00Z` snapshot `38a37807db50f427216462c45a0a4ad43de7d2bced15746076fd39cbc4a12f36` (OHLCV 230.385 / 230.385 / 230.22 / 230.26 / 10777).

`Hub.OnIngestion` does not copy `MarketSnapshotID` or AcceptedSequence onto the Event. Correlation prints N/A. Lineage is on Initiating Bar. That matches P-02 “process-local capability” timestamps (Effective = publish time).

`GLOBAL:3` is heartbeat / `refreshInfer`, not accept. See §15.

A new accepted Bar alone did not create extra P-02 events (only three in 2.4 hours).

**Result: PASS.** Close-time capability drop is correct current realtime behavior, not a feed-health failure.

## 8. P-03 Findings

140 events. Authoritative equality (`AdaptiveState` / `StageState.Equal`): Kind, Side or SkipReason, ModelStatus, EmitterPosition. Code is display-only. Path/floats are facts.

Observed progression per symbol:

```text
ABSENT → SKIP:INFER_OFF → SKIP:INITIALIZING → DECISION:{HOLD|BUY|SELL}*
```

Representative first actionable:

```text
P03_ADAPTIVE:AAPL:3
Effective: 2026-09-04T18:00:00Z
Previous: SKIP:INITIALIZING / Model Status INITIALIZING
Current:  DECISION:HOLD / Model Status ACTIONABLE / Emitter FLAT
Initiating Bar: AAPL 18:00:00Z snapshot fe62aae9154f4603e3ea5307c696e7f1eebeeb20ff863937decdd87df4daf4ee
  OHLCV 321.01 / 321.15 / 320.98 / 321.15 / 755
Accepted Sequence: 15
Source Event ID: AAPL:evt:16
Facts: Path UPWARD; Strength 0.823…; Confidence 0.279…; Uncertainty 0.331…
```

`AcceptedSequence: 15` on the first HOLD is Adaptive’s `completedCount` **before** the actionable step increments (`engine.go`). The 16th engine step is first ACTIONABLE (`ActionableAfter = 16`). This is existing Adaptive numbering, not a StageTransition off-by-one.

Last P-03 per symbol remained a DECISION (no post-close `INFER_OFF` skip). That is because `Host.handle` only runs on a bar; P-02 infer-off does not synthesize a P-03 skip.

**Result: PASS.** Deferred EOD question recorded in §22.

## 9. P-03 Synthetic Transition Audit

Search: every P-03 with `Initiating Bar: N/A`.

**Count: 0.**

`Host.refreshPathStatus` (50 ms poll) emits a skip with `domain.Bar{}` only on the **first** discontinuity latch (`markDisc` true). That path did not fire. Polling therefore produced **zero** StageTransitions. Not periodic noise. Not one transition per poll.

The three `SKIP:INFER_OFF` events are **Bar-driven** host-gate skips (17:44:00Z bars, accepted sequence 0, source IDs `*:host:*`). Example `P03_ADAPTIVE:AAPL:1` snapshot `dcc87305c18c02630c30758fdeb7fb9620a2a78ac07bc6bf35ce47c64a5deda0`.

**Result: PASS.** Synthetic/path-status transitions were sparse (absent) and not noisy.

## 10. P-04 Findings

145 events. Authoritative equality: Kind, PricingStatus, Color, SkipReason, TrajectoryPhase, DomainState, ConfidenceState, DomainExit.

Per symbol: `SKIP:WARMUP_DERIVATIVE` → `SKIP:WARMUP_F4` → `EMITTED:{color}` with later color/phase/domain/confidence/exit changes.

First color (accepted 45) examples:

| ID | Accepted | Effective (PriceEvent) | Bar interval | Color |
| :--- | ---: | :--- | :--- | :--- |
| `P04_PRICE_ENGINE:AAPL:3` | 45 | 18:29:00Z | 18:30:00Z | AMBER |
| `P04_PRICE_ENGINE:NVDA:3` | 45 | 18:29:00Z | 18:30:00Z | (first EMITTED) |
| `P04_PRICE_ENGINE:MSFT:3` | 45 | 18:30:00Z | 18:31:00Z | RED |

No `PROJECTION_FAILURE` / failed committed P-04 transition appears. Failed prepare does not call `emitPrice` (`host.go` commit gate). `TestHostPricingPrepareFailureNoP04` covers that path. Live: NOT PROVABLE that a prepare failed (none visible); CONSISTENT WITH EVIDENCE that only committed pricing published.

**Result: PASS**, with the active-row EffectiveTime observation in §12 and §21.

## 11. Transition Sparsity Analysis

Estimated accepted pricing minutes from last printed Accepted Sequence: AAPL 134, MSFT 133, NVDA 133.

| Symbol | Est. accepted minutes | P-03 transitions | P-03 ratio | P-04 transitions | P-04 ratio |
| :--- | ---: | ---: | ---: | ---: | ---: |
| AAPL | 134 | 43 | 0.321 | 48 | 0.358 |
| MSFT | 133 | 45 | 0.338 | 48 | 0.361 |
| NVDA | 133 | 52 | 0.391 | 49 | 0.368 |

If StageTransition dumped every DecisionEvent / PriceEvent, P-03 and P-04 counts would be ~130+ each. They are ~⅓. Adjacent identical authoritative state: **0**.

AAPL last P-03 accepted 130 vs last P-04 accepted 134: the last four AAPL minutes stayed HOLD (no P-03 publish) while P-04 still had at least one authoritative change (`P04_PRICE_ENGINE:AAPL:48`, AMBER→AMBER, phase `DOWN_DECELERATING` → `DOWN_ACCELERATING`).

**P-03 sparsity: PASS.**  
**P-04 sparsity: PASS.** Same color is not used as the sole equality test; 43 same-color publishes are explained by phase/domain/confidence/exit.

## 12. Initiating Bar Integrity

`domain.Bar` fields: Symbol, InstrumentID, InstrumentType, Tradable, Interval, IntervalStart, IntervalEnd, Open, High, Low, Close, Volume, EventCount, SourceTimestamp, ReceiptTime, Source, QualityStatus, IsFinal, IsBackfilled, SourceTransition, MarketSnapshotID.

TXT prints: Symbol, Market Snapshot ID, Interval Start/End, OHLCV, Source, Source Timestamp, Quality.

**Not printed (NOT PROVABLE FROM ARTIFACT):** InstrumentID, InstrumentType, Tradable, Interval (`1Min`), EventCount, ReceiptTime, IsFinal, IsBackfilled, SourceTransition. Full payload on the in-memory Event is PROVEN BY TEST (`TestFullInitiatingBarPreserved`, `TestHostP03P04ShareAcceptedBar`).

Printed-field completeness on every Bar-driven event: no missing Symbol / snapshot / interval / OHLCV / source / quality.

P-03: Effective Time = Bar Interval Start on **all 140** events. Correlation snapshot = bar snapshot.

P-04 warm-up (6 events): Effective Time = Bar Interval Start.

P-04 `EMITTED` (139 events): Effective Time is **exactly one minute earlier** than Initiating Bar Interval Start / Source Timestamp. Correlation snapshot still equals the initiating (accepted) Bar snapshot.

Cause (implementation, do not change science):

```233:247:internal/pricing/pipeline.go
	// Active-row observation for numerical/engine uses the *active* bar, not the newest pending.
	activeObs := Observation{
		// ...
		Snapshot:  obs.Snapshot,          // newest accepted bar
		Interval:  time.UnixMilli(int64(next.minutes[active] * 60_000)).UTC(), // active row
	}
```

`Hub.OnPrice` sets `EffectiveEventTime = ev.IntervalStart` (active row) and `InitiatingBar =` the accepted Bar passed by `emitPrice`. `Event.BarAgrees()` therefore returns false on those 139 events: EffectiveTime ≠ Bar.IntervalStart, while MarketSnapshotID agrees.

P-02 `GLOBAL:2` also fails that timestamp clause because P-02 EffectiveTime is publish time by design (`17:46:02.1168753Z` vs bar `17:45:00Z`).

This is a **contract tension**, not ST inventing a wrong Bar. See §21.

## 13. P-03 / P-04 Same-Bar Lineage

Pairs joined by printed `market_snapshot_id` present on both a P-03 and a P-04 initiating Bar:

| Metric | Count |
| :--- | ---: |
| Shared snapshots | 58 |
| Printed Bar payload matches (symbol, snapshot, interval, OHLCV, source, quality) | 58 |
| Payload discrepancies | 0 |

Example (warm-up, times agree): AAPL 17:45:00Z snapshot `45f230f44f8ccb3aca03d460696def7d99b2a43c96d93e8417b39eb3447d5d0e`

- `P04_PRICE_ENGINE:AAPL:1` `SKIP:WARMUP_DERIVATIVE` then
- `P03_ADAPTIVE:AAPL:2` `SKIP:INITIALIZING`

Same OHLCV 321.05 / 321.105 / 320.96 / 321.105 / 1285. Publication order P-04 then P-03, matching `emitPrice` then `emit`.

Example (actionable): AAPL 18:00:00Z snapshot `fe62aae9154f4603e3ea5307c696e7f1eebeeb20ff863937decdd87df4daf4ee`

- `P04_PRICE_ENGINE:AAPL:2` `SKIP:WARMUP_F4` accepted 15
- `P03_ADAPTIVE:AAPL:3` `DECISION:HOLD` accepted 15

Not every accepted minute is a pair: a minute may change only P-03, only P-04, or neither. 58 pairs vs 140/145 totals is expected sparsity, not missing lineage.

On EMITTED pairs, P-04 EffectiveTime lags one minute; printed Bars still match. Same-Bar identity is the accepted Bar, not PriceEvent.IntervalStart.

**Result: PASS on accepted-Bar payload identity. PASS WITH OBSERVATION on P-04 EffectiveTime vs that Bar.**

## 14. Warm-Up Progression

Independent clocks on the same accepted stream. Do not change 15 or 45.

### AAPL

| Step | ID | Accepted | Effective | State |
| :--- | :--- | ---: | :--- | :--- |
| Infer off (host gate) | `P03_ADAPTIVE:AAPL:1` | 0 | 17:44:00Z | `SKIP:INFER_OFF` |
| Adaptive init (once) | `P03_ADAPTIVE:AAPL:2` | 0 | 17:45:00Z | `SKIP:INITIALIZING` |
| Price derivative | `P04_PRICE_ENGINE:AAPL:1` | 1 | 17:45:00Z | `SKIP:WARMUP_DERIVATIVE` |
| Price F4 | `P04_PRICE_ENGINE:AAPL:2` | 15 | 18:00:00Z | `SKIP:WARMUP_F4` |
| First HOLD | `P03_ADAPTIVE:AAPL:3` | 15 | 18:00:00Z | `DECISION:HOLD` ACTIONABLE |
| First color | `P04_PRICE_ENGINE:AAPL:3` | 45 | 18:29:00Z | `EMITTED:AMBER` |

INITIALIZING did not republish for bars 2–15. WARMUP_DERIVATIVE did not republish until F4. WARMUP_F4 did not republish until first EMITTED.

### MSFT

Infer-off at 17:44; first INITIALIZING / first P-04 derivative at 17:46 (one minute later than AAPL/NVDA — infer became true on the 17:45 accept, MSFT’s 17:45 bar was still `INFER_OFF` and therefore silent). First HOLD `P03_ADAPTIVE:MSFT:3` accepted 15 at 18:01:00Z. First color `P04_PRICE_ENGINE:MSFT:3` accepted 45 at 18:30:00Z (`EMITTED:RED`).

### NVDA

Same pattern as AAPL: INFER_OFF 17:44, INITIALIZING + WARMUP_DERIVATIVE 17:45, first decision accepted 15, first EMITTED accepted 45.

P-03 became inference-capable after P-02 `OBSERVE_INFER` (17:46:02Z) and scientifically actionable on the 16th engine step. P-04 color at accepted 45. Clocks stayed independent (AAPL HOLD at accepted 15 while still `WARMUP_F4`).

**Result: PASS. Live accepted-Bar progression matches the unchanged 15 / 45 gates.**

## 15. Market-Close Behavior

Regular U.S. cash close 16:00 ET = 20:00Z. Last regular minutes in the artifact: 19:59:00Z–20:00:00Z.

Close-window StageTransitions:

1. `P03_ADAPTIVE:MSFT:45` — Effective 19:59:00Z, Published 20:00:02.2269411Z, `SELL → HOLD`, accepted 133, full MSFT bar (499.655 / 499.735 / 499.38 / 499.545 / 39007), snapshot `b62a41a372b07f157bdd4752ba6f644c28ae952985788aaeda74a9ca7d4873d2`.
2. `P04_PRICE_ENGINE:AAPL:48` — Published 20:00:02.2727446Z, AMBER→AMBER phase change, accepted 134, accepted bar 19:59:00Z.
3. `P02_INGESTION:GLOBAL:3` — Effective/Published **20:01:30.8109281Z**, `OBSERVE_INFER → OBSERVE_ONLY`, FeedState **HEALTHY**, Observe true, Infer false, Filling false, Initiating Bar **N/A**.

Trigger for (3): `Pipeline` heartbeat (`config.HeartbeatInterval = 1s`) → `refreshInfer` → `domain.InferReady` → `last.LiveFresh(now)` with `MaxFinalLateness = 90s`. Last finalized interval end 20:00:00Z; at 20:01:30.810Z elapsed 90.810s > 90s → infer false. Breaker/feed still HEALTHY. No initiating Bar is correct (heartbeat, not `accept`).

After 20:01:30Z: no P-03/P-04 transitions until 20:08:31Z shutdown. P-03/P-04 remain at last DECISION / last EMITTED. No `INPUT_GAP`, no `STATE_DISCONTINUOUS`, no latch. Path-status poll stayed quiet.

This is **not** a StageTransition defect. It is current P-02 freshness policy plus “no bar ⇒ no model emit.” Session/EOD synthesis is deferred (§22).

**Result: PASS (capability-state behavior, healthy feed).**

## 16. Nonblocking Publication Audit

| Requirement | Implementation | Live |
| :--- | :--- | :--- |
| Publish does not block realtime | `Publisher.Publish` copies subs, non-blocking send | CONSISTENT (process ran 2.4h; 0 drops) |
| Bounded subscriber queue | `DefaultSubscriberBuffer = 128` | PROVEN BY CODE |
| Full queue = drop + count | `default: dropped.Add(1)` | PROVEN BY CODE; live Dropped=0 |
| Zero subscribers safe | Publish increments Published only | PROVEN BY TEST |
| TXT I/O on subscriber goroutine | `Diagnostic.loop` | PROVEN BY CODE |
| Diagnostic failure cannot stop realtime | write errors counted/logged | Live Write Errors=0 |
| Shutdown drains then footer | `Close` → Unsubscribe/close ch → footer | Footer present after last P-02 |

`TestSlowDiagnosticDoesNotBlockHub` passed.

**Result: PASS.**

## 17. Scientific Noninterference

| Check | Result |
| :--- | :--- |
| `TestTransitionsDoNotChangeScientificHashes` (`-count=1`) | PASS |
| `go test ./internal/adaptive ./internal/pricing -count=1 -run Equivalence\|Hash` | PASS |
| Cached `internal/adaptive`, `internal/pricing`, `internal/modelhost` on `go test ./...` | PASS |
| D01/D02/D04/EXPM source not modified by this audit | yes |
| Warm-up constants unchanged | yes |
| `TestResetSymbolReplayMatchesUninterrupted` | not failed in this run (cached/not flaking here) |

Known pre-existing flake: D01 `computeCoherence` map-iteration last-bit hash. **Not fixed. Not encountered as a failure this audit.**

StageTransition is after `emit` / `emitPrice`. It does not call `PrepareStep`.

**Result: PASS.**

## 18. Test Results

| Command | Result |
| :--- | :--- |
| `go test ./...` | **FAIL** — only `TestDiagnosticDoesNotWriteRepoRoot` |
| `go test ./internal/stagetransition -run` (excluding that test) | PASS |
| `go test ./internal/modelhost -count=1 -run TestTransitionsDoNotChangeScientificHashes` | PASS |
| `go vet ./...` | PASS (no findings) |
| `git diff --check` | PASS (CRLF `core.autocrlf` warnings only) |
| `go test -race ./internal/stagetransition ...` | **UNAVAILABLE** — `go: -race requires cgo; enable cgo by setting CGO_ENABLED=1`. Not forced. |

`TestDiagnosticDoesNotWriteRepoRoot` treats **existence** of repo-root `stage_transitions.txt` as a unit-test leak. The file is the **live diagnostic** (git-ignored). The test cannot distinguish “unit test wrote the file” from “operator left a live run artifact.” This is test hygiene, not a StageTransition runtime defect. Not fixed in this audit.

## 19. Design / Implementation / Live-Evidence Reconciliation

| Requirement / Invariant | Design Says | Implementation Does | Test Evidence | Live Run Evidence | Result | Notes |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| Publish only on StageState change | V1.1 §3, §20 | `Detector.consider` + `StageState.Equal` | lineage / detector tests | 0 identical adjacent states | PASS | |
| Bar not in equality | §20b | Bar excluded from Equal | `TestDifferentBarSameStateDoesNotPublish` | sparsity << accepted minutes | PASS | |
| Identity `Stage:Entity:Seq` | §13 | `transitionID` | unit | all 289 IDs | PASS | |
| Sequence monotonic per scope | §14 | detector `seq[key]+1` | unit | 8 scopes, no gaps | PASS | |
| First state Previous ABSENT | §21 | unseen key | unit | all first events | PASS | |
| P-01 Kind+FeedState; no Bar | §16, §20b | `OnFeed` | `TestP01HasNoInitiatingBar` | 1 HEALTHY, N/A bar | PASS | |
| P-02 Kind+Capability; Bar only if accept caused change | §17 | `OnIngestion(causedBy)` | pipeline tests | 3 events; only :2 has Bar | PASS | |
| P-03 Kind/Side/Skip/Model/Emitter | V1.1 table | `AdaptiveState` | lineage | HOLD streaks silent | PASS | |
| P-04 includes phase/domain/confidence/exit | V1.1 table | `PricingState` | `TestStateVsFactsVsBarEquality` | 43 same-color publishes | PASS | |
| Design §8 “AMBER→AMBER does not publish” | stale V1 wording | contradicted by V1.1 table + Equal | tests publish on phase change | 38 AMBER→AMBER | PASS WITH OBSERVATION | Document vs code discrepancy; code wins |
| Code not equality | §11, §20 | Equal ignores Code | unit | — | PASS | |
| P-03/P-04 same accepted Bar B | §20b | `emitPrice(bar)` then `emit(bar)` | `TestP03P04SameBarLineage`, host test | 58/58 printed payload match | PASS | |
| `Event.BarAgrees` Effective==Bar.IntervalStart | §20b / `model.go` | checked in tests with matching times | unit (constructed) | P-03 140/140; P-04 warmup 6/6; P-04 EMITTED 0/139 | PASS WITH OBSERVATION | Pre-existing PriceEngine active-row IntervalStart; ST copies both |
| P-02 Effective = publish time | §12 | OnIngestion leaves Effective zero → publish time | — | GLOBAL:2 publish ≠ bar start | PASS WITH OBSERVATION | Conflicts with BarAgrees if a Bar is attached |
| Failed pricing: no P-04 | §22 | no `emitPrice` unless both commit | host test | no failed P-04 visible | CONSISTENT / test-proven | |
| Nonblocking / drop | §26 | bus.go | bus tests | dropped 0 | PASS | |
| TXT subscriber only I/O | §28–30 | Diagnostic.loop | diagnostic tests | footer 0 write errors | PASS | |
| Science unchanged | §7 | after emit only | hash test | hashes unchanged | PASS | |
| Reconstruction LatestAll | §35 | detector last map | `TestReconstructionMatchesDetector` | printed prev/cur chain | PASS | |
| No path-status spam | prompt Task 7 | `markDisc` once | host tests | 0 synthetic P-03 | PASS | |

Where this prompt and the repository differ: the prompt’s P-04 “AMBER→AMBER may be valid” matches **code + V1.1 table**, not design §8’s leftover V1 sentence. Discrepancy documented; not silently reconciled by changing code.

## 20. Defects Found

**No StageTransition defect is authorized as a code change, and none is classified as requiring a FAIL verdict.**

The BarAgrees timestamp failures on P-04 `EMITTED` are a **pre-existing PriceEvent active-row vs accepted-Bar packaging tension**. StageTransition did not invent a second Bar and did not alter science.

Smallest *possible* later corrections (not implemented):

1. Leave science unchanged; document that P-04 EffectiveEventTime is PriceEvent (active row) and InitiatingBar is the accepted trigger Bar; treat `BarAgrees` timestamp check as warmup/P-03-oriented.
2. If later authorized as a StageTransition-only change: set P-04 `EffectiveEventTime` from `bar.IntervalStart` when a Bar is present, and keep PriceEvent.IntervalStart in facts. That would make BarAgrees pass without touching EXPM.
3. Do **not** retarget Price Engine `active` to the newest pending row as a StageTransition fix.

Human authorization is required before any of those.

## 21. Observations — Not Defects

1. **Design §8 vs V1.1 equality.** Stale “AMBER→AMBER does not publish” vs typed P-04 fields. Live and tests follow V1.1.
2. **TXT Bar subset.** Diagnostic omits several `domain.Bar` fields. Sufficient for this audit’s printed-field lineage; not a full-Bar dump.
3. **P-02 correlation IDs.** Accept-caused P-02 has Initiating Bar but Correlation snapshot N/A.
4. **P-04 EMITTED EffectiveTime lag.** 139/139 events; one minute; snapshot still matches accepted Bar. Price Engine `active := index - 1` plus `Snapshot: obs.Snapshot`.
5. **P-02 EffectiveTime vs Bar.** Publish time by design when a Bar is attached.
6. **No BUY↔SELL direct transitions.** All side changes passed through HOLD. Science, not ST.
7. **No post-close P-03 INFER_OFF.** Last decisions remain until a bar arrives or the process stops.
8. **`TestDiagnosticDoesNotWriteRepoRoot` vs live artifact.** Environmental fail when the git-ignored live file is present.
9. **Race detector unavailable** without cgo.
10. **Warm-up is meaningful published state** (`INFER_OFF`, `INITIALIZING`, `WARMUP_*`). Relevant to future Snapshot (§23).

## 22. Deferred Realtime / Discontinuity Questions

Not StageTransition defects:

- Whether P-03 should publish `INFER_OFF` when P-02 drops infer without a new bar (EOD / 90s freshness).
- Official session calendar vs `LiveFresh` 90s.
- Extended hours / post-close bars if a provider sends them.
- Whether PriceEvent.IntervalStart (active row) should remain the scientific timestamp while the accepted Bar is the causal trigger. Science-owned; do not change here.

## 23. Architectural Observations for Future Snapshot Integration

DO NOT IMPLEMENT.

Warm-up is already on `stage.transitions`:

- P-03 `SKIP:INFER_OFF`, `SKIP:INITIALIZING`, then ACTIONABLE decisions
- P-04 `WARMUP_DERIVATIVE`, `WARMUP_F4`, then `EMITTED`
- P-02 `OBSERVE_ONLY` ↔ `OBSERVE_INFER` including market-close

A future Snapshot Service that subscribed only after Adaptive 15 or Price 45 would miss the start of the Aperture/realtime lifecycle. Future Snapshot should be capable of seeing initialization, warm-up, actionable operation, steady state, and the close capability transition.

Snapshot still reconstructs **latest meaningful StageState**, not latest silent-bar floats. Identity remains process-lifetime `Stage:Entity:Seq` until Aperture namespaces it.

Rule unchanged: **SNAPSHOT EXTRACTS. PERSISTENCE CONSUMES.** Neither exists today.

## 24. Known Limitations

- TXT is diagnostic, not replay authority.
- One process, one truncated file per run.
- P-01 EffectiveTime is LastMessage or publish time.
- P-02 is process-global capability.
- `BarAgrees` as coded is stricter than live P-04 EMITTED + P-02-with-Bar.
- Race tests not run (no cgo).
- Known D01 hash flake not reopened.

## 25. Final Verdict

**PASS WITH OBSERVATIONS**

Live evidence, implementation, and tests support the StageTransition V1.1 contract: publish only on reconstructable StageState change; monotonic process-lifetime identity; full printed initiating Bar on Bar-driven P-03/P-04; same accepted Bar on sibling pairs; independent 15/45 warm-up; healthy-feed close capability drop; nonblocking diagnostic; no scientific interference.

Observations are documentation discrepancies, diagnostic lossiness, pre-existing PriceEvent active-row timestamps, EOD infer policy, and an environmental unit test that conflicts with a leftover live file.

This is not `FAIL — STAGE TRANSITION DEFECT FOUND` because StageTransition did not emit bar-only noise, did not reset sequences, did not drop/write-error the diagnostic, and did not alter science.

**Commit recommendation:** Yes — StageTransition V1.1 is safe to commit as the publication mechanism, provided the observations (especially P-04 active-row EffectiveTime and the leftover-file unit test) stay recorded. Do not bundle unrelated semantic-test dirty files unless the human asks. This audit does not commit.

## 26. Change Log

| Date | Change |
| :--- | :--- |
| 2026-09-04 | Initial forensic audit of the 17:44:31Z–20:08:31Z live IEX run. Audit only. No production code change. |

---

## Appendix A — Evidence excerpts (not the full TXT)

P-01 first / only:

```text
Transition ID: P01_MARKET_FEED:GLOBAL:1
Previous: ABSENT
Current: FEED / HEALTHY
Initiating Bar: N/A
```

P-02 close:

```text
Transition ID: P02_INGESTION:GLOBAL:3
Previous: OBSERVE_INFER
Current:  OBSERVE_ONLY
Facts: Feed State HEALTHY; Infer false
Initiating Bar: N/A
Effective/Published: 2026-09-04T20:01:30.8109281Z
```

P-04 last AAPL (same-color phase change; active-row lag):

```text
Transition ID: P04_PRICE_ENGINE:AAPL:48
Effective Time: 2026-09-04T19:58:00Z
Initiating Bar Interval Start: 2026-09-04T19:59:00Z
Snapshot: 3e30bac0ef5e38d98727c8a25193f775fb65ad173bdbbd5e9f7be6bd90c5183e
Previous: EMITTED:AMBER DOWN_DECELERATING OUT_OF_DOMAIN LOW exit true
Current:  EMITTED:AMBER DOWN_ACCELERATING OUT_OF_DOMAIN LOW exit true
Accepted Sequence: 134
```
