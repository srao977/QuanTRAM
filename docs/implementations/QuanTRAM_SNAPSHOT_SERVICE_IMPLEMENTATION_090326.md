# QuanTRAM Snapshot Service V1 Implementation

**Date:** September 3, 2026  
**Branch:** `mongodb-persistene-v1`  
**Baseline HEAD:** `1e2becd38393153a38837f53c7297ba5e211986a`  
**Validation mode:** Offline only

## Refactor

The custom persistence-owned Snapshot subsystem was replaced by a first-class service.

Removed custom files:

- `internal/persistence/snapshot_policy.go`
- `internal/persistence/snapshot.go`
- `internal/persistence/snapshot_manager.go`
- `internal/persistence/snapshot_manager_test.go`

Reused as Mongo adapter infrastructure:

- `internal/persistence/mongo.go`: existing connection, detailed ledger collections, and ledger capture remain; Snapshot semantic methods were removed.
- `internal/persistence/model.go`: detailed Aperture/Payload/Decision BSON models remain; physical Snapshot collection constants remain provider-only.
- `cmd/quantram-server/main.go`: the custom manager hook was replaced with first-class service composition, gRPC registration, and background lifecycle.

No Snapshot-specific scientific callback existed, so none was removed. Existing detailed ledger hooks after accepted Payload and committed Decision/Price output remain unchanged.

## Public Contract

`api/proto/quantram/v1/quantram.proto` now additively defines:

- Enums: `SnapshotPolicyStatus`, `SnapshotTriggerType`, `SnapshotRunStatus`.
- Domain messages: `SnapshotTrigger`, `SnapshotPolicy`, `Snapshot`, `SnapshotRunError`, `SnapshotRun`.
- Typed request/response messages for policy get/list/create/update, Snapshot get/list, and run list.
- `SnapshotService` with seven RPCs.

There is no public CreateSnapshot RPC and no Mongo concept in the public contract. Existing proto fields and field numbers are unchanged.

Buf generated these files; they were not manually edited:

- `gen/quantram/v1/quantram.pb.go`
- `gen/quantram/v1/quantram_grpc.pb.go`

## Database-Agnostic Core

`internal/snapshot/model.go` defines opaque-string application models. `internal/snapshot/ports.go` defines:

```go
type Source interface {
    ListPayloads(context.Context, string) ([]Payload, error)
    DecisionComplete(context.Context, string, string) (bool, error)
}
```

`Store` defines policy CRUD, Snapshot reads/idempotent creation, and SnapshotRun start/finalize/list operations without BSON, ObjectId, Mongo cursors, collection names, or provider errors.

`internal/snapshot/service.go` owns:

- ACTIVE/INACTIVE and EVERY_N_BARS validation.
- Per-Aperture/per-symbol durable Payload counting.
- deterministic candidate ordering.
- terminal `decision_event` readiness gating through Source.
- reference-only Snapshot construction.
- three-attempt bounded persistence retry.
- STARTED to SUCCESS/ERROR run transitions.
- serialized evaluation and idempotency orchestration.
- immediate plus periodic background evaluation.
- provider-neutral policy CRUD and Snapshot/run reads.

## Mongo V1 Adapter

`internal/persistence/snapshot_adapter.go` implements both `snapshot.Source` and `snapshot.Store` on `MongoWriter`.

It owns ObjectId conversion, BSON document types, policy/Snapshot/run mapping, collection queries, ObjectId-backed pagination tokens, no-document error conversion, index definitions, and idempotent `$setOnInsert` checkpoint upsert.

The unique checkpoint identity is:

```text
(aperture_id, policy_id, symbol, payload_id)
```

A unique partial run index on `(aperture_id, policy_id, symbol, trigger_payload_id)` where status is SUCCESS guarantees at most one successful outcome across concurrent service processes. A losing finalization becomes `ERROR/CHECKPOINT_ALREADY_SUCCEEDED`.

The adapter queries the existing Decisions record for `decision_event` existence. It does not invoke scientific code.

## gRPC Adapter

`internal/server/snapshot.go` implements the generated server interface. It maps proto to core models and maps core invalid/not-found failures to `InvalidArgument`/`NotFound`; unavailable configuration returns `Unavailable`. Snapshot IDs remain opaque at this boundary.

`internal/server/server.go` embeds `UnimplementedSnapshotServiceServer` and accepts the core service. `cmd/quantram-server/main.go` registers SnapshotService and starts the core background loop only when Mongo persistence is configured.

## Tests

Database-agnostic core tests in `internal/snapshot/service_test.go` cover:

- policy CRUD and validation;
- inactive policy;
- N=10 candidates at 10, 20, and 30, proving 1-9 produce none;
- AAPL/SPY and Aperture isolation;
- irregular timestamps;
- service recreation and durable resume;
- incomplete versus terminal-ready candidate;
- idempotent repeat evaluation;
- STARTED to SUCCESS and STARTED to ERROR;
- exactly three persistence attempts;
- failure at 30 while 40 succeeds;
- later recovery of 30;
- no science invocation surface.

Offline adapter tests in `internal/persistence/snapshot_adapter_test.go` cover ObjectId/opaque-string conversion, invalid and not-found conversion, policy BSON mapping, Snapshot mapping, SnapshotRun mapping, collection index counts, exact checkpoint index fields, and uniqueness.

## Schema and Documentation

`docs/design/QuanTRAM_MONGODB_COMPASS_SETUP_090326.js` retains six collections and now validates `trigger.every_n_bars`. Its N=10 insertion remains commented and was not executed.

The obsolete Snapshot subsystem design and implementation records were removed and replaced by the first-class service records.

## Protected Boundary

This refactor made no changes to:

- `internal/adaptive/**`
- `internal/pricing/**`
- `internal/ingestion/**`
- `internal/modelhost/**`
- continuity or scientific engine behavior
- existing scientific tests
- existing approximately-100-Bar behavior
- existing Bar, DecisionEvent, or PriceEvent proto fields
- dashboard code or dashboard proto copies

Pre-existing detailed-ledger modifications in protected files remain in the uncommitted worktree but were not changed for this refactor.

## Live Operations

The Go server was not started. MongoDB, Compass, Alpaca, IEX, WebSockets, and market clients were not contacted. No live Aperture, policy, Snapshot, run, Payload, or Decision was created.
