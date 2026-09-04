# QuanTRAM Pre-Existing Code Findings Investigation

| Field | Value |
|---|---|
| Date | September 4, 2026 |
| Version | V1.0 |
| Status | INVESTIGATION COMPLETE |
| Investigation Type | READ-ONLY CODEBASE ANALYSIS |
| Branch | `mongodb-persistene-v1` |
| Implementation Changes Authorized | NO |
| Immediate Context | Persistence / Aperture / Snapshot live-test readiness |

## Executive Overview

This investigation was performed because the repository-wide Go source documentation audit identified six possible pre-existing issues. No issue was fixed. Production code, tests, generated code, scripts, configuration, dependencies, and existing documentation were not changed by this investigation.

The investigation separates general QuanTRAM correctness risk from the narrower question of whether the current MongoDB, Aperture, and Snapshot mechanisms can be credibly exercised with a controlled live market feed. Finding 1 is divided into two independent concerns: F1-A, possible concurrent `ResetSymbol` mutation, and F1-B, unordered floating-point accumulation in `computeCoherence`.

All seven resulting finding units exist on `main` and predate the MongoDB/Aperture/Snapshot branch work. The persistence branch added capture callbacks and Snapshot/lifecycle composition around the existing ingestion and scientific paths; it did not introduce the controlling behavior for any finding.

The readiness conclusion is **YES WITH CONDITIONS**. There are no direct persistence-test blockers. The controlled test must not invoke `ResetSymbol`, add a second model-path subscriber, claim cross-run scientific reproducibility, or use `StreamBars` delivery counts as the authoritative persistence count. Durable MongoDB identities and records, rather than the observer stream, remain the evidence boundary.

## Purpose And Scope

### Purpose

The purpose is to determine:

- whether each reported issue is real;
- when and where it entered the repository;
- which runtime and scientific surfaces it can affect;
- where it sits relative to persistence capture;
- whether it blocks the controlled live market-feed persistence test; and
- what later remediation and validation should be scheduled.

### Scope

The investigation covers:

1. F1-A: `ResetSymbol` concurrency concern.
2. F1-B: `computeCoherence` floating-point/map-iteration nondeterminism.
3. F2: model-path multi-subscriber prefix divergence.
4. F3: WebSocket trade/event-count conversion to `uint32`.
5. F4: CSV source health after terminal parse failures.
6. F5: `StreamBars` window/subscription overlap.
7. F6: redundant empty semantics-test conditional.

The relationship to persistence testing is limited to proving that accepted live observations and committed outcomes are durably correlated, Snapshot policies operate over durable facts, and coordinated shutdown closes the Aperture truthfully. This is not certification of all scientific behavior for production or paper execution.

### Exclusions

- No implementation or remediation.
- No new diagnostic tests or instrumentation.
- No live server, MongoDB, Alpaca, or other feed access.
- No protocol, schema, fixture, dashboard, script, configuration, or dependency change.
- No claim that passing offline tests eliminates concurrency risk.

This document does **not** authorize implementation changes.

## Repository Baseline

The baseline was captured before this document was created.

| Item | Value |
|---|---|
| Current branch | `mongodb-persistene-v1` |
| HEAD | `0e670241d3d194e1959e1f8cbc324d6d9c4419b7` (`docs specs finalized`) |
| `main` HEAD | `1e2becd38393153a38837f53c7297ba5e211986a` |
| `origin/main` | `1e2becd38393153a38837f53c7297ba5e211986a` |
| Merge base with `main` | `1e2becd38393153a38837f53c7297ba5e211986a` |
| Persistence implementation commit | `cb3dfafa89d0af7bdb7189301b3a73a26a007b36` |
| Pre-existing worktree state | 83 modified Go files; no untracked file |

The 83 starting modifications belonged to the source-documentation pass and were not caused by this investigation. `git status --short` reported only `M` entries, grouped exactly as follows:

- `cmd`: `quantram-ingest-client/main.go`, `quantram-semantics/main.go`, `quantram-server/main.go`.
- `internal/adaptive` (26): all 26 production Go files.
- `internal/config` (1): `config.go`.
- `internal/domain` (6): `bar.go`, `continuity.go`, `decision.go`, `health.go`, `price.go`, `quality.go`.
- `internal/ingestion` (5): `breaker.go`, `model_path.go`, `pipeline.go`, `pipeline_test.go`, `window.go`.
- `internal/marketfeed` (5): `alpaca_rest.go`, `alpaca_ws.go`, `csv_source.go`, `decode.go`, `source.go`.
- `internal/modelhost` (2): `host.go`, `host_test.go`.
- `internal/persistence` (4): `async.go`, `model.go`, `mongo.go`, `snapshot_adapter.go`.
- `internal/pricing` (14): 13 production files and `equivalence_test.go`.
- `internal/semantics` (9): `catalog/v1.go`, `embed.go`, `loader.go`, `model.go`, `validator.go`, and four `tooling` files.
- `internal/server` (5): `decision.go`, `price.go`, `semantics.go`, `server.go`, `snapshot.go`.
- `internal/snapshot` (3): `model.go`, `ports.go`, `service.go`.

## Methodology

The investigation used current-source inspection, caller and interface tracing, runtime ownership analysis, existing comments and design documents, `git log`, `git blame`, `git show`, `git diff`, `git grep` against `main`, and branch comparison. Existing offline tests were run for `adaptive`, `ingestion`, `marketfeed`, `modelhost`, and `server`.

The package set passed its normal test invocation. `TestUnitRun001Equivalence` also passed 50 repeated executions. `TestResetSymbolReplayMatchesUninterrupted` passed in the normal package run but failed intermittently when the existing test was repeated with `-count=50`; its combined assertion does not identify whether the adaptive hash, pricing hash, or both differed. No repository file was created to diagnose it further.

| Activity | Performed? |
|---|---|
| Production code modified | NO |
| Tests modified | NO |
| Existing documents modified | NO |
| Safe offline existing tests run | YES |
| Race detector run | NO |
| Live server started | NO |
| MongoDB contacted | NO |
| Market feed contacted | NO |

## Code-Backed Architecture And Pipeline Map

```text
Alpaca WebSocket
    |
    v
decodeMessageArray -> barFromRaw -> barFromAlpaca
                         [F3]
    |
    v
Pipeline.Run -> Pipeline.accept -> WindowStore.Add (accept/deduplicate)
    |                                  |
    |                                  +----> AsyncStore.CaptureBar
    |                                         -> MongoWriter.WriteBar
    |                                         -> quantram_payloads
    |
    +----> observation fanout -------------------------------> StreamBars [F5]
    |
    +----> fanoutModel [F2] -> Host.Run -> per-symbol worker
                                      |          |
                                      |          +----> ResetSymbol [F1-A]
                                      |          |
                                      |          +----> D01 computeCoherence [F1-B]
                                      |                  -> FMO -> D02 -> D04
                                      |          |
                                      |          +----> P-04 pricing commit
                                      |                    |
                                      |                    +-> CapturePrice
                                      |                       -> DecisionRecord.price_event
                                      |          |
                                      |          +----> terminal DecisionEvent commit
                                      |                    |
                                      |                    +-> CaptureDecision
                                      |                       -> DecisionRecord.decision_event
                                      |                       -> adaptive_outputs
                                      v
                              decision/price read streams

CSVSource [F4] -> Pipeline.Run (alternative source, not the planned live path)

MongoDB durable ledger
    |
    +---- quantram_payloads ----> Snapshot Service exact-N counting
    |                                  |
    +---- quantram_decisions ----------+-> DecisionComplete
                                       |
                                       +-> Snapshot + SnapshotRun
    |
    +---- Aperture OPEN -> coordinated drain/final evaluation -> SHUT

Semantic service test-only artifact [F6] is outside the runtime topology.
```

The decisive ordering is `WindowStore.Add` -> `CaptureBar` -> general fanout -> model fanout. Price capture occurs after coordinated adaptive/pricing commit and before terminal decision capture for the same bar. Snapshot counting reads durable Payloads; eligibility additionally requires a durable `decision_event` for the trigger Payload.

## Classification And Impact Scales

General severity:

- S0: cosmetic.
- S1: minor correctness or diagnostic issue.
- S2: meaningful correctness issue.
- S3: significant runtime or scientific-integrity risk.
- S4: critical data-corruption or system-safety issue.

Persistence-test severity:

- P0: irrelevant.
- P1: observe.
- P2: test constraint or caution.
- P3: persistence-test blocker.

Impact ratings are NONE, LOW, MEDIUM, or HIGH. In each finding, any dimension not listed in the non-NONE impact paragraph was evaluated as **NONE**.

## F1-A: ResetSymbol Concurrency Concern

**FINDING:** F1-A, possible concurrent replacement of per-symbol engine state.

**FILE(S):** `internal/modelhost/host.go`, `internal/modelhost/host_test.go`, `internal/ingestion/model_path.go`.

**RELEVANT TYPES/FUNCTIONS:** `Host`, `worker`, `Host.Run`, `runWorker`, `handle`, `dispatch`, `ResetSymbol`, `Pipeline.ResetModelPath`.

**STATUS:** THEORETICAL RISK NOT CURRENTLY REPRODUCED.

**PROVENANCE:** PRE-EXISTING.

**GIT EVIDENCE:** `ResetSymbol` entered in `2c0a8c2` (`Input Gap fix changes`) and exists on `main`. The `main..HEAD` diff adds `EventCapture` and capture calls elsewhere in `host.go`; it does not change `ResetSymbol`. Git blame attributes the reset body to the pre-persistence commit.

**CODE PATH:** `Host.Run` starts one `runWorker` goroutine per symbol. `runWorker` calls `handle`, which reads and mutates `w.engine`, `w.pricing`, `w.hasAccepted`, and `w.lastAccepted`. `ResetSymbol` directly replaces and clears those same fields without routing through the worker inbox or taking a lock that covers them.

**TRIGGER:** An external goroutine calls `ResetSymbol(symbol)` while that symbol's worker is dispatching or handling a bar.

**NORMAL LIVE RUN EXPOSURE:** NO. Repository search found only the method definition and a sequential test caller. There is no gRPC/operator reset endpoint and no automatic reset on irregular intervals or gaps. A proven gap latches discontinuity; it does not call `ResetSymbol`.

**DATA AFFECTED:** If triggered, in-flight adaptive/pricing state and the resulting terminal events.

**SCIENTIFIC STATE AFFECTED:** CONDITIONAL.

**PERSISTED DATA AFFECTED:** CONDITIONAL. Persistence faithfully captures any event emitted from the raced state; Payload capture remains independent.

**SNAPSHOT AFFECTED:** CONDITIONAL. Counting is unaffected, but a trigger Payload could lack or receive an invalid terminal event if the race disrupts handling.

**APERTURE OPEN AFFECTED:** NO.

**APERTURE SHUT AFFECTED:** NO.

**DASHBOARD AFFECTED:** CONDITIONAL, through any raced event that reaches live or historical views.

**FUTURE EXECUTION AFFECTED:** CONDITIONAL; an eventual operator reset path would make the concurrency contract operationally relevant.

**GENERAL SEVERITY:** S3.

**PERSISTENCE TEST SEVERITY:** P2.

**PERSISTENCE TEST BLOCKER:** NO.

**RECOMMENDED ACTION FOR CURRENT TEST:** AVOID TRIGGERING.

**FUTURE REMEDIATION REQUIRED:** YES.

**RATIONALE:** The worker ownership model serializes ordinary symbol processing, but `ResetSymbol` bypasses that owner. The race is structurally possible, yet current production composition has no caller. The existing reset/replay test calls reset after collecting the preceding event and does not reproduce concurrent access. A race detector was not run, so this investigation does not claim a measured data race.

**NON-NONE IMPACT:** Scientific model state, D01, FMO, D02, D04, P-04/pricing, DecisionEvent correctness, PriceEvent correctness, Mongo Decision persistence, Mongo Price persistence, Snapshot eligibility, and future paper execution are **HIGH if triggered**. Dashboard historical playback is **MEDIUM if triggered** because it can faithfully display a bad persisted outcome. Market-feed integrity, accepted-Bar integrity, per-symbol input ordering, Mongo Payload persistence, Snapshot counting, Snapshot idempotency, Aperture OPEN, and Aperture SHUT are NONE.

## F1-B: computeCoherence Reproducibility

**FINDING:** F1-B, randomized map iteration controls floating-point accumulation order.

**FILE(S):** `internal/adaptive/coherence.go`, `internal/adaptive/d01.go`, `internal/adaptive/engine.go`, `internal/modelhost/host_test.go`.

**RELEVANT TYPES/FUNCTIONS:** `computeCoherence`, `stepD01`, `Model.StateHash`, `Engine.StateHash`, `TestUnitRun001Equivalence`, `TestResetSymbolReplayMatchesUninterrupted`.

**STATUS:** CONFIRMED DEFECT.

**PROVENANCE:** PRE-EXISTING.

**GIT EVIDENCE:** `computeCoherence` entered in `4896bad` (`Science Phases A - C done with Go refactoring`) and is identical on `main`; there is no `main..HEAD` executable diff for this file. The persistence branch only added documentation around it in the current worktree.

**CODE PATH:** `stepD01` constructs four-channel `evidence` and configuration weight maps. `computeCoherence` ranges over the evidence map and accumulates `num += w*value` and `den += w*abs(value)`. Go map order is unspecified and floating-point addition is not associative. Coherence then contributes to strength, uncertainty, DMO/D02/D04 values, and decisions.

**TRIGGER:** Any D01 step with evidence terms whose rounding differs under another legal map order. A visible downstream difference requires that rounding to survive later calculations or cross a policy boundary.

**NORMAL LIVE RUN EXPOSURE:** YES. Every D01 step uses this function, although most order differences may round to the same final values.

**DATA AFFECTED:** Scientific outputs and hashes; potentially terminal decision values near a boundary.

**SCIENTIFIC STATE AFFECTED:** YES.

**PERSISTED DATA AFFECTED:** YES for the run-specific scientific values captured in `decision_event` and `adaptive_outputs`; not for the source Payload.

**SNAPSHOT AFFECTED:** NO for counting, eligibility, and idempotency. Snapshot requires terminal decision presence, not cross-run numerical equality.

**APERTURE OPEN AFFECTED:** NO.

**APERTURE SHUT AFFECTED:** NO.

**DASHBOARD AFFECTED:** CONDITIONAL, where exact scientific values or replay comparison are displayed.

**FUTURE EXECUTION AFFECTED:** YES because deterministic replay is a stated promotion concern before outcomes are trusted for execution.

**GENERAL SEVERITY:** S2.

**PERSISTENCE TEST SEVERITY:** P1.

**PERSISTENCE TEST BLOCKER:** NO.

**RECOMMENDED ACTION FOR CURRENT TEST:** OBSERVE.

**FUTURE REMEDIATION REQUIRED:** YES.

**RATIONALE:** The nondeterministic accumulation mechanism is directly present in code. The existing D01 equivalence fixture passed 50 repetitions, so broad fixture equivalence remained inside its tolerances. The combined reset/replay hash test intermittently failed under 50 repetitions after passing normally. That is evidence of replay instability, but its assertion combines adaptive and pricing hashes and therefore does not isolate `computeCoherence` as the sole cause. No decision-side flip was reproduced. The persistence test can still prove that the exact outcome produced in one run is linked to and durably stored with its Payload.

**NON-NONE IMPACT:** Scientific model state, D01, FMO, D02, D04, DecisionEvent correctness, Mongo Decision persistence, dashboard historical replay, and future paper execution are **LOW**, elevated to **MEDIUM for reproducibility-dependent validation**. P-04/pricing calculations, PriceEvent correctness, market-feed integrity, accepted-Bar integrity, ordering, Mongo Payload/Price persistence, all Snapshot properties, and Aperture lifecycle are NONE.

## F2: Multi-Subscriber Model-Path Prefix Divergence

**FINDING:** F2, partial fanout before a full model-subscriber queue latches discontinuity.

**FILE(S):** `internal/ingestion/model_path.go`, `internal/ingestion/pipeline.go`, `internal/modelhost/host.go`.

**RELEVANT TYPES/FUNCTIONS:** `Pipeline.modelSubs`, `SubscribeModelBars`, `fanoutModel`, `Host.Run`.

**STATUS:** CONFIRMED LIMITATION.

**PROVENANCE:** PRE-EXISTING.

**GIT EVIDENCE:** `fanoutModel` entered in `042356a` (`P03 completed`) and exists unchanged on `main`. Later `2c0a8c2` work adjusted gap handling but did not originate the fanout behavior. The persistence branch does not change `model_path.go` executable code.

**CODE PATH:** `fanoutModel` holds the pipeline registry lock and iterates the `modelSubs` map. Each send is nonblocking. If one channel is full, it records `SkipQueueOverflow` and returns immediately. Subscribers visited before the full channel receive the triggering bar; the full subscriber and subscribers not yet visited do not. Because map order is unspecified, which prefix each subscriber receives is also unspecified.

**TRIGGER:** At least two active model-path subscribers and at least one full subscriber queue during delivery of a model-eligible Bar.

**NORMAL LIVE RUN EXPOSURE:** NO in current composition. `Host.Run` creates one model-path subscription. Persistence is a capture callback, not a model subscriber. gRPC/dashboard bar clients use the separate general subscriber path.

**DATA AFFECTED:** Model-delivery prefixes only when the unsupported multi-subscriber condition exists.

**SCIENTIFIC STATE AFFECTED:** CONDITIONAL.

**PERSISTED DATA AFFECTED:** CONDITIONAL for later scientific outcomes, but not Payloads. `CaptureBar` occurs before model fanout.

**SNAPSHOT AFFECTED:** CONDITIONAL. Durable count remains correct; eligibility can wait if the trigger Payload never receives a terminal decision from an affected model consumer.

**APERTURE OPEN AFFECTED:** NO.

**APERTURE SHUT AFFECTED:** NO.

**DASHBOARD AFFECTED:** NO in the current topology.

**FUTURE EXECUTION AFFECTED:** CONDITIONAL if a second model consumer is introduced.

**GENERAL SEVERITY:** S2.

**PERSISTENCE TEST SEVERITY:** P0.

**PERSISTENCE TEST BLOCKER:** NO.

**RECOMMENDED ACTION FOR CURRENT TEST:** IGNORE.

**FUTURE REMEDIATION REQUIRED:** YES before broadening model-path consumers.

**RATIONALE:** Prefix divergence follows directly from partial iteration plus early return, but the necessary multi-subscriber topology does not exist. The current live test uses one `Host`, one `SubscribeModelBars` call, and independent persistence callbacks. The first accepted-Bar capture is therefore insulated. Existing tests cover one-subscriber overflow, not multi-subscriber fairness.

**NON-NONE IMPACT:** Per-symbol ordering is **HIGH under the trigger**. Scientific state, D01, FMO, D02, D04, P-04, DecisionEvent/PriceEvent correctness, Mongo Decision/Price persistence, Snapshot eligibility, and future execution are **MEDIUM to HIGH conditionally**, depending on which model consumer misses the bar. All other required dimensions are NONE.

## F3: WebSocket EventCount uint32 Truncation

**FINDING:** F3, unchecked `uint64` to `uint32` conversion for Alpaca WebSocket trade count.

**FILE(S):** `internal/marketfeed/decode.go`, `internal/domain/bar.go`, `internal/server/server.go`.

**RELEVANT TYPES/FUNCTIONS:** `alpacaBar.Trades`, `barFromRaw`, `barFromAlpaca`, `parseUint`, `parseUint32`, `domain.Bar.EventCount`.

**STATUS:** CONFIRMED DEFECT.

**PROVENANCE:** PRE-EXISTING.

**GIT EVIDENCE:** `parseUint32` and its ignored error call entered in `d686b8a` (`Alpaca api and data ingestion P0-P01 started`) and exist on `main`. No persistence-branch executable diff touches this conversion.

**CODE PATH:** WebSocket JSON field `n` is retained as `json.Number`, parsed to `uint64`, cast to `uint32` without a range check, and assigned to `Bar.EventCount`. The returned parse error is discarded. `EventCount` is then serialized as `uint32` over gRPC and as the Bar field in MongoDB.

**TRIGGER:** A nonnegative WebSocket `n` value greater than `math.MaxUint32`, exactly **4,294,967,295**. For example, 4,294,967,296 converts to 0 modulo $2^{32}$. Invalid or fractional representations can also be reduced to zero or truncated because the conversion error is ignored.

**NORMAL LIVE RUN EXPOSURE:** CONDITIONAL but not credible for the planned symbols under ordinary conditions. The threshold is more than 4.29 billion contributing trades in one one-minute bar. This repository contains no provider-limit evidence, and no live measurement was made; the investigation therefore does not assign a speculative occurrence probability.

**DATA AFFECTED:** Trade/event count only. This is not volume. Both final (`b`) and updated (`u`) WebSocket bar messages pass through the conversion. REST uses a `uint32` destination during JSON unmarshal, so out-of-range REST input is rejected rather than silently cast by `parseUint32`.

**SCIENTIFIC STATE AFFECTED:** NO. D01 consumes close, volume, and event time; `EventCount` is not a scientific input.

**PERSISTED DATA AFFECTED:** YES, limited to the Payload's incorrect `bar.event_count`.

**SNAPSHOT AFFECTED:** NO. Counting uses Payload rows and eligibility uses terminal decision presence, not event count.

**APERTURE OPEN AFFECTED:** NO.

**APERTURE SHUT AFFECTED:** NO.

**DASHBOARD AFFECTED:** CONDITIONAL if trade count is displayed or audited.

**FUTURE EXECUTION AFFECTED:** NO under current contracts; CONDITIONAL if event count becomes an execution or science input.

**GENERAL SEVERITY:** S1.

**PERSISTENCE TEST SEVERITY:** P1.

**PERSISTENCE TEST BLOCKER:** NO.

**RECOMMENDED ACTION FOR CURRENT TEST:** OBSERVE.

**FUTURE REMEDIATION REQUIRED:** YES.

**RATIONALE:** The conversion defect is certain above the exact boundary, but the planned test does not require event count for model or Snapshot correctness and is not expected to approach the boundary. `MarketSnapshotID` does not include event count, so that identity cannot reveal truncation; direct plausibility review of the persisted field is the available evidence.

**NON-NONE IMPACT:** Market-feed integrity, accepted-Bar field fidelity, Mongo Payload persistence, and dashboard historical playback are **LOW**. All scientific stages, terminal outputs, Mongo Decision/Price persistence, Snapshot behavior, ordering, Aperture lifecycle, and current future-execution behavior are NONE.

## F4: CSV Source Health After Terminal Parse Failure

**FINDING:** F4, some terminal CSV errors return while source health remains `HEALTHY`.

**FILE(S):** `internal/marketfeed/csv_source.go`, `internal/ingestion/pipeline.go`, `internal/server/server.go`.

**RELEVANT TYPES/FUNCTIONS:** `CSVSource`, `NewCSVSource`, `Run`, `setState`, `barFromCSV`, `Pipeline.Run`, `GetFeedHealth`, `GetHealth`.

**STATUS:** CONFIRMED DEFECT.

**PROVENANCE:** PRE-EXISTING.

**GIT EVIDENCE:** `CSVSource.Run` and the affected returns entered in `d686b8a` and exist on `main`. The persistence branch has no executable change in this file.

**CODE PATH:** Construction initializes health to `HEALTHY`; `Run` sets it healthy again. File-open and row-reader errors set `FAILED`. Header-read failure, header mismatch, and `barFromCSV` field/timestamp/volume errors return without changing health. `Pipeline.Run` receives the returned error, drains already queued Bars, observes the still-healthy source once, sets inference false, and returns it. The server-owned pipeline goroutine logs that error and exits; the gRPC server remains running until its own completion or a shutdown signal.

**TRIGGER:** CSV header read failure, unexpected header, malformed row width, malformed OHLCV field, or malformed timestamp.

**NORMAL LIVE RUN EXPOSURE:** NO. The planned test uses Alpaca live input, not `CSVSource`.

**DATA AFFECTED:** Health reporting. Valid Bars emitted before failure remain valid and are delivered; the malformed row is not accepted or persisted. The source error terminates the pipeline goroutine and is logged, but does not itself stop the server.

**SCIENTIFIC STATE AFFECTED:** NO for the planned live path; CONDITIONAL in a CSV replay because processing stops at the bad row.

**PERSISTED DATA AFFECTED:** NO corruption. A CSV replay can end with only the valid prefix persisted.

**SNAPSHOT AFFECTED:** NO corruption; a finite prefix can alter whether an exact-N candidate is reached in a CSV replay.

**APERTURE OPEN AFFECTED:** NO.

**APERTURE SHUT AFFECTED:** NO. The pipeline error neither closes nor bypasses the Aperture; a later gRPC completion or shutdown signal still enters coordinated shutdown.

**DASHBOARD AFFECTED:** CONDITIONAL because feed-health state can remain stale while the server continues serving after the pipeline has stopped.

**FUTURE EXECUTION AFFECTED:** NO for Alpaca operation.

**GENERAL SEVERITY:** S1.

**PERSISTENCE TEST SEVERITY:** P0.

**PERSISTENCE TEST BLOCKER:** NO.

**RECOMMENDED ACTION FOR CURRENT TEST:** IGNORE.

**FUTURE REMEDIATION REQUIRED:** YES.

**RATIONALE:** This is incorrect diagnostic state, not silent acceptance of malformed data. The terminal error is propagated and logged. The planned live persistence test does not instantiate this source.

**NON-NONE IMPACT:** Market-feed integrity and dashboard health reporting are **LOW**. A CSV-only replay's accepted prefix, per-symbol progression, scientific state, terminal outputs, Mongo records, and Snapshot threshold reachability are **LOW conditionally** because the run terminates early, not because malformed data is delivered. All planned Alpaca-test and Aperture dimensions are NONE.

## F5: StreamBars Window/Subscription Duplication

**FINDING:** F5, the same accepted Bar can appear in catch-up and subscribed delivery.

**FILE(S):** `internal/server/server.go`, `internal/ingestion/pipeline.go`, `internal/ingestion/window.go`, `internal/domain/bar.go`.

**RELEVANT TYPES/FUNCTIONS:** `Server.StreamBars`, `Pipeline.Subscribe`, `Pipeline.SubscribeFinalized`, `Pipeline.Window`, `Pipeline.fanout`, `Bar.DedupKey`.

**STATUS:** CONFIRMED LIMITATION.

**PROVENANCE:** PRE-EXISTING.

**GIT EVIDENCE:** `StreamBars` entered in `d686b8a` and exists on `main`. The persistence branch adds Snapshot service composition to `server.go`, not StreamBars ordering or deduplication. The current explanatory comment was added by the documentation pass and does not create the behavior.

**CODE PATH:** `StreamBars` subscribes first, then calls `Window` for each catch-up symbol, then drains the subscription. An accepted Bar that reaches the new subscriber after subscription registration but before its symbol's `Window` snapshot is copied can be present in both the copied window and the subscription queue. There is no stream-level seen set. An arrival after that snapshot copy is only queued and is not duplicated by that snapshot.

**TRIGGER:** A Bar is accepted in the subscribe-to-window-copy overlap interval. Multiple requested symbols extend the interval for symbols whose window is copied later.

**NORMAL LIVE RUN EXPOSURE:** CONDITIONAL for clients that open `StreamBars` while live Bars arrive.

**DATA AFFECTED:** Read-side delivery only. The duplicate can be recognized by identical `market_snapshot_id`; `symbol` plus `interval_start` (`DedupKey`) identifies the interval but can also group legitimate replacement generations.

**SCIENTIFIC STATE AFFECTED:** NO. Science consumes `SubscribeModelBars`, not the gRPC method.

**PERSISTED DATA AFFECTED:** NO. `CaptureBar` occurs before general fanout and Mongo upserts by Aperture plus `market_snapshot_id`.

**SNAPSHOT AFFECTED:** NO.

**APERTURE OPEN AFFECTED:** NO.

**APERTURE SHUT AFFECTED:** NO.

**DASHBOARD AFFECTED:** YES. Current-Aperture charts can display a duplicate unless the client deduplicates. REV/FWD playback or chart continuity is at risk if it treats this observer stream as authoritative history.

**FUTURE EXECUTION AFFECTED:** CONDITIONAL and potentially serious only if future execution incorrectly consumes this read surface.

**GENERAL SEVERITY:** S2.

**PERSISTENCE TEST SEVERITY:** P2.

**PERSISTENCE TEST BLOCKER:** CONDITIONAL. It blocks only a test design that treats StreamBars counts as authoritative evidence of accepted or persisted counts.

**RECOMMENDED ACTION FOR CURRENT TEST:** AVOID TRIGGERING as an evidence error: do not use StreamBars counts as the persistence oracle.

**FUTURE REMEDIATION REQUIRED:** YES.

**RATIONALE:** The overlap follows from subscription-before-snapshot ordering and the absence of a seen set. The ordering prevents loss during catch-up but permits duplicates. It is downstream of acceptance, science, and persistence capture, so it cannot create duplicate Payloads or scientific steps. The upcoming test remains valid when MongoDB identities and records are authoritative.

**NON-NONE IMPACT:** Dashboard historical playback and chart continuity are **MEDIUM**. Future paper execution is **HIGH conditionally** only if it consumes StreamBars without deduplication. Every feed, accepted-Bar, ordering, science, persistence, Snapshot, and Aperture dimension is NONE.

## F6: Redundant Empty Semantics-Test Conditional

**FINDING:** F6, an empty conditional repeats the real assertion's comparison.

**FILE(S):** `internal/server/semantics_test.go`. The originally reported path `internal/semantics/semantics_test.go` does not exist.

**RELEVANT TYPES/FUNCTIONS:** `TestSemanticServiceGetListContract`.

**STATUS:** CONFIRMED DEFECT.

**PROVENANCE:** PRE-EXISTING.

**GIT EVIDENCE:** The test and empty conditional entered directly on `main` in `1e2becd` (`Go semantic dictionary builder implemented`). The branch has no change to this file.

**CODE PATH:** The condition is equivalent to `A != B && B != A`, which reduces to `A != B`, but the body contains only a comment. The immediately following conditional performs the same comparison and calls `t.Fatalf` on mismatch.

**TRIGGER:** Any execution of the semantics service test reaches the code; it has no runtime trigger.

**NORMAL LIVE RUN EXPOSURE:** NO.

**DATA AFFECTED:** None.

**SCIENTIFIC STATE AFFECTED:** NO.

**PERSISTED DATA AFFECTED:** NO.

**SNAPSHOT AFFECTED:** NO.

**APERTURE OPEN AFFECTED:** NO.

**APERTURE SHUT AFFECTED:** NO.

**DASHBOARD AFFECTED:** NO.

**FUTURE EXECUTION AFFECTED:** NO.

**GENERAL SEVERITY:** S0.

**PERSISTENCE TEST SEVERITY:** P0.

**PERSISTENCE TEST BLOCKER:** NO.

**RECOMMENDED ACTION FOR CURRENT TEST:** IGNORE.

**FUTURE REMEDIATION REQUIRED:** YES, as maintenance only.

**RATIONALE:** The empty branch does not weaken the test or hide failure because the following assertion enforces the intended contract-count equality. Production semantics are unaffected.

**NON-NONE IMPACT:** None. All required impact dimensions are NONE.

## Persistence Architecture Isolation

### Capture Boundaries

**Accepted Bar -> CaptureBar**

`Pipeline.accept` first requires `WindowStore.Add` to accept or replace the Bar. It then calls `CaptureBar` before general or model fanout. The asynchronous store validates `market_snapshot_id`, enqueues without waiting for capacity, and `MongoWriter.WriteBar` upserts by Aperture plus market snapshot identity.

**Committed Price -> CapturePrice**

The model host prepares adaptive and pricing work from committed states. When both permit commit, it commits both, assigns the price event ID, and calls `CapturePrice` before terminal adaptive event capture. Mongo resolves the Payload by `market_snapshot_id` and upserts `price_event` into the shared DecisionRecord.

**Committed Decision -> CaptureDecision**

After coordinated commit and any price capture, the host calls `CaptureDecision` with the terminal DecisionEvent and retained adaptive outputs. Mongo resolves the same Payload and upserts `decision_event` plus optional `adaptive_outputs`.

| Finding | Relative to Capture Boundary | Persistence Consequence |
|---|---|---|
| F1-A ResetSymbol | Upstream of Price/Decision capture; unrelated to Bar capture | If externally triggered concurrently, persistence can faithfully store a raced outcome. Current composition has no trigger. |
| F1-B computeCoherence | Upstream of Decision capture; unrelated to Bar capture | Stores the exact run-specific output, but does not guarantee cross-run numerical identity. |
| F2 model fanout | Downstream of Bar capture; upstream of Price/Decision capture | Payload remains durable; a hypothetical affected model subscriber can miss output production. Current topology has one subscriber. |
| F3 event-count conversion | Upstream of Bar capture | An out-of-range WebSocket count is already wrong when persisted; all correlation and science inputs remain intact. |
| F4 CSV health | Upstream of capture but outside planned source | Malformed rows are not captured; a valid prefix may be persisted before the propagated error. |
| F5 StreamBars | Downstream of all relevant capture | Can duplicate client observations but cannot duplicate Payloads or model steps. |
| F6 test conditional | Unrelated | No consequence. |

Despite these findings, the persistence mechanism can still validly prove queue acceptance and drain behavior, Aperture/symbol/identity correlation, shared Payload-to-outcome linkage, exact-N durable counting, Snapshot/Run persistence, final evaluation, and OPEN-to-SHUT closure. It cannot by itself prove cross-run scientific determinism or observer-stream uniqueness.

## QuanTRAM Pre-Existing Findings Matrix

| # | Finding | Classification | Provenance | General Severity | Persistence Test Severity | Test Blocker? | Future Fix? |
|---|---|---|---|---|---|---|---|
| F1-A | ResetSymbol concurrent mutation | THEORETICAL RISK NOT CURRENTLY REPRODUCED | PRE-EXISTING | S3 | P2 | NO | YES |
| F1-B | computeCoherence reproducibility | CONFIRMED DEFECT | PRE-EXISTING | S2 | P1 | NO | YES |
| F2 | Model-path subscriber prefix divergence | CONFIRMED LIMITATION | PRE-EXISTING | S2 | P0 | NO | YES |
| F3 | WebSocket EventCount truncation | CONFIRMED DEFECT | PRE-EXISTING | S1 | P1 | NO | YES |
| F4 | CSV terminal-error health state | CONFIRMED DEFECT | PRE-EXISTING | S1 | P0 | NO | YES |
| F5 | StreamBars overlap duplication | CONFIRMED LIMITATION | PRE-EXISTING | S2 | P2 | CONDITIONAL | YES |
| F6 | Redundant empty test conditional | CONFIRMED DEFECT | PRE-EXISTING | S0 | P0 | NO | YES |

## Live Persistence Test Readiness

### CAN WE PROCEED WITH THE CONTROLLED LIVE MARKET-FEED MONGODB/APERTURE/SNAPSHOT PERSISTENCE TEST WITHOUT FIXING THESE FINDINGS?

**YES WITH CONDITIONS**

The conditions necessary to preserve test validity are:

1. Do not invoke `ResetSymbol` or add a custom reset caller during the run.
2. Retain the current single `SubscribeModelBars` consumer topology.
3. Treat the test as proof of same-run persistence mechanics, not proof of cross-run scientific numerical reproducibility.
4. Use MongoDB Payload, DecisionRecord, Snapshot, SnapshotRun, and Aperture identities/counts as authoritative evidence; do not use `StreamBars` delivery count as the persistence oracle.

These are test-validity constraints, not a general production-hardening list. F3's exact threshold and F4's CSV-only path do not require additional conditions for the planned Alpaca live test.

## Existing Test Observability

Only observability already present in current code is listed.

| Observable | Existing evidence | What it can establish |
|---|---|---|
| Server logs | feed termination, model skips/decisions, pricing emissions, persistence retry exhaustion, Snapshot run failures, signal receipt, graceful/forced gRPC stop | Whether discontinuity, capture failure, Snapshot failure, or shutdown-path events occurred. Logs do not expose successful queue counts. |
| `GetFeedHealth` / `GetActiveSource` | source ID, state, last message, pong RTT, heartbeat failures, last error | Live Alpaca connectivity and heartbeat condition; F4 is relevant only under CSV. |
| `GetHealth` / `GetReadiness` | ingestion, model, pricing component state and observe/infer gates | Whether the scientific path was paused, unavailable, or discontinuous. Persistence health is not included. |
| Payload documents | `_id`, `aperture_id`, complete Bar, `market_snapshot_id`, symbol, interval timestamps | Accepted-Bar durability, order, source identity, F3 event-count plausibility, and Aperture correlation. |
| DecisionRecord documents | `payload_id`, `aperture_id`, optional `decision_event`, `adaptive_outputs`, `price_event` | Same-Payload correlation and capture of committed Price/Decision outcomes. |
| Snapshot documents | Aperture, policy, Payload, symbol, `snapshot_num`, capture time | Exact-N checkpoint identity and idempotent checkpoint persistence. |
| SnapshotRun documents | trigger Payload/count, STARTED/SUCCESS/ERROR, times, Snapshot ID/error | Attempt lifecycle, retry outcome, and final checkpoint audit. |
| Snapshot RPCs | list/get policies, Snapshots, and SnapshotRuns | Provider-backed read access to existing durable Snapshot evidence. |
| Aperture document | `_id`, `sequence_num`, `open`, nullable `shut`, `status` | One-run lineage and truthful OPEN -> SHUT result. |
| `AsyncStore.Health` | queue depth, dropped, written, failures, last error | Exists internally, but is not exposed by current gRPC health composition; it is not an external live-test observable without code changes. |

For F1-B, exact persisted adaptive values and hashes can be inspected, but there is no existing runtime signal that labels a value nondeterministic. For F5, repeated `market_snapshot_id` values reveal duplicate observer delivery if the client retains the stream, but the stream must not be used as authoritative persistence evidence.

## Future Remediation Backlog

No backlog item is authorized for implementation by this document.

### Priority A: Before Execution Or Paper Trading

| Finding | Reason | Likely isolated subsystem | Expected future validation surface |
|---|---|---|---|
| F1-A ResetSymbol concurrency | An eventual operator/execution reset path would violate per-worker state ownership if called concurrently. | `internal/modelhost` and reset caller contract | Race-enabled concurrent reset/step tests; state and event atomicity checks. |
| F1-B computeCoherence reproducibility | Scientific replay must have stable accumulation order before persisted outcomes are promoted as reproducible execution evidence. | `internal/adaptive` | Repeated exact replay, state-hash comparison, frozen equivalence and boundary-decision tests. |

### Priority B: Before Broader Runtime Or Dashboard Validation

| Finding | Reason | Likely isolated subsystem | Expected future validation surface |
|---|---|---|---|
| F2 model-path multi-subscriber divergence | Additional scientific consumers would receive inconsistent prefixes under overflow. | `internal/ingestion` | Two-or-more subscriber overflow and prefix-consistency tests. |
| F3 EventCount conversion | Provider metadata should not silently wrap before persistence. | `internal/marketfeed` | Boundary tests at 4,294,967,295 and 4,294,967,296; WebSocket/REST error behavior. |
| F4 CSV health | Terminal replay errors should agree with reported source health. | `internal/marketfeed`, ingestion health projection | Existing-source tests for header, row, timestamp, and read failures plus health RPC assertions. |
| F5 StreamBars overlap | Current-Aperture and replay dashboards need unique, continuous observer delivery. | `internal/server` streaming boundary | Controlled arrival between subscribe/window snapshot; multi-symbol catch-up; identity-based duplicate assertions. |

### Priority C: Maintenance And Cleanup

| Finding | Reason | Likely isolated subsystem | Expected future validation surface |
|---|---|---|---|
| F6 redundant conditional | Cosmetic dead test structure obscures the real assertion. | `internal/server` tests | Existing semantic service test suite. |

## Known Limitations

1. No race-enabled test was run; F1-A remains a code-backed concurrency risk rather than a measured race report.
2. No diagnostic test was created to isolate the adaptive and pricing components of the repeated reset/replay hash failure. The existing combined assertion limits root-cause attribution.
3. The exact decision-level consequence of alternate `computeCoherence` accumulation orders was not reproduced; observed evidence establishes replay hash instability, not a BUY/SELL/HOLD flip.
4. F2 was reasoned from the implementation and current one-subscriber topology; no existing multi-subscriber test reproduces it.
5. F5 was reasoned from the subscribe/window ordering; no existing test deterministically schedules a Bar in the overlap interval.
6. No provider contract or live sample was consulted to establish a maximum Alpaca `n` value. The exact code boundary is known; real-world incidence is not.
7. No live server, MongoDB, market feed, signal delivery, or dashboard was exercised. The persistence lifecycle conclusions are static and existing-test based.

## Authoritative Boundary

This document is an **INVESTIGATION record**.

It does not redefine:

- QuanTRAM architecture;
- scientific mathematics;
- persistence contracts;
- Aperture semantics; or
- Snapshot semantics.

The current definitive specifications remain authoritative:

- [MongoDB/Aperture Persistence V1 Design](../design/QuanTRAM_MONGODB_APERTURE_PERSISTENCE_V1_090326.md)
- [MongoDB/Aperture Implementation](../implementations/QuanTRAM_MONGODB_APERTURE_IMPLEMENTATION_090326.md)
- [Snapshot Service V1 Design](../design/QuanTRAM_SNAPSHOT_SERVICE_V1_090326.md)
- [Snapshot Service Implementation](../implementations/QuanTRAM_SNAPSHOT_SERVICE_IMPLEMENTATION_090326.md)

This investigation records observed code behavior, Git provenance, risk, persistence-test relevance, and future remediation classification. Where an explanatory source comment and executable ordering differ, executable code and the definitive specifications control this investigation.

## Final Repository-State Contract

The only repository change authorized and caused by this investigation is:

`docs/investigations/QUANTRAM_PREEXISTING_CODE_FINDINGS_INVESTIGATION_2026-09-04.md`

| Validation item | Result |
|---|---|
| Other files created | NO |
| Existing files modified | NO |
| Production code modified | NO |
| Test code modified | NO |
| Proto modified | NO |
| Scripts modified | NO |
| Live server started | NO |
| MongoDB contacted | NO |
| Market feed contacted | NO |

## Change Log

| Date | Version | Change | Author/Process |
|---|---|---|---|
| 2026-09-04 | V1.0 | Initial read-only investigation of code findings discovered during the repository-wide QuanTRAM source documentation audit. | GitHub Copilot read-only investigation process |