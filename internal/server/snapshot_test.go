package server

import (
	"errors"
	"testing"
	"time"

	quantramv1 "quantram/gen/quantram/v1"
	"quantram/internal/snapshot"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSnapshotProtoMappingsPreserveOpaqueContract(t *testing.T) {
	now := time.Date(2026, 9, 3, 15, 0, 0, 0, time.UTC)
	policy := toProtoSnapshotPolicy(snapshot.Policy{
		ID: "opaque-policy", Name: "Every ten", Status: snapshot.PolicyActive,
		Trigger:   snapshot.Trigger{Type: snapshot.TriggerEveryNBars, EveryNBars: 10},
		CreatedAt: now, UpdatedAt: now.Add(time.Minute),
	})
	if policy.GetId() != "opaque-policy" || policy.GetStatus() != quantramv1.SnapshotPolicyStatus_SNAPSHOT_POLICY_STATUS_ACTIVE ||
		policy.GetTrigger().GetType() != quantramv1.SnapshotTriggerType_SNAPSHOT_TRIGGER_TYPE_EVERY_N_BARS || policy.GetTrigger().GetEveryNBars() != 10 {
		t.Fatalf("policy proto=%+v", policy)
	}
	item := toProtoSnapshot(snapshot.Snapshot{
		ID: "opaque-snapshot", ApertureID: "opaque-aperture", PolicyID: "opaque-policy",
		PayloadID: "opaque-payload", Symbol: "AAPL", SnapshotNum: 3, CapturedAt: now,
	})
	if item.GetPayloadId() != "opaque-payload" || item.GetSnapshotNum() != 3 || item.GetCapturedAtUnixMs() != now.UnixMilli() {
		t.Fatalf("Snapshot proto=%+v", item)
	}
	completed := now.Add(time.Second)
	run := toProtoSnapshotRun(snapshot.Run{
		ID: "opaque-run", ApertureID: "opaque-aperture", PolicyID: "opaque-policy", Symbol: "AAPL",
		TriggerPayloadID: "opaque-payload", TriggerCount: 30, StartedAt: now, CompletedAt: &completed,
		Status: snapshot.RunError, Error: &snapshot.RunErrorInfo{Code: "WRITE", Message: "failed"},
	})
	if run.GetStatus() != quantramv1.SnapshotRunStatus_SNAPSHOT_RUN_STATUS_ERROR || run.GetError().GetCode() != "WRITE" || run.GetCompletedAtUnixMs() != completed.UnixMilli() {
		t.Fatalf("SnapshotRun proto=%+v", run)
	}
}

func TestSnapshotServiceErrorsUseStandardGRPCCodes(t *testing.T) {
	if code := status.Code(snapshotStatusError(snapshot.ErrInvalid)); code != codes.InvalidArgument {
		t.Fatalf("invalid code=%s", code)
	}
	if code := status.Code(snapshotStatusError(snapshot.ErrNotFound)); code != codes.NotFound {
		t.Fatalf("not-found code=%s", code)
	}
	if code := status.Code(snapshotStatusError(errors.New("provider failed"))); code != codes.Internal {
		t.Fatalf("provider code=%s", code)
	}
	if _, err := (&Server{}).requireSnapshotService(); status.Code(err) != codes.Unavailable {
		t.Fatalf("unconfigured code=%s", status.Code(err))
	}
}
