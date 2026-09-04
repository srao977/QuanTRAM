// Package snapshot implements database-neutral checkpoint policy evaluation.
// This file owns periodic and final scans for one Aperture; persistence,
// scientific computation, and process lifecycle remain external concerns.
package snapshot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	// ErrInvalid classifies invalid Snapshot API input.
	ErrInvalid = errors.New("invalid snapshot request")
	// ErrNotFound classifies absent Snapshot resources.
	ErrNotFound = errors.New("snapshot resource not found")
)

const persistenceAttempts = 3

// Service evaluates checkpoint policies for one Aperture and delegates all
// durable reads and writes through Source and Store.
type Service struct {
	source     Source
	store      Store
	apertureID string
	interval   time.Duration
	now        func() time.Time
	evaluateMu sync.Mutex
}

// NewService binds checkpoint evaluation to one Aperture and applies a
// one-second scan interval when interval is not positive.
func NewService(source Source, store Store, apertureID string, interval time.Duration) *Service {
	if interval <= 0 {
		interval = time.Second
	}
	return &Service{
		source: source, store: store, apertureID: apertureID, interval: interval,
		now: func() time.Time { return time.Now().UTC() },
	}
}

// Run evaluates immediately and then periodically until ctx is canceled.
// Scan errors are logged and do not terminate the background lifecycle.
func (s *Service) Run(ctx context.Context) {
	s.evaluateAndLog(ctx)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.evaluateAndLog(ctx)
		}
	}
}

func (s *Service) evaluateAndLog(ctx context.Context) {
	if err := s.Evaluate(ctx); err != nil && ctx.Err() == nil {
		log.Printf("snapshot service scan failed: %v", err)
	}
}

// Evaluate serializes one policy scan. Counts are reconstructed from durable
// Payloads and partitioned by symbol; only the Nth, 2Nth, and later exact
// multiples become candidates, regardless of timestamp regularity.
func (s *Service) Evaluate(ctx context.Context) error {
	s.evaluateMu.Lock()
	defer s.evaluateMu.Unlock()

	policies, err := s.store.ActivePolicies(ctx)
	if err != nil {
		return fmt.Errorf("load active snapshot policies: %w", err)
	}
	payloads, err := s.source.ListPayloads(ctx, s.apertureID)
	if err != nil {
		return fmt.Errorf("load aperture payloads: %w", err)
	}
	sort.Slice(payloads, func(i, j int) bool {
		if payloads[i].Symbol != payloads[j].Symbol {
			return payloads[i].Symbol < payloads[j].Symbol
		}
		if !payloads[i].IntervalStart.Equal(payloads[j].IntervalStart) {
			return payloads[i].IntervalStart.Before(payloads[j].IntervalStart)
		}
		return payloads[i].ID < payloads[j].ID
	})

	for _, policy := range policies {
		if policy.Status != PolicyActive {
			continue
		}
		if err := validatePolicy(policy, true); err != nil {
			log.Printf("snapshot policy %s ignored: %v", policy.ID, err)
			continue
		}
		counts := make(map[string]uint64)
		for _, payload := range payloads {
			if payload.ApertureID != s.apertureID {
				continue
			}
			counts[payload.Symbol]++
			count := counts[payload.Symbol]
			if count%uint64(policy.Trigger.EveryNBars) != 0 {
				continue
			}
			// Checkpoint identity is stable across scans and service restarts;
			// CapturedAt is metadata, not part of idempotency.
			candidate := Snapshot{
				ApertureID: s.apertureID, PolicyID: policy.ID, PayloadID: payload.ID,
				Symbol: payload.Symbol, SnapshotNum: count / uint64(policy.Trigger.EveryNBars), CapturedAt: s.now(),
			}
			exists, err := s.store.SnapshotExists(ctx, candidate)
			if err != nil {
				return fmt.Errorf("check snapshot checkpoint: %w", err)
			}
			if exists {
				continue
			}
			complete, err := s.source.DecisionComplete(ctx, s.apertureID, payload.ID)
			if err != nil {
				return fmt.Errorf("check snapshot completeness: %w", err)
			}
			if !complete {
				continue
			}
			s.runCandidate(ctx, candidate, count)
		}
	}
	return nil
}

// FinalEvaluate performs the same policy-exact, idempotent scan as a periodic
// evaluation. The server calls it after producers stop and queued facts become
// durable, so orderly shutdown cannot miss an eligible final checkpoint.
func (s *Service) FinalEvaluate(ctx context.Context) error {
	return s.Evaluate(ctx)
}

func (s *Service) runCandidate(ctx context.Context, candidate Snapshot, triggerCount uint64) {
	run, err := s.store.StartRun(ctx, Run{
		ApertureID: candidate.ApertureID, PolicyID: candidate.PolicyID, Symbol: candidate.Symbol,
		TriggerPayloadID: candidate.PayloadID, TriggerCount: triggerCount, StartedAt: s.now(), Status: RunStarted,
	})
	if err != nil {
		log.Printf("snapshot run start failed checkpoint=%s: %v", candidate.PayloadID, err)
		return
	}
	// Only the checkpoint write is retried. The STARTED audit record is created
	// once and finalized after success or after all bounded attempts fail.
	var created Snapshot
	for attempt := 0; attempt < persistenceAttempts; attempt++ {
		created, err = s.store.CreateSnapshot(ctx, candidate)
		if err == nil {
			break
		}
	}
	if err != nil {
		info := &RunErrorInfo{Code: "SNAPSHOT_PERSISTENCE_ERROR", Message: err.Error()}
		if finishErr := s.store.FinishRun(ctx, run.ID, RunError, "", info); finishErr != nil {
			log.Printf("snapshot run error finalization failed run=%s: %v", run.ID, finishErr)
		}
		return
	}
	if err := s.store.FinishRun(ctx, run.ID, RunSuccess, created.ID, nil); err != nil {
		log.Printf("snapshot run success finalization failed run=%s: %v", run.ID, err)
	}
}

// GetPolicy returns one policy after validating its opaque ID.
func (s *Service) GetPolicy(ctx context.Context, id string) (Policy, error) {
	if strings.TrimSpace(id) == "" {
		return Policy{}, fmt.Errorf("%w: policy ID is required", ErrInvalid)
	}
	return s.store.GetPolicy(ctx, id)
}

// ListPolicies returns a normalized provider-backed page of policies.
func (s *Service) ListPolicies(ctx context.Context, page Page) (PolicyPage, error) {
	return s.store.ListPolicies(ctx, normalizePage(page))
}

// CreatePolicy validates mutable fields and assigns service-owned timestamps.
func (s *Service) CreatePolicy(ctx context.Context, policy Policy) (Policy, error) {
	policy.Name = strings.TrimSpace(policy.Name)
	if err := validatePolicy(policy, false); err != nil {
		return Policy{}, err
	}
	now := s.now()
	policy.ID = ""
	policy.CreatedAt = now
	policy.UpdatedAt = now
	return s.store.CreatePolicy(ctx, policy)
}

// UpdatePolicy preserves the original creation time and replaces mutable
// fields with a service-owned update timestamp.
func (s *Service) UpdatePolicy(ctx context.Context, policy Policy) (Policy, error) {
	policy.ID = strings.TrimSpace(policy.ID)
	policy.Name = strings.TrimSpace(policy.Name)
	if err := validatePolicy(policy, true); err != nil {
		return Policy{}, err
	}
	existing, err := s.store.GetPolicy(ctx, policy.ID)
	if err != nil {
		return Policy{}, err
	}
	policy.CreatedAt = existing.CreatedAt
	policy.UpdatedAt = s.now()
	return s.store.UpdatePolicy(ctx, policy)
}

// GetSnapshot returns one checkpoint after validating its opaque ID.
func (s *Service) GetSnapshot(ctx context.Context, id string) (Snapshot, error) {
	if strings.TrimSpace(id) == "" {
		return Snapshot{}, fmt.Errorf("%w: snapshot ID is required", ErrInvalid)
	}
	return s.store.GetSnapshot(ctx, id)
}

// ListSnapshots returns a normalized provider-backed checkpoint page.
func (s *Service) ListSnapshots(ctx context.Context, filter SnapshotFilter) (SnapshotPage, error) {
	filter.Page = normalizePage(filter.Page)
	return s.store.ListSnapshots(ctx, filter)
}

// ListRuns returns a normalized provider-backed checkpoint audit page.
func (s *Service) ListRuns(ctx context.Context, filter RunFilter) (RunPage, error) {
	filter.Page = normalizePage(filter.Page)
	return s.store.ListRuns(ctx, filter)
}

func validatePolicy(policy Policy, requireID bool) error {
	if requireID && strings.TrimSpace(policy.ID) == "" {
		return fmt.Errorf("%w: policy ID is required", ErrInvalid)
	}
	if strings.TrimSpace(policy.Name) == "" {
		return fmt.Errorf("%w: policy name is required", ErrInvalid)
	}
	if policy.Status != PolicyActive && policy.Status != PolicyInactive {
		return fmt.Errorf("%w: unsupported policy status %q", ErrInvalid, policy.Status)
	}
	if policy.Trigger.Type != TriggerEveryNBars {
		return fmt.Errorf("%w: unsupported trigger %q", ErrInvalid, policy.Trigger.Type)
	}
	if policy.Trigger.EveryNBars == 0 {
		return fmt.Errorf("%w: every_n_bars must be positive", ErrInvalid)
	}
	return nil
}

func normalizePage(page Page) Page {
	if page.Size == 0 {
		page.Size = 100
	} else if page.Size > 1000 {
		page.Size = 1000
	}
	return page
}
