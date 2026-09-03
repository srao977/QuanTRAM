package persistence

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"quantram/internal/snapshot"
)

func TestSnapshotAdapterObjectIDMapping(t *testing.T) {
	id := bson.NewObjectID()
	got, err := parseSnapshotID("snapshot", id.Hex())
	if err != nil || got != id {
		t.Fatalf("parseSnapshotID()=(%s, %v), want %s", got.Hex(), err, id.Hex())
	}
	if _, err := parseSnapshotID("snapshot", "public-opaque-but-not-mongo"); !errors.Is(err, snapshot.ErrInvalid) {
		t.Fatalf("invalid provider ID error=%v", err)
	}
	if !errors.Is(convertSnapshotError(mongo.ErrNoDocuments), snapshot.ErrNotFound) {
		t.Fatal("MongoDB no-document error was not converted")
	}
}

func TestSnapshotAdapterPolicyMapping(t *testing.T) {
	now := time.Date(2026, 9, 3, 14, 0, 0, 0, time.UTC)
	original := snapshot.Policy{
		ID: bson.NewObjectID().Hex(), Name: "Every ten", Status: snapshot.PolicyActive,
		Trigger:   snapshot.Trigger{Type: snapshot.TriggerEveryNBars, EveryNBars: 10},
		CreatedAt: now, UpdatedAt: now.Add(time.Minute),
	}
	document, err := policyDocumentWithID(original)
	if err != nil {
		t.Fatal(err)
	}
	got := policyFromDocument(document)
	if got != original {
		t.Fatalf("policy round trip=%+v want=%+v", got, original)
	}
	data, err := bson.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	raw := bson.Raw(data)
	trigger := raw.Lookup("trigger").Document()
	if trigger.Lookup("type").StringValue() != string(snapshot.TriggerEveryNBars) || trigger.Lookup("every_n_bars").AsInt64() != 10 {
		t.Fatalf("policy BSON trigger=%v", trigger)
	}
}

func TestSnapshotAdapterSnapshotAndRunMapping(t *testing.T) {
	now := time.Date(2026, 9, 3, 14, 10, 0, 0, time.UTC)
	item := snapshot.Snapshot{
		ID: bson.NewObjectID().Hex(), ApertureID: bson.NewObjectID().Hex(),
		PolicyID: bson.NewObjectID().Hex(), PayloadID: bson.NewObjectID().Hex(),
		Symbol: "AAPL", SnapshotNum: 3, CapturedAt: now,
	}
	document, err := snapshotDocumentWithID(item)
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshotFromDocument(document); got != item {
		t.Fatalf("Snapshot round trip=%+v want=%+v", got, item)
	}

	completed := now.Add(time.Second)
	run := snapshot.Run{
		ApertureID: item.ApertureID, PolicyID: item.PolicyID, TriggerPayloadID: item.PayloadID,
		Symbol: item.Symbol, TriggerCount: 30, StartedAt: now, CompletedAt: &completed,
		Status: snapshot.RunError, Error: &snapshot.RunErrorInfo{Code: "WRITE", Message: "failed"},
	}
	runDocument, err := runDocumentWithID(run)
	if err != nil {
		t.Fatal(err)
	}
	runDocument.ID = bson.NewObjectID()
	gotRun := runFromDocument(runDocument)
	if gotRun.ID != runDocument.ID.Hex() || gotRun.ApertureID != run.ApertureID || gotRun.TriggerPayloadID != run.TriggerPayloadID || gotRun.Error == nil || *gotRun.Error != *run.Error {
		t.Fatalf("SnapshotRun mapping=%+v", gotRun)
	}
}

func TestSnapshotAdapterIndexDefinitions(t *testing.T) {
	models := snapshotIndexModels()
	if len(models[SnapshotPoliciesCollection]) != 1 || len(models[SnapshotsCollection]) != 2 || len(models[SnapshotRunsCollection]) != 3 {
		t.Fatalf("Snapshot index definitions=%v", models)
	}
	checkpoint := models[SnapshotsCollection][0]
	keys, ok := checkpoint.Keys.(bson.D)
	if !ok {
		t.Fatalf("checkpoint index keys type=%T", checkpoint.Keys)
	}
	want := bson.D{{Key: "aperture_id", Value: 1}, {Key: "policy_id", Value: 1}, {Key: "symbol", Value: 1}, {Key: "payload_id", Value: 1}}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("checkpoint keys=%v want=%v", keys, want)
	}
	indexOptions := &options.IndexOptions{}
	if checkpoint.Options == nil {
		t.Fatal("checkpoint identity index must be unique")
	}
	for _, apply := range checkpoint.Options.List() {
		if err := apply(indexOptions); err != nil {
			t.Fatal(err)
		}
	}
	if indexOptions.Unique == nil || !*indexOptions.Unique {
		t.Fatal("checkpoint identity index must be unique")
	}
	success := models[SnapshotRunsCollection][2]
	successOptions := &options.IndexOptions{}
	for _, apply := range success.Options.List() {
		if err := apply(successOptions); err != nil {
			t.Fatal(err)
		}
	}
	if successOptions.Unique == nil || !*successOptions.Unique || successOptions.PartialFilterExpression == nil {
		t.Fatal("successful run checkpoint index must be unique and partial")
	}
}
