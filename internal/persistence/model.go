package persistence

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"quantram/internal/adaptive"
	"quantram/internal/domain"
)

const (
	DatabaseName               = "quantram_db"
	AperturesCollection        = "quantram_apertures"
	PayloadsCollection         = "quantram_payloads"
	DecisionsCollection        = "quantram_decisions"
	SnapshotPoliciesCollection = "quantram_snapshot_policies"
	SnapshotsCollection        = "quantram_snapshots"
	SnapshotRunsCollection     = "quantram_snapshot_runs"
)

type Aperture struct {
	ID                      bson.ObjectID `bson:"_id"`
	SequenceNum             int64         `bson:"sequence_num"`
	Open                    time.Time     `bson:"open"`
	Shut                    *time.Time    `bson:"shut"`
	Status                  string        `bson:"status"`
	SemanticContractVersion string        `bson:"semantic_contract_version"`
	ModelVersion            string        `bson:"model_version"`
	SchemaVersion           string        `bson:"schema_version"`
	CreatedAt               time.Time     `bson:"created_at"`
}

type Payload struct {
	ID         bson.ObjectID `bson:"_id"`
	ApertureID bson.ObjectID `bson:"aperture_id"`
	Bar        domain.Bar    `bson:"bar"`
}

type DecisionRecord struct {
	ID              bson.ObjectID             `bson:"_id"`
	ApertureID      bson.ObjectID             `bson:"aperture_id"`
	PayloadID       bson.ObjectID             `bson:"payload_id"`
	DecisionEvent   *domain.DecisionEvent     `bson:"decision_event,omitempty"`
	AdaptiveOutputs *adaptive.PipelineOutputs `bson:"adaptive_outputs,omitempty"`
	PriceEvent      *domain.PriceEvent        `bson:"price_event,omitempty"`
}

type Health struct {
	QueueDepth int
	Dropped    uint64
	Written    uint64
	Failures   uint64
	LastError  string
}
