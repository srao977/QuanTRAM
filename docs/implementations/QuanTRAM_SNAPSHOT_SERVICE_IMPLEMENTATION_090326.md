# QuanTRAM Snapshot Service V1 Implementation

## 1. Implementation Record

| Item | Value |
|---|---|
| Date | September 4, 2026 |
| Branch | `mongodb-persistene-v1` |
| Implementation commit | `cb3dfafa89d0af7bdb7189301b3a73a26a007b36` |
| Baseline parent | `1e2becd38393153a38837f53c7297ba5e211986a` |
| Validation mode | Offline deterministic |
| Live Mongo status | Not run by the agent |

The former persistence-owned Snapshot subsystem was replaced by a first-class, database-agnostic service. Current code is implementation authority.

## 2. Refactor Inventory

Removed persistence-owned semantic files:

- `internal/persistence/snapshot_policy.go`
- `internal/persistence/snapshot.go`
- `internal/persistence/snapshot_manager.go`
- `internal/persistence/snapshot_manager_test.go`

Added/changed ownership:

| Path | Implemented responsibility |
|---|---|
| `internal/snapshot/model.go` | Provider-neutral models, enums, filters, pages |
| `internal/snapshot/ports.go` | Source and Store interfaces |
| `internal/snapshot/service.go` | Policy semantics, scans, candidates, runs, retries, CRUD/reads |
| `internal/snapshot/service_test.go` | In-memory behavioral verification |
| `internal/persistence/snapshot_adapter.go` | Mongo documents, mappings, queries, indexes, pagination |
| `internal/persistence/snapshot_adapter_test.go` | Offline adapter/mapping/index verification |
| `internal/server/snapshot.go` | Generated gRPC interface implementation and mappings |
| `internal/server/snapshot_test.go` | Proto mapping and gRPC status verification |
| `api/proto/quantram/v1/quantram.proto` | Canonical public Snapshot contract |
| generated Go files | Buf output; not manually edited |
| `cmd/quantram-server/main.go` | Composition, registration, background lifecycle |

Detailed ledger capture remains in ingestion/modelhost/persistence and is not duplicated by Snapshot.

## 3. Generated Public Contract

Buf generated `gen/quantram/v1/quantram.pb.go` and `quantram_grpc.pb.go` from the additive proto. The contract contains three enums, five domain messages, fourteen request/response messages, and seven RPCs:

```text
GetSnapshotPolicy
ListSnapshotPolicies
CreateSnapshotPolicy
UpdateSnapshotPolicy
GetSnapshot
ListSnapshots
ListSnapshotRuns
```

There is no public `CreateSnapshot` RPC and no Mongo/ObjectId/BSON concept in the proto.

## 4. Core Interfaces

`Source` is exactly:

```go
type Source interface {
    ListPayloads(context.Context, string) ([]Payload, error)
    DecisionComplete(context.Context, string, string) (bool, error)
}
```

`Store` is exactly the policy operations `ActivePolicies`, `GetPolicy`, `ListPolicies`, `CreatePolicy`, `UpdatePolicy`; Snapshot operations `GetSnapshot`, `ListSnapshots`, `SnapshotExists`, `CreateSnapshot`; and run operations `StartRun`, `FinishRun`, `ListRuns`.

Compile-time assertions establish that `*MongoWriter` implements both interfaces.

## 5. Core Evaluation Algorithm

`Run` calls `evaluateAndLog` immediately, creates a ticker, and repeats until cancellation. Server interval is one second. `Evaluate` locks `evaluateMu` for the entire scan.

```text
load ACTIVE policies
load Payload references for fixed Service Aperture
sort Payloads by symbol, interval start, ID
for each active policy:
  validate policy; log and skip invalid stored policy
  create empty per-symbol count map
  for each sorted Payload:
    verify same Aperture
    increment symbol count
    if count % every_n_bars != 0: continue
    candidate.snapshot_num = count / every_n_bars
    if SnapshotExists: continue
    if DecisionComplete is false: continue
    runCandidate(candidate, count)
```

Top-level policy/source/existence/readiness errors return from `Evaluate` and are logged by the loop. One invalid stored policy is skipped without aborting other policies.

## 6. Candidate Run Algorithm

`runCandidate` calls `StartRun` with STARTED, trigger count, trigger Payload, and current UTC time. Start failure logs and returns. It then calls `CreateSnapshot` up to three times immediately. It does not sleep or classify retryable errors.

On exhausted error, it calls `FinishRun(ERROR, no Snapshot ID, SNAPSHOT_PERSISTENCE_ERROR)`. On success, it calls `FinishRun(SUCCESS, created Snapshot ID, nil)`. Finalization errors are logged and not propagated.

## 7. Policy CRUD and Pagination

Create trims name, validates ACTIVE/INACTIVE plus EVERY_N_BARS and positive N, clears any supplied ID, and sets both service-owned times. Update trims/validates ID/name, reads the existing record, preserves `CreatedAt`, sets `UpdatedAt`, and performs provider replacement.

Page size normalization uses 100 for zero and caps values above 1000. Core does not interpret tokens.

## 8. Mongo Provider Operations

| Port method | Mongo implementation |
|---|---|
| `ListPayloads` | Find by `aperture_id`; sort `bar.symbol`, `bar.interval_start_unix_ms`, `_id`; project into minimal core Payloads |
| `DecisionComplete` | Find Decisions by Aperture/Payload and `decision_event: {$exists: true}` |
| `ActivePolicies` | Find `status=ACTIVE`, sort `_id` |
| Policy get/list/create/update | ObjectId lookup, `_id` page filter, insert, full `ReplaceOne` |
| Snapshot get/list | ObjectId lookup or typed optional filters plus `_id` page filter |
| `SnapshotExists` | Check checkpoint composite identity |
| `CreateSnapshot` | `FindOneAndUpdate` with `$setOnInsert`, upsert, return-after |
| `StartRun` | Insert complete STARTED document with explicit null terminal values |
| `FinishRun` | Update only a matching STARTED record to terminal status |
| `ListRuns` | Typed optional filters plus `_id` page filter |

List methods sort `_id` ascending, request `pageSize + 1`, return at most pageSize items, and use the last returned ID as next token. Invalid ObjectId strings map to core invalid errors. `mongo.ErrNoDocuments` maps to core not-found.

## 9. Mongo Documents

### 9.1 Policy

`_id`, `name`, `status`, nested trigger `{type, every_n_bars}`, `created_at`, `updated_at`.

### 9.2 Snapshot

`_id`, ObjectId `aperture_id`, `policy_id`, `payload_id`, `symbol`, integer `snapshot_num`, `captured_at`.

### 9.3 SnapshotRun

`_id`, Aperture/policy IDs, symbol, trigger Payload ID/count, start/completion times, status, Snapshot ID, and nested error. `CompletedAt`, `SnapshotID`, and `Error` use pointer fields without `omitempty`; STARTED inserts therefore contain explicit BSON null values. This matches the Compass required-plus-nullable schema.

## 10. Index Enforcement

`snapshotIndexModels` returns:

1. Policies: `(status, trigger.type)`.
2. Snapshots unique checkpoint: `(aperture_id, policy_id, symbol, payload_id)`.
3. Snapshots sequence lookup: `(aperture_id, policy_id, symbol, snapshot_num)`.
4. Runs history: `(aperture_id, policy_id, symbol, trigger_count, started_at)`.
5. Runs audit: `(status, started_at)`.
6. Runs unique successful checkpoint: `(aperture_id, policy_id, symbol, trigger_payload_id)`, partial where status is SUCCESS.

These are installed by `OpenMongo` before Aperture creation.

## 11. Concurrent Success Resolution

Snapshot upsert is independently idempotent. If a run’s SUCCESS finalization hits the partial unique index because another run already succeeded, adapter code conditionally converts the still-STARTED losing run to ERROR with code `CHECKPOINT_ALREADY_SUCCEEDED`, null Snapshot ID, and a completion time. Failure of this fallback is returned.

## 12. gRPC Adapter Mapping

`internal/server/snapshot.go` implements all seven generated methods. It normalizes list symbols with trim/uppercase, converts all enums, opaque IDs, page fields, and UTC times, and conditionally maps run completion/error.

| Core result | gRPC result |
|---|---|
| `snapshot.ErrInvalid` | `InvalidArgument` |
| `snapshot.ErrNotFound` | `NotFound` |
| nil configured service | `Unavailable` |
| other error | `Internal` |
| unknown run status enum | `InvalidArgument` |

Unspecified run status is an empty core filter. Unspecified policy/trigger values become empty core values and fail policy validation.

## 13. Proto/Core/Mongo Mapping Matrix

| Concept | Proto | Core | Mongo |
|---|---|---|---|
| ID | `string` | opaque `string` | `ObjectID` |
| Policy status | generated enum | `PolicyStatus` string | `ACTIVE`/`INACTIVE` string |
| Trigger | enum + `uint32` N | `TriggerType` + `uint32` | nested string/integer |
| Times | Unix milliseconds `int64` | `time.Time` / pointer | BSON date / null |
| Snapshot ordinal | `uint64` | `uint64` | integer |
| Run error | optional message | pointer struct | pointer nested doc / null |
| Page token | opaque string | opaque string | ObjectId hex continuation |

No layer copies scientific values into Snapshot.

## 14. Runtime Composition

When Mongo URI is empty, `newPersistence` returns nil store and nil Snapshot service. The gRPC service remains registered, but requests return Unavailable. When configured, one `MongoWriter` acts as ledger writer, Source, and Store; one Service receives `writer.ApertureID`; one AsyncStore handles only Bar/Decision/Price capture. Snapshot Service uses the writer directly, not the async queue.

The background loop has a dedicated context and `WaitGroup`. During `shutdownRuntime`, gRPC and runtime producers stop first, `AsyncStore.Drain` makes their accepted facts durable, `Service.FinalEvaluate` runs synchronously, then the Snapshot context is canceled and its `Run` goroutine is joined before Aperture SHUT and Mongo disconnect. `FinalEvaluate` is a named lifecycle wrapper over serialized `Evaluate`; it adds no trigger semantics.

## 15. Test Traceability Matrix

| Named test | Verified requirements |
|---|---|
| `TestPolicyCRUDAndValidation` | create/update semantics, status/trigger/name validation |
| `TestEveryTenPolicyActivationAndExactCandidates` | inactive behavior; N=10 candidates exactly at 10/20/30 |
| `TestCountsAreDurableAndIsolated` | symbol/Aperture isolation, irregular times, restart recount/resume |
| `TestCandidateWaitsForTerminalDecisionAndIsIdempotent` | readiness wait, successful run, repeated evaluation skip |
| `TestFailureAtThirtyDoesNotBlockFortyOrInvokeScience` | three attempts, ERROR, 40 progress, 30 recovery, no science surface |
| `TestFinalEvaluatePreservesOnlyEligibleCompleteCheckpoint` | final bar 40 checkpoint, no bar 37 checkpoint, incomplete bar 40 remains absent |
| `TestShutdownRuntimeCoordinatesProducersSnapshotAndPersistence` | drain before final scan; Snapshot join before persistence close |
| `TestSnapshotAdapterObjectIDMapping` | valid/invalid opaque conversion and provider errors |
| `TestSnapshotAdapterPolicyMapping` | policy BSON round trip |
| `TestSnapshotAdapterSnapshotAndRunMapping` | Snapshot/Run mapping and nullable fields |
| `TestSnapshotAdapterIndexDefinitions` | collection index counts, keys, uniqueness/partial behavior |
| `TestSnapshotProtoMappingsPreserveOpaqueContract` | enums, IDs, times, optional result mapping |
| `TestSnapshotServiceErrorsUseStandardGRPCCodes` | invalid/not-found/unavailable status contract |

Tests use in-memory stores/fakes and mapping inspection. They do not start MongoDB or gRPC as an external process.

## 16. Behavioral Trace Examples

### 16.1 N=10, one symbol

```text
Payload counts 1..9: no candidate
10: wait until DecisionEvent exists -> run -> Snapshot 1
20: Snapshot 2
30: three failed creates -> ERROR
40: Snapshot 4 still succeeds
later scan: 30 can be retried and recovered because no Snapshot exists
```

### 16.2 Two symbols

```text
AAPL: 1..10 -> AAPL Snapshot 1
SPY:  1..10 -> SPY Snapshot 1
```

Counts do not merge across symbols or Apertures.

## 17. Schema and Compass Alignment

| Check | Result |
|---|---|
| Six collections | Aligned |
| `every_n_bars` nested trigger | Aligned |
| ObjectId reference fields | Aligned |
| Snapshot unique checkpoint index | Aligned |
| Run partial unique SUCCESS index | Aligned |
| Required nullable run terminal fields | Aligned; Go emits explicit null |
| Optional commented N=10 insert | Not runtime configuration and not claimed executed |
| Actual Go writes accepted by validators | Not live-tested |

The operator reported manual execution of the Compass setup. This documentation task did not execute it.

## 18. Protected Scientific Boundary

Snapshot core and adapter do not import or invoke adaptive/pricing engines. Existing accepted-Bar and committed-output ledger hooks supply source facts. The refactor did not change D01, FMO, D02, D04, emitter, P-04, continuity, or warm-up semantics. The current in-memory WindowStore limit is 64, not approximately 100.

## 19. Known Gaps and Risks

1. **GAP:** no live Mongo E2E verifies validator compatibility, idempotent upsert, partial-index conflict, or restart recount.
2. `DecisionComplete` accepts field existence even if malformed/null data bypassed normal writers.
3. StartRun failure and FinishRun failure are logged; a failed finalization can leave STARTED indefinitely.
4. Retry is three immediate attempts without delay or retryability classification.
5. There is no stale-run reconciler, policy deletion, retention, or migration.
6. Public list order is provider ObjectId order; explicit Snapshot-number navigation is not implemented.
7. There is no authorization layer for Create/Update policy RPCs.
8. Dashboard CURRENT/HISTORY/REV/FWD and expanded ledger reads are not implemented.

## 20. Validation and Operations Not Run

After coordinated-lifecycle implementation, `go test ./...` passed 115 tests, `go vet ./...` passed, and `git diff --check` passed. Focused race testing was attempted but unavailable because this Windows Go environment has CGO disabled; no environment change was made. The Go server, ingestion script, clients, MongoDB, Compass, Alpaca, IEX, WebSockets, and live market were not started or contacted.

## 21. Change Log

| Date | Change |
|---|---|
| September 3, 2026 | Initial implementation record. |
| September 4, 2026 | Rewritten from code with exact interfaces, algorithms, BSON/null behavior, gRPC mapping, test traceability, schema alignment, and residual gaps. |
| September 4, 2026 | Added `FinalEvaluate` lifecycle integration, dedicated Snapshot cancellation/join, and deterministic final-checkpoint tests. |
