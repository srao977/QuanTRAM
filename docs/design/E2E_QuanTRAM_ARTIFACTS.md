# E2E QuanTRAM Artifacts

**Date:** August 28, 2026  
**Status:** Initial functional artifact specification  
**Implementation Language:** Go  
**Parent Architecture:** [QuanTRAM System Specification](QuanTRAM_hi-level_design_082826.md)

**Open Design Gaps:** [QuanTRAM Decision Integrity and Design Gap Analysis](QuanTRAM_DECISION_INTEGRITY_GAP_ANALYSIS_082826.md)

**Process Model:** [QuanTRAM Process Model](QuanTRAM_PROCESS_MODEL_082926.md) — runtime units, gRPC surfaces, local Alpaca-paper topology, and Azure scale-out. This document proposes a resolution for process decomposition; the parent architecture remains authoritative for behavior.

## 1. Overview and Purpose

This document is the child artifact specification derived from the parent QuanTRAM System Specification. The parent provides the architectural and functional basis: the integrated flow, semantic system boundaries, component responsibilities, required live-event recording, optional benchmark behavior, and telemetry requirements. This document translates that basis into Go-oriented software artifacts, interfaces, package ownership, provisional gRPC surfaces, processing semantics, and acceptance criteria.

The parent architecture remains authoritative for system intent and end-to-end behavior. This child specification is authoritative for the proposed artifact decomposition and Go contract surface. Changes that alter system behavior must first be reconciled with the parent architecture; implementation refinements that preserve that behavior are maintained here.

QuanTRAM v1 supports U.S. stocks, ETFs, and published market indices. Stocks and ETFs may be tradable when reference data and risk policy permit. Indices are non-tradable calculated series that may inform features, signals, and market-regime context but cannot be routed as broker orders.

The end-to-end QuanTRAM software artifacts are assigned to these semantic system ownership boundaries:

1. External Market Feeds
2. Ingestion and Data Quality
3. Analytics and Risk
4. Execution and Harness
5. Required Live Event Recording
6. Optional Benchmark Analysis

Each boundary owns its domain contracts, state, processing rules, and infrastructure adapters. Communication across boundaries occurs through explicit Go interfaces and versioned message contracts rather than shared internal state.

The initial deployment may place these components in one Go gRPC server. That is a provisional deployment topology, not a domain constraint. Package contracts must preserve the ability to move a boundary into a separate process without redesigning its domain model. The Harness Benchmark Dashboard may operate as a read-only gRPC client.

## 2. Architectural Principles

- **Semantic ownership:** Each artifact has one owning boundary and exposes only explicit contracts to other boundaries.
- **Ports and adapters:** Domain behavior depends on Go interfaces; broker, feed, event-stream, database, and gRPC implementations are adapters.
- **Required live path:** Feed ingestion, decision processing, risk evaluation, live routing, event recording, and ledger projection form the required production path.
- **Non-blocking benchmark path:** Paper execution and benchmark analysis must never delay or reject live execution.
- **Continuous authoritative recording:** Live execution events are captured continuously, independent of benchmark mode.
- **Immutable event history:** Execution events are append-only, versioned, and processed idempotently.
- **Independent consumers:** Ledger and benchmark consumers maintain separate checkpoints and failure domains.
- **Technology-neutral domain:** gRPC, event transport, and persistence choices must not leak into core domain types.
- **Observable boundaries:** Every boundary emits structured logs, metrics, health state, and trace correlation identifiers.

## 3. Provisional Go and gRPC Topology

The initial implementation may use one Go process with one gRPC server and internal package calls for low-overhead communication. gRPC services expose control, query, and streaming boundaries to operators and clients.

```text
cmd/quantram-server
internal/
  marketfeed/
  ingestion/
  analytics/
  risk/
  execution/
  liveevents/
  ledger/
  paper/
  benchmark/
  transport/grpc/
api/proto/quantram/v1/quantram.proto
cmd/quantram-benchmark-dashboard
```

Recommended initial gRPC service surfaces:

- `MarketFeedService`: feed health and active-source queries.
- `ExecutionService`: order submission, cancellation, and order-status queries.
- `LedgerService`: orders, fills, positions, cash, PnL, and reconciliation queries.
- `BenchmarkService`: benchmark configuration, comparisons, metrics, and event streaming.
- `OperationsService`: component health, readiness, and operational state.

The dashboard is a read-only client of `BenchmarkService`, `LedgerService`, and selected health methods. Benchmark-mode mutation should be restricted to an authenticated operational client rather than embedded in ordinary dashboard queries.

### 3.1 Single Proto File Policy

QuanTRAM v1 will define its protobuf contract in one versioned file:

```text
api/proto/quantram/v1/quantram.proto
```

The file will contain the `quantram.v1` service definitions, shared enums, common value messages, request and response messages, streaming event envelopes, identifiers, filters, and pagination contracts. It should be organized internally by contract area: common types, market data, execution, ledger, benchmark, operations, and service definitions.

The proto file will not be split merely because it becomes long. Decomposition into multiple proto files requires demonstrated review, navigation, generation, build, ownership, or independent deployment friction. Any later split should preserve the `quantram.v1` package and wire compatibility.

## 4. External Market Feeds

**Ownership:** Connectivity and translation for external market-data providers.

### 4.1 Artifacts

- `MarketDataSource`: streams normalized trade and quote candidates from a provider.
- `HistoricalBarSource`: retrieves historical bars for recovery and gap filling.
- `AlpacaSIPAdapter`: Alpaca SIP WebSocket and REST implementation.
- `DatabentoAdapter`: Databento real-time and historical implementation.
- `ProviderMessageDecoder`: maps provider payloads into QuanTRAM ingress types.
- `InstrumentClassifier`: assigns stock, ETF, or index type and resolves tradability metadata.
- `FeedCredentialsProvider`: supplies credentials without exposing secrets to domain packages.

### 4.2 Go Contracts

```go
type MarketDataSource interface {
    Stream(ctx context.Context, request StreamRequest) (<-chan MarketEvent, <-chan error)
    Health(ctx context.Context) FeedHealth
}

type HistoricalBarSource interface {
    Bars(ctx context.Context, request BarRangeRequest) ([]Bar, error)
}
```

### 4.3 Functional Requirements

- Alpaca SIP is the primary real-time source.
- Databento provides fallback real-time data and historical recovery data.
- Accept only U.S. stocks, ETFs, and published market indices in the v1 market-data universe.
- Preserve instrument type and tradability metadata on normalized market events and bars.
- Provider adapters preserve source timestamps and assign local receipt timestamps.
- Provider-specific schemas and reconnect mechanics remain inside their adapters.
- Authentication material must not enter logs or domain events.

## 5. Ingestion and Data Quality

**Ownership:** Feed health, source selection, normalization, continuity, and OHLCV construction.

### 5.1 Artifacts

- `HeartbeatMonitor`: evaluates heartbeat failures and latency thresholds.
- `FeedCircuitBreaker`: owns healthy, degraded, failed-over, and recovering states.
- `FailoverRouter`: selects the active real-time source.
- `MarketEventNormalizer`: validates and normalizes provider events.
- `OHLCVAggregator`: constructs configured time-bucketed bars.
- `GapDetector`: identifies missing or partial bar intervals.
- `BarBackfiller`: retrieves, deduplicates, and injects missing bars.
- `RollingWindowStore`: maintains bounded indicator and model windows.

### 5.2 Go Contracts

```go
type FeedSelector interface {
    ActiveSource(ctx context.Context) (SourceID, error)
    ObserveHealth(ctx context.Context, health FeedHealth) error
}

type BarAggregator interface {
    Apply(ctx context.Context, event MarketEvent) ([]Bar, error)
}

type BarBackfiller interface {
    Fill(ctx context.Context, from, to time.Time) ([]Bar, error)
}
```

### 5.3 Functional Requirements

- Send or evaluate heartbeat activity every 1000 ms.
- Trigger failover after three consecutive heartbeat failures or latency above 1500 ms.
- Reconnect to Alpaca with exponential backoff.
- Record the last valid bar time, request the missing interval after reconnection, deduplicate returned bars, and restore state before model inference resumes.
- Emit a continuous normalized OHLCV sequence with source and data-quality metadata.

## 6. Analytics and Risk

**Ownership:** Feature production, signal generation, decision vectors, and pre-trade controls.

### 6.1 Artifacts

- `FeatureEngine`: derives volatility, spread, and microstructure features.
- `SignalEngine`: produces long, short, or flat signals with confidence.
- `DecisionEngine`: converts signals into target size and entry or exit instructions.
- `RiskEvaluator`: applies portfolio-level and order-level controls.
- `RiskRule`: independently testable guardrail contract.
- `MaxOrderValueRule`: enforces asset-class order caps.
- `LeverageRule`: applies volatility-regime leverage limits.
- `SlippageRule`: rejects orders when spread or modeled slippage exceeds thresholds.

### 6.2 Go Contracts

```go
type SignalEngine interface {
    Evaluate(ctx context.Context, input ModelInput) (Signal, error)
}

type RiskRule interface {
    Evaluate(ctx context.Context, order OrderIntent, state PortfolioState) RiskResult
}

type RiskEvaluator interface {
    Evaluate(ctx context.Context, order OrderIntent) (RiskDecision, error)
}
```

### 6.3 Functional Requirements

- Process only normalized, causally ordered market inputs.
- Attach `signal_id` and `decision_id` to every actionable decision.
- Return approved, resized, or rejected outcomes with machine-readable reasons.
- Record the market-data age, volatility regime, risk configuration version, and evaluated limits.
- Emit only risk-approved order instructions to the Execution Router.

## 7. Execution and Harness

**Ownership:** Live order routing, broker interaction, benchmark selection, and paper execution.

### 7.1 Artifacts

- `ExecutionRouter`: routes approved orders to live execution and applies benchmark policy.
- `BenchmarkPolicy`: selects `OFF`, `SAMPLED`, or `FULL` paper mirroring.
- `Broker`: technology-neutral live broker port.
- `AlpacaBrokerAdapter`: Alpaca order and event implementation.
- `IBKRBrokerAdapter`: IBKR order and event implementation.
- `PaperExecutionEngine`: simulates exchange matching from contemporaneous L2 state.
- `PaperExecutionEventPublisher`: publishes simulated execution outcomes.

### 7.2 Go Contracts

```go
type BenchmarkMode string

const (
    BenchmarkOff     BenchmarkMode = "OFF"
    BenchmarkSampled BenchmarkMode = "SAMPLED"
    BenchmarkFull    BenchmarkMode = "FULL"
)

type BenchmarkPolicy interface {
    Select(order OrderIntent) bool
}

type Broker interface {
    Submit(ctx context.Context, order OrderIntent) (BrokerOrder, error)
    Cancel(ctx context.Context, brokerOrderID string) error
    Events(ctx context.Context) (<-chan BrokerEvent, <-chan error)
}

type OrderRouter interface {
    Route(ctx context.Context, order OrderIntent) error
}

type FillSimulator interface {
    Simulate(ctx context.Context, order OrderIntent, book L2Snapshot) ([]PaperExecutionEvent, error)
}
```

### 7.3 Functional Requirements

- Route every approved order to the configured live broker while live mode is active.
- Permit direct order routing only for instruments classified as tradable stocks or ETFs; reject index order intents before broker submission.
- Assign `order_id` and an optional `benchmark_id` before fan-out.
- Send selected benchmark orders to paper execution at live submission time.
- Do not wait for paper execution, correlation, storage, or dashboard processing.
- Convert broker acknowledgments, replacements, cancellations, rejections, expirations, partial fills, and final fills into Live Execution Events.

## 8. Required Live Event Recording

**Ownership:** Durable live execution history and authoritative execution projections.

The architectural artifact is named the **Live Execution Event Stream**. In Go, producers and consumers use behavior-oriented interfaces rather than a framework-specific `Sink` type.

### 8.1 Artifacts

- `ExecutionEventPublisher`: appends events to the stream.
- `ExecutionEventConsumer`: independently consumes and checkpoints events.
- `LiveExecutionEventStream`: durable append-only transport adapter.
- `ExecutionLedgerProjector`: applies events to authoritative projections.
- `ExecutionLedger`: stores order, fill, position, cash, fee, and PnL records.
- `BrokerReconciler`: compares broker state with ledger state.
- `AuditArchive`: retains immutable events and configuration provenance.

### 8.2 Go Contracts

```go
type ExecutionEventPublisher interface {
    Publish(ctx context.Context, event ExecutionEvent) error
}

type ExecutionEventHandler interface {
    Handle(ctx context.Context, event ExecutionEvent) error
}

type ExecutionEventConsumer interface {
    Consume(ctx context.Context, handler ExecutionEventHandler) error
}

type ExecutionLedger interface {
    Apply(ctx context.Context, event ExecutionEvent) error
    Order(ctx context.Context, orderID string) (OrderRecord, error)
    Position(ctx context.Context, accountID, symbol string) (Position, error)
}
```

### 8.3 Event Contract

Every immutable `ExecutionEvent` includes:

- Event identity, schema version, order identity, broker identity, and sequence number.
- Signal, decision, and optional benchmark correlation IDs.
- Account, strategy, symbol, side, order type, time-in-force, price, and quantities.
- Event type, status, rejection reason, venue, commission, and fees.
- Exchange, broker, event, and local receipt timestamps.
- Market snapshot reference, feed source, position effect, and cash effect.

### 8.4 Functional Requirements

- Capture all live order-lifecycle events regardless of benchmark mode.
- Preserve append-only history and reject conflicting reuse of an `event_id`.
- Support idempotent replay and at-least-once consumer delivery.
- Preserve ordering within the selected partition key.
- Maintain independent ledger and benchmark consumer checkpoints.
- Never allow benchmark failure or backlog to block publishing or ledger consumption.

## 9. Optional Benchmark Analysis

**Ownership:** Correlation, derived execution-quality metrics, telemetry queries, and visualization.

### 9.1 Artifacts

- `BenchmarkCorrelator`: pairs live and paper outcomes by `benchmark_id`.
- `BenchmarkMetricEngine`: calculates fill, latency, slippage, and PnL comparisons.
- `BenchmarkRepository`: stores derived benchmark records.
- `BenchmarkQueryService`: supplies read-only dashboard queries.
- `BenchmarkService`: gRPC transport for configuration, queries, and updates.
- `HarnessBenchmarkDashboard`: separate gRPC client application.

### 9.2 Go Contracts

```go
type BenchmarkCorrelator interface {
    ObserveLive(ctx context.Context, event ExecutionEvent) error
    ObservePaper(ctx context.Context, event PaperExecutionEvent) error
}

type BenchmarkRepository interface {
    Save(ctx context.Context, result BenchmarkResult) error
    Query(ctx context.Context, filter BenchmarkFilter) ([]BenchmarkResult, error)
}

type BenchmarkQueryService interface {
    Summary(ctx context.Context, filter BenchmarkFilter) (BenchmarkSummary, error)
    Comparisons(ctx context.Context, filter BenchmarkFilter) ([]ExecutionComparison, error)
}
```

### 9.3 Functional Requirements

- Consume selected live events without owning or modifying the authoritative execution record.
- Correlate only orders selected under `SAMPLED` or `FULL` mode.
- Compare fill rates, modeled and realized slippage, lifecycle latency, and PnL variance.
- Identify missing live or paper counterparts and incomplete order lifecycles.
- Permit dashboard access on demand without requiring the dashboard to be continuously connected.
- Keep benchmark retention and failure handling independent of the execution ledger.

### 9.4 Dashboard gRPC Client

The dashboard should:

- Use server-side filters for time range, symbol, strategy, side, order type, volatility regime, and benchmark mode.
- Request summaries and paginated comparisons through unary RPCs.
- Optionally receive live metric updates through a server-streaming RPC.
- Remain read-only for execution, ledger, and audit data.
- Display feed health, fill comparison, slippage, latency, PnL variance, risk exceptions, and unmatched correlations.

## 10. Cross-Boundary Identifiers and Types

The following identifiers propagate end to end where applicable:

- `event_id`
- `signal_id`
- `decision_id`
- `order_id`
- `broker_order_id`
- `benchmark_id`
- `market_snapshot_id`
- `account_id`
- `strategy_id`

Shared classification contracts include `InstrumentType` with `STOCK`, `ETF`, and `INDEX` values, plus an explicit tradability indicator. These values belong in `quantram.proto` and map to equivalent Go domain values at the gRPC adapter boundary. An `INDEX` instrument is always non-tradable in QuanTRAM v1.

Shared message contracts must be versioned. Go domain types may map to protobuf messages at the gRPC adapter boundary, but generated protobuf types must not become the internal domain model.

## 11. End-to-End Processing Semantics

1. External feed adapters publish provider events with source and receipt timestamps.
2. Ingestion evaluates health, selects a source, normalizes events, fills gaps, and emits continuous OHLCV bars.
3. Analytics produces signals and order intentions; Risk approves, resizes, or rejects them.
4. The Execution Router sends approved orders to the live broker and optionally to the paper engine.
5. The broker adapter continuously publishes order-lifecycle events to the Live Execution Event Stream.
6. The ledger consumer builds authoritative execution, position, cash, fee, and PnL projections.
7. When benchmarking is enabled, the benchmark consumer correlates selected live events with paper events.
8. Derived benchmark results are stored and queried by the dashboard gRPC client as needed.

## 12. Deployment Decision Status

### 12.1 Decided

- Go is the implementation language.
- The six semantic boxes are ownership boundaries.
- Live event recording is required and continuous.
- Benchmark processing is optional, asynchronous, and non-blocking.
- The execution ledger is authoritative; benchmark telemetry is derived.
- The dashboard may be a separate read-only gRPC client.
- The v1 instrument universe is limited to U.S. stocks, ETFs, and published indices; indices are analytics-only and non-tradable.
- All QuanTRAM v1 protobuf services, enums, and messages reside in `api/proto/quantram/v1/quantram.proto` by default; splitting requires demonstrated operational or ownership need.

### 12.2 Provisional

- One Go gRPC server may initially host all server-side boundaries.
- Internal package calls may be used between co-located components.
- gRPC will expose external control and query surfaces.

### 12.3 Open

- Event transport: NATS JetStream, Kafka-compatible log, database outbox, or another durable mechanism.
- Ledger and benchmark persistence technologies.
- Partition key and ordering guarantees.
- Retry, checkpoint, and dead-letter policies.
- Protobuf package and RPC method definitions.
- Authentication, authorization, and transport security.
- Process decomposition and independent scaling thresholds. Proposed in the [QuanTRAM Process Model](QuanTRAM_PROCESS_MODEL_082926.md); not yet accepted as a closed decision.
- Paper matching model and L2 snapshot schema.
- PnL accounting, fee, corporate-action, and trading-calendar conventions.
- Retention and audit immutability requirements.

## 13. Initial Acceptance Criteria

- Every architecture artifact maps to exactly one owning boundary.
- Core packages compile without importing concrete broker, event transport, database, or gRPC implementations.
- Provider and broker adapters satisfy contract tests against their interfaces.
- Live execution events can be replayed without changing final ledger state.
- Benchmark processing can be stopped or fail without affecting live routing or ledger updates.
- `OFF`, `SAMPLED`, and `FULL` modes produce the specified paper-routing behavior.
- The dashboard can query stored benchmark results through gRPC without write access to execution state.
- Correlation IDs permit tracing from market input through decision, order, live event, ledger record, and benchmark result.
- Market-data contracts classify every instrument as `STOCK`, `ETF`, or `INDEX`, and execution rejects direct orders for `INDEX` instruments.
- The initial protobuf contract defines all QuanTRAM v1 services, enums, and messages in `api/proto/quantram/v1/quantram.proto`.

## 14. Change Log

| Date | Version | Change |
| :--- | :--- | :--- |
| August 28, 2026 | 0.5 | Limited the QuanTRAM v1 universe to U.S. stocks, ETFs, and published indices; added instrument classification and prohibited direct index order routing. |
| August 28, 2026 | 0.4 | Aligned the artifact specification, Go command names, protobuf path, package namespace, and parent reference with QuanTRAM naming. |
| August 28, 2026 | 0.3 | Established a single-file protobuf policy for `quantram.v1`: services, enums, and messages remain in `api/proto/quantram/v1/quantram.proto` unless demonstrated review, build, ownership, or deployment friction justifies decomposition. |
| August 28, 2026 | 0.2 | Added explicit parent-child provenance: the external market feed integration specification is the architecture and functional basis, while this document derives the Go artifacts, ownership boundaries, gRPC surfaces, and acceptance criteria. |
| August 28, 2026 | 0.1 | Initial E2E QuanTRAM artifact specification organized by six semantic ownership boundaries; established Go implementation direction, provisional single-server gRPC topology, required Live Execution Event Stream, optional benchmark analysis, and dashboard gRPC client. |
