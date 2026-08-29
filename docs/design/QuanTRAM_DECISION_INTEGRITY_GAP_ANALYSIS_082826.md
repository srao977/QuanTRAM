# QuanTRAM Decision Integrity and Design Gap Analysis

**Date:** August 28, 2026  
**Status:** Open design-gap register  
**Parent Architecture:** [QuanTRAM System Specification](QuanTRAM_hi-level_design_082826.md)  
**Derived Artifact Specification:** [E2E QuanTRAM Artifacts](E2E_QuanTRAM_ARTIFACTS.md)  
**Process Model:** [QuanTRAM Process Model](QuanTRAM_PROCESS_MODEL_082926.md)

## 1. Purpose and Authority

This document records unresolved requirements that could compromise market-data correctness, model decisions, risk enforcement, execution validity, replay, or performance measurement. It is a gap register, not a replacement for the parent architecture or artifact specification.

When a gap is resolved, the behavioral decision must be incorporated into the parent architecture and the corresponding contracts, ownership, and acceptance criteria must be incorporated into the artifact specification. The gap may then be marked closed here with links to those authoritative sections.

QuanTRAM v1 is limited to U.S. stocks, exchange-traded funds (ETFs), and published market indices. Stocks and ETFs may be tradable subject to reference data and risk approval. Indices are analytics-only calculated series and cannot be routed directly as broker orders.

## 2. Priority Definitions

| Priority | Meaning | Delivery Effect |
| :--- | :--- | :--- |
| **P0** | Required to preserve causal, numerical, or market-data correctness | Blocks implementation of the affected live decision path |
| **P1** | Required to validate risk, execution, or measured strategy performance | Blocks live-order capability or use of results for strategy changes |
| **P2** | Required for reliable production operation and long-term maintainability | Blocks production readiness unless explicitly accepted |

## 3. Gap Summary

| ID | Priority | Gap | Primary Failure |
| :--- | :--- | :--- | :--- |
| DI-01 | P0 | Canonical bar semantics and provider qualification | Provider changes alter features and decisions |
| DI-02 | P0 | Temporal causality and bar finalization | Late data introduces look-ahead or stale decisions |
| DI-03 | P0 | Failover state reconciliation | Duplicate, missing, or discontinuous model inputs |
| DI-04 | P0 | Session, calendar, halt, and auction semantics | Incompatible market regimes are mixed silently |
| DI-05 | P0 | Point-in-time instrument reference data | Incorrect prices, universes, and tradability |
| DI-06 | P0 | Data-quality propagation and decision gating | Models act on incomplete or reconstructed data unknowingly |
| DI-07 | P0 | Decision provenance and reproducibility | Executions cannot be reconstructed from exact inputs |
| RV-01 | P1 | Pending exposure and broker reconciliation | Concurrent orders exceed position or leverage limits |
| RV-02 | P1 | Stale, duplicate, and unsafe order prevention | Obsolete or repeated decisions reach the broker |
| RV-03 | P1 | Independent health domains | Unhealthy execution or data paths remain active |
| MV-01 | P1 | Point-in-time model evaluation | Leakage and survivorship bias inflate performance |
| BV-01 | P1 | Paper-fill and slippage methodology | Paper/live comparisons create false confidence |
| OP-01 | P2 | Broker error and retry policy | Orders fail silently or are retried unsafely |
| OP-02 | P2 | Event-stream and ledger guarantees | Audit and position state cannot be trusted |
| OP-03 | P2 | Credential lifecycle | Feeds or brokers disconnect during operation |
| OP-04 | P2 | Contract and telemetry evolution | Historical data becomes unreadable or incomparable |
| OP-05 | P2 | Inference latency and watchdog policy | Signals arrive after their market context expires |
| OP-06 | P2 | Benchmark configuration recovery | Optional benchmark failures affect operation or evidence |
| OP-07 | P2 | Rollout, entitlement, and licensing controls | Unqualified behavior or unauthorized data use reaches production |

## 4. P0 Decision-Integrity Gaps

### DI-01: Canonical Bar Semantics and Provider Qualification

**Failure mode:** Alpaca and Databento can legitimately differ in trade filtering, corrections, event ordering, timestamps, volume, and bar-close rules. A source transition can therefore change volatility, momentum, liquidity, and other features even when both providers are healthy.

**Current coverage:** The architecture identifies normalization, aggregation, deduplication, and gap filling, but does not define one canonical OHLCV contract or a behavioral qualification threshold between providers.

**Required additions:**

- Define eligible trade conditions and quote inputs.
- Define interval boundaries, open and close selection, high and low updates, and volume calculation.
- Define correction, cancellation, duplicate, out-of-order, and late-event handling.
- Define whether provider bars or locally aggregated events are authoritative for each interval.
- Define price precision, rounding, nullable fields, and zero-volume behavior.
- Qualify Alpaca and Databento using differences in bars, features, signals, risk decisions, and hypothetical orders.
- State explicitly that exact provider-bar equality is not required; downstream behavioral differences must remain within approved tolerances.

**Exit criteria:** A replay corpus demonstrates deterministic aggregation per provider and documents acceptable cross-provider drift for every decision-relevant feature and output.

### DI-02: Temporal Causality and Bar Finalization

**Failure mode:** If source, exchange, receipt, and processing times are used inconsistently, bars can move between intervals and late events or backfills can influence decisions that precede them. This creates live/backtest disagreement and potential look-ahead bias.

**Current coverage:** Multiple timestamps are retained, but timestamp authority, skew tolerance, event-time processing, and bar-finalization behavior remain open.

**Required additions:**

- Use exchange event time as canonical market time where the provider supplies it.
- Preserve provider, local receipt, processing, decision, and broker timestamps separately.
- Normalize internal timestamps to UTC with defined precision.
- Define host clock synchronization, maximum skew, and clock-health monitoring.
- Define event-time watermarks and allowed lateness per interval.
- Mark bars as provisional or finalized and prohibit ordinary live inference from reading unfinalized bars unless a strategy explicitly opts in.
- Define whether late corrections revise historical state, produce correction events, or affect future state only.

**Exit criteria:** Tests prove that future or late-arriving data cannot affect an earlier decision and that the same ordered input produces the same finalized bars and decisions.

### DI-03: Failover State Reconciliation

**Failure mode:** A feed switch during an open interval can double-count events, omit volume, revise a close, corrupt rolling windows, or emit more than one decision for the same symbol and interval.

**Current coverage:** Heartbeats, circuit breaking, reconnects, historical gap filling, and source selection exist, but source-transition invariants are not complete.

**Required additions:**

- Define primary, warm-standby, degraded, failed-over, reconciling, and failback states.
- Define source-specific health thresholds, hysteresis, minimum dwell time, and anti-flapping behavior.
- Define overlap windows and canonical deduplication keys.
- Define the handling of a partially built bar when the source changes.
- Reconcile and backfill before model inference resumes.
- Preserve or deterministically rebuild rolling feature and model state.
- Suppress duplicate decisions for a symbol and interval.
- Quarantine decision generation when neither source can prove continuity.
- Explicitly exclude multi-feed consensus aggregation from v1 unless separately approved.

**Exit criteria:** Injected outage, reconnect, overlap, delayed-event, and failback scenarios produce no unexplained bar gaps, duplicates, volume inflation, or duplicate decisions.

### DI-04: Session, Calendar, Halt, and Auction Semantics

**Failure mode:** Mixing regular trading hours, extended hours, auctions, halts, holidays, and early closes without explicit features changes liquidity and volatility distributions and can produce invalid bars or orders.

**Current coverage:** The supported U.S. instrument universe is defined, but the session policy is not.

**Required additions:**

- Decide whether each strategy consumes regular trading hours, extended hours, or both.
- Use an authoritative exchange calendar with holidays and early closes.
- Define pre-market, opening auction, continuous session, closing auction, and after-hours boundaries.
- Define trading-halt, limit-state, crossed-market, and locked-market behavior.
- Define session resets for bars, rolling features, and model state.
- Add session and market-state fields to market snapshots and decisions.

**Exit criteria:** Calendar-boundary, daylight-saving, early-close, halt, and auction tests produce the documented bars and suppress prohibited decisions.

### DI-05: Point-in-Time Instrument Reference Data

**Failure mode:** Incorrect or current-only instrument metadata causes wrong universe membership, price adjustments, order sizing, and tradability. Corporate actions can appear as extreme returns if historical data is not adjusted consistently.

**Current coverage:** `STOCK`, `ETF`, and `INDEX` classification and non-tradable indices are defined, but the reference-data lifecycle is not.

**Required additions:**

- Maintain stable instrument identifiers independently of mutable ticker symbols.
- Record symbol, listing exchange, currency, tick size, lot size, status, and effective-time ranges.
- Define split, dividend, merger, delisting, and symbol-change processing.
- Define adjusted versus unadjusted inputs for training, inference, risk, and execution.
- Preserve point-in-time universe membership to prevent survivorship bias.
- Treat index volume and other non-standard fields as nullable or provider-defined; do not silently interpret missing index volume as equity volume.
- Assign enforcement ownership for non-tradable indices and emit an auditable rejection before broker submission.

**Exit criteria:** Corporate-action and symbol-change replays preserve economic continuity, and every index order intent is rejected before any broker call.

### DI-06: Data-Quality Propagation and Decision Gating

**Failure mode:** A syntactically continuous bar series can contain stale, partial, backfilled, or provider-transition data. Without propagated quality, analytics treats every value as equally reliable.

**Current coverage:** Feed and gap metrics exist, but bar-level quality and model/risk responses are not formalized.

**Required additions:**

- Attach `quality_status`, `is_final`, `is_backfilled`, `source`, `source_transition`, `data_age`, `missing_event_count`, and `correction_count` to bars or snapshots.
- Define complete, degraded, stale, partial, reconstructed, and invalid states.
- Define per-strategy eligibility for degraded or backfilled data.
- Require risk to recheck data age immediately before broker submission.
- Define fail-closed behavior when quality is unknown or below the strategy threshold.
- Record all quality state used by a decision.

**Exit criteria:** No decision or order can be produced without an explicit quality classification, and tests demonstrate the configured skip, degrade, or reject behavior for every quality state.

### DI-07: Decision Provenance and Reproducibility

**Failure mode:** Correlation IDs alone do not prove which exact data, feature code, model, or risk configuration produced an order. Model defects and execution outcomes therefore cannot be reconstructed reliably.

**Current coverage:** End-to-end identifiers are listed, but generation, immutability, versioning, and propagation invariants remain incomplete.

**Required additions:**

- Define immutable generation and uniqueness rules for `market_snapshot_id`, `signal_id`, `decision_id`, `order_id`, and `benchmark_id`.
- Link every signal to the exact finalized bars and market snapshot used.
- Record feature-set, model, parameter, risk-policy, configuration, instrument-data, and schema versions.
- Preserve upstream identifiers when risk resizes or rejects an intent.
- Include idempotency and causation identifiers on all lifecycle events.
- Store enough input state to deterministically replay a decision without consulting mutable current data.

**Exit criteria:** Any execution, rejection, or benchmark result can be traced to and replayed from its exact market inputs and versioned decision configuration.

## 5. P1 Risk and Execution Validity Gaps

### RV-01: Pending Exposure and Broker Reconciliation

**Failure mode:** Risk based only on confirmed positions ignores orders awaiting acknowledgment, partial fills, pending cancels, and delayed broker updates. Concurrent orders may collectively exceed position or leverage limits.

**Required additions:**

- Model confirmed, reserved, pending-fill, partially filled, pending-cancel, and uncertain exposure.
- Evaluate new orders against worst-case fills of all active orders.
- Reserve buying power or exposure atomically before routing.
- Reconcile broker orders, fills, positions, cash, and fees against the ledger.
- Define disconnect recovery and unknown-order-state behavior.

**Exit criteria:** Concurrent-order and delayed-event tests cannot exceed configured limits under any valid fill ordering.

### RV-02: Stale, Duplicate, and Unsafe Order Prevention

**Failure mode:** Queueing, reconnects, retries, or delayed inference can send an obsolete or duplicate order after its market context has changed.

**Required additions:**

- Assign expiration times to signals, decisions, and order intents.
- Enforce maximum market-data age at risk approval and broker submission.
- Use idempotency keys and duplicate suppression across process restarts.
- Define per-symbol, strategy, account, and global order-rate limits.
- Define price collars, maximum notional, and fat-finger controls.
- Provide strategy, symbol, account, and global kill switches.
- Revalidate tradability, session state, and risk immediately before submission.

**Exit criteria:** Duplicate, expired, stale-data, out-of-collar, non-tradable index, and kill-switch tests produce auditable rejections and no broker submission.

### RV-03: Independent Health Domains

**Failure mode:** A single healthy/unhealthy status can hide the distinction between usable market data and usable execution. For example, healthy data with a disconnected broker must maintain market state while preventing new live orders.

**Required additions:**

- Define independent health state machines for each market-data provider, historical recovery, model inference, broker execution, live-event publication, ledger consumption, and benchmark processing.
- Define which health combinations allow observation, inference, paper execution, live submission, cancellation, or reconciliation.
- Keep benchmark health outside the live-execution availability decision.
- Emit health transitions with reason, time, and affected capabilities.

**Exit criteria:** A capability matrix and fault-injection tests prove that each degraded component disables only the documented operations.

### MV-01: Point-in-Time Model Evaluation

**Failure mode:** Leakage, survivorship bias, current-only reference data, unrealistic costs, and random train/test splits can make an ineffective strategy appear profitable.

**Required additions:**

- Build point-in-time datasets using only information available at each simulated decision time.
- Preserve historical universe membership, delistings, and corporate actions.
- Use time-ordered walk-forward and out-of-sample evaluation.
- Test explicitly for target, feature, normalization, and revision leakage.
- Include fees, spread, latency, partial fills, rejection, slippage, and market impact.
- Segment results by instrument type, liquidity, session, volatility regime, and source provider.
- Define promotion thresholds and statistical uncertainty, not only point estimates.

**Exit criteria:** A reproducible evaluation report passes leakage tests and promotion thresholds on untouched out-of-sample periods.

## 6. P1 Benchmark-Validity Gap

### BV-01: Paper-Fill and Slippage Methodology

**Failure mode:** A paper engine using stale L2, immediate midpoint fills, or an undefined reference price can materially overstate decision performance and produce misleading model adjustments.

**Current coverage:** Benchmark fields and dashboard views are described, but the simulator, slippage, validity, and calibration contracts remain open.

**Required additions:**

- Define the arrival-price authority and implementation-shortfall formula.
- Define the fill algorithm for market, limit, partial-fill, cancel, and replace lifecycles.
- Define queue-position, available-depth, spread, latency, and market-impact assumptions.
- Define an L2 snapshot schema and maximum allowed age.
- Define missing, stale, locked, crossed, halted, and auction-market behavior.
- Version the simulator and calibration parameters.
- Label results `VALID`, `DEGRADED`, or `UNUSABLE` based on input quality and lifecycle completeness.
- Calibrate modeled outcomes against live fills without allowing benchmark processing to block execution.

**Exit criteria:** Benchmark records disclose simulator version and validity, and live/paper error distributions remain within approved thresholds for qualified cohorts.

## 7. P2 Production-Completeness Gaps

### OP-01: Broker Error and Retry Policy

- Classify responses as terminal, conditionally retriable, or safely idempotent.
- Define bounded backoff with jitter and submission deadlines.
- Never retry an order unless broker state or an idempotency key proves duplication cannot occur.
- Emit terminal and exhausted-retry events to the authoritative stream with normalized reason codes.
- Treat a live rejection paired with a paper fill as benchmark divergence.

### OP-02: Event-Stream and Ledger Guarantees

- Define publication acknowledgment, ordering scope, partition key, durability, replication, retention, and recovery objectives.
- Define consumer checkpointing, idempotency, duplicate handling, poison-event handling, and replay.
- Detect missing or conflicting order sequence numbers.
- Rebuild projections and reconcile them with broker truth after failures.
- Ensure query and benchmark workloads cannot block authoritative writes.

### OP-03: Credential Lifecycle

- Define secure storage, retrieval, caching, rotation, expiry, and revocation.
- Rotate without exposing credentials in logs or domain messages.
- Define provider-specific behavior when refresh fails.
- Test that planned rotation does not lose events or create duplicate sessions.

### OP-04: Contract and Telemetry Evolution

- Include schema versions on durable market, execution, ledger, and benchmark records.
- Define additive-change, deprecation, compatibility, and migration rules.
- Preserve the reader and configuration needed to interpret retained historical events.
- Prevent comparisons across incompatible model, feature, simulator, or accounting versions unless explicitly normalized.

### OP-05: Inference Latency and Watchdog Policy

- Define latency budgets by strategy and bar interval.
- Cancel or discard inference that exceeds its decision deadline.
- Prefer no new order over reusing an unexplained stale signal.
- Record deadline misses, fallback behavior, and latency distributions.
- Include inference health in the capability matrix.

### OP-06: Benchmark Configuration Recovery

- Define global and per-strategy ownership of `OFF`, `SAMPLED`, and `FULL` modes.
- Persist and audit mode changes.
- Define deterministic sampling and restart behavior.
- Bound queues and shed optional benchmark work when dependencies fail.
- Guarantee that storage, dashboard, or benchmark backlog cannot reject or delay a live order.

### OP-07: Rollout, Entitlement, and Licensing Controls

- Require staged progression through deterministic replay, outage injection, shadow inference, paper execution, restricted live trading, and broader live operation.
- Define approval and rollback criteria for every stage.
- Verify provider entitlements for real-time, historical, derived, stored, and displayed data.
- Define retention and redistribution restrictions for raw and derived market data.
- Record the provider and entitlement context of retained datasets.

## 8. Required Acceptance Scenarios

The final specifications and implementation test plan must cover at least these scenarios:

1. Primary feed fails before, during, and after a bar boundary.
2. Primary and fallback overlap with duplicate, delayed, corrected, and conflicting events.
3. Historical gap fill arrives after live processing has resumed.
4. Both feeds are stale or disagree beyond qualification limits.
5. Early close, daylight-saving transition, opening auction, closing auction, and trading halt.
6. Split, dividend, symbol change, delisting, and ETF or index field differences.
7. An index signal attempts to produce an executable order.
8. A signal or intent expires between inference, risk approval, and broker submission.
9. Duplicate submission occurs across timeout, retry, and process restart.
10. Multiple pending orders would exceed exposure if all were filled.
11. Broker events are duplicated, missing, reordered, or received after reconnect.
12. Event-stream publication, ledger consumption, or benchmark storage is unavailable.
13. L2 data is missing or stale during paper execution.
14. A live order is rejected while its paper counterpart fills.
15. A retained decision is replayed using its original data and versioned configuration.

## 9. Implementation Gates

### Gate A: Before Market-Data and Analytics Implementation

Resolve DI-01 through DI-07. Provider adapters may be prototyped earlier, but their output must not be treated as a production decision contract until these gaps close.

### Gate B: Before Live-Order Capability

Resolve RV-01 through RV-03 and the execution-critical portions of OP-01, OP-02, OP-03, and OP-05. Complete fault-injection and broker-reconciliation tests.

### Gate C: Before Strategy Promotion from Benchmark Results

Resolve MV-01 and BV-01. A paper result without a valid simulation classification, point-in-time input provenance, and realistic cost model cannot support strategy promotion.

### Gate D: Before Production Readiness

Resolve all remaining P2 gaps, complete the staged rollout, and record any accepted residual risk with owner, rationale, scope, and expiration date.

## 10. Resolution Record

Use this table to track authoritative incorporation and closure.

| Gap ID | Status | Owner | Decision / Authority Link | Validation Evidence |
| :--- | :--- | :--- | :--- | :--- |
| DI-01 | Open | TBD | TBD | TBD |
| DI-02 | Open | TBD | TBD | TBD |
| DI-03 | Open | TBD | TBD | TBD |
| DI-04 | Open | TBD | TBD | TBD |
| DI-05 | Open | TBD | TBD | TBD |
| DI-06 | Open | TBD | TBD | TBD |
| DI-07 | Open | TBD | TBD | TBD |
| RV-01 | Open | TBD | TBD | TBD |
| RV-02 | Open | TBD | TBD | TBD |
| RV-03 | Open | TBD | TBD | TBD |
| MV-01 | Open | TBD | TBD | TBD |
| BV-01 | Open | TBD | TBD | TBD |
| OP-01 | Open | TBD | TBD | TBD |
| OP-02 | Open | TBD | TBD | TBD |
| OP-03 | Open | TBD | TBD | TBD |
| OP-04 | Open | TBD | TBD | TBD |
| OP-05 | Open | TBD | TBD | TBD |
| OP-06 | Open | TBD | TBD | TBD |
| OP-07 | Open | TBD | TBD | TBD |

## 11. Immediate Decision Order

Resolve the gaps in this sequence because later decisions depend on earlier contracts:

1. Canonical bars, temporal causality, and session semantics.
2. Provider qualification and failover reconciliation.
3. Instrument reference data and quality gating.
4. Provenance and deterministic replay.
5. Pending exposure, stale-order protection, and health isolation.
6. Point-in-time model evaluation.
7. Paper-fill, slippage, and benchmark-validity methodology.
8. Production durability, recovery, security, schema evolution, and rollout controls.

The highest combined risk is the interaction among provider failover, bar finalization, timestamp authority, and analytical state continuity. That interaction can produce plausible but causally incorrect inputs, which may evade simple availability monitoring while materially changing decisions.