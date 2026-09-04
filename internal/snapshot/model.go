package snapshot

import "time"

// PolicyStatus controls whether a checkpoint policy participates in scans.
type PolicyStatus string

const (
	// PolicyActive enables policy evaluation.
	PolicyActive PolicyStatus = "ACTIVE"
	// PolicyInactive retains a policy without evaluating it.
	PolicyInactive PolicyStatus = "INACTIVE"
)

// TriggerType identifies a checkpoint candidate-selection rule.
type TriggerType string

// TriggerEveryNBars selects each exact multiple of a durable per-symbol count.
const TriggerEveryNBars TriggerType = "EVERY_N_BARS"

// Trigger configures a policy's candidate-selection rule.
type Trigger struct {
	Type       TriggerType
	EveryNBars uint32
}

// Policy defines one persisted Snapshot checkpoint policy.
type Policy struct {
	ID        string
	Name      string
	Status    PolicyStatus
	Trigger   Trigger
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Payload is the durable ledger projection used for candidate ordering.
type Payload struct {
	ID            string
	ApertureID    string
	Symbol        string
	IntervalStart time.Time
}

// Snapshot records an idempotent checkpoint at one trigger Payload.
type Snapshot struct {
	ID          string
	ApertureID  string
	PolicyID    string
	PayloadID   string
	Symbol      string
	SnapshotNum uint64
	CapturedAt  time.Time
}

// RunStatus is the durable lifecycle state of a checkpoint attempt.
type RunStatus string

const (
	// RunStarted marks an attempt whose checkpoint write is in progress.
	RunStarted RunStatus = "STARTED"
	// RunSuccess marks an attempt that published a checkpoint.
	RunSuccess RunStatus = "SUCCESS"
	// RunError marks an attempt that exhausted checkpoint persistence.
	RunError RunStatus = "ERROR"
)

// RunErrorInfo carries the stable classification and provider detail for a
// failed checkpoint attempt.
type RunErrorInfo struct {
	Code    string
	Message string
}

// Run is the audit record for one eligible checkpoint attempt.
type Run struct {
	ID               string
	ApertureID       string
	PolicyID         string
	Symbol           string
	TriggerPayloadID string
	TriggerCount     uint64
	StartedAt        time.Time
	CompletedAt      *time.Time
	Status           RunStatus
	SnapshotID       string
	Error            *RunErrorInfo
}

// Page carries provider-neutral cursor pagination parameters.
type Page struct {
	Size  uint32
	Token string
}

// PolicyPage is one page of policies and its continuation token.
type PolicyPage struct {
	Items     []Policy
	NextToken string
}

// SnapshotFilter selects checkpoints by lineage, policy, and symbol.
type SnapshotFilter struct {
	ApertureID string
	PolicyID   string
	Symbol     string
	Page       Page
}

// SnapshotPage is one page of checkpoints and its continuation token.
type SnapshotPage struct {
	Items     []Snapshot
	NextToken string
}

// RunFilter selects checkpoint attempts by lineage, policy, symbol, and state.
type RunFilter struct {
	ApertureID string
	PolicyID   string
	Symbol     string
	Status     RunStatus
	Page       Page
}

// RunPage is one page of checkpoint attempts and its continuation token.
type RunPage struct {
	Items     []Run
	NextToken string
}
