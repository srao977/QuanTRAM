package snapshot

import "context"

// Source exposes only the durable ledger facts needed for checkpoint selection.
// ListPayloads is scoped to one Aperture; DecisionComplete identifies a
// terminal decision record without requiring optional scientific children.
type Source interface {
	ListPayloads(context.Context, string) ([]Payload, error)
	DecisionComplete(context.Context, string, string) (bool, error)
}

// Store persists Snapshot application state without exposing provider types.
// Snapshot creation must be idempotent for the same Aperture, policy, symbol,
// and trigger Payload.
type Store interface {
	ActivePolicies(context.Context) ([]Policy, error)
	GetPolicy(context.Context, string) (Policy, error)
	ListPolicies(context.Context, Page) (PolicyPage, error)
	CreatePolicy(context.Context, Policy) (Policy, error)
	UpdatePolicy(context.Context, Policy) (Policy, error)

	GetSnapshot(context.Context, string) (Snapshot, error)
	ListSnapshots(context.Context, SnapshotFilter) (SnapshotPage, error)
	SnapshotExists(context.Context, Snapshot) (bool, error)
	CreateSnapshot(context.Context, Snapshot) (Snapshot, error)

	StartRun(context.Context, Run) (Run, error)
	FinishRun(context.Context, string, RunStatus, string, *RunErrorInfo) error
	ListRuns(context.Context, RunFilter) (RunPage, error)
}
