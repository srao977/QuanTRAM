# QuanTRAM MongoDB Aperture Persistence V1 Implementation

**Date:** September 3, 2026  
**Branch:** `mongodb-persistene-v1`  
**Validation:** OFFLINE / DETERMINISTIC ONLY

## Implemented

- Added official `go.mongodb.org/mongo-driver/v2` dependency.
- Added `internal/persistence` with canonical documents, Mongo write operations, indexes, a server-owned Aperture lifecycle, and a bounded asynchronous worker.
- Retained the exact DMO, FMO, ReturnShape, and CapturabilityResult produced by a successful adaptive evaluation. Prepared outputs move through the existing commit operation and are returned as defensive copies.
- Added explicit BSON names to existing Bar, adaptive, DecisionEvent, and complete domain PriceEvent types. No generic scientific representation was introduced.
- Captured each ingestion-accepted Bar immediately after `WindowStore.Add` succeeds and before lossy fanout.
- Captured P-04 and adaptive events after coordinated host commit. Existing P-04-before-adaptive order remains unchanged.
- Added opt-in runtime composition and bounded shutdown flushing. Orderly runtime shutdown SHUTs the process Aperture after the queue drains and before Mongo disconnects.
- Added queue depth, written, dropped, failure, and last-error health state plus operational logging after exhausted retries.
- Replaced the stale four-collection/UUID/flattened Compass design with the approved three-collection ObjectId/nested design.

## Environment

| Variable | Default | Rule |
|---|---:|---|
| `QUANTRAM_MONGODB_URI` | disabled | Enables Mongo persistence when configured |
| `QUANTRAM_MONGODB_DATABASE` | `quantram_db` | Database name |
| `QUANTRAM_MONGODB_QUEUE` | `1024` | Positive bounded queue capacity |

When disabled, no Mongo client, Aperture, worker, capture copy, Snapshot Service, or network operation is created. The server validates configuration and binds its gRPC listener before persistence composition. When enabled, `OpenMongo` then connects and pings MongoDB, selects the configured database, and ensures required indexes before creating one new process Aperture. Creation uses one UTC timestamp for `open` and `created_at`, leaves `shut` null, sets `status` to `OPEN`, and stores current semantic/model/schema version metadata.

The Mongo driver generates the Aperture ObjectId. `MongoWriter` retains it as application/persistence context and applies it to Payload and Decisions writes; the same opaque ID initializes the database-agnostic Snapshot Service. No ObjectId enters scientific state or calculation.

Sequence allocation reads the highest existing sequence and inserts the next value. The unique sequence index is authoritative, and a bounded retry handles concurrent insert races. Every persistence initialization inserts a new Aperture; it does not select, reattach, repair, or delete an existing OPEN record.

## Persistence Flow

```text
WindowStore.Add accepted
  -> nonblocking Payload capture
  -> existing observation fanout
  -> existing model fanout

modelhost coordinated commit
  -> existing P-04 emit + nonblocking PriceEvent capture
  -> existing adaptive emit + nonblocking DecisionEvent/full-output capture

single persistence worker
  -> idempotent Payload upsert by (aperture_id, market_snapshot_id)
  -> resolve Mongo payload_id
  -> idempotent Decisions upsert by payload_id

orderly close
  -> reject new captures
  -> drain bounded queue under existing shutdown timeout
  -> set current Aperture shut timestamp and status SHUT
  -> disconnect MongoDB
```

If the process terminates without orderly close, no automatic repair occurs. The record remains `OPEN` with `shut: null`, and the next server run creates a different Aperture.

## Deterministic Validation

Validated without starting the QuanTRAM server:

- Adaptive retained output equals the exact prepared output after commit.
- Retained maps/slices are defensively copied.
- Failed evaluations do not replace retained output.
- Accepted duplicate Bars produce one Payload capture.
- Host capture order remains PriceEvent then adaptive DecisionEvent.
- Host adaptive capture contains committed DMO/FMO/D02/D04 output.
- BSON preserves nested `bar.open`, `bar.market_snapshot_id`, and `adaptive_outputs.dmo`; Payload does not flatten Bar and Decisions has no `decision_values`.
- Bounded queue handoff is nonblocking, reports overflow, preserves FIFO order, and flushes on Close.
- Persistence initialization creates one OPEN Aperture with generated identity, sequential number, null `shut`, one opening timestamp, and approved version metadata.
- Recreating persistence creates a distinct Aperture without reattaching to or SHUTting an old OPEN record.
- The generated identity is retained as Payload, Decisions, and Snapshot application lineage context.
- Explicit orderly writer close SHUTs the same Aperture without changing its identity, sequence, or opening time, then disconnects.
- URI-absent composition creates no persistence or Snapshot service.
- Touched packages compile and pass their component tests.

An existing adaptive reproducibility risk was observed during repeated modelhost testing: `computeCoherence` accumulates floating-point values by ranging over a Go map, so equivalent runs can produce one of two final state hashes depending on map iteration order. Pricing hashes remain stable. This pre-existing D01 scientific behavior was not changed under this authorization.

## Operations Not Run

By explicit human policy, the following were not run and remain deferred:

- QuanTRAM Go server
- `Start-QuantramIngestion.ps1`
- decision-stream client
- Alpaca, IEX, or any WebSocket/live market feed
- live-market validation
- live MongoDB connection, Aperture creation/shut, Compass script, or database writes

The Compass script is an operator artifact only and was not executed.
