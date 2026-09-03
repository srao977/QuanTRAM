# QuanTRAM Go/Proto -> MongoDB Persistence Forensic Mapping

**Date:** September 3, 2026  
**Status:** HUMAN REVIEW REQUIRED  
**Implementation:** NOT AUTHORIZED

## 1. Executive finding

QuanTRAM already has the three contracts needed for minimum scientific-history persistence: parent-proto `Bar`, `DecisionEvent`, and `PriceEvent`. MongoDB should preserve those contracts as nested documents rather than flattening Bar or translating events into generic `decision_values`.

The minimum recommended collections are:

1. `quantram_apertures`: new bounded persistence identity and lifecycle.
2. `quantram_payloads`: `{_id, aperture_id, bar}` where `bar` preserves the parent `Bar` contract.
3. `quantram_decisions`: `{_id, aperture_id, payload_id, decision_event?, price_event?}` where event fields preserve the parent proto contracts.

`quantram_decision_stages` is not required for V1 and should be deferred. The parent proto and SemanticService already own output structure and meaning. A later definition collection is justified only by a concrete stage-discovery/query requirement and must reference, not duplicate, those authorities.

All writes are owned by QuanTRAM Go. The dashboard has no write role, credentials, reconstruction role, or semantic authority. MongoDB is not on the scientific critical path.

## 2. Sources of authority and ownership

Precedence used in this investigation:

1. Current executable QuanTRAM Go implementation.
2. Parent proto: `api/proto/quantram/v1/quantram.proto`.
3. Current Go domain types.
4. Semantic Contract V1.0.
5. Human-approved Aperture/Payload decisions in the investigation request.

| Concern | Authority |
|---|---|
| Contract authority | Parent QuanTRAM repository and parent proto |
| Scientific authority | QuanTRAM Go pipeline |
| Write authority | QuanTRAM Go only |
| Persistence authority | Future Go-side persistence adapter/service |
| Database | MongoDB `quantram_db` |
| Read access | Future Go gRPC persistence/query service |
| Dashboard role | Future read/render/explain consumer only |

The dashboard was not inspected or modified. MongoDB credentials and scientific writes must never reside there.

## 3. Current QuanTRAM proto persistence classification

The parent proto contains **30 messages**. Embedded messages are persisted only as part of their canonical parent event; they are not independent Mongo documents.

| Message | Category | Go producer | Existing ID(s) | Persistence value | Persist? | Proposed container | Reason |
|---|---|---|---|---|---|---|---|
| `Bar` | SCIENTIFIC INPUT | marketfeed constructors; `server.toProtoBar` maps | `market_snapshot_id` | Authoritative accepted market input | Yes | `quantram_payloads.bar` | Causal input contract |
| `GetFeedHealthRequest` | REQUEST WRAPPER | gRPC client | None | None | No | - | Empty point-in-time request |
| `FeedHealth` | HEALTH / READINESS | `Pipeline.FeedHealth`; `server.toProtoHealth` | `source_id` (identity, not event ID) | Transient operational state | No | - | No historical-health requirement |
| `GetActiveSourceRequest` | REQUEST WRAPPER | gRPC client | None | None | No | - | Empty point-in-time request |
| `ActiveSource` | OPERATIONAL TELEMETRY | `Server.GetActiveSource` | `source_id` | Current source/state | No | - | Derivable point-in-time response |
| `StreamBarsRequest` | STREAM WRAPPER | gRPC client | None | None | No | - | Subscription filter/control |
| `GetBarWindowRequest` | REQUEST WRAPPER | gRPC client | symbol | None | No | - | Query parameters only |
| `BarWindow` | RESPONSE WRAPPER | `Server.GetBarWindow` | symbol; embedded Bar IDs | No independent value | No | - | Reconstructed from Payloads |
| `TriggerGapFillRequest` | CONTROL PLANE | gRPC client | symbol/time bounds | Operator command | No | - | Command, not scientific event |
| `GapFillResult` | RESPONSE WRAPPER / OPERATIONAL TELEMETRY | `Server.TriggerGapFill` | symbol | Operation summary | No | - | No approved audit requirement |
| `GetHealthRequest` | REQUEST WRAPPER | gRPC client | None | None | No | - | Empty point-in-time request |
| `ComponentHealth` | HEALTH / READINESS | Pipeline/modelhost; `Server.GetHealth` maps | component name | Transient component state | No | - | No approved history requirement |
| `HealthReport` | HEALTH / READINESS | `Server.GetHealth` | None | Aggregate current health | No | - | Point-in-time composition |
| `GetReadinessRequest` | REQUEST WRAPPER | gRPC client | None | None | No | - | Empty point-in-time request |
| `ReadinessReport` | HEALTH / READINESS | `Server.GetReadiness` | None | Current observe/infer readiness | No | - | Transient gate state |
| `Decision` | SCIENTIFIC OUTPUT | `adaptive.Engine.stepLocked`; `server.toProtoDecision` | Parent `decision_id` | Adaptive terminal decision | Yes, embedded | `quantram_decisions.decision_event.decision` | Part of DecisionEvent oneof |
| `Skip` | ERROR / STATUS | adaptive/modelhost; `server.toProtoSkip` | Parent `event_id` | Typed adaptive non-decision | Yes, embedded | `quantram_decisions.decision_event.skip` | Part of DecisionEvent oneof; Skip is not HOLD |
| `DecisionEvent` | DERIVED EVENT | adaptive/modelhost; `server.toProtoDecisionEvent` | `event_id`, `signal_id`, `decision_id`, `market_snapshot_id` | Canonical committed/gated adaptive event | Yes | `quantram_decisions.decision_event` | Existing complete transport contract |
| `StreamDecisionsRequest` | STREAM WRAPPER | gRPC client | None | None | No | - | Subscription filter/control |
| `PriceEmission` | SCIENTIFIC OUTPUT | `pricing.EmissionPolicy`; `server.toProtoPriceEmission` | Parent `event_id` | Canonical exposed P-04 emission | Yes, embedded | `quantram_decisions.price_event.emission` | Part of PriceEvent |
| `PriceCockpit` | SCIENTIFIC OUTPUT | `pricing.cockpitInterpreter`; `server.toProtoPriceCockpit` | Parent `event_id` | Canonical exposed cockpit interpretation | Yes, embedded | `quantram_decisions.price_event.cockpit` | Part of PriceEvent |
| `PricingSkip` | ERROR / STATUS | pricing/modelhost; `server.toProtoPricingSkip` | Parent `event_id` | Typed pricing non-emission | Yes, embedded | `quantram_decisions.price_event.skip` | Part of PriceEvent |
| `PriceEvent` | DERIVED EVENT | pricing/modelhost; `server.toProtoPriceEvent` | `event_id`, `market_snapshot_id` | Canonical P-04 transport event | Yes | `quantram_decisions.price_event` | Existing transport contract |
| `StreamPriceEventsRequest` | STREAM WRAPPER | gRPC client | None | None | No | - | Subscription filter/control |
| `GetSemanticTermRequest` | REQUEST WRAPPER | gRPC client | semantic ID | None | No | - | Lookup parameters only |
| `ListSemanticTermsRequest` | REQUEST WRAPPER | gRPC client | None | None | No | - | Lookup filters only |
| `GetSemanticContractRequest` | REQUEST WRAPPER | gRPC client | None | None | No | - | Empty point-in-time request |
| `SemanticContractInfo` | CONFIGURATION / RESPONSE | SemanticService | contract version | Interpretation metadata | Reference version only | Aperture metadata | Persist version, not dictionary response |
| `SemanticTerm` | CONFIGURATION / RESPONSE | SemanticService | semantic ID | Canonical explanation | No | - | SemanticService remains dictionary owner |
| `ListSemanticTermsResponse` | RESPONSE WRAPPER | SemanticService | embedded semantic IDs | No independent value | No | - | Recreated from SemanticService |

Top-level persistence candidates are `Bar`, `DecisionEvent`, and `PriceEvent`. `Decision`, `Skip`, `PriceEmission`, `PriceCockpit`, and `PricingSkip` persist only through their parent events.

## 4. Existing scientific input contract: Bar

The current parent `Bar` has exactly 22 fields; the list in the request is current and complete.

| Parent proto field | Proto type | Go source | Forensic note |
|---|---|---|---|
| `symbol` | string | `domain.Bar.Symbol` | normalized symbol |
| `instrument_id` | string | `domain.Bar.InstrumentID` | currently symbol for implemented sources |
| `instrument_type` | `InstrumentType` | `domain.Bar.InstrumentType` | STOCK/ETF/INDEX |
| `tradable` | bool | `domain.Bar.Tradable` | classification result |
| `interval` | string | `domain.Bar.Interval` | currently `1Min`; not an adjacency guarantee |
| `interval_start_unix_ms` | int64 | `domain.Bar.IntervalStart` | event/interval start |
| `interval_end_unix_ms` | int64 | `domain.Bar.IntervalEnd` | interval end |
| `open` | double | `domain.Bar.Open` | OHLC |
| `high` | double | `domain.Bar.High` | OHLC |
| `low` | double | `domain.Bar.Low` | OHLC |
| `close` | double | `domain.Bar.Close` | OHLC and current model price input |
| `volume` | uint64 | `domain.Bar.Volume` | volume |
| `event_count` | uint32 | `domain.Bar.EventCount` | contributing provider events |
| `source_timestamp` | string | `domain.Bar.SourceTimestamp` | exact provider text; QuanTRAM assigns no timing semantics |
| `receipt_unix_ms` | int64 | `domain.Bar.ReceiptTime` | local receipt time |
| `source` | string | `domain.Bar.Source` | provider/feed identity |
| `quality_status` | `QualityStatus` | `domain.Bar.QualityStatus` | canonical quality classification |
| `is_final` | bool | `domain.Bar.IsFinal` | finalized status |
| `is_backfilled` | bool | `domain.Bar.IsBackfilled` | historical injection marker |
| `source_transition` | bool | `domain.Bar.SourceTransition` | source transition marker |
| `data_age_ms` | int64 | `bar.DataAge(time.Now())` in `server.toProtoBar` | derived when serialized; not stored in `domain.Bar` |
| `market_snapshot_id` | string | `domain.Bar.MarketSnapshotID` | SHA-256 over symbol/source/source timestamp/OHLCV |

Mongo `bar` should use these exact proto field names and nesting. It must not be flattened, renamed, or interpreted through viewer terminology. `data_age_ms` requires one explicit implementation decision: persisting a proto-shaped Bar freezes the existing `now - IntervalStart` value at persistence serialization time. It cannot remain a dynamically recalculated response value in immutable history. This is not a reason to invent another Bar shape; it is a proto-contract timing question for human approval.

## 5. Bar authoritative producer and acceptance trace

```text
Alpaca WebSocket JSON
  -> marketfeed.barFromRaw
  -> marketfeed.barFromAlpaca

Alpaca REST response
  -> marketfeed.AlpacaREST.fetchPage
  -> marketfeed.barFromAlpaca(backfilled=true)

CSV row
  -> marketfeed.barFromCSV

domain.Bar
  -> ingestion.Pipeline.accept
  -> WindowStore.Add(bar) == true          [ingestion-accepted]
  -> Pipeline.fanout(bar)                  [observe path]
  -> Pipeline.fanoutModel(bar)
       -> Bar.ModelEligible()
       -> ClassifyBarContinuity            [first/normal/irregular accepted]
       -> modelhost Host worker             [science path]
```

`barFromAlpaca` validates symbol, timestamp, OHLC, volume, instrument classification, quality/finality, and creates `MarketSnapshotID`. WebSocket closed bars are final/complete/not-backfilled. REST bars are final/reconstructed/backfilled and therefore not model eligible. CSV rows are final/complete/not-backfilled.

There are two legitimate acceptance meanings:

| Boundary | Exact point | Meaning |
|---|---|---|
| Ingestion acceptance | `Pipeline.accept`, immediately after `p.window.Add(bar)` returns true | Authoritative Bar generation accepted into QuanTRAM runtime history; includes partial/backfilled observations |
| Scientific model eligibility | `Pipeline.fanoutModel`, after `ModelEligible` and continuity checks, immediately before model subscriber enqueue | Bar is eligible to enter D01/P-04 |

For the stated goal of preserving existing Go-side runtime history, the Payload capture point is the first boundary: immediately after `WindowStore.Add` returns true and before lossy `fanout`. This persists every accepted canonical Bar generation independently of gRPC or dashboard availability. Only the model-eligible subset later receives Decisions. If human intent is instead “only observations consumed by science,” move capture to successful model-path enqueue; that intentionally loses accepted partial/backfill history and must be approved explicitly.

## 6. Existing scientific output contract: DecisionEvent

Executable adaptive lineage:

```text
domain.Bar
  -> adaptive.ObservationFromBar
  -> Model.Step                  -> DMOOutput + FMOOutput
  -> BuildReturnShape            -> ReturnShape
  -> EvaluateCapturability       -> CapturabilityResult
  -> adaptive.Engine.stepLocked  -> domain.DecisionEvent
  -> modelhost.Host.handle prepare/commit
  -> modelhost.Host.emit
  -> server.toProtoDecisionEvent -> parent proto DecisionEvent
```

Parent `DecisionEvent` fields are:

| Field | Meaning |
|---|---|
| `event_id` | terminal event identity generated by adaptive engine or host gate |
| `signal_id` | adaptive signal checkpoint; empty for some gate/error events |
| `decision_id` | actionable decision identity; empty for Skip |
| `symbol` | symbol |
| `interval_start_unix_ms` | source Bar interval start |
| `market_snapshot_id` | triggering Bar snapshot hash when a real Bar is present |
| `source_timestamp` | provider timestamp text |
| `accepted_sequence` | adaptive engine completed-count position; not Aperture sequence |
| `received_at_unix_ms` | adaptive evaluation start/host gate time |
| `completed_at_unix_ms` | adaptive evaluation completion time |
| `latency_ms` | evaluation duration |
| `model_version` | current adaptive model version (`0.2`) |
| `schema_version` | current event schema (`quantram.adaptive.v1`) |
| `pre_state_hash` | committed D01 state hash before evaluation |
| `post_state_hash` | resulting state hash; unchanged on non-commit |
| `decision` | oneof terminal BUY/SELL/HOLD with D02/D04 evidence |
| `skip` | oneof typed non-decision with detail/status |

`domain.DecisionEvent` contains the same information with Go-native timestamps/duration and pointers. `server.toProtoDecisionEvent` is the authoritative domain-to-parent-proto mapping.

**Is DecisionEvent already the canonical persistable adaptive output? YES.** It is the existing terminal contract after D01/D02/D04/emitter processing. It intentionally does not contain every internal DMO/FMO/ReturnShape field. Persisting those intermediates would be a new contract decision; Mongo must not infer or manufacture them.

Gate-generated DecisionEvents are also real runtime events, but `Host.refreshPathStatus` can synthesize a minimal Bar without `market_snapshot_id`. Such an event has no repository-proven Payload correlation and cannot be forced into a Payload-owned Decisions document. V1 should persist only DecisionEvents with a resolvable accepted Payload; a future operational-event requirement may address uncorrelated host status events.

## 7. Existing scientific output contract: PriceEvent

P-04 is not downstream of DecisionEvent. The current host sends the same model-eligible Bar to adaptive and pricing prepare paths, coordinates their commit, emits PriceEvent first when pricing is enabled, and then emits DecisionEvent.

Parent `PriceEvent` fields are:

| Field | Meaning |
|---|---|
| `event_id` | host-assigned price event identity |
| `symbol` | symbol |
| `interval_start_unix_ms` | pricing active-row interval |
| `market_snapshot_id` | snapshot copied through pricing Observation |
| `source_timestamp` | pricing active-row provider timestamp |
| `accepted_sequence` | pricing observation/active-row sequence |
| `latency_ms` | pricing evaluation duration |
| `status` | warm-up, unavailable, emitted, or canonical projection failure |
| `emitted` | pricing emission flag |
| `domain_exit` | EXPM projection domain-exit flag |
| `rk_success` | legacy field name meaning projection succeeded; production is EXPM |
| `emission` | exposed color/phase/tendency/confidence/domain/stability/direction/reasons/numerical-stability values |
| `skip` | typed pricing non-emission/failure |
| `cockpit` | exposed cockpit interpretation |

`PriceEmission` has 13 proto fields: `color`, `trajectory_phase`, `turning_tendency`, `confidence_state`, `domain_state`, `stability_state`, `current_direction`, `projected_direction`, `reason_codes`, `rk_success`, `condition_number`, `max_real_eigenvalue`, and `perturbation_amplification`.

`PriceCockpit` has six: `cockpit_color`, `refined_internal_state`, `persistence_state`, `turn_candidate`, `domain_state`, and `confidence_state`. `PricingSkip` has `reason` and `detail`.

**Is PriceEvent already the canonical persistable P-04 output? YES, when “canonical” means the parent proto contract.** `domain.PriceEmission` and `domain.PriceCockpit` contain additional internal numerical/cockpit fields which the parent proto deliberately does not expose. V1 should not create a Mongo-only superset. If complete internal-output retention is required, the parent proto must be extended first in a separately authorized contract change.

Current executable P-04 preserves a one-row lag: the newest close is pending while derivatives/F4/cover evaluate `active = current - 1`. During emission, `interval_start` and `source_timestamp` identify that active row, while the code currently assigns `Snapshot: obs.Snapshot` from the newest triggering input. Therefore these fields do not necessarily identify the same timestamped row. Persistence must preserve them exactly and must not “correct” either value. Human review must decide whether `market_snapshot_id` is intentionally the triggering Payload correlation for P-04.

## 8. Go authoritative write-point matrix

| Event/object | Go package/type | Producer function | When authoritative | Existing correlation | Mongo candidate | Proposed write | Async? | Notes |
|---|---|---|---|---|---|---|---|---|
| Bar | `domain.Bar` | `marketfeed.barFromAlpaca`, `barFromCSV`; accepted by `Pipeline.accept` | `WindowStore.Add(bar)` returns true | `market_snapshot_id`, symbol, interval/source timestamps, DedupKey | `quantram_payloads` | Insert `{_id, aperture_id, bar: toProtoBar-at-capture}` | Yes | Before lossy fanout; replacement generations may create distinct Payloads |
| DecisionEvent | `domain.DecisionEvent` | `adaptive.Engine.stepLocked`; host gate helpers | After coordinated commit decision or terminal gate outcome, on entry to `Host.emit` | event/signal/decision IDs; snapshot; symbol/time; accepted sequence; state hashes | `quantram_decisions` | Upsert by resolved `payload_id`, set canonical `decision_event` once | Yes | Capture before subscriber fanout; synthetic uncorrelated gate events excluded/deferred |
| PriceEvent | `domain.PriceEvent` | `pricing.Engine.event`, finalized in `Host.handle` | After adaptive and pricing working states both commit and host assigns event ID | event ID; snapshot; symbol/time; accepted sequence | `quantram_decisions` | Upsert by resolved `payload_id`, set canonical `price_event` once | Yes | Host currently calls `emitPrice` before adaptive `emit` |
| FeedHealth | `domain.FeedHealth` | marketfeed/Pipeline | Point-in-time read | source ID | None | None | - | Transient only |
| HealthReport | `domain.HealthReport` | `Server.GetHealth` composition | Point-in-time read | component names | None | None | - | No historical requirement |
| Readiness | `domain.Readiness` | `Pipeline.Readiness` | Point-in-time read | None | None | None | - | Transient gate state |

Future persistence capture must not subscribe to the current public channels: Bar observe fanout drops/replaces for slow subscribers, model event fanout drops when subscriber buffers fill, and those channels depend on subscribers. The owning Go adapter needs explicit nonblocking capture hooks at the authoritative points above.

## 9. Current executable lineage and persistence capture

```mermaid
flowchart TD
    A[Aperture: persistence lifecycle] --> P[Payload: Mongo _id plus aperture_id plus canonical Bar]
    B[domain.Bar accepted by Pipeline] --> C[D01: DMOOutput plus FMOOutput]
    C --> D[D02: ReturnShape]
    D --> E[D04: CapturabilityResult]
    E --> F[Adaptive Emitter]
    F --> G[domain.DecisionEvent]
    B --> H[P-04 pricing prepare: active row is current minus 1]
    H --> I[domain.PriceEvent]
    G --> J[modelhost coordinated commit]
    I --> J
    J --> K[Host.emitPrice then Host.emit]
    B -. nonblocking Go capture after WindowStore.Add .-> P
    K -. nonblocking Go capture before lossy fanout .-> M[Decisions: canonical DecisionEvent and PriceEvent]
    P --> M
```

MongoDB does not sit between Bar, D01, D02, D04, emitter, or P-04. The diagram’s `Payload -> Decisions` arrows are persistence lineage, not scientific invocation order.

## 10. Existing ID and correlation matrix

| ID / correlation field | Created where | Meaning | Scope | Propagated through | Persistent? | Relationship to `payload_id` |
|---|---|---|---|---|---|---|
| Aperture `_id` | Future Mongo Aperture insert | Bounded persistence parent | Global Mongo identity | Payload and Decisions wrapper | Yes | Parent of Payload |
| Payload `_id` | Mongo Payload insert | Permanent persisted Payload identity | Global Mongo identity | Decisions wrapper/future lineage | Yes | Is `payload_id` |
| `market_snapshot_id` | `domain.SnapshotID` in marketfeed constructors | Hash of symbol, source, provider timestamp, OHLCV | Input snapshot; collision-resistant, not process-global sequence | Bar -> adaptive event; Bar -> pricing Observation/event | Yes, inside canonical contracts | Existing lookup key used by persistence adapter to resolve Payload `_id`; not replaced or redefined |
| Bar DedupKey | `domain.Bar.DedupKey` | symbol + interval start | WindowStore per-symbol runtime dedup | WindowStore only | No independent field | Not a Payload identity |
| Adaptive `event_id` | `adaptive.Engine.stepLocked`; host gate helpers | Terminal adaptive/host event counter label | Engine/host process lifetime; resets on restart | DecisionEvent/proto | Yes inside event | Child event identity, not Payload identity |
| `signal_id` | `adaptive.Engine.stepLocked` | accepted adaptive signal checkpoint | Per-symbol engine lifetime | DecisionEvent/proto | Yes inside event | Child correlation only |
| `decision_id` | `adaptive.Engine.stepLocked` | actionable adaptive Decision identity | Per-symbol engine lifetime | DecisionEvent/proto | Yes inside event | Child decision identity only |
| Price `event_id` | `Host.handle` | host-sequenced PriceEvent label | Host process lifetime, across symbols | PriceEvent/proto | Yes inside event | Child event identity only |
| `accepted_sequence` | adaptive completed count / pricing observation index | Component-local accepted/active sequence | Engine lifetime; adaptive and pricing meanings differ | Respective event/proto | Yes inside event | Not a Payload identity or fixed-time bar number |
| `interval_start_unix_ms` | Bar; P-04 active Observation | Event interval; Price uses active row | Event-specific | Bar/events/proto | Yes | Query/cross-check only; Price can lag triggering Payload |
| `source_timestamp` | provider Bar / P-04 active Observation | Exact provider text | Provider event | Bar/events/proto | Yes | Query/cross-check only |
| `pre_state_hash` | adaptive engine before evaluation | Committed D01 state before event | Adaptive state transition | DecisionEvent/proto | Yes | Audit evidence for child event |
| `post_state_hash` | adaptive engine after prepare or unchanged on failure | Resulting D01 state | Adaptive state transition | DecisionEvent/proto | Yes | Audit evidence for child event |

No current event field is a durable replacement for Mongo `payload_id`. `market_snapshot_id` is nevertheless a suitable existing correlation carrier for resolving that identity in the persistence layer because it is already propagated from the triggering Bar. This use does not alter its scientific meaning.

## 11. Payload correlation problem and minimum mechanism

The approved Payload `_id` is Mongo-generated, so it does not exist when `Pipeline.accept` first receives the Bar and may not exist before modelhost emits results. Science must not wait for MongoDB.

Minimum mechanism:

1. The Go persistence adapter accepts a Bar capture keyed by `(aperture_id, market_snapshot_id)`.
2. Its asynchronous worker inserts/upserts `{_id, aperture_id, bar}` and records the resulting Mongo `_id` in its persistence-owned correlation state.
3. DecisionEvent and PriceEvent captures retain their existing `market_snapshot_id` and are buffered by the same key until `payload_id` resolves.
4. The worker upserts one Decisions wrapper by `payload_id`; PriceEvent and DecisionEvent may arrive in either order.
5. A unique V1 index on `{aperture_id: 1, "bar.market_snapshot_id": 1}` makes retry and lookup deterministic.

This requires no D01, D02, D04, emitter, EXPM, domain-event, or proto change. It requires a lightweight persistence correlation context owned entirely by the adapter. If human review determines P-04’s current snapshot assignment does not identify its triggering Payload, stop before implementation and establish the intended parent-proto correlation contract; do not guess from `accepted_sequence` or timestamps.

## 12. Aperture relationship and ownership

Aperture is the only genuinely new domain concept in this design. Minimum metadata remains:

```javascript
{
  _id: ObjectId,
  sequence_num: NumberLong,
  opened_at: Date,
  shut_at: Date | null,
  status: "OPEN" | "SHUT",
  created_at: Date,
  semantic_contract_version: "1.0"
}
```

No speculative provenance object is required for the minimum architecture. Adaptive `model_version` and `schema_version` already travel in DecisionEvent. PriceEvent has no model/schema version; adding one must be proto-first if required.

Future Go-side Aperture state logically belongs in an application-level Aperture manager adjacent to the persistence adapter, not ingestion or science. Startup receives or resolves an OPEN Aperture, verifies status/version, and keeps its `_id` in runtime application context. Process stop does not SHUT the Aperture; a later process may reattach. Ambiguous multiple OPEN Apertures require explicit operator selection.

## 13. Persistence candidates

```javascript
// quantram_payloads
{
  _id: ObjectId(),              // payload_id, generated by MongoDB
  aperture_id: ObjectId(),
  bar: {                        // exact parent proto field names
    symbol: "AAPL",
    instrument_id: "AAPL",
    instrument_type: "INSTRUMENT_TYPE_STOCK",
    tradable: true,
    interval: "1Min",
    interval_start_unix_ms: NumberLong(...),
    interval_end_unix_ms: NumberLong(...),
    open: 0.0,
    high: 0.0,
    low: 0.0,
    close: 0.0,
    volume: NumberLong(...),
    event_count: NumberInt(...),
    source_timestamp: "...",
    receipt_unix_ms: NumberLong(...),
    source: "ALPACA_IEX",
    quality_status: "QUALITY_STATUS_COMPLETE",
    is_final: true,
    is_backfilled: false,
    source_transition: false,
    data_age_ms: NumberLong(...),
    market_snapshot_id: "..."
  }
}

// quantram_decisions
{
  _id: ObjectId(),
  aperture_id: ObjectId(),
  payload_id: ObjectId(),
  decision_event: { /* exact parent DecisionEvent contract; optional */ },
  price_event: { /* exact parent PriceEvent contract; optional */ }
}
```

Enum encoding must be chosen once and shared with the standard protobuf serialization policy. The example uses canonical proto enum names to avoid ambiguous English strings. There are no top-level copies of Bar or event fields.

Recommended indexes:

```text
quantram_apertures: unique sequence_num
quantram_payloads: unique (aperture_id, bar.market_snapshot_id)
quantram_payloads: (aperture_id, bar.symbol, bar.interval_start_unix_ms)
quantram_decisions: unique payload_id
quantram_decisions: (aperture_id, payload_id)
```

## 14. Transient and non-persistent contracts

Request/stream wrappers, BarWindow, GapFillResult, SemanticService response wrappers, FeedHealth, ActiveSource, ComponentHealth, HealthReport, and ReadinessReport are not V1 persistence documents. They are commands, query envelopes, reconstructed views, or point-in-time operational state.

Health failures should still expose future persistence-adapter health through existing Go health composition, but storing every health response is neither required nor semantically useful. If an operational audit requirement emerges, design an explicit operational event contract first; do not scrape periodic response snapshots into MongoDB.

## 15. Decisions representation comparison

| Dimension | Option A: generic stage results | Option B: canonical events | Option C: separate event collections |
|---|---|---|---|
| Duplication | High: second field model | Low: wrapper only | Low event duplication, more collection structure |
| Semantic fidelity | Translation can drift/omit oneof and IDs | Exact parent contracts | Exact contracts |
| Extensibility | Generic array is structurally open but weakly governed | Add a new canonical event field after proto contract exists | Add collection after contract exists |
| Queryability | Dynamic paths and stage joins | Stable named event paths | Strong per-event queries; extra joins |
| Document growth | Small today; unknown generic stages | Bounded by one current event of each kind | Small individual documents |
| Versioning | Invented per-stage versions | Existing event schema where present; proto evolution governs | Proto evolution governs |
| Go write complexity | Mapping every output twice | Reuse authoritative proto mappers | Reuse mappers plus more repositories |
| Future stage addition | Append generic result without parent contract | Proto-first named addition | Proto-first collection addition |
| Backward traversal | Wrapper IDs provide direct path | Wrapper IDs provide direct path | Requires indexed lookup across collections |
| Synchronization risk | High | Low | Medium across writes/collections |

**Recommendation: Option B.** `quantram_decisions` remains useful as the one-Payload causal wrapper, but its children are canonical `DecisionEvent` and `PriceEvent`, not generic `decision_values`. This preserves current contracts and supports events arriving independently. Option A creates a second scientific representation. Option C adds collections and joins without a current scale or query requirement.

## 16. DecisionStage assessment

`quantram_decision_stages` is **DEFERRED, not deleted as a concept**. Current executable stage identity is expressed by packages/functions; terminal output contracts are expressed by the proto; semantic meaning is expressed by SemanticService. A Mongo definition document would currently duplicate those sources and could misleadingly imply D01/D02/D04 intermediates are persisted independently.

If a future API needs a data-driven ordered catalog, DecisionStage may contain only stable ID, name, purpose, display order, and parent-proto output-contract reference. It must not contain generated results or copy semantic definitions.

## 17. Async persistence boundary

Recommended future application ports:

```text
captureAcceptedBar(aperture_id, domain.Bar)
captureDecisionEvent(aperture_id, domain.DecisionEvent)
capturePriceEvent(aperture_id, domain.PriceEvent)
```

Each call performs a bounded, nonblocking handoff to a persistence-owned durable or explicitly loss-accounted spool. Mapping to parent proto occurs Go-side before BSON encoding. The database worker handles insert/upsert, payload-ID resolution, retries, and idempotency.

Capture points are immediately after `WindowStore.Add` returns true for Bar, and at entry to `Host.emit` / `Host.emitPrice` before current lossy subscriber fanout for events. These are recommendations for a later implementation, not changes made by this investigation.

## 18. Persistence failure considerations

| Failure | Design response |
|---|---|
| Mongo temporarily unavailable | Keep science running; mark adapter degraded; retry from bounded spool |
| Mongo slow | Never wait between stages; observe queue depth/oldest age/write latency |
| Mongo disconnected/restarting | Reconnect with bounded backoff; preserve ordered work where required |
| Spool capacity reached | Emit explicit persistence-loss/backpressure health and counters; apply human-approved drop/stop-capture policy, never silent loss |
| Duplicate delivery | Idempotent Payload key and unique `payload_id` Decisions upsert |
| Process shutdown | Time-bounded spool flush; report unflushed count; do not SHUT Aperture automatically |

The existing health surface can report persistence adapter `HEALTHY`, `DEGRADED`, or `UNAVAILABLE` later. No retry subsystem, health change, or Event Hubs integration is authorized here.

## 19. Minimum MongoDB recommendation

Use three normal collections in V1:

```text
quantram_apertures
  bounded metadata only

quantram_payloads
  {_id, aperture_id, bar: canonical Bar}

quantram_decisions
  {_id, aperture_id, payload_id,
   decision_event?: canonical DecisionEvent,
   price_event?: canonical PriceEvent}
```

No `bars`, `decision_events`, `price_events`, `health`, payload-definition, or stage-definition collection is currently required. No `payload_type` or generic discriminator is required. Payload remains the architectural collection name while V1 contains only Bar.

## 20. What MongoDB genuinely adds

MongoDB adds durable identities (`aperture_id`, `payload_id`), process-independent Aperture lifetime, historical retention beyond in-memory windows/last-event caches, indexed Payload-to-event traversal, idempotent recovery, and future Go-owned query support. It does not add scientific fields or meaning.

## 21. What MongoDB must not redefine

MongoDB must not redefine Bar fields, enum meanings, Decision versus Skip, HOLD semantics, P-04 color/phase/tendency, EXPM projection status, event IDs, component-local accepted sequences, state hashes, timing, or Semantic Contract definitions. It must not flatten Bar, reconstruct events from UI streams, use presentation aliases as science, or expose dashboard writes.

## 22. Repository discrepancies and constraints

1. Parent proto `Bar` contains `data_age_ms`, but `domain.Bar` does not; `server.toProtoBar` computes it at send time. Immutable persistence needs a reviewed capture-time interpretation.
2. `domain.PriceEmission` and `domain.PriceCockpit` have more fields than their parent proto messages. Parent-proto-first persistence intentionally excludes those fields unless proto is extended first.
3. P-04 preserves required active-row lag, but its emitted active-row timestamps are paired with the newest input’s snapshot ID. The intended causal meaning needs confirmation before relying on that field as Payload correlation.
4. Adaptive/price event IDs and accepted sequences reset with runtime engine/host state; none is a durable Payload identity.
5. Some host gate events are created from synthetic minimal Bars and have no snapshot ID; they cannot be assigned to a Payload without inventing lineage.
6. The existing Compass script and prior Mongo design encode assumptions from an earlier generic-stage design. Both remain stale/pending review and were not modified or executed.

## 23. Open questions

1. Does Payload history include every ingestion-accepted Bar generation, or only the model-eligible subset? This investigation recommends every ingestion-accepted generation for complete Go runtime history.
2. Should persisted `data_age_ms` freeze the existing calculation at capture time, or should the parent proto first distinguish dynamic response age from durable Bar state?
3. Is P-04 `market_snapshot_id` intentionally the newest triggering Payload while its interval/source timestamp identify the prior active row?
4. Are synthetic host gate events intentionally transient, or is a future non-Payload operational-event ledger required?
5. Is parent-proto P-04 output sufficient, or must currently domain-only numerical/cockpit fields become parent-proto fields before persistence?
6. What bounded spool and capacity-exhaustion policy meets the no-block/no-silent-loss requirement?
7. May multiple Apertures be OPEN concurrently, and how is one selected at startup?

## 24. Human decisions required

Human approval is required for the seven questions above, the three-collection recommendation, ObjectId identity, canonical enum encoding, and Option B Decisions shape. In particular, P-04 correlation and `data_age_ms` must be resolved before implementing validators or changing the stale Compass script.

## 25. Proposed next implementation step

After human approval, revise the existing Mongo design and Compass script to the approved proto-preserving schema. Then define persistence DTO/mapping tests that prove BSON round-trips against parent proto messages before adding a MongoDB driver or runtime write hooks. Runtime persistence remains a later, separately authorized increment.

## 26. Validation and change log

Baseline results for this investigation:

```text
go test ./...                                  PASS
go run ./cmd/quantram-semantics validate      PASS
go run ./cmd/quantram-semantics build --check PASS
go run ./cmd/quantram-semantics audit         DIAGNOSTIC
```

Audit summary: 97 findings, 6 missing tokens, 0 orphaned terms, and 9 review collisions. No audit finding was fixed or converted into a persistence field during this task.

| Date | Change |
|---|---|
| 2026-09-03 | Initial parent-Go/proto-to-MongoDB forensic mapping. No executable, proto, dashboard, Compass, database, or scientific change. |