package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"quantram/internal/adaptive"
	"quantram/internal/domain"
)

type MongoConfig struct {
	URI                     string
	Database                string
	SemanticContractVersion string
	ModelVersion            string
	SchemaVersion           string
}

type MongoWriter struct {
	client     *mongo.Client
	database   *mongo.Database
	apertures  apertureRepository
	disconnect func(context.Context) error
	apertureID bson.ObjectID
	payloads   *mongo.Collection
	decisions  *mongo.Collection
}

const apertureCreateAttempts = 5

type apertureRepository interface {
	LatestSequence(context.Context) (int64, error)
	Insert(context.Context, Aperture) (bson.ObjectID, error)
	Shut(context.Context, bson.ObjectID, time.Time) error
}

type mongoApertureRepository struct {
	collection *mongo.Collection
}

func OpenMongo(ctx context.Context, cfg MongoConfig) (*MongoWriter, error) {
	if cfg.URI == "" {
		return nil, fmt.Errorf("MongoDB URI is required")
	}
	if cfg.Database == "" {
		cfg.Database = DatabaseName
	}
	client, err := mongo.Connect(options.Client().ApplyURI(cfg.URI))
	if err != nil {
		return nil, fmt.Errorf("connect MongoDB: %w", err)
	}
	cleanup := func(err error) (*MongoWriter, error) {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return cleanup(fmt.Errorf("ping MongoDB: %w", err))
	}
	database := client.Database(cfg.Database)
	writer := &MongoWriter{
		client:     client,
		database:   database,
		disconnect: client.Disconnect,
		payloads:   database.Collection(PayloadsCollection),
		decisions:  database.Collection(DecisionsCollection),
	}
	if err := writer.ensureIndexes(ctx, database); err != nil {
		return cleanup(err)
	}
	repository := mongoApertureRepository{collection: database.Collection(AperturesCollection)}
	aperture, err := createProcessAperture(ctx, repository, cfg, time.Now().UTC())
	if err != nil {
		return cleanup(fmt.Errorf("create process aperture: %w", err))
	}
	writer.apertures = repository
	writer.apertureID = aperture.ID
	return writer, nil
}

func (w *MongoWriter) ApertureID() string {
	return w.apertureID.Hex()
}

func (w *MongoWriter) ensureIndexes(ctx context.Context, database *mongo.Database) error {
	unique := options.Index().SetUnique(true)
	if _, err := database.Collection(AperturesCollection).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "sequence_num", Value: 1}}, Options: unique}); err != nil {
		return fmt.Errorf("index apertures: %w", err)
	}
	if _, err := w.payloads.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "aperture_id", Value: 1}, {Key: "bar.market_snapshot_id", Value: 1}}, Options: unique},
		{Keys: bson.D{{Key: "aperture_id", Value: 1}, {Key: "bar.symbol", Value: 1}, {Key: "bar.interval_start_unix_ms", Value: 1}}},
	}); err != nil {
		return fmt.Errorf("index payloads: %w", err)
	}
	if _, err := w.decisions.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "payload_id", Value: 1}}, Options: unique},
		{Keys: bson.D{{Key: "aperture_id", Value: 1}, {Key: "payload_id", Value: 1}}},
	}); err != nil {
		return fmt.Errorf("index decisions: %w", err)
	}
	return w.ensureSnapshotIndexes(ctx)
}

func (w *MongoWriter) WriteBar(ctx context.Context, bar domain.Bar) error {
	if bar.MarketSnapshotID == "" {
		return fmt.Errorf("bar has no market_snapshot_id")
	}
	id := bson.NewObjectID()
	filter := bson.D{{Key: "aperture_id", Value: w.apertureID}, {Key: "bar.market_snapshot_id", Value: bar.MarketSnapshotID}}
	update := bson.D{{Key: "$setOnInsert", Value: Payload{ID: id, ApertureID: w.apertureID, Bar: bar}}}
	_, err := w.payloads.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))
	return err
}

func (w *MongoWriter) payloadID(ctx context.Context, snapshotID string) (bson.ObjectID, error) {
	if snapshotID == "" {
		return bson.NilObjectID, fmt.Errorf("event has no market_snapshot_id")
	}
	var payload struct {
		ID bson.ObjectID `bson:"_id"`
	}
	err := w.payloads.FindOne(ctx, bson.D{{Key: "aperture_id", Value: w.apertureID}, {Key: "bar.market_snapshot_id", Value: snapshotID}}).Decode(&payload)
	if err != nil {
		return bson.NilObjectID, fmt.Errorf("resolve payload: %w", err)
	}
	return payload.ID, nil
}

func (w *MongoWriter) WriteDecision(ctx context.Context, event domain.DecisionEvent, outputs *adaptive.PipelineOutputs) error {
	payloadID, err := w.payloadID(ctx, event.MarketSnapshotID)
	if err != nil {
		return err
	}
	set := bson.D{{Key: "decision_event", Value: event}}
	if outputs != nil {
		set = append(set, bson.E{Key: "adaptive_outputs", Value: *outputs})
	}
	return w.upsertDecision(ctx, payloadID, set)
}

func (w *MongoWriter) WritePrice(ctx context.Context, event domain.PriceEvent) error {
	payloadID, err := w.payloadID(ctx, event.MarketSnapshotID)
	if err != nil {
		return err
	}
	return w.upsertDecision(ctx, payloadID, bson.D{{Key: "price_event", Value: event}})
}

func (w *MongoWriter) upsertDecision(ctx context.Context, payloadID bson.ObjectID, set bson.D) error {
	update := bson.D{
		{Key: "$setOnInsert", Value: bson.D{{Key: "_id", Value: bson.NewObjectID()}, {Key: "aperture_id", Value: w.apertureID}, {Key: "payload_id", Value: payloadID}}},
		{Key: "$set", Value: set},
	}
	_, err := w.decisions.UpdateOne(ctx, bson.D{{Key: "payload_id", Value: payloadID}}, update, options.UpdateOne().SetUpsert(true))
	return err
}

func (w *MongoWriter) Close(ctx context.Context) error {
	shutErr := w.apertures.Shut(ctx, w.apertureID, time.Now().UTC())
	disconnectErr := w.disconnect(ctx)
	return errors.Join(shutErr, disconnectErr)
}

func createProcessAperture(ctx context.Context, repository apertureRepository, cfg MongoConfig, at time.Time) (Aperture, error) {
	for attempt := 0; attempt < apertureCreateAttempts; attempt++ {
		latest, err := repository.LatestSequence(ctx)
		if err != nil {
			return Aperture{}, err
		}
		aperture := Aperture{
			SequenceNum: latest + 1, Open: at, Status: "OPEN", CreatedAt: at,
			SemanticContractVersion: cfg.SemanticContractVersion,
			ModelVersion:            cfg.ModelVersion, SchemaVersion: cfg.SchemaVersion,
		}
		aperture.ID, err = repository.Insert(ctx, aperture)
		if err == nil {
			return aperture, nil
		}
		if !mongo.IsDuplicateKeyError(err) {
			return Aperture{}, err
		}
	}
	return Aperture{}, fmt.Errorf("allocate unique aperture sequence after %d attempts", apertureCreateAttempts)
}

func (r mongoApertureRepository) LatestSequence(ctx context.Context) (int64, error) {
	var aperture Aperture
	err := r.collection.FindOne(ctx, bson.D{}, options.FindOne().SetSort(bson.D{{Key: "sequence_num", Value: -1}})).Decode(&aperture)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, nil
	}
	return aperture.SequenceNum, err
}

func (r mongoApertureRepository) Insert(ctx context.Context, aperture Aperture) (bson.ObjectID, error) {
	document := bson.D{
		{Key: "sequence_num", Value: aperture.SequenceNum}, {Key: "open", Value: aperture.Open},
		{Key: "shut", Value: nil}, {Key: "status", Value: aperture.Status},
		{Key: "semantic_contract_version", Value: aperture.SemanticContractVersion},
		{Key: "model_version", Value: aperture.ModelVersion}, {Key: "schema_version", Value: aperture.SchemaVersion},
		{Key: "created_at", Value: aperture.CreatedAt},
	}
	result, err := r.collection.InsertOne(ctx, document)
	if err != nil {
		return bson.NilObjectID, err
	}
	id, ok := result.InsertedID.(bson.ObjectID)
	if !ok || id.IsZero() {
		return bson.NilObjectID, fmt.Errorf("MongoDB returned invalid aperture ObjectId %T", result.InsertedID)
	}
	return id, nil
}

func (r mongoApertureRepository) Shut(ctx context.Context, id bson.ObjectID, at time.Time) error {
	result, err := r.collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}, {Key: "status", Value: "OPEN"}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: "SHUT"}, {Key: "shut", Value: at}}}},
	)
	if err == nil && result.ModifiedCount != 1 {
		return fmt.Errorf("aperture %s is absent or not OPEN", id.Hex())
	}
	return err
}
