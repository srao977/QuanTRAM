package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"quantram/internal/snapshot"
)

var (
	_ snapshot.Source = (*MongoWriter)(nil)
	_ snapshot.Store  = (*MongoWriter)(nil)
)

type snapshotTriggerDocument struct {
	Type       string `bson:"type"`
	EveryNBars uint32 `bson:"every_n_bars"`
}

type snapshotPolicyDocument struct {
	ID        bson.ObjectID           `bson:"_id"`
	Name      string                  `bson:"name"`
	Status    string                  `bson:"status"`
	Trigger   snapshotTriggerDocument `bson:"trigger"`
	CreatedAt time.Time               `bson:"created_at"`
	UpdatedAt time.Time               `bson:"updated_at"`
}

type snapshotDocument struct {
	ID          bson.ObjectID `bson:"_id"`
	ApertureID  bson.ObjectID `bson:"aperture_id"`
	PolicyID    bson.ObjectID `bson:"policy_id"`
	PayloadID   bson.ObjectID `bson:"payload_id"`
	Symbol      string        `bson:"symbol"`
	SnapshotNum uint64        `bson:"snapshot_num"`
	CapturedAt  time.Time     `bson:"captured_at"`
}

type snapshotRunErrorDocument struct {
	Code    string `bson:"code"`
	Message string `bson:"message"`
}

type snapshotRunDocument struct {
	ID               bson.ObjectID             `bson:"_id"`
	ApertureID       bson.ObjectID             `bson:"aperture_id"`
	PolicyID         bson.ObjectID             `bson:"policy_id"`
	Symbol           string                    `bson:"symbol"`
	TriggerPayloadID bson.ObjectID             `bson:"trigger_payload_id"`
	TriggerCount     uint64                    `bson:"trigger_count"`
	StartedAt        time.Time                 `bson:"started_at"`
	CompletedAt      *time.Time                `bson:"completed_at"`
	Status           string                    `bson:"status"`
	SnapshotID       *bson.ObjectID            `bson:"snapshot_id"`
	Error            *snapshotRunErrorDocument `bson:"error"`
}

func (w *MongoWriter) ensureSnapshotIndexes(ctx context.Context) error {
	for collection, models := range snapshotIndexModels() {
		if _, err := w.database.Collection(collection).Indexes().CreateMany(ctx, models); err != nil {
			return fmt.Errorf("index %s: %w", collection, err)
		}
	}
	return nil
}

func snapshotIndexModels() map[string][]mongo.IndexModel {
	unique := options.Index().SetUnique(true)
	return map[string][]mongo.IndexModel{
		SnapshotPoliciesCollection: {
			{Keys: bson.D{{Key: "status", Value: 1}, {Key: "trigger.type", Value: 1}}},
		},
		SnapshotsCollection: {
			{Keys: bson.D{{Key: "aperture_id", Value: 1}, {Key: "policy_id", Value: 1}, {Key: "symbol", Value: 1}, {Key: "payload_id", Value: 1}}, Options: unique},
			{Keys: bson.D{{Key: "aperture_id", Value: 1}, {Key: "policy_id", Value: 1}, {Key: "symbol", Value: 1}, {Key: "snapshot_num", Value: 1}}},
		},
		SnapshotRunsCollection: {
			{Keys: bson.D{{Key: "aperture_id", Value: 1}, {Key: "policy_id", Value: 1}, {Key: "symbol", Value: 1}, {Key: "trigger_count", Value: 1}, {Key: "started_at", Value: 1}}},
			{Keys: bson.D{{Key: "status", Value: 1}, {Key: "started_at", Value: 1}}},
			{
				// Only one SUCCESS may claim a checkpoint; failed attempts remain
				// append-only audit records and may be retried by a later scan.
				Keys:    bson.D{{Key: "aperture_id", Value: 1}, {Key: "policy_id", Value: 1}, {Key: "symbol", Value: 1}, {Key: "trigger_payload_id", Value: 1}},
				Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.D{{Key: "status", Value: string(snapshot.RunSuccess)}}),
			},
		},
	}
}

// ListPayloads returns one Aperture's durable payload projection in stable
// symbol, interval, and opaque-ID order for exact-N counting.
func (w *MongoWriter) ListPayloads(ctx context.Context, apertureID string) ([]snapshot.Payload, error) {
	apertureObjectID, err := parseSnapshotID("aperture", apertureID)
	if err != nil {
		return nil, err
	}
	cursor, err := w.payloads.Find(ctx,
		bson.D{{Key: "aperture_id", Value: apertureObjectID}},
		options.Find().SetSort(bson.D{{Key: "bar.symbol", Value: 1}, {Key: "bar.interval_start_unix_ms", Value: 1}, {Key: "_id", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var documents []Payload
	if err := cursor.All(ctx, &documents); err != nil {
		return nil, err
	}
	payloads := make([]snapshot.Payload, 0, len(documents))
	for _, document := range documents {
		payloads = append(payloads, snapshot.Payload{
			ID: document.ID.Hex(), ApertureID: document.ApertureID.Hex(),
			Symbol: document.Bar.Symbol, IntervalStart: document.Bar.IntervalStart,
		})
	}
	return payloads, nil
}

// DecisionComplete reports whether the Payload has a durable terminal
// decision_event; optional adaptive and pricing children are not required.
func (w *MongoWriter) DecisionComplete(ctx context.Context, apertureID, payloadID string) (bool, error) {
	apertureObjectID, err := parseSnapshotID("aperture", apertureID)
	if err != nil {
		return false, err
	}
	payloadObjectID, err := parseSnapshotID("payload", payloadID)
	if err != nil {
		return false, err
	}
	err = w.decisions.FindOne(ctx, bson.D{
		{Key: "aperture_id", Value: apertureObjectID},
		{Key: "payload_id", Value: payloadObjectID},
		{Key: "decision_event", Value: bson.D{{Key: "$exists", Value: true}}},
	}, options.FindOne().SetProjection(bson.D{{Key: "_id", Value: 1}})).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	return err == nil, err
}

// ActivePolicies returns policies eligible for evaluation in stable ID order.
func (w *MongoWriter) ActivePolicies(ctx context.Context) ([]snapshot.Policy, error) {
	cursor, err := w.database.Collection(SnapshotPoliciesCollection).Find(ctx, bson.D{
		{Key: "status", Value: string(snapshot.PolicyActive)},
	}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var documents []snapshotPolicyDocument
	if err := cursor.All(ctx, &documents); err != nil {
		return nil, err
	}
	items := make([]snapshot.Policy, 0, len(documents))
	for _, document := range documents {
		items = append(items, policyFromDocument(document))
	}
	return items, nil
}

// GetPolicy returns one Snapshot policy by provider-backed opaque ID.
func (w *MongoWriter) GetPolicy(ctx context.Context, id string) (snapshot.Policy, error) {
	objectID, err := parseSnapshotID("policy", id)
	if err != nil {
		return snapshot.Policy{}, err
	}
	var document snapshotPolicyDocument
	if err := w.database.Collection(SnapshotPoliciesCollection).FindOne(ctx, bson.D{{Key: "_id", Value: objectID}}).Decode(&document); err != nil {
		return snapshot.Policy{}, convertSnapshotError(err)
	}
	return policyFromDocument(document), nil
}

// ListPolicies returns an ObjectID-cursor page ordered by ascending ID.
func (w *MongoWriter) ListPolicies(ctx context.Context, page snapshot.Page) (snapshot.PolicyPage, error) {
	filter, err := pageFilter(page.Token)
	if err != nil {
		return snapshot.PolicyPage{}, err
	}
	cursor, err := w.database.Collection(SnapshotPoliciesCollection).Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(page.Size)+1))
	if err != nil {
		return snapshot.PolicyPage{}, err
	}
	defer cursor.Close(ctx)
	var documents []snapshotPolicyDocument
	if err := cursor.All(ctx, &documents); err != nil {
		return snapshot.PolicyPage{}, err
	}
	items := make([]snapshot.Policy, 0, min(len(documents), int(page.Size)))
	for index, document := range documents {
		if index == int(page.Size) {
			return snapshot.PolicyPage{Items: items, NextToken: items[len(items)-1].ID}, nil
		}
		items = append(items, policyFromDocument(document))
	}
	return snapshot.PolicyPage{Items: items}, nil
}

// CreatePolicy persists a policy with a newly allocated provider identity.
func (w *MongoWriter) CreatePolicy(ctx context.Context, policy snapshot.Policy) (snapshot.Policy, error) {
	document := policyToDocument(policy)
	document.ID = bson.NewObjectID()
	if _, err := w.database.Collection(SnapshotPoliciesCollection).InsertOne(ctx, document); err != nil {
		return snapshot.Policy{}, err
	}
	return policyFromDocument(document), nil
}

// UpdatePolicy replaces one existing policy document.
func (w *MongoWriter) UpdatePolicy(ctx context.Context, policy snapshot.Policy) (snapshot.Policy, error) {
	document, err := policyDocumentWithID(policy)
	if err != nil {
		return snapshot.Policy{}, err
	}
	result, err := w.database.Collection(SnapshotPoliciesCollection).ReplaceOne(ctx, bson.D{{Key: "_id", Value: document.ID}}, document)
	if err != nil {
		return snapshot.Policy{}, err
	}
	if result.MatchedCount != 1 {
		return snapshot.Policy{}, snapshot.ErrNotFound
	}
	return policyFromDocument(document), nil
}

// GetSnapshot returns one checkpoint by provider-backed opaque ID.
func (w *MongoWriter) GetSnapshot(ctx context.Context, id string) (snapshot.Snapshot, error) {
	objectID, err := parseSnapshotID("snapshot", id)
	if err != nil {
		return snapshot.Snapshot{}, err
	}
	var document snapshotDocument
	if err := w.database.Collection(SnapshotsCollection).FindOne(ctx, bson.D{{Key: "_id", Value: objectID}}).Decode(&document); err != nil {
		return snapshot.Snapshot{}, convertSnapshotError(err)
	}
	return snapshotFromDocument(document), nil
}

// ListSnapshots returns an ObjectID-cursor page matching optional filters.
func (w *MongoWriter) ListSnapshots(ctx context.Context, filter snapshot.SnapshotFilter) (snapshot.SnapshotPage, error) {
	query, err := snapshotListFilter(filter)
	if err != nil {
		return snapshot.SnapshotPage{}, err
	}
	cursor, err := w.database.Collection(SnapshotsCollection).Find(ctx, query,
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(filter.Page.Size)+1))
	if err != nil {
		return snapshot.SnapshotPage{}, err
	}
	defer cursor.Close(ctx)
	var documents []snapshotDocument
	if err := cursor.All(ctx, &documents); err != nil {
		return snapshot.SnapshotPage{}, err
	}
	items := make([]snapshot.Snapshot, 0, min(len(documents), int(filter.Page.Size)))
	for index, document := range documents {
		if index == int(filter.Page.Size) {
			return snapshot.SnapshotPage{Items: items, NextToken: items[len(items)-1].ID}, nil
		}
		items = append(items, snapshotFromDocument(document))
	}
	return snapshot.SnapshotPage{Items: items}, nil
}

// SnapshotExists checks the checkpoint idempotency key without loading the
// checkpoint body.
func (w *MongoWriter) SnapshotExists(ctx context.Context, item snapshot.Snapshot) (bool, error) {
	filter, err := snapshotCheckpointFilter(item)
	if err != nil {
		return false, err
	}
	err = w.database.Collection(SnapshotsCollection).FindOne(ctx, filter, options.FindOne().SetProjection(bson.D{{Key: "_id", Value: 1}})).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	return err == nil, err
}

// CreateSnapshot idempotently inserts or returns the checkpoint identified by
// Aperture, policy, symbol, and trigger Payload.
func (w *MongoWriter) CreateSnapshot(ctx context.Context, item snapshot.Snapshot) (snapshot.Snapshot, error) {
	filter, err := snapshotCheckpointFilter(item)
	if err != nil {
		return snapshot.Snapshot{}, err
	}
	document, err := snapshotDocumentWithID(item)
	if err != nil {
		return snapshot.Snapshot{}, err
	}
	if document.ID.IsZero() {
		document.ID = bson.NewObjectID()
	}
	if err := w.database.Collection(SnapshotsCollection).FindOneAndUpdate(ctx, filter,
		bson.D{{Key: "$setOnInsert", Value: document}},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
	).Decode(&document); err != nil {
		return snapshot.Snapshot{}, err
	}
	return snapshotFromDocument(document), nil
}

// StartRun appends a STARTED checkpoint-attempt audit record.
func (w *MongoWriter) StartRun(ctx context.Context, run snapshot.Run) (snapshot.Run, error) {
	document, err := runDocumentWithID(run)
	if err != nil {
		return snapshot.Run{}, err
	}
	document.ID = bson.NewObjectID()
	if _, err := w.database.Collection(SnapshotRunsCollection).InsertOne(ctx, document); err != nil {
		return snapshot.Run{}, err
	}
	return runFromDocument(document), nil
}

// FinishRun transitions a STARTED attempt exactly once. A competing SUCCESS
// is converted into an ERROR audit record rather than claiming the checkpoint.
func (w *MongoWriter) FinishRun(ctx context.Context, runID string, status snapshot.RunStatus, snapshotID string, errorInfo *snapshot.RunErrorInfo) error {
	runObjectID, err := parseSnapshotID("run", runID)
	if err != nil {
		return err
	}
	var snapshotObjectID *bson.ObjectID
	if snapshotID != "" {
		id, parseErr := parseSnapshotID("snapshot", snapshotID)
		if parseErr != nil {
			return parseErr
		}
		snapshotObjectID = &id
	}
	var errorDocument *snapshotRunErrorDocument
	if errorInfo != nil {
		errorDocument = &snapshotRunErrorDocument{Code: errorInfo.Code, Message: errorInfo.Message}
	}
	result, err := w.database.Collection(SnapshotRunsCollection).UpdateOne(ctx,
		bson.D{{Key: "_id", Value: runObjectID}, {Key: "status", Value: string(snapshot.RunStarted)}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "status", Value: string(status)}, {Key: "completed_at", Value: time.Now().UTC()},
			{Key: "snapshot_id", Value: snapshotObjectID}, {Key: "error", Value: errorDocument},
		}}},
	)
	if status == snapshot.RunSuccess && mongo.IsDuplicateKeyError(err) {
		duplicate := &snapshotRunErrorDocument{Code: "CHECKPOINT_ALREADY_SUCCEEDED", Message: "another run already completed this checkpoint"}
		_, fallbackErr := w.database.Collection(SnapshotRunsCollection).UpdateOne(ctx,
			bson.D{{Key: "_id", Value: runObjectID}, {Key: "status", Value: string(snapshot.RunStarted)}},
			bson.D{{Key: "$set", Value: bson.D{
				{Key: "status", Value: string(snapshot.RunError)}, {Key: "completed_at", Value: time.Now().UTC()},
				{Key: "snapshot_id", Value: nil}, {Key: "error", Value: duplicate},
			}}},
		)
		return fallbackErr
	}
	if err == nil && result.ModifiedCount != 1 {
		return fmt.Errorf("%w: SnapshotRun %s is absent or not STARTED", snapshot.ErrNotFound, runID)
	}
	return err
}

// ListRuns returns an ObjectID-cursor page matching optional audit filters.
func (w *MongoWriter) ListRuns(ctx context.Context, filter snapshot.RunFilter) (snapshot.RunPage, error) {
	query, err := runListFilter(filter)
	if err != nil {
		return snapshot.RunPage{}, err
	}
	cursor, err := w.database.Collection(SnapshotRunsCollection).Find(ctx, query,
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(filter.Page.Size)+1))
	if err != nil {
		return snapshot.RunPage{}, err
	}
	defer cursor.Close(ctx)
	var documents []snapshotRunDocument
	if err := cursor.All(ctx, &documents); err != nil {
		return snapshot.RunPage{}, err
	}
	items := make([]snapshot.Run, 0, min(len(documents), int(filter.Page.Size)))
	for index, document := range documents {
		if index == int(filter.Page.Size) {
			return snapshot.RunPage{Items: items, NextToken: items[len(items)-1].ID}, nil
		}
		items = append(items, runFromDocument(document))
	}
	return snapshot.RunPage{Items: items}, nil
}

func parseSnapshotID(kind, id string) (bson.ObjectID, error) {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return bson.NilObjectID, fmt.Errorf("%w: %s ID: %v", snapshot.ErrInvalid, kind, err)
	}
	return objectID, nil
}

func convertSnapshotError(err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return snapshot.ErrNotFound
	}
	return err
}

func pageFilter(token string) (bson.D, error) {
	if token == "" {
		return bson.D{}, nil
	}
	objectID, err := parseSnapshotID("page token", token)
	if err != nil {
		return nil, err
	}
	return bson.D{{Key: "_id", Value: bson.D{{Key: "$gt", Value: objectID}}}}, nil
}

func snapshotListFilter(filter snapshot.SnapshotFilter) (bson.D, error) {
	query, err := pageFilter(filter.Page.Token)
	if err != nil {
		return nil, err
	}
	for _, value := range []struct {
		key, kind, id string
	}{{"aperture_id", "aperture", filter.ApertureID}, {"policy_id", "policy", filter.PolicyID}} {
		if value.id != "" {
			objectID, parseErr := parseSnapshotID(value.kind, value.id)
			if parseErr != nil {
				return nil, parseErr
			}
			query = append(query, bson.E{Key: value.key, Value: objectID})
		}
	}
	if filter.Symbol != "" {
		query = append(query, bson.E{Key: "symbol", Value: filter.Symbol})
	}
	return query, nil
}

func runListFilter(filter snapshot.RunFilter) (bson.D, error) {
	query, err := pageFilter(filter.Page.Token)
	if err != nil {
		return nil, err
	}
	for _, value := range []struct {
		key, kind, id string
	}{{"aperture_id", "aperture", filter.ApertureID}, {"policy_id", "policy", filter.PolicyID}} {
		if value.id != "" {
			objectID, parseErr := parseSnapshotID(value.kind, value.id)
			if parseErr != nil {
				return nil, parseErr
			}
			query = append(query, bson.E{Key: value.key, Value: objectID})
		}
	}
	if filter.Symbol != "" {
		query = append(query, bson.E{Key: "symbol", Value: filter.Symbol})
	}
	if filter.Status != "" {
		query = append(query, bson.E{Key: "status", Value: string(filter.Status)})
	}
	return query, nil
}

func snapshotCheckpointFilter(item snapshot.Snapshot) (bson.D, error) {
	apertureID, err := parseSnapshotID("aperture", item.ApertureID)
	if err != nil {
		return nil, err
	}
	policyID, err := parseSnapshotID("policy", item.PolicyID)
	if err != nil {
		return nil, err
	}
	payloadID, err := parseSnapshotID("payload", item.PayloadID)
	if err != nil {
		return nil, err
	}
	return bson.D{
		{Key: "aperture_id", Value: apertureID}, {Key: "policy_id", Value: policyID},
		{Key: "symbol", Value: item.Symbol}, {Key: "payload_id", Value: payloadID},
	}, nil
}

func policyToDocument(policy snapshot.Policy) snapshotPolicyDocument {
	return snapshotPolicyDocument{
		Name: policy.Name, Status: string(policy.Status),
		Trigger:   snapshotTriggerDocument{Type: string(policy.Trigger.Type), EveryNBars: policy.Trigger.EveryNBars},
		CreatedAt: policy.CreatedAt, UpdatedAt: policy.UpdatedAt,
	}
}

func policyDocumentWithID(policy snapshot.Policy) (snapshotPolicyDocument, error) {
	document := policyToDocument(policy)
	id, err := parseSnapshotID("policy", policy.ID)
	document.ID = id
	return document, err
}

func policyFromDocument(document snapshotPolicyDocument) snapshot.Policy {
	return snapshot.Policy{
		ID: document.ID.Hex(), Name: document.Name, Status: snapshot.PolicyStatus(document.Status),
		Trigger:   snapshot.Trigger{Type: snapshot.TriggerType(document.Trigger.Type), EveryNBars: document.Trigger.EveryNBars},
		CreatedAt: document.CreatedAt, UpdatedAt: document.UpdatedAt,
	}
}

func snapshotDocumentWithID(item snapshot.Snapshot) (snapshotDocument, error) {
	apertureID, err := parseSnapshotID("aperture", item.ApertureID)
	if err != nil {
		return snapshotDocument{}, err
	}
	policyID, err := parseSnapshotID("policy", item.PolicyID)
	if err != nil {
		return snapshotDocument{}, err
	}
	payloadID, err := parseSnapshotID("payload", item.PayloadID)
	if err != nil {
		return snapshotDocument{}, err
	}
	var id bson.ObjectID
	if item.ID != "" {
		id, err = parseSnapshotID("snapshot", item.ID)
		if err != nil {
			return snapshotDocument{}, err
		}
	}
	return snapshotDocument{
		ID: id, ApertureID: apertureID, PolicyID: policyID, PayloadID: payloadID,
		Symbol: item.Symbol, SnapshotNum: item.SnapshotNum, CapturedAt: item.CapturedAt,
	}, nil
}

func snapshotFromDocument(document snapshotDocument) snapshot.Snapshot {
	return snapshot.Snapshot{
		ID: document.ID.Hex(), ApertureID: document.ApertureID.Hex(), PolicyID: document.PolicyID.Hex(),
		PayloadID: document.PayloadID.Hex(), Symbol: document.Symbol,
		SnapshotNum: document.SnapshotNum, CapturedAt: document.CapturedAt,
	}
}

func runDocumentWithID(run snapshot.Run) (snapshotRunDocument, error) {
	apertureID, err := parseSnapshotID("aperture", run.ApertureID)
	if err != nil {
		return snapshotRunDocument{}, err
	}
	policyID, err := parseSnapshotID("policy", run.PolicyID)
	if err != nil {
		return snapshotRunDocument{}, err
	}
	payloadID, err := parseSnapshotID("payload", run.TriggerPayloadID)
	if err != nil {
		return snapshotRunDocument{}, err
	}
	var errorDocument *snapshotRunErrorDocument
	if run.Error != nil {
		errorDocument = &snapshotRunErrorDocument{Code: run.Error.Code, Message: run.Error.Message}
	}
	return snapshotRunDocument{
		ApertureID: apertureID, PolicyID: policyID, Symbol: run.Symbol, TriggerPayloadID: payloadID,
		TriggerCount: run.TriggerCount, StartedAt: run.StartedAt, CompletedAt: run.CompletedAt,
		Status: string(run.Status), Error: errorDocument,
	}, nil
}

func runFromDocument(document snapshotRunDocument) snapshot.Run {
	var errorInfo *snapshot.RunErrorInfo
	if document.Error != nil {
		errorInfo = &snapshot.RunErrorInfo{Code: document.Error.Code, Message: document.Error.Message}
	}
	snapshotID := ""
	if document.SnapshotID != nil {
		snapshotID = document.SnapshotID.Hex()
	}
	return snapshot.Run{
		ID: document.ID.Hex(), ApertureID: document.ApertureID.Hex(), PolicyID: document.PolicyID.Hex(),
		Symbol: document.Symbol, TriggerPayloadID: document.TriggerPayloadID.Hex(), TriggerCount: document.TriggerCount,
		StartedAt: document.StartedAt, CompletedAt: document.CompletedAt, Status: snapshot.RunStatus(document.Status),
		SnapshotID: snapshotID, Error: errorInfo,
	}
}
