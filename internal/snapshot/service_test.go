package snapshot

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
)

type fakeBackend struct {
	mu            sync.Mutex
	policies      map[string]Policy
	payloads      map[string][]Payload
	complete      map[string]bool
	snapshots     map[string]Snapshot
	runs          map[string]Run
	failPayload   string
	failRemaining int
	createCalls   map[string]int
	nextID        int
	scienceCalls  int
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		policies: make(map[string]Policy), payloads: make(map[string][]Payload), complete: make(map[string]bool),
		snapshots: make(map[string]Snapshot), runs: make(map[string]Run), createCalls: make(map[string]int),
	}
}

func (f *fakeBackend) id(prefix string) string {
	f.nextID++
	return fmt.Sprintf("%s-%d", prefix, f.nextID)
}

func (f *fakeBackend) ListPayloads(_ context.Context, apertureID string) ([]Payload, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Payload(nil), f.payloads[apertureID]...), nil
}

func (f *fakeBackend) DecisionComplete(_ context.Context, apertureID, payloadID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.complete[apertureID+"/"+payloadID], nil
}

func (f *fakeBackend) ActivePolicies(context.Context) ([]Policy, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var policies []Policy
	for _, policy := range f.policies {
		if policy.Status == PolicyActive {
			policies = append(policies, policy)
		}
	}
	return policies, nil
}

func (f *fakeBackend) GetPolicy(_ context.Context, id string) (Policy, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	policy, ok := f.policies[id]
	if !ok {
		return Policy{}, ErrNotFound
	}
	return policy, nil
}

func (f *fakeBackend) ListPolicies(_ context.Context, page Page) (PolicyPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	items := make([]Policy, 0, len(f.policies))
	for _, policy := range f.policies {
		items = append(items, policy)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return PolicyPage{Items: items}, nil
}

func (f *fakeBackend) CreatePolicy(_ context.Context, policy Policy) (Policy, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	policy.ID = f.id("policy")
	f.policies[policy.ID] = policy
	return policy, nil
}

func (f *fakeBackend) UpdatePolicy(_ context.Context, policy Policy) (Policy, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.policies[policy.ID]; !ok {
		return Policy{}, ErrNotFound
	}
	f.policies[policy.ID] = policy
	return policy, nil
}

func (f *fakeBackend) GetSnapshot(_ context.Context, id string) (Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, item := range f.snapshots {
		if item.ID == id {
			return item, nil
		}
	}
	return Snapshot{}, ErrNotFound
}

func (f *fakeBackend) ListSnapshots(_ context.Context, filter SnapshotFilter) (SnapshotPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var items []Snapshot
	for _, item := range f.snapshots {
		if (filter.ApertureID == "" || item.ApertureID == filter.ApertureID) &&
			(filter.PolicyID == "" || item.PolicyID == filter.PolicyID) &&
			(filter.Symbol == "" || item.Symbol == filter.Symbol) {
			items = append(items, item)
		}
	}
	return SnapshotPage{Items: items}, nil
}

func checkpointKey(item Snapshot) string {
	return item.ApertureID + "/" + item.PolicyID + "/" + item.Symbol + "/" + item.PayloadID
}

func (f *fakeBackend) SnapshotExists(_ context.Context, item Snapshot) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.snapshots[checkpointKey(item)]
	return ok, nil
}

func (f *fakeBackend) CreateSnapshot(_ context.Context, item Snapshot) (Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls[item.PayloadID]++
	if item.PayloadID == f.failPayload && f.failRemaining > 0 {
		f.failRemaining--
		return Snapshot{}, errors.New("injected snapshot write failure")
	}
	key := checkpointKey(item)
	if existing, ok := f.snapshots[key]; ok {
		return existing, nil
	}
	item.ID = f.id("snapshot")
	f.snapshots[key] = item
	return item, nil
}

func (f *fakeBackend) StartRun(_ context.Context, run Run) (Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	run.ID = f.id("run")
	f.runs[run.ID] = run
	return run, nil
}

func (f *fakeBackend) FinishRun(_ context.Context, id string, status RunStatus, snapshotID string, errorInfo *RunErrorInfo) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	run, ok := f.runs[id]
	if !ok || run.Status != RunStarted {
		return errors.New("run is absent or not STARTED")
	}
	completed := time.Now().UTC()
	run.CompletedAt, run.Status, run.SnapshotID, run.Error = &completed, status, snapshotID, errorInfo
	f.runs[id] = run
	return nil
}

func (f *fakeBackend) ListRuns(_ context.Context, filter RunFilter) (RunPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var items []Run
	for _, item := range f.runs {
		if (filter.ApertureID == "" || item.ApertureID == filter.ApertureID) &&
			(filter.PolicyID == "" || item.PolicyID == filter.PolicyID) &&
			(filter.Symbol == "" || item.Symbol == filter.Symbol) &&
			(filter.Status == "" || item.Status == filter.Status) {
			items = append(items, item)
		}
	}
	return RunPage{Items: items}, nil
}

func everyTenPolicy(status PolicyStatus) Policy {
	return Policy{ID: "policy-10", Name: "Development Every 10 Bars", Status: status, Trigger: Trigger{Type: TriggerEveryNBars, EveryNBars: 10}}
}

func addPayloads(backend *fakeBackend, apertureID, symbol string, count int, irregular bool) []Payload {
	start := time.Date(2026, 9, 3, 13, 30, 0, 0, time.UTC)
	offset := 0
	for _, existing := range backend.payloads[apertureID] {
		if existing.Symbol == symbol {
			offset++
		}
	}
	result := make([]Payload, 0, count)
	for index := 0; index < count; index++ {
		sequence := offset + index
		minute := sequence
		if irregular && sequence >= 2 {
			minute += 7
		}
		payload := Payload{
			ID: fmt.Sprintf("%s-%d", symbol, sequence+1), ApertureID: apertureID, Symbol: symbol,
			IntervalStart: start.Add(time.Duration(minute) * time.Minute),
		}
		backend.payloads[apertureID] = append(backend.payloads[apertureID], payload)
		backend.complete[apertureID+"/"+payload.ID] = true
		result = append(result, payload)
	}
	return result
}

func TestPolicyCRUDAndValidation(t *testing.T) {
	ctx := context.Background()
	backend := newFakeBackend()
	service := NewService(backend, backend, "aperture-a", time.Second)
	created, err := service.CreatePolicy(ctx, Policy{Name: "Every ten", Status: PolicyActive, Trigger: Trigger{Type: TriggerEveryNBars, EveryNBars: 10}})
	if err != nil || created.ID == "" || created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("CreatePolicy()=(%+v, %v)", created, err)
	}
	created.Status = PolicyInactive
	updated, err := service.UpdatePolicy(ctx, created)
	if err != nil || updated.Status != PolicyInactive || !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("UpdatePolicy()=(%+v, %v)", updated, err)
	}
	if _, err := service.CreatePolicy(ctx, Policy{Name: "bad", Status: PolicyActive, Trigger: Trigger{Type: TriggerEveryNBars}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero EveryNBars error=%v", err)
	}
}

func TestEveryTenPolicyActivationAndExactCandidates(t *testing.T) {
	ctx := context.Background()
	backend := newFakeBackend()
	backend.policies["policy-10"] = everyTenPolicy(PolicyInactive)
	payloads := addPayloads(backend, "aperture-a", "AAPL", 30, true)
	service := NewService(backend, backend, "aperture-a", time.Second)
	if err := service.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	if len(backend.snapshots) != 0 {
		t.Fatal("INACTIVE policy created a Snapshot")
	}
	backend.policies["policy-10"] = everyTenPolicy(PolicyActive)
	if err := service.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	if len(backend.snapshots) != 3 {
		t.Fatalf("Snapshots=%d want checkpoints 10, 20, 30", len(backend.snapshots))
	}
	for ordinal, index := range map[uint64]int{1: 9, 2: 19, 3: 29} {
		key := checkpointKey(Snapshot{ApertureID: "aperture-a", PolicyID: "policy-10", Symbol: "AAPL", PayloadID: payloads[index].ID})
		if got := backend.snapshots[key]; got.SnapshotNum != ordinal || got.PayloadID != payloads[index].ID {
			t.Fatalf("checkpoint %d=%+v", ordinal, got)
		}
	}
}

func TestCountsAreDurableAndIsolated(t *testing.T) {
	ctx := context.Background()
	backend := newFakeBackend()
	backend.policies["policy-10"] = everyTenPolicy(PolicyActive)
	addPayloads(backend, "aperture-a", "AAPL", 9, false)
	addPayloads(backend, "aperture-a", "SPY", 10, false)
	addPayloads(backend, "aperture-b", "AAPL", 10, false)
	service := NewService(backend, backend, "aperture-a", time.Second)
	if err := service.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	if len(backend.snapshots) != 1 {
		t.Fatalf("only SPY in aperture A should checkpoint, got %d", len(backend.snapshots))
	}
	addPayloads(backend, "aperture-a", "AAPL", 1, true)
	restarted := NewService(backend, backend, "aperture-a", time.Second)
	if err := restarted.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	if len(backend.snapshots) != 2 {
		t.Fatalf("recreated service did not resume AAPL count, snapshots=%d", len(backend.snapshots))
	}
	for _, item := range backend.snapshots {
		if item.ApertureID != "aperture-a" {
			t.Fatal("count leaked across Apertures")
		}
	}
}

func TestCandidateWaitsForTerminalDecisionAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	backend := newFakeBackend()
	backend.policies["policy-10"] = everyTenPolicy(PolicyActive)
	payloads := addPayloads(backend, "aperture-a", "AAPL", 10, false)
	backend.complete["aperture-a/"+payloads[9].ID] = false
	service := NewService(backend, backend, "aperture-a", time.Second)
	if err := service.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	if len(backend.snapshots) != 0 || len(backend.runs) != 0 {
		t.Fatal("incomplete candidate was published or audited as an attempt")
	}
	backend.complete["aperture-a/"+payloads[9].ID] = true
	if err := service.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := service.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	if len(backend.snapshots) != 1 || len(backend.runs) != 1 {
		t.Fatalf("idempotency snapshots=%d runs=%d", len(backend.snapshots), len(backend.runs))
	}
	for _, run := range backend.runs {
		if run.Status != RunSuccess || run.SnapshotID == "" || run.Error != nil || run.CompletedAt == nil {
			t.Fatalf("success run=%+v", run)
		}
	}
}

func TestFailureAtThirtyDoesNotBlockFortyOrInvokeScience(t *testing.T) {
	ctx := context.Background()
	backend := newFakeBackend()
	backend.policies["policy-10"] = everyTenPolicy(PolicyActive)
	payloads := addPayloads(backend, "aperture-a", "AAPL", 40, true)
	backend.failPayload, backend.failRemaining = payloads[29].ID, 3
	service := NewService(backend, backend, "aperture-a", time.Second)
	if err := service.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	if len(backend.snapshots) != 3 || backend.createCalls[payloads[29].ID] != 3 {
		t.Fatalf("snapshots=%d retries=%d", len(backend.snapshots), backend.createCalls[payloads[29].ID])
	}
	statuses := make(map[RunStatus]int)
	for _, run := range backend.runs {
		statuses[run.Status]++
		if run.Status == RunError && (run.Error == nil || run.Error.Code != "SNAPSHOT_PERSISTENCE_ERROR" || run.SnapshotID != "") {
			t.Fatalf("error run=%+v", run)
		}
	}
	if statuses[RunError] != 1 || statuses[RunSuccess] != 3 {
		t.Fatalf("run statuses=%v", statuses)
	}
	if backend.scienceCalls != 0 {
		t.Fatalf("Snapshot service invoked science %d times", backend.scienceCalls)
	}
	if err := service.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	if len(backend.snapshots) != 4 {
		t.Fatal("failed checkpoint was not resumable on a later scan")
	}
}
