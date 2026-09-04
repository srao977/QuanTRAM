# QuanTRAM Realtime Stage State-Transition Publication V1

**Title:** QuanTRAM Realtime Stage State-Transition Publication V1  
**Date:** September 4, 2026  
**Version:** V1.1  
**Status:** IMPLEMENTED / READY FOR REVIEW  
**Scope:** Provider-neutral in-process StageTransition publication plus a human-readable TXT diagnostic subscriber. Snapshot Service, Persistence Service, MongoDB, Aperture, proto, and dashboard are out of scope.  
**Authoritative references:** [Process Model](QuanTRAM_PROCESS_MODEL_082926.md), [P-04 Price Engine](QuanTRAM_P04_PRICE_ENGINE_090226.md), current `internal/` implementation.  
**Implementation status:** V1.1 landed on `main` (uncommitted). Typed StageState equality, full initiating `domain.Bar` on Bar-driven transitions, explicit state/fact/causal split.

## 1. Document Control

| Field | Value |
| :--- | :--- |
| Contract version | `1.1` (`stagetransition.ContractVersion`) |
| Authoring date | September 4, 2026 |
| Code package | `internal/stagetransition` |
| Canonical Bar | `domain.Bar` in `internal/domain/bar.go` |
| Diagnostic file | `./stage_transitions.txt` (git-ignored, repo-root working directory) |
| Proto | Unchanged |
| Dashboard | Unchanged |

Where this document and the code disagree, the code is authority for current behavior. This document was written after implementation.

## 2. Executive Overview

QuanTRAM now publishes a compact `Event` when a realtime stage's **meaningful categorical state** changes. Publication is sideways output from P-01–P-04. It is not a new pipeline stage, not a second model mailbox, and not Snapshot or Persistence.

Subscribers consume the same in-process contract. V1 ships one subscriber: a diagnostic TXT writer. A future Snapshot Service may subscribe later. Realtime stages never write files.

## 3. Purpose

Give operators and a future Snapshot Service a provider-neutral fact stream of **state changes**, not a bar/event dump.

Governing rule:

```text
new meaningful StageState != previous meaningful StageState  →  publish
otherwise                                                 →  do not publish
```

A new Bar, DecisionEvent, PriceEvent, timestamp, latency, or ID is not by itself a transition.

## 4. Scope

**In scope**

- Stage IDs for P-01–P-04 and future-safe ID space
- `Event` domain contract
- Detector (equality, sequence, identity)
- Bounded in-process pub/sub
- TXT diagnostic subscriber
- Host/pipeline hooks after committed or otherwise authoritative outcomes
- Offline tests

**Out of scope**

- Snapshot Service, Snapshot N/offset/documents
- Persistence Service, MongoDB, SQLite sessions
- Aperture
- Proto / dashboard / external brokers
- P-05–P-08 implementation
- D01/D02/D04/Price Engine mathematics

## 5. Authoritative Realtime Boundary

The required path is unchanged:

```text
P-01 Market Feed
      |
      v
P-02 Ingestion / Data Quality
      |
      v
accepted eligible Bar
      |
      v
P-03 Adaptive + P-04 Price Engine   (collocated siblings, same bar)
      |
      v
future P-05 / P-06 / P-07 / P-08
```

P-03 and P-04 still share one keyed symbol worker, one `SubscribeModelBars` subscription, and one prepare/commit transaction. StageTransition does not subscribe to bars.

## 6. Terminology

Do **not** use Observer / Observation / ObservationEvent. QuanTRAM already uses Observation as a scientific input.

| Term | Meaning |
| :--- | :--- |
| StageID | Stable machine identity (`P03_ADAPTIVE`) |
| StageName | Presentation (`Adaptive Model`) |
| StageState | Equality key for “did meaning change?” |
| Event | Immutable StageTransitionEvent envelope |
| Hub | Process-level publisher + detector |
| Subscriber | Receives Event values; TXT diagnostic is one |
| EntityID | Symbol, or `GLOBAL` for process-level stages |

“Snapshot” appears only when naming the future consumer.

## 7. Architectural Invariants

1. Publish only on StageState change.
2. Do not invent scientific state for publication.
3. Do not recompute D01/D02/D04/EXPM/policy.
4. Publish committed / authoritative outcomes only.
5. Publish must not block on subscriber or disk work.
6. Zero subscribers is valid and must not change science.
7. Realtime stages perform no filesystem I/O.
8. `stage_transitions.txt` is never an application input.
9. No Aperture, Mongo, Snapshot ID, or Snapshot N/offset in this contract.

## 8. Current Code State-Semantics Audit

| Stage | Existing Authoritative State | Owning Type/File | Transition Equality | Source Fact | Implementation Decision |
| :--- | :--- | :--- | :--- | :--- | :--- |
| P-01 | `domain.FeedState` on `LiveBarSource.Health()` | `internal/marketfeed` (`AlpacaStream.setState`, `CSVSource.setState`); sampled by `ingestion.Pipeline` | `HEALTHY` / `DEGRADED` / `FAILED` / `RECOVERING` | `FeedHealth.State` | **Implemented.** Entity `GLOBAL`. LastMessage/RTT/error text are facts, not equality. Unspecified state is ignored. |
| P-02 | Circuit breaker + `inferReady` + gap-fill `filling` | `internal/ingestion/pipeline.go`, `breaker.go` | `NOT_READY` / `OBSERVE_ONLY` / `OBSERVE_INFER` / `RECOVERING` | breaker + infer + filling | **Implemented.** Entity `GLOBAL`. A new accepted Bar is not a transition. Per-symbol model-path discontinuity is already a P-03 skip (`STATE_DISCONTINUOUS` / `QUEUE_OVERFLOW`) and is not duplicated here. |
| P-03 | Committed `DecisionEvent` decision or host/engine skip | `internal/domain/decision.go`, `adaptive.Engine`, `modelhost.Host.emit` | `DECISION:{BUY\|SELL\|HOLD}` or `SKIP:{reason}` | `Decision.Side` or `Skip.Reason` | **Implemented.** HOLD→HOLD does not publish. PathDirection/strength/confidence are facts only. |
| P-04 | Committed `PriceEvent` | `internal/domain/price.go`, `pricing.Engine`, `modelhost.Host.emitPrice` | `EMITTED:{color}` or `SKIP:{reason}` or bare `PricingStatus` | Status + color or skip reason | **Implemented.** Only after successful P-03/P-04 commit (`emitPrice`). AMBER→AMBER does not publish. |

`modelhost.SymbolStatus` (cold/ready/paused) is derived health, not a second published StageState.

V1.1 authoritative table (implementation):

| Stage | Authoritative State Fields | Equality Fields | Transition Facts | Causal Input | Bar Required? | Owning Code |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| P-01 | FeedState | Kind, FeedState | SourceID, LastError, SubscribedSymbols | none | NO | Hub.OnFeed |
| P-02 | Capability | Kind, Capability | FeedState, Observe, Infer, Filling, SourceID | Bar from accept when it changed capability | optional | Hub.OnIngestion |
| P-03 | Kind, Side or SkipReason, ModelStatus, EmitterPosition | those fields | PathDirection, Strength, Confidence, Uncertainty | considered/accepted domain.Bar | YES for host emit with a real bar | Hub.OnDecision |
| P-04 | Kind, PricingStatus, Color, SkipReason, TrajectoryPhase, DomainState, ConfidenceState, DomainExit | those fields | Emitted | committed accepted domain.Bar | YES | Hub.OnPrice |

`Code` is display-only and is not an equality field.

## 9. Stage Definition

A stage is a logical process from the process model (P-01…P-0n). V1 implements four. The Hub does not cap the number of StageIDs.

## 10. Stage IDs

| StageID | StageName | Entity scope |
| :--- | :--- | :--- |
| `P01_MARKET_FEED` | Market Feed | `GLOBAL` |
| `P02_INGESTION` | Ingestion | `GLOBAL` |
| `P03_ADAPTIVE` | Adaptive Model | symbol |
| `P04_PRICE_ENGINE` | Price Engine | symbol |

Future IDs (not implemented): `P05_OMS_RISK`, `P06_EXECUTION`, `P07_EXECUTION_EVENTS`, `P08_EXECUTION_LEDGER`, plus later position/cash/P&L stages.

## 11. StageTransitionEvent Contract

Type: `stagetransition.Event` in `internal/stagetransition/model.go`.

| Field | Role |
| :--- | :--- |
| `TransitionID` | `StageID:EntityID:Sequence` |
| `Sequence` | Monotonic per (StageID, EntityID) |
| `StageID` / `StageName` | Identity + presentation |
| `EntityID` | Symbol or `GLOBAL` |
| `Previous` / `Current` | StageState (`ABSENT` if none) |
| `EffectiveEventTime` | When the new state became effective |
| `PublishedTime` | Local wall clock at publish |
| `MarketSnapshotID` | Copied when present |
| `AcceptedSequence` | Copied when present |
| `SourceEventID` | DecisionEvent/PriceEvent ID when present |
| `InitiatingBar` | Optional `*domain.Bar`. Required for Bar-caused P-03/P-04. Absent for P-01. |
| `ReasonCode` | Compact cause (side, skip, feed state, …) |
| `ContractVersion` | `1.1` |
| `Feed` / `Ingestion` / `Adaptive` / `Pricing` | Typed **transition facts** (not equality) |

No `map[string]any`. No `aperture_id`. No Snapshot fields. InitiatingBar does not participate in equality.

```text
    StageTransitionEvent
          |
          +-- Identity (TransitionID, Sequence)
          |
          +-- Stage / Entity
          |
          +-- PreviousState / CurrentState
          |      |
          |      +-- authoritative equality dimensions
          |
          +-- InitiatingBar
          |      |
          |      +-- full causal domain.Bar payload (value copy)
          |      +-- DOES NOT participate in equality
          |
          +-- TransitionFacts
          |      |
          |      +-- contextual values at this transition
          |      +-- DO NOT participate in equality
          |
          +-- Correlation (MarketSnapshotID, AcceptedSequence, SourceEventID)
          |
          +-- EffectiveEventTime
          |
          +-- PublishedTime
```

## 12. Timestamp Semantics

**EffectiveEventTime**

- P-03/P-04: `DecisionEvent.IntervalStart` / `PriceEvent.IntervalStart` (authoritative bar interval).
- P-01: `FeedHealth.LastMessage` when set; otherwise publish time (feed has no bar interval).
- P-02: publish time (capability changes are process-local; they are not a bar identity).

**PublishedTime**

`time.Now()` at detector accept (injectable via `Hub.SetClock` in tests).

These are never collapsed into one `Timestamp`. Receive time is not substituted for bar interval time on model stages.

## 13. Transition Identity

```text
TransitionID = "{StageID}:{EntityID}:{Sequence}"
```

Example: `P03_ADAPTIVE:AAPL:3`.

Provider-neutral. No Mongo ObjectId, filesystem, or Snapshot ID. `PublishedTime` is excluded so replay of the same sequence at the same scope yields the same ID.

## 14. Transition Sequence

Ownership scope: **StageID + EntityID**.

AAPL/P-03 and AAPL/P-04 are independent counters. AAPL/P-03 and SPY/P-03 are independent. There is no process-global sequence.

Sequence starts at 1 on first publication. `ResetSymbol` clears last-state for that symbol’s P-03/P-04 so the next state publishes even if the code matches, but the sequence **continues** (still monotonic).

## 15. Stage-Specific Facts

Compact copies of already-computed values:

- `FeedFacts`: SourceID, LastError, SubscribedSymbols
- `IngestionFacts`: FeedState, Observe, Infer, Filling, SourceID
- `AdaptiveFacts`: Kind, Side, SkipReason, ModelStatus, EmitterPosition, PathDirection, Strength, Confidence, Uncertainty
- `PricingFacts`: Status, Color, SkipReason, TrajectoryPhase, DomainState, ConfidenceState, DomainExit, Emitted

Facts are not equality keys.

## 16. P-01 State Mapping

`Hub.OnFeed(domain.FeedHealth)` from `Pipeline.publishTransitions`, which samples `live.Health()`.

Equality: `string(FeedHealth.State)`.

Alpaca: `FAILED` (construct / disconnect) → `RECOVERING` (session start) → `HEALTHY` (authenticated + subscribed) → `FAILED` (session end). CSV: typically `HEALTHY`, `FAILED` on open error.

`FeedDegraded` exists in the domain and is published if the adapter ever sets it. The current circuit breaker usually overlays `HEALTHY`/`FAILED`/`RECOVERING` for ingestion capability; P-01 still reports adapter health, not the breaker.

## 17. P-02 State Mapping

`Hub.OnIngestion(IngestionInput)`:

| Condition | StageState |
| :--- | :--- |
| `filling` or breaker `RECOVERING` | `RECOVERING` |
| breaker not HEALTHY/DEGRADED/RECOVERING | `NOT_READY` |
| infer true | `OBSERVE_INFER` |
| else (observe, infer false) | `OBSERVE_ONLY` |

Quality COMPLETE on every bar is not a transition.

## 18. P-03 State Mapping

`Hub.OnDecision` from `Host.emit` (every authoritative DecisionEvent, including host gate skips).

| Outcome | StageState |
| :--- | :--- |
| Decision | `DECISION:BUY` / `DECISION:SELL` / `DECISION:HOLD` |
| Skip | `SKIP:INITIALIZING`, `SKIP:INFER_OFF`, `SKIP:ENGINE_ERROR`, … |

HOLD after HOLD does not publish. INITIALIZING after INITIALIZING does not publish.

## 19. P-04 State Mapping

`Hub.OnPrice` from `Host.emitPrice` only (committed pricing).

| Outcome | StageState |
| :--- | :--- |
| Emission with color | `{Status}:{Color}` e.g. `EMITTED:GREEN` |
| Skip with reason | `SKIP:WARMUP_DERIVATIVE`, `SKIP:F4_FIT_UNAVAILABLE`, … |
| Status only | `WARMUP_DERIVATIVE`, … |

Failed pricing prepare does not call `emitPrice`; no false P-04 transition.

## 20. State Equality

Typed field comparison in `StageState.Equal`. `Code`, timestamps, IDs, floats, facts, and `InitiatingBar` are excluded.

A new Bar with the same authoritative StageState does **not** publish.

## 20a. Authoritative StageState vs Transition Facts vs Causal Input

**Authoritative StageState** participates in equality and is reconstructable from the transition stream.

**Transition Facts** are captured at publish time. The stream does **not** guarantee their latest value after later non-transitioning bars. Strength, confidence, uncertainty, and PathDirection are facts.

**Causal Input** is the initiating `domain.Bar` (full payload). It is lineage, not state.

## 20b. Full Initiating Bar Contract

Type: `domain.Bar` (`internal/domain/bar.go`). No duplicate schema.

Copy: assignment. The struct has no maps, slices, or pointers.

P-03 and P-04 transitions caused by the same accepted Bar B both carry a value copy of B.

P-01: InitiatingBar absent. P-02: present only when `accept` caused the capability change.

Correlation `MarketSnapshotID` and `EffectiveEventTime` must agree with the attached Bar (`Event.BarAgrees`).

```text
                       Accepted Bar B
                            |
                +-----------+-----------+
                |                       |
                v                       v
          P-03 Adaptive            P-04 Pricing
                |                       |
             commit                   commit
                |                       |
        meaningful state?       meaningful state?
               YES                     YES
                |                       |
                v                       v
       StageTransition P03     StageTransition P04
                |                       |
                | InitiatingBar = B     | InitiatingBar = B
                | (full payload copy)   | (full payload copy)
                +-----------+-----------+
                            |
                            v
                    stage.transitions
```

## 21. Initial-State Semantics

The first observed non-empty StageState for a (stage, entity) **does** publish. Previous displays as `ABSENT`. This is intentional so a run starts with a known baseline in the diagnostic file.

## 22. Commit Boundary

P-03/P-04 remain one transaction:

```text
prepare Adaptive
prepare Pricing
if both ok:
    commit both
    emitPrice  →  P-04 transition if state changed
    emit decision  →  P-03 transition if state changed
else:
    do not commit
    emit skip DecisionEvent  →  P-03 skip transition if changed
    do not emitPrice
```

Deadline/panic/infer-off/not-eligible paths emit a skip without scientific commit. That skip is host-authoritative and may publish a P-03 transition. It must not publish a P-04 committed-state transition.

## 23. Publication Ordering

When both stages change on one accepted bar, order matches existing fan-out:

1. P-04 (`emitPrice`)
2. P-03 (`emit`)

Underlying commit order is unchanged (Adaptive then Pricing). Transition order was not used to rewrite commit.

P-01/P-02 publish from the ingestion loop when capability/feed state changes, independently of the model worker.

## 24. Pub/Sub Architecture

```text
                 AUTHORITATIVE REALTIME FLOW

 P-01 Feed
      |
      v
 P-02 Ingestion
      |
      v
 P-03 Adaptive + P-04 Price Engine
      |
      v
 future realtime stages

      |              |               |
      | meaningful stage changes     |
      +--------------+---------------+
                     |
                     v
                    Hub
              (detector + Publisher)
                     |
                     v
               stage.transitions
               (in-process channels)
                     |
                     v
              TXT Diagnostic
                Subscriber
                     |
                     v
           stage_transitions.txt
```

TXT I/O is not on the realtime path.

`Publisher.Publish` copies the subscriber list, then non-blocking-sends. Full channels drop that delivery and increment `Dropped`.

## 25. Bounded Resource Policy

| Resource | Bound |
| :--- | :--- |
| Per-subscriber channel | `DefaultSubscriberBuffer` = 128 (overridable) |
| Detector maps | one last-state + sequence per (stage, entity) |
| Diagnostic writer | 16 KiB `bufio.Writer` |

There is no unbounded transition queue.

## 26. Failure / Backpressure Semantics

| Condition | Behavior |
| :--- | :--- |
| Zero subscribers | Detector still records last-state; Publish increments `Published` only |
| Slow subscriber | Realtime continues |
| Full subscriber | Drop that Event for that subscriber; `Dropped++` |
| Closed publisher | Further Publish ignored |
| Diagnostic open/write fail | Logged and counted; Hub and science continue |
| Failed subscriber | Isolated; does not roll back model state |

## 27. Health / Counters

`Publisher.Stats()` / `Hub.Stats()`:

- `Published`
- `Delivered`
- `Dropped`
- `Subscribers`

Not exported on proto (proto unchanged). Diagnostic footer records written / dropped / write-error counts.

## 28. TXT Diagnostic Subscriber

`stagetransition.Diagnostic` subscribes to the Hub. Default path `./stage_transitions.txt`. Optional override: `QUANTRAM_STAGE_TRANSITION_LOG`.

`scripts/Start-QuantramIngestion.ps1` sets location to the repository root, so the default file is `<repo-root>/stage_transitions.txt`.

Unit tests use `t.TempDir()` and must not create the repo-root file.

## 29. TXT File Format

One structured block per transition (not `%+v`, not JSON dumps):

```text
----------------------------------------------------------------------
STAGE TRANSITION
----------------------------------------------------------------------

Transition ID:      ...
Sequence:           ...

Stage ID:           ...
Stage Name:         ...
Entity:             ...

Effective Time:     RFC3339Nano UTC or N/A
Published Time:     RFC3339Nano UTC or N/A

Previous State:
    Kind / Decision / Model Status / Emitter / ...

Current State:
    Kind / Decision / Model Status / Emitter / ...

Initiating Bar:
    Symbol / Market Snapshot ID / Interval / OHLCV / Source
    or N/A for non-Bar transitions (P-01)

Transition Facts:
    Path / Strength / Confidence / Uncertainty  (P-03)
    Emitted                                     (P-04)

Correlation:
    Market Snapshot ID: ...
    Accepted Sequence:  ...
    Source Event ID:    ...

Reason:
    ...

----------------------------------------------------------------------
```

Missing strings render as `N/A`. Initiating Bar is always printed.

## 30. TXT Lifecycle

Startup: create/truncate, write run header, start subscriber goroutine.

Live: format and write in that goroutine; `bufio.Flush` every 1s (not `fsync`).

Shutdown (`Diagnostic.Close`): stop accepting, unsubscribe (closes channel, draining buffered Events), write footer, flush, close file.

Diagnostic does not own server lifecycle.

## 31. Realtime Integration Points

| Site | Hook |
| :--- | :--- |
| `cmd/quantram-server/main.go` | `NewHub`, `NewDiagnostic`, `pipeline.SetTransitions`, `modelhost.Options.Transitions`, defer Close |
| `ingestion.Pipeline` | `publishTransitions` after accept, heartbeat, recovery, gap-fill, live-source end |
| `modelhost.Host.emit` | `OnDecision` |
| `modelhost.Host.emitPrice` | `OnPrice` |
| `modelhost.Host.ResetSymbol` | `ResetEntity` (last-state only) |

No second `SubscribeModelBars`. Market-feed adapters are not imported by `stagetransition`.

## 32. Package/File Ownership

```text
internal/stagetransition/
  model.go         Event, StageID, facts
  detector.go      last-state, sequence, TransitionID
  bus.go           bounded Publisher
  hub.go           OnFeed / OnIngestion / OnDecision / OnPrice
  diagnostic.go    TXT subscriber
  *_test.go
internal/ingestion/pipeline.go     P-01/P-02 consider
internal/modelhost/host.go         P-03/P-04 consider after emit
internal/config/config.go          QUANTRAM_STAGE_TRANSITION_LOG
cmd/quantram-server/main.go        wiring
```

## 33. Test Specification

Covered in `internal/stagetransition` plus host/pipeline/config tests:

- first-state, A→B, B→B silent, B→C
- event time vs published time
- TransitionID / monotonic sequence / entity and stage isolation
- zero, multiple, slow, full subscribers; drop accounting
- close / unsubscribe / concurrent symbols
- diagnostic header/block/footer, same-state no extra block, temp files only
- P-04 then P-03 after first commit
- pricing prepare fail: P-03 skip, no P-04
- Hub attached does not change adaptive/pricing hashes
- P-02 capability publishes once
- config default and override
- full initiating Bar on P-03/P-04; same Bar B on both
- Bar copy independent of later mutation of the source value
- different OHLCV/interval/snapshot with same StageState does not publish
- P-01 InitiatingBar absent; TXT prints N/A
- state-field change publishes; fact-only and Bar-only do not
- reducer reconstruction matches `Hub.LatestAll`

## 34. Performance Boundary

Synchronous realtime cost: derive compact StageState, compare, optionally construct Event, non-blocking send.

Not done synchronously: filesystem, Mongo, network, protobuf, Snapshot construction.

Benchmarks: `BenchmarkPublishZeroSubscribers`, `BenchmarkHubUnchangedDecision`.

## 35. Future Snapshot Integration

```text
                    REALTIME FLOW
                         |
                  stage state changes
                         |
                         v
                       Hub
                         |
                         v
                  stage.transitions
                         |
               +---------+---------+
               |                   |
               v                   v
         TXT Diagnostic       FUTURE Snapshot
           Subscriber             Service
               |                   |
               v                   | freeze on
     stage_transitions.txt         | Snapshot boundary
                                   v
                               Snapshot
                                   |
                                   v
                         FUTURE Persistence
                              Service
```

Snapshot and Persistence: **FUTURE — NOT IMPLEMENTED BY THIS TASK.**

Snapshot interval logic does not belong in StageTransition. No N, offset, snapshot_position, or Snapshot ID here.

### Snapshot reconstruction contract (future)

A future Snapshot Service may keep `latestStageState[StageID][EntityID] = event.Current` for every received Event in sequence order. It may also retain `event.InitiatingBar` as causal lineage of the latest **state change**.

A future Snapshot record may conceptually contain: StageID, EntityID, CurrentState, LastTransitionID, LastTransitionSequence, StateEffectiveTime, InitiatingBar.

That reconstructs **latest meaningful StageState**. It does **not** reconstruct latest strength/confidence/uncertainty/PathDirection after later bars that did not change StageState. Whether Snapshot also needs latest continuous outputs is a later design.

Identity (`StageID:EntityID:Sequence`) is process-lifetime scoped. Future Aperture can namespace it. ResetSymbol clears last-state for that symbol’s P-03/P-04 so the next state can republish; sequence continues.

## 36. Future P-05/P-06/P-07/P-08 Extension

Add a StageID, a typed facts struct, a `Hub.On…` mapper, and a consider call at that stage’s commit boundary. Do not assume only four stages exist. Do not implement those stages now.

## 37. Known Limitations

- P-01 EffectiveEventTime is `LastMessage` or publish time; the adapter does not carry a bar interval.
- P-02 is process-global capability, not per-symbol quality.
- P-02 does not publish every continuity class (`first` / `unaligned`); those remain host skips or logs.
- Circuit breaker in increment 1 is thin; `DEGRADED` is rare in live Alpaca.
- Diagnostic is one file per process run (truncated at start).
- `TestResetSymbolReplayMatchesUninterrupted` can fail from pre-existing D01 `computeCoherence` map-iteration floating-point order. Not fixed here.

## 38. Design Decisions

| Decision | Choice | Why |
| :--- | :--- | :--- |
| Authoring vs runtime | JSON/runtime unchanged; this is a new event stream | Dictionary is a different contract |
| Equality | Typed StageState fields, not Code or Bar | Reconstructable categorical state |
| P-03 key | Kind, Side/Skip, ModelStatus, Emitter | Path/floats remain facts |
| P-04 key | Status, color, skip, phase, domain, confidence class, domain-exit | Numerics remain out |
| Identity | `stage:entity:seq` | Process-lifetime; no clock, no DB |
| Sequence scope | Stage+entity | Matches symbol-worker ownership |
| Same-commit order | Price then Adaptive | Matches existing emit order |
| Transport | In-process channels | No broker in V1 |
| Buffer | 128 | Same class as model event buffers |
| TXT | Subscriber only | Realtime must not touch disk |
| Config | One optional path env | Fits existing `QUANTRAM_*` pattern |

## 39. Implementation Status

V1.1 implemented on `main` (uncommitted) as of September 4, 2026. Ready for human review and a live-market inspection of `stage_transitions.txt`. Snapshot/Persistence not implemented.

## 40. Change Log

| Date | Version | Change |
| :--- | :--- | :--- |
| September 4, 2026 | V1.1 | Typed StageState equality; full initiating `domain.Bar` on Bar-driven P-03/P-04; explicit state vs facts vs causal input; Snapshot reconstruction contract; TXT prints Initiating Bar. Science unchanged. |
| September 4, 2026 | V1.0 | Initial StageTransition publication + TXT diagnostic subscriber. No science, proto, or dashboard changes. |
