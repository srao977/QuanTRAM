package snapshot

import "time"

type PolicyStatus string

const (
	PolicyActive   PolicyStatus = "ACTIVE"
	PolicyInactive PolicyStatus = "INACTIVE"
)

type TriggerType string

const TriggerEveryNBars TriggerType = "EVERY_N_BARS"

type Trigger struct {
	Type       TriggerType
	EveryNBars uint32
}

type Policy struct {
	ID        string
	Name      string
	Status    PolicyStatus
	Trigger   Trigger
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Payload struct {
	ID            string
	ApertureID    string
	Symbol        string
	IntervalStart time.Time
}

type Snapshot struct {
	ID          string
	ApertureID  string
	PolicyID    string
	PayloadID   string
	Symbol      string
	SnapshotNum uint64
	CapturedAt  time.Time
}

type RunStatus string

const (
	RunStarted RunStatus = "STARTED"
	RunSuccess RunStatus = "SUCCESS"
	RunError   RunStatus = "ERROR"
)

type RunErrorInfo struct {
	Code    string
	Message string
}

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

type Page struct {
	Size  uint32
	Token string
}

type PolicyPage struct {
	Items     []Policy
	NextToken string
}

type SnapshotFilter struct {
	ApertureID string
	PolicyID   string
	Symbol     string
	Page       Page
}

type SnapshotPage struct {
	Items     []Snapshot
	NextToken string
}

type RunFilter struct {
	ApertureID string
	PolicyID   string
	Symbol     string
	Status     RunStatus
	Page       Page
}

type RunPage struct {
	Items     []Run
	NextToken string
}
