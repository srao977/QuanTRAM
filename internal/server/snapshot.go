package server

import (
	"context"
	"errors"
	"strings"

	quantramv1 "quantram/gen/quantram/v1"
	"quantram/internal/snapshot"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) SetSnapshotService(service *snapshot.Service) {
	s.snapshots = service
}

func (s *Server) GetSnapshotPolicy(ctx context.Context, request *quantramv1.GetSnapshotPolicyRequest) (*quantramv1.GetSnapshotPolicyResponse, error) {
	service, err := s.requireSnapshotService()
	if err != nil {
		return nil, err
	}
	policy, err := service.GetPolicy(ctx, request.GetId())
	if err != nil {
		return nil, snapshotStatusError(err)
	}
	return &quantramv1.GetSnapshotPolicyResponse{Policy: toProtoSnapshotPolicy(policy)}, nil
}

func (s *Server) ListSnapshotPolicies(ctx context.Context, request *quantramv1.ListSnapshotPoliciesRequest) (*quantramv1.ListSnapshotPoliciesResponse, error) {
	service, err := s.requireSnapshotService()
	if err != nil {
		return nil, err
	}
	page, err := service.ListPolicies(ctx, snapshot.Page{Size: request.GetPageSize(), Token: request.GetPageToken()})
	if err != nil {
		return nil, snapshotStatusError(err)
	}
	policies := make([]*quantramv1.SnapshotPolicy, 0, len(page.Items))
	for _, policy := range page.Items {
		policies = append(policies, toProtoSnapshotPolicy(policy))
	}
	return &quantramv1.ListSnapshotPoliciesResponse{Policies: policies, NextPageToken: page.NextToken}, nil
}

func (s *Server) CreateSnapshotPolicy(ctx context.Context, request *quantramv1.CreateSnapshotPolicyRequest) (*quantramv1.CreateSnapshotPolicyResponse, error) {
	service, err := s.requireSnapshotService()
	if err != nil {
		return nil, err
	}
	policy, err := service.CreatePolicy(ctx, snapshot.Policy{
		Name: request.GetName(), Status: snapshotPolicyStatusFromProto(request.GetStatus()),
		Trigger: snapshotTriggerFromProto(request.GetTrigger()),
	})
	if err != nil {
		return nil, snapshotStatusError(err)
	}
	return &quantramv1.CreateSnapshotPolicyResponse{Policy: toProtoSnapshotPolicy(policy)}, nil
}

func (s *Server) UpdateSnapshotPolicy(ctx context.Context, request *quantramv1.UpdateSnapshotPolicyRequest) (*quantramv1.UpdateSnapshotPolicyResponse, error) {
	service, err := s.requireSnapshotService()
	if err != nil {
		return nil, err
	}
	policy, err := service.UpdatePolicy(ctx, snapshot.Policy{
		ID: request.GetId(), Name: request.GetName(), Status: snapshotPolicyStatusFromProto(request.GetStatus()),
		Trigger: snapshotTriggerFromProto(request.GetTrigger()),
	})
	if err != nil {
		return nil, snapshotStatusError(err)
	}
	return &quantramv1.UpdateSnapshotPolicyResponse{Policy: toProtoSnapshotPolicy(policy)}, nil
}

func (s *Server) GetSnapshot(ctx context.Context, request *quantramv1.GetSnapshotRequest) (*quantramv1.GetSnapshotResponse, error) {
	service, err := s.requireSnapshotService()
	if err != nil {
		return nil, err
	}
	item, err := service.GetSnapshot(ctx, request.GetId())
	if err != nil {
		return nil, snapshotStatusError(err)
	}
	return &quantramv1.GetSnapshotResponse{Snapshot: toProtoSnapshot(item)}, nil
}

func (s *Server) ListSnapshots(ctx context.Context, request *quantramv1.ListSnapshotsRequest) (*quantramv1.ListSnapshotsResponse, error) {
	service, err := s.requireSnapshotService()
	if err != nil {
		return nil, err
	}
	page, err := service.ListSnapshots(ctx, snapshot.SnapshotFilter{
		ApertureID: request.GetApertureId(), PolicyID: request.GetPolicyId(),
		Symbol: strings.ToUpper(strings.TrimSpace(request.GetSymbol())),
		Page:   snapshot.Page{Size: request.GetPageSize(), Token: request.GetPageToken()},
	})
	if err != nil {
		return nil, snapshotStatusError(err)
	}
	items := make([]*quantramv1.Snapshot, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, toProtoSnapshot(item))
	}
	return &quantramv1.ListSnapshotsResponse{Snapshots: items, NextPageToken: page.NextToken}, nil
}

func (s *Server) ListSnapshotRuns(ctx context.Context, request *quantramv1.ListSnapshotRunsRequest) (*quantramv1.ListSnapshotRunsResponse, error) {
	service, err := s.requireSnapshotService()
	if err != nil {
		return nil, err
	}
	runStatus, err := snapshotRunStatusFromProto(request.GetStatus())
	if err != nil {
		return nil, err
	}
	page, err := service.ListRuns(ctx, snapshot.RunFilter{
		ApertureID: request.GetApertureId(), PolicyID: request.GetPolicyId(),
		Symbol: strings.ToUpper(strings.TrimSpace(request.GetSymbol())), Status: runStatus,
		Page: snapshot.Page{Size: request.GetPageSize(), Token: request.GetPageToken()},
	})
	if err != nil {
		return nil, snapshotStatusError(err)
	}
	runs := make([]*quantramv1.SnapshotRun, 0, len(page.Items))
	for _, run := range page.Items {
		runs = append(runs, toProtoSnapshotRun(run))
	}
	return &quantramv1.ListSnapshotRunsResponse{Runs: runs, NextPageToken: page.NextToken}, nil
}

func (s *Server) requireSnapshotService() (*snapshot.Service, error) {
	if s.snapshots == nil {
		return nil, status.Error(codes.Unavailable, "SnapshotService unavailable")
	}
	return s.snapshots, nil
}

func snapshotStatusError(err error) error {
	switch {
	case errors.Is(err, snapshot.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, snapshot.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func snapshotPolicyStatusFromProto(value quantramv1.SnapshotPolicyStatus) snapshot.PolicyStatus {
	switch value {
	case quantramv1.SnapshotPolicyStatus_SNAPSHOT_POLICY_STATUS_ACTIVE:
		return snapshot.PolicyActive
	case quantramv1.SnapshotPolicyStatus_SNAPSHOT_POLICY_STATUS_INACTIVE:
		return snapshot.PolicyInactive
	default:
		return ""
	}
}

func snapshotTriggerFromProto(value *quantramv1.SnapshotTrigger) snapshot.Trigger {
	if value == nil {
		return snapshot.Trigger{}
	}
	triggerType := snapshot.TriggerType("")
	if value.GetType() == quantramv1.SnapshotTriggerType_SNAPSHOT_TRIGGER_TYPE_EVERY_N_BARS {
		triggerType = snapshot.TriggerEveryNBars
	}
	return snapshot.Trigger{Type: triggerType, EveryNBars: value.GetEveryNBars()}
}

func snapshotRunStatusFromProto(value quantramv1.SnapshotRunStatus) (snapshot.RunStatus, error) {
	switch value {
	case quantramv1.SnapshotRunStatus_SNAPSHOT_RUN_STATUS_UNSPECIFIED:
		return "", nil
	case quantramv1.SnapshotRunStatus_SNAPSHOT_RUN_STATUS_STARTED:
		return snapshot.RunStarted, nil
	case quantramv1.SnapshotRunStatus_SNAPSHOT_RUN_STATUS_SUCCESS:
		return snapshot.RunSuccess, nil
	case quantramv1.SnapshotRunStatus_SNAPSHOT_RUN_STATUS_ERROR:
		return snapshot.RunError, nil
	default:
		return "", status.Error(codes.InvalidArgument, "unsupported SnapshotRun status")
	}
}

func toProtoSnapshotPolicy(policy snapshot.Policy) *quantramv1.SnapshotPolicy {
	statusValue := quantramv1.SnapshotPolicyStatus_SNAPSHOT_POLICY_STATUS_UNSPECIFIED
	if policy.Status == snapshot.PolicyActive {
		statusValue = quantramv1.SnapshotPolicyStatus_SNAPSHOT_POLICY_STATUS_ACTIVE
	} else if policy.Status == snapshot.PolicyInactive {
		statusValue = quantramv1.SnapshotPolicyStatus_SNAPSHOT_POLICY_STATUS_INACTIVE
	}
	triggerType := quantramv1.SnapshotTriggerType_SNAPSHOT_TRIGGER_TYPE_UNSPECIFIED
	if policy.Trigger.Type == snapshot.TriggerEveryNBars {
		triggerType = quantramv1.SnapshotTriggerType_SNAPSHOT_TRIGGER_TYPE_EVERY_N_BARS
	}
	return &quantramv1.SnapshotPolicy{
		Id: policy.ID, Name: policy.Name, Status: statusValue,
		Trigger:         &quantramv1.SnapshotTrigger{Type: triggerType, EveryNBars: policy.Trigger.EveryNBars},
		CreatedAtUnixMs: policy.CreatedAt.UnixMilli(), UpdatedAtUnixMs: policy.UpdatedAt.UnixMilli(),
	}
}

func toProtoSnapshot(item snapshot.Snapshot) *quantramv1.Snapshot {
	return &quantramv1.Snapshot{
		Id: item.ID, ApertureId: item.ApertureID, PolicyId: item.PolicyID, PayloadId: item.PayloadID,
		Symbol: item.Symbol, SnapshotNum: item.SnapshotNum, CapturedAtUnixMs: item.CapturedAt.UnixMilli(),
	}
}

func toProtoSnapshotRun(run snapshot.Run) *quantramv1.SnapshotRun {
	statusValue := quantramv1.SnapshotRunStatus_SNAPSHOT_RUN_STATUS_UNSPECIFIED
	switch run.Status {
	case snapshot.RunStarted:
		statusValue = quantramv1.SnapshotRunStatus_SNAPSHOT_RUN_STATUS_STARTED
	case snapshot.RunSuccess:
		statusValue = quantramv1.SnapshotRunStatus_SNAPSHOT_RUN_STATUS_SUCCESS
	case snapshot.RunError:
		statusValue = quantramv1.SnapshotRunStatus_SNAPSHOT_RUN_STATUS_ERROR
	}
	result := &quantramv1.SnapshotRun{
		Id: run.ID, ApertureId: run.ApertureID, PolicyId: run.PolicyID, Symbol: run.Symbol,
		TriggerPayloadId: run.TriggerPayloadID, TriggerCount: run.TriggerCount,
		StartedAtUnixMs: run.StartedAt.UnixMilli(), Status: statusValue, SnapshotId: run.SnapshotID,
	}
	if run.CompletedAt != nil {
		result.CompletedAtUnixMs = run.CompletedAt.UnixMilli()
	}
	if run.Error != nil {
		result.Error = &quantramv1.SnapshotRunError{Code: run.Error.Code, Message: run.Error.Message}
	}
	return result
}
