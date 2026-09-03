# QuanTRAM MongoDB Aperture Persistence V1

**Date:** September 3, 2026  
**Status:** IMPLEMENTED, OFFLINE VALIDATED

## Contract

QuanTRAM Go remains the scientific and write authority. MongoDB is an asynchronous historical ledger outside the D01 -> FMO -> D02 -> D04 -> emitter and P-04 computation paths. The dashboard is a future read-only consumer.

V1 uses database `quantram_db` and exactly three collections:

1. `quantram_apertures`
2. `quantram_payloads`
3. `quantram_decisions`

`quantram_decision_stages` is deferred. Generic `decision_values`, flattened Bars, dashboard writes, and Mongo-owned scientific semantics are forbidden.

## Documents

```javascript
// quantram_apertures
{
  _id: ObjectId(),
  sequence_num: NumberLong(1),
  open: ISODate(),
  shut: null,
  status: "OPEN", // OPEN -> SHUT on orderly server shutdown
  semantic_contract_version: "1.0",
  model_version: "...",
  schema_version: "...",
  created_at: ISODate()
}

// quantram_payloads
{
  _id: ObjectId(),
  aperture_id: ObjectId(),
  bar: { /* canonical nested domain.Bar BSON fields, including open and market_snapshot_id */ }
}

// quantram_decisions
{
  _id: ObjectId(),
  aperture_id: ObjectId(),
  payload_id: ObjectId(),
  decision_event: { /* canonical domain.DecisionEvent */ },
  adaptive_outputs: {
    dmo: { /* adaptive.DMOOutput */ },
    fmo: { /* adaptive.FMOOutput */ },
    return_shape: { /* adaptive.ReturnShape */ },
    capturability: { /* adaptive.CapturabilityResult */ }
  },
  price_event: { /* complete existing domain.PriceEvent */ }
}
```

Decision and Price children are independently optional because P-04 and adaptive events arrive separately. Updates converge by unique `payload_id`. Full adaptive outputs are retained from the already-computed prepared engine and are never recomputed for persistence.

## Capture And Ordering

- Payload capture occurs immediately after `WindowStore.Add` accepts a Bar and before subscriber/model fanout.
- P-04 capture occurs at `Host.emitPrice` after coordinated commit.
- Adaptive capture occurs at `Host.emitWithOutputs` after coordinated commit.
- Existing host order remains P-04 first, adaptive second.
- Uncorrelated synthetic events with no `market_snapshot_id` are excluded and loss-accounted.
- Capture is a bounded, nonblocking enqueue. Mongo writes, payload identity resolution, and retries occur on a persistence-owned worker.

## Identity And Indexes

- Aperture and Payload identities are Mongo-generated ObjectIds.
- `(aperture_id, bar.market_snapshot_id)` uniquely resolves Payload identity without redefining the snapshot.
- Decisions are unique by `payload_id`.
- `accepted_sequence`, event IDs, timestamps, and hashes retain their current component-specific meanings.

Required indexes:

```javascript
quantram_apertures: unique { sequence_num: 1 }
quantram_payloads:  unique { aperture_id: 1, "bar.market_snapshot_id": 1 }
quantram_payloads:         { aperture_id: 1, "bar.symbol": 1, "bar.interval_start_unix_ms": 1 }
quantram_decisions: unique { payload_id: 1 }
quantram_decisions:        { aperture_id: 1, payload_id: 1 }
```

## Lifecycle

Runtime persistence remains opt-in through `QUANTRAM_MONGODB_URI`; `QUANTRAM_MONGODB_DATABASE` defaults to `quantram_db`. No external Aperture identity is accepted or required.

After Mongo connects, pings successfully, and establishes all required indexes, `OpenMongo` creates exactly one new Aperture. The Mongo driver generates its ObjectId, and the server retains that identity in `MongoWriter` for Payload, Decisions, and Snapshot lineage. The opening event uses one UTC timestamp for both `open` and `created_at`; `shut` is null and status is `OPEN`.

Each startup allocates the next unique `sequence_num` and creates a new Aperture. It never queries for or reattaches to an existing OPEN Aperture. A bounded retry resolves concurrent sequence allocation races through the unique sequence index.

On orderly shutdown, `AsyncStore.Close` rejects new capture work, drains its bounded queue under the existing timeout, and invokes `MongoWriter.Close`. The writer sets `shut` to one UTC shutdown timestamp and status to `SHUT` on the same Aperture before disconnecting MongoDB. It does not modify `_id`, `sequence_num`, or `open`.

Abnormal termination does not run that explicit close path. Its Aperture intentionally remains `OPEN` with `shut: null`; the next process creates a different Aperture and does not repair, delete, or reuse the old record.

The server validates configuration and binds its gRPC listener before persistence composition, preventing basic configuration or port-binding failures from creating a run record. The Aperture is then created at the end of successful Mongo persistence initialization, after connection, ping, database selection, and index setup, but before the async capture worker and Snapshot Service are exposed to runtime event producers. Aperture creation failure fails persistence composition and therefore server startup. When the Mongo URI is absent, no Mongo client, Aperture, worker, or Snapshot Service is created.

## Failure Policy

Mongo latency or failure cannot block or roll back science. Queue overflow and uncorrelated captures increment dropped counters. Exhausted writes increment failure counters, retain the last error, and emit an operational log. `AsyncStore.Health` exposes queue depth, written, dropped, failures, and last error.

## Validation Boundary

The implementation is validated with compilation, unit/component tests, synthetic Bars, BSON contract tests, mocked persistence, and deterministic call-order tests. The QuanTRAM server, ingestion script, stream client, Alpaca/IEX, WebSockets, live market, and live MongoDB Aperture were not started or contacted.
