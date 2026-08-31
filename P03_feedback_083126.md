# QuanTRAM P-03 Adaptive Model Host Feedback

**Date:** August 31, 2026  
**Requested scope:** Completeness review for P-03 Adaptive Model Host consuming P-02 Finalized Bar Streams  
**Assessment:** Scientifically actionable; host and stream contracts require clarification before P-03 can be considered causally complete

## Purpose

This document reviews the proposed P-03 Adaptive Model Host and implementation increment for completeness against the current QuanTRAM process model, P-02 data-quality design, decision-integrity gap register, and implemented Go ingestion behavior.

The review focuses on whether P-03 can safely consume finalized bars, preserve per-symbol causal order, reproduce the SADE adaptive path, emit unambiguous decisions or skips, and provide enough evidence to diagnose and replay its behavior. It does not evaluate the trading merit of the adaptive model and does not expand P-03 into risk, execution, or the later RK45/PriceEngine increment.

## Documents and Code Reviewed

Primary documents:

- `C:\Users\chino\QuanTRAM\docs\design\QuanTRAM_P03_ADAPTIVE_MODEL_HOST_083126.md`
- `C:\Users\chino\QuanTRAM\docs\design\QuanTRAM_P03_IMPLEMENTATION_083126.md`
- `C:\Users\chino\QuanTRAM\docs\design\QuanTRAM_PROCESS_MODEL_082926.md`
- `C:\Users\chino\QuanTRAM\docs\design\QuanTRAM_INGESTION_P02_DATA_QUALITY_083126.md`
- `C:\Users\chino\QuanTRAM\docs\design\QuanTRAM_INGESTION_INCREMENT_1_083026.md`
- `C:\Users\chino\QuanTRAM\docs\design\QuanTRAM_DECISION_INTEGRITY_GAP_ANALYSIS_082826.md`
- `C:\Users\chino\QuanTRAM\docs\design\QuanTRAM_hi-level_design_082826.md`
- `C:\Users\chino\QuanTRAM\docs\design\E2E_QuanTRAM_ARTIFACTS.md`

Implementation surfaces checked:

- `C:\Users\chino\QuanTRAM\internal\ingestion\pipeline.go`
- `C:\Users\chino\QuanTRAM\internal\ingestion\window.go`
- `C:\Users\chino\QuanTRAM\internal\domain\bar.go`
- `C:\Users\chino\QuanTRAM\internal\domain\quality.go`
- `C:\Users\chino\QuanTRAM\api\proto\quantram\v1\quantram.proto`

Scientific and supporting references identified by the design documents:

- `C:\Users\chino\SADE` adaptive D01, D02, D04, emitter, baseline, and Unit Run 001 artifacts
- SADE Go refactorability and scaled-runtime investigation dated August 27, 2026

## Executive Assessment

The P-03 documents make several strong decisions:

- P-03 consumes only the P-02 finalized path.
- The Go host owns the proved adaptive D01 → D02 → D04 → emitter path.
- P-03 does not reconnect the live path to SDX.
- P-04 and `ModelInferenceService` are intentionally deferred to the later RK45/PriceEngine join.
- Scientific state is isolated per symbol.
- Ineligible bars, initialization, errors, and deadline misses produce explicit skips rather than stale decision reuse.
- The Python Unit Run 001 sequence is the numerical authority for the Go port.
- Risk, order routing, and broker submission remain outside P-03.

These decisions are coherent and sufficient to start Phase A and most of the science port. P-04 is **not** a blocker for this increment because the P-03 design explicitly amends the older process-model topology for the adaptive-only Go implementation.

The principal remaining gap is not the model mathematics. It is the reliability and observability of the boundary between P-02 and P-03. The current finalized subscriber is a bounded, lossy channel. A P-03 implementation that merely calls `SubscribeFinalized(2)` cannot claim that it evaluates every eligible finalized bar, preserves an unbroken state trajectory, or detects every dropped input. That matters because D01 is recursive: dropping one bar changes every later state and decision even if processing resumes in timestamp order.

P-03 should therefore be described as **ready for scientific implementation but not yet complete for live finalized-stream integration**. The critical delivery, startup, deadline, provenance, and output-event decisions below should be closed before Phase D is accepted.

## What Is Already Complete and Sound

### Adaptive ownership and P-04 boundary

The design correctly narrows this increment to the proved adaptive path in Go. The older process model describes P-03 calling a Python P-04 worker, but the newer P-03 design explicitly amends that topology:

- adaptive D01/D02/D04/emitter is local Go code
- no Python adaptive sidecar is required
- RK45/PriceEngine remains a later scientific increment
- `ModelInferenceService` should not be implemented yet

The future P-03/P-04 request-response contract will need a separate design when pricing is added, but it should not block the current adaptive host.

### Input quality intent

The intended gate is clear and conservative. A bar is evaluated only when it:

1. arrives on the finalized path
2. is `COMPLETE`
3. is not backfilled
4. passes the current `infer` capability gate
5. is strictly newer than the last accepted interval for that symbol

Scientific state remains unchanged for a skipped input. This is the correct default for avoiding unfinalized, reconstructed, duplicate, and regressive data.

### Finalization precedence

The current P-02 policy is stronger than the original increment:

- forming `u` may replace forming `u`
- final `b` replaces forming `u`
- forming `u` after final `b` is rejected
- REST reconstructed data cannot replace live complete data
- bars are retained in chronological interval order

Therefore ordinary Alpaca forming updates do not revise an already accepted live finalized bar. A generalized correction/version contract remains a future DI-02 concern, but it is not accurate to characterize current P-02 as freely replacing finalized live bars.

### Scientific equivalence

The proposed frozen-sequence comparison is appropriate:

- same 100 AAPL observations as SADE Unit Run 001
- same operation order and constants
- exact decision/status comparison
- documented floating-point tolerance for transcendental operations
- no dependency on live IEX or SDX during CI

The requirement to investigate a threshold flip rather than widening tolerance is particularly important.

### State and scope boundaries

Per-symbol D01 state, 15-context, and emitter position state are bounded and operationally reasonable. Excluding unbounded SADE diagnostics and historical pricing refits prevents accidental growth and latency on the live path.

## Priority Findings

### Critical Findings

#### C1. Finalized-bar delivery is lossy and can silently change the recursive model trajectory

The implementation plan directs P-03 to call `pipeline.SubscribeFinalized(2)`. The current P-02 `fanout` implementation uses a drop-oldest policy when a subscriber channel is full, then attempts to enqueue the newest bar.

This creates the following possible sequence:

```text
P-02 finalizes bar N
P-02 finalizes bar N+1
P-02 finalizes bar N+2 while P-03 is busy
subscriber drops N and retains newer bars
P-03 resumes at N+1 or N+2
```

P-03's `interval_start > last_interval_start` check detects duplicates and regressions, but it does not prove adjacency and does not detect a missing minute by itself. D01 is recursive, so evaluating N+2 without N+1 is not merely a missed decision; it creates a different state trajectory for all subsequent observations.

There is also a documentation/code mismatch. The P-02 document says finalized-subscriber overflow clears `infer`. In the current `fanout` code, `infer` is cleared only if the second send fails after the oldest queued item is removed. If dropping the oldest makes room for the newest, the loss occurs without clearing `infer`.

**Required decision:** Introduce a distinct P-03 delivery contract. Recommended Phase 0 behavior:

- finalized P-03 delivery must never silently drop an eligible bar
- each message must carry or imply a per-symbol expected interval/sequence
- overflow or an interval gap must immediately mark that symbol's model state discontinuous
- P-03 must stop evaluating that symbol until state is deterministically rebuilt or explicitly reset
- the observe/dashboard stream may remain best effort; it is a different contract

A small blocking queue can be acceptable at one-minute cadence if it is isolated from the WebSocket reader and bounded by the 200 ms host deadline. A stronger option is a per-symbol mailbox plus explicit sequence/gap detection and rebuild. A durable bus is unnecessary for this local increment, but silent drop-oldest is not sufficient for recursive inference.

**Acceptance test:** Force the P-03 consumer to stall while three finalized bars are published to a depth-2 transport. The test must prove either all bars are processed in order or the model enters a named discontinuous/gated state before any later bar is evaluated.

#### C2. Startup, restart, and catch-up semantics are not defined

`SubscribeFinalized` delivers future fan-out only. It does not send the current P-02 window as catch-up. If P-03 starts after P-02 already accumulated eligible bars, or if only the model host restarts, the engine starts empty and requires about 16 new eligible minutes before its first actionable emission.

The design states that process start resets the engine and estimates a 16-minute warm-up, which is internally consistent, but the operational contract is incomplete:

- Is a cold 16-minute warm-up always intended?
- May P-03 seed from the current chronological P-02 window?
- If seeded, what exact snapshot and watermark prevent a race between window retrieval and live subscription?
- Are reconstructed bars allowed for state warm-up even though they are prohibited from live evaluation?
- Is a host restart distinguishable from a new market session?

**Recommended Phase 0 decision:** Choose one explicit mode.

**Cold-start mode:** Subscribe first, ignore historical window contents, initialize only from newly finalized eligible bars, and report `INITIALIZING n/16`. This is simplest and safest but imposes a known warm-up after every process restart.

**Deterministic rebuild mode:** Subscribe first, capture a watermark, retrieve a chronological finalized window, rebuild through the same engine path under documented eligibility rules, deduplicate queued overlap by symbol and interval, then continue live. This shortens recovery but requires a precise snapshot/overlap algorithm.

Do not leave the choice implicit. The host status and tests must reveal which mode is active.

#### C3. A context deadline does not automatically stop synchronous CPU-bound `engine.Step`

The implementation requires a 200 ms deadline but sketches `engine.Step(bar)` as a direct call. A Go `context.WithTimeout` around a synchronous CPU function does not interrupt it unless the function cooperatively checks the context or runs in separately controlled work.

This matters for both latency and state integrity. If a timed-out step continues mutating engine state after P-03 emits `TIMEOUT`, the next bar may observe state from a computation that was declared skipped.

**Required contract:** Define atomic step behavior:

- `Step` must either commit one complete state transition or leave state unchanged
- timeout/error/panic must not partially mutate committed per-symbol state
- no second step may run concurrently for the same symbol
- timeout handling must not leave an uncontrolled goroutine mutating state

**Recommended implementation:** Compute against a copy of the per-symbol state and commit only after successful completion within the deadline. Pass context into expensive loops if cancellation points are meaningful. Given the small scalar model, also treat any 200 ms breach as a model-health defect rather than routine flow control.

**Acceptance test:** Inject a deliberately slow step that attempts a state mutation after the deadline. Verify that P-03 emits exactly one `TIMEOUT`, does not emit a DecisionVector for that interval, and the committed state hash remains unchanged.

#### C4. Global `infer` gating conflicts with per-symbol model ownership unless explicitly intended

P-03 owns independent engines per symbol, but the current P-02 `Readiness().Infer` is global: it is true only when every configured symbol satisfies continuity, eligibility, and freshness. One missing or stale symbol therefore gates AAPL, MSFT, and NVDA together.

That may be an intentional conservative Phase 0 policy, but it is not stated in the P-03 documents. The design language can be read as though each bar is gated per symbol.

**Required decision:** State whether inference readiness is:

- global across the configured universe, or
- per symbol, with independent eligibility and pause/recovery

For a model explicitly designed to scale and fail independently by symbol, per-symbol readiness is the more natural eventual contract. If global gating is retained for Phase 0, document that one symbol pauses all P-03 engines and add a test proving scientific state is paused, not reset.

### High-Priority Findings

#### H1. The canonical event-time source should be `IntervalStart`, not reparsing provider text

The field map proposes parsing `Bar.SourceTimestamp` for D01 event time while using `Bar.IntervalStart` for ordering. `SourceTimestamp` is preserved provider text; `IntervalStart` is already the normalized UTC domain time used by P-02 for window ordering, continuity, deduplication, and finalization.

Using two representations for model time creates avoidable failure modes: parse differences, provider-format changes, and disagreement between ordering time and scientific event time.

**Recommendation:** Use `Bar.IntervalStart` as D01 `event_time`. Preserve `SourceTimestamp` in provenance only. The offline equivalence adapter can construct `IntervalStart` from the frozen CSV timestamp before invoking the same mapper.

#### H2. Skip is described as an output but not modeled as a complete event contract

The design says P-03 produces either a DecisionVector or an explicit skip. The proto sketch places `skipped` and `skip_reason` inside `DecisionVector`, while `decision_id` exists only for actionable output. This leaves several questions:

- Does `StreamDecisions` include skips and initialization events?
- What identifies a skip if it has no `decision_id`?
- Is HOLD actionable or merely evaluated?
- Are duplicate/regression, discontinuity, invalid input, panic, and overflow distinct reasons?
- Does every accepted finalized bar produce exactly one terminal outcome event?

**Recommendation:** Define a `DecisionEvent` envelope with one outcome per input:

```text
DecisionEvent
  event_id
  signal_id
  symbol
  interval_start
  market_snapshot_id
  received_at
  completed_at
  latency
  model_version
  outcome = decision | skip
```

A decision contains side, confidence, and quality/regime. A skip contains an enum reason and human-readable detail. HOLD remains a decision. `INITIALIZING` remains a skip or a distinct non-actionable evaluated outcome, but the choice must be consistent in domain and proto types.

Minimum skip reasons should cover:

- `INFER_OFF`
- `NOT_MODEL_ELIGIBLE`
- `INITIALIZING`
- `DUPLICATE_OR_REGRESSION`
- `INPUT_GAP`
- `QUEUE_OVERFLOW`
- `TIMEOUT`
- `INVALID_INPUT`
- `ENGINE_ERROR`
- `ENGINE_PANIC`
- `STATE_DISCONTINUOUS`

#### H3. Provenance is not sufficient for deterministic replay

`market_snapshot_id`, identifiers, and `model_version` are necessary but do not fully satisfy DI-07. D01 decisions depend on prior recursive state and 15-context, not only the latest bar. Replaying a decision requires the exact ordered input sequence or a versioned pre-step state snapshot plus the current bar.

The design intentionally avoids durable bar persistence, which is acceptable for this coding increment, but it should not claim production-grade replay without another evidence mechanism.

**Recommended Phase 0 evidence:**

- check in immutable frozen input and expected output fixtures for scientific equivalence
- define a deterministic model-version fingerprint from all decision-relevant constants and implementation identity
- include `market_snapshot_id`, interval, accepted sequence, pre-step state hash, post-step state hash, and model version in structured outcome records
- retain a bounded in-memory diagnostic ring only when explicitly enabled for local validation
- document that process restart loses live replay evidence

Durable decision-input recording can be assigned to the later recording plane, but DI-07 remains open until it exists.

#### H4. Model-version construction is underspecified

“Rule + implementation fingerprints + Go module version” is directionally correct but not reproducible until canonicalization is defined.

Specify:

- every D01/D02/D04/emitter constant included
- stable field ordering and serialization
- source/baseline revision identity
- Go implementation revision identity
- schema version
- hash algorithm and printable format
- behavior when the working tree is dirty or no commit metadata is available

A model version should change whenever a decision-relevant equation, operation order, clamp, threshold, warm-up rule, or constant changes. Build-environment noise that cannot change results should not alter the scientific fingerprint.

#### H5. Engine input and output numeric validation is incomplete

P-02 validates live OHLC and volume, but P-03 also needs defensive checks at its ownership boundary and after every scientific stage.

Define behavior for:

- non-finite prices or derived values
- zero or implausible time deltas
- `uint64` volume conversion to `float64`
- non-finite or out-of-domain half-life
- non-finite H, Q_G, Q_S, Q_R, or C
- impossible confidence ranges
- a state transition whose timestamp does not advance

Do not invent broad market-price limits such as a fixed maximum stock price. Validate mathematical domains and representational safety. On failure, leave committed state unchanged and emit a typed skip.

#### H6. Health and status semantics need an implementable definition

The model component is to report healthy/degraded/unavailable separately, but no transition rules or status payload are defined.

At minimum, expose per-symbol:

- enabled/off state
- cold/initializing/ready/paused/discontinuous/error state
- accepted-step count and warm-up progress
- last received and last accepted intervals
- last DecisionEvent time and outcome
- last skip reason
- consecutive errors/timeouts
- queue depth or lag
- current model version
- pre/post state hashes when diagnostics are enabled

Suggested aggregate semantics:

- **Healthy:** host enabled, no discontinuous symbol, last eligible input completed within deadline
- **Degraded:** one or more symbols initializing, paused, lagging, or recently skipped for operational reasons
- **Unavailable:** host failed to start, scientific baseline/config invalid, or all enabled symbol engines are terminally failed

Initialization should not be reported as an error, but it should be visible.

### Medium-Priority Findings

#### M1. Session and reset behavior is intentionally deferred but needs a safe Phase 0 rule

DI-04 remains open. Until an exchange calendar and session policy exist, P-03 should not perform an automatic “market open” reset based on local clock assumptions.

The documents should explicitly state for Phase 0:

- process restart resets all model state
- `infer=false` pauses without reset
- no automatic daily/session reset occurs
- any operator reset is explicit, authenticated when exposed remotely, and emits an audit event
- extended-hours inputs are governed by P-02/provider behavior until a formal session contract is added

This prevents an implementation from silently inventing calendar semantics.

#### M2. Emitter `position_state` must not be confused with an executed portfolio position

P-03 retains LONG/SHORT/HOLD state because that state is part of the frozen SADE emitter. It is scientific context, not an account position, risk reservation, or confirmation of a fill.

Rename or document it as `emitter_position_state` and include it only as model diagnostics/provenance. P-05/P-08 must remain authoritative for intended and actual positions when those processes are implemented.

#### M3. The output quality/regime schema is only a prose list

The design maps `quality / regime` to H, Q_G, Q_S, Q_R, path direction, and status, while the proto sketch contains only `capturability` and `hard_eligibility`. Before Phase E, define exact field names, types, ranges, units, enum values, and whether each value is required for HOLD and skipped outcomes.

Avoid a generic string or opaque map for decision-critical values. Versioned explicit fields are easier to validate and replay.

#### M4. `signal_id` and `decision_id` generation rules need exact invariants

Define:

- ID format and generator
- when `signal_id` is created
- whether initialization and gate skips receive one
- when `decision_id` is created
- uniqueness scope
- retry/idempotency behavior
- whether a duplicate input returns the prior outcome or a new skip event

A practical invariant is: one immutable `event_id` per terminal processing outcome, one `signal_id` per evaluated model step, and one `decision_id` per emitted BUY/SELL/HOLD decision. Preserve the same upstream identifiers through future risk and execution stages.

#### M5. Configuration validation and startup failure behavior are absent

For `QUANTRAM_MODEL` and `QUANTRAM_MODEL_DEADLINE`, define invalid-value behavior. Recommended:

- reject unknown model modes at startup
- reject zero, negative, or unreasonably large deadlines
- validate all scientific constants and fingerprints before subscribing
- keep ingestion available if the optional model is off
- when adaptive is explicitly requested but cannot initialize, report model unavailable and fail model startup visibly rather than silently reverting to off

#### M6. Multi-symbol scheduling and fairness need one explicit strategy

“One symbol at a time” can mean globally serial or serial per symbol. The design also says symbols are independent.

Use one owner/worker per symbol or an equivalent keyed executor that guarantees:

- no concurrent steps for the same symbol
- independent progress across symbols
- bounded total concurrency
- no map races during engine creation/reset/status reads
- one slow symbol does not consume another symbol's deadline

At one-minute cadence, the simplest correct keyed design is preferable to a generalized worker framework.

## Testing and Acceptance Gaps

The validation table is a useful start but should become an executable acceptance matrix.

### Scientific equivalence

Required:

- checked-in input fixture with origin and SHA-256
- checked-in expected output fixture with SADE revision/fingerprint
- exact operation count and warm-up boundary
- per-field tolerance table rather than “small ulp band” alone
- exact match for status, path direction, position decision, and decision count
- first divergence report containing symbol, sequence, field, expected, actual, and pre-step state
- deterministic repeated runs and race-enabled Go tests

### Finalized-stream integration

Required:

- partial `u` never reaches P-03
- final `b` reaches P-03 once
- same-interval duplicate/regression does not mutate state
- out-of-order finalized input produces a typed skip
- missing interval or queue overflow marks state discontinuous
- `infer=false` pauses without state mutation
- `infer=true` resumes according to the selected continuity/rebuild policy
- reconstructed head never enables evaluation
- startup behavior matches the selected cold-start or rebuild mode

### Failure isolation

Required:

- panic in AAPL engine does not stop MSFT/NVDA or P-02
- timeout leaves AAPL state unchanged
- repeated AAPL errors cause a visible AAPL model-health transition
- unsubscribe/shutdown closes workers cleanly without send-on-closed-channel races
- disabled P-03 leaves current ingestion behavior unchanged

### Performance

Required:

- p50, p95, and p99 step latency on the frozen corpus
- allocation budget per step
- race test for multi-symbol execution
- slow-engine test for delivery overflow behavior
- at least one regular-market live validation long enough to cross initialization and emit evaluated outcomes

A manual requirement such as “after about 16 minutes, host logs ACTIONABLE or HOLD/BUY/SELL” should be corrected: `ACTIONABLE` appears to be a status category, while BUY/SELL/HOLD is the side. The pass criterion should name the exact domain fields and expected combinations.

## Recommended Contract Refinements

### P-02 → P-03 interface

Define a model-specific Go interface rather than exposing the general observation subscriber directly:

```go
type FinalizedBarConsumer interface {
    SubscribeModelBars(buffer int) (subscriptionID uint64, bars <-chan domain.Bar)
    Unsubscribe(subscriptionID uint64)
    ReadinessFor(symbol string) domain.ModelReadiness
    Window(symbol string, limit int) []domain.Bar
}
```

The exact names may follow repository style. The important distinction is behavioral: this interface must document ordering, overflow, continuity, shutdown, and catch-up semantics for recursive model consumption.

### P-03 outcome domain

Prefer one event envelope with a decision-or-skip union over a `DecisionVector` carrying a `skipped` boolean. This prevents contradictory states such as `side=BUY` and `skipped=true`.

Required common fields:

- event, signal, and optional decision identifiers
- symbol/instrument and interval
- source `market_snapshot_id`
- accepted sequence
- model/schema/config versions
- start/completion timestamps and latency
- pre/post state hashes

Decision payload:

- BUY, SELL, or HOLD
- confidence/capturability
- H, Q_G, Q_S, Q_R
- path direction and model status
- emitter position state as diagnostic context

Skip payload:

- typed reason
- detail
- whether state continuity remains valid
- whether operator intervention or rebuild is required

### Scientific state transaction

Define `AdaptiveEngine.Step` as a transaction:

```text
validate bar and time
copy/read immutable pre-state
calculate D01 → D02 → D04 → emitter
validate all outputs
check deadline
atomically commit post-state
emit one outcome
```

Any failure before commit produces no state change.

## Suggested Changes to the Two P-03 Documents

### Add to the design document

1. A **Finalized Delivery Guarantees** subsection covering ordering, loss, overflow, sequence gaps, and discontinuity recovery.
2. A **Startup and Rebuild Policy** subsection selecting cold-start or deterministic rebuild.
3. A **Per-Symbol Readiness** subsection clarifying the current global `infer` behavior and intended evolution.
4. A **State Transaction and Deadline** subsection defining atomic commit and cancellation.
5. A complete **DecisionEvent / Skip** semantic contract.
6. A **Provenance and Replay Boundary** subsection stating what Phase 0 can and cannot reproduce.
7. A **Model Health State Machine** with per-symbol status.
8. A Phase 0 statement for session reset and extended-hours behavior.

### Add to the implementation document

1. Replace direct dependence on the generic `SubscribeFinalized(2)` behavior with a model-consumer interface or explicitly repair its loss semantics first.
2. Add Phase D tests for overflow, missing intervals, startup race, shutdown, and per-symbol gating.
3. Specify how a deadline controls CPU-bound `Step` and how state commit remains atomic.
4. Add exact input/output validation and typed errors to Phases B and C.
5. Finalize domain outcome types before the proto sketch.
6. Add config-validation and startup-failure tests.
7. Add `go test -race ./...` to the pre-merge validation plan.
8. Replace vague live-log acceptance with exact status, side, skip, latency, and warm-up assertions.

## Completeness Matrix

| Area | Current assessment | Required before |
| :--- | :--- | :--- |
| Scientific ownership | Complete | Phase A |
| SADE module/port map | Strong | Phase B |
| Frozen-run equivalence approach | Strong, tolerances need detail | Phase C exit |
| Finalized input eligibility | Strong | Phase D |
| Finalized delivery guarantee | Incomplete and currently lossy | Phase D integration |
| Per-symbol causal continuity | Partial; monotonic but not adjacent | Phase D integration |
| Startup/restart warm-up | Ambiguous | Phase D integration |
| Deadline/state atomicity | Incomplete | Phase D exit |
| Multi-symbol readiness | Ambiguous/global today | Phase D exit |
| Decision versus skip event | Partial | Domain/proto freeze |
| Model version/provenance | Partial | Proto freeze and live evidence |
| Replay evidence | Limited to frozen fixture | Before execution use |
| Model health/observability | Concept only | Phase D exit |
| Session reset | Safely deferrable with explicit Phase 0 rule | Live validation |
| P-04/RK45 integration | Correctly deferred | Next scientific increment |
| Risk/orders | Correctly out of scope | P-05/P-06 |

## Recommended Implementation Sequence

1. Freeze the Unit Run 001 input/output fixtures and scientific fingerprints.
2. Implement Phase A mapper/config with `IntervalStart` as canonical model event time.
3. Port and validate D01, then D02/D04/emitter with transactional state semantics.
4. Define `DecisionEvent`, decision payload, skip payload, and typed reason taxonomy in the domain package.
5. Resolve P-02 → P-03 delivery guarantees before wiring the live host.
6. Select and implement cold-start or deterministic rebuild behavior.
7. Implement keyed per-symbol workers, readiness behavior, deadline enforcement, and health/status.
8. Add proto only after domain semantics and integration tests are stable.
9. Validate with race tests, forced overflow, timeout/panic isolation, and regular-market live data.
10. Keep `QUANTRAM_MODEL=off` as the default until scientific equivalence and finalized-stream acceptance tests are green.

## Minimum Definition of Done for P-03

P-03 should be considered complete for this adaptive-only increment when all of the following are true:

- Go output matches the frozen SADE Unit Run 001 under a documented tolerance matrix.
- Every finalized input produces exactly one observable terminal outcome: decision or typed skip.
- No partial, reconstructed head, duplicate, regressive, or unknown-quality bar mutates model state.
- A missing eligible finalized bar cannot pass silently; the affected symbol pauses or rebuilds.
- Per-symbol processing is ordered, isolated, race-free, and bounded.
- Timeout, error, panic, and shutdown cannot partially mutate committed state.
- Startup and restart warm-up behavior is deterministic and visible.
- The current global-versus-per-symbol `infer` policy is explicit and tested.
- Model version and source snapshot identity accompany every evaluated outcome.
- Health/status distinguishes disabled, initializing, ready, paused, discontinuous, and failed states.
- Ingestion behavior is unchanged when `QUANTRAM_MODEL=off`.
- `go test ./...` and `go test -race ./...` pass.
- A regular-market live run crosses warm-up and demonstrates decisions/skips with measured latency.
- No P-03 output is connected to orders in this increment.

## Final Recommendation

Proceed with the mapper, configuration, D01/D02/D04/emitter port, and frozen equivalence harness. Those portions are sufficiently specified and provide the fastest way to retire scientific-port risk.

Before accepting the live host integration, revise the P-03 documents and P-02 consumer boundary to close the finalized-delivery gap. A lossy depth-2 channel is acceptable for a viewer but not for a recursive adaptive state machine unless loss is detected and forces an explicit pause/rebuild. Also define startup recovery, state-transaction deadline behavior, per-symbol readiness, and a complete decision-or-skip event contract.

With those changes, the P-03 design is a strong and appropriately scoped bridge from the proved SADE adaptive path to QuanTRAM's later risk and execution processes. Without them, numerical equivalence may be green while the live state trajectory silently diverges from the finalized bar sequence it is supposed to represent.
