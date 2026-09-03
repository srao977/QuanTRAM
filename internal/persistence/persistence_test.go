package persistence

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"quantram/internal/adaptive"
	"quantram/internal/domain"
)

func TestPayloadAndDecisionBSONPreserveCanonicalNesting(t *testing.T) {
	apertureID := bson.NewObjectID()
	payloadID := bson.NewObjectID()
	bar := domain.Bar{Symbol: "AAPL", Open: 101.25, MarketSnapshotID: "snapshot-1"}
	payloadBytes, err := bson.Marshal(Payload{ID: payloadID, ApertureID: apertureID, Bar: bar})
	if err != nil {
		t.Fatal(err)
	}
	payload := bson.Raw(payloadBytes)
	storedBar := payload.Lookup("bar").Document()
	if storedBar.Lookup("open").Double() != 101.25 || storedBar.Lookup("market_snapshot_id").StringValue() != "snapshot-1" {
		t.Fatalf("canonical nested Bar not preserved: %v", storedBar)
	}
	if payload.Lookup("open").Type != 0 {
		t.Fatal("Payload must not flatten Bar fields")
	}

	recordBytes, err := bson.Marshal(DecisionRecord{
		ID:              bson.NewObjectID(),
		ApertureID:      apertureID,
		PayloadID:       payloadID,
		DecisionEvent:   &domain.DecisionEvent{MarketSnapshotID: "snapshot-1"},
		AdaptiveOutputs: &adaptive.PipelineOutputs{DMO: adaptive.DMOOutput{EntityID: "AAPL"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	record := bson.Raw(recordBytes)
	outputs := record.Lookup("adaptive_outputs").Document()
	if outputs.Lookup("dmo").Type != bson.TypeEmbeddedDocument {
		t.Fatalf("canonical DMO nesting missing: %v", outputs)
	}
	if record.Lookup("decision_values").Type != 0 {
		t.Fatal("generic decision_values representation is forbidden")
	}
}

func TestApertureBSONUsesOpenShutVocabulary(t *testing.T) {
	open := time.Date(2026, 9, 3, 13, 0, 0, 0, time.UTC)
	data, err := bson.Marshal(Aperture{ID: bson.NewObjectID(), Open: open})
	if err != nil {
		t.Fatal(err)
	}
	document := bson.Raw(data)
	if document.Lookup("open").Type != bson.TypeDateTime || document.Lookup("shut").Type == 0 {
		t.Fatalf("open/shut fields missing: %v", document)
	}
	if document.Lookup("opened_at").Type != 0 || document.Lookup("shut_at").Type != 0 {
		t.Fatal("superseded opened_at/shut_at fields must not be persisted")
	}
}

type fakeApertureRepository struct {
	apertures []Aperture
	shutIDs   []bson.ObjectID
}

func (r *fakeApertureRepository) LatestSequence(context.Context) (int64, error) {
	var latest int64
	for _, aperture := range r.apertures {
		if aperture.SequenceNum > latest {
			latest = aperture.SequenceNum
		}
	}
	return latest, nil
}

func (r *fakeApertureRepository) Insert(_ context.Context, aperture Aperture) (bson.ObjectID, error) {
	aperture.ID = bson.NewObjectID()
	r.apertures = append(r.apertures, aperture)
	return aperture.ID, nil
}

func (r *fakeApertureRepository) Shut(_ context.Context, id bson.ObjectID, at time.Time) error {
	for index := range r.apertures {
		if r.apertures[index].ID != id || r.apertures[index].Status != "OPEN" {
			continue
		}
		r.apertures[index].Status = "SHUT"
		r.apertures[index].Shut = &at
		r.shutIDs = append(r.shutIDs, id)
		return nil
	}
	return fmt.Errorf("aperture not OPEN")
}

func TestCreateProcessApertureOwnsNewLifecycle(t *testing.T) {
	repository := &fakeApertureRepository{}
	openedAt := time.Date(2026, 9, 3, 14, 30, 0, 0, time.UTC)
	cfg := MongoConfig{SemanticContractVersion: "semantic-v1", ModelVersion: "model-v1", SchemaVersion: "schema-v1"}

	first, err := createProcessAperture(context.Background(), repository, cfg, openedAt)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID.IsZero() || first.SequenceNum != 1 || first.Status != "OPEN" || first.Shut != nil {
		t.Fatalf("first Aperture=%+v", first)
	}
	if !first.Open.Equal(openedAt) || !first.CreatedAt.Equal(openedAt) {
		t.Fatalf("opening timestamps differ: open=%s created=%s", first.Open, first.CreatedAt)
	}
	if first.SemanticContractVersion != "semantic-v1" || first.ModelVersion != "model-v1" || first.SchemaVersion != "schema-v1" {
		t.Fatalf("version metadata=%+v", first)
	}
	if len(repository.shutIDs) != 0 {
		t.Fatal("creating or recreating persistence must not SHUT an Aperture")
	}

	second, err := createProcessAperture(context.Background(), repository, cfg, openedAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID || second.SequenceNum != 2 || len(repository.apertures) != 2 {
		t.Fatalf("next lifecycle reused Aperture: first=%+v second=%+v", first, second)
	}
	if repository.apertures[0].Status != "OPEN" || repository.apertures[0].Shut != nil {
		t.Fatalf("old abnormal Aperture was repaired: %+v", repository.apertures[0])
	}
}

func TestGeneratedApertureIsWriterLineageContext(t *testing.T) {
	repository := &fakeApertureRepository{}
	aperture, err := createProcessAperture(context.Background(), repository, MongoConfig{}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	writer := &MongoWriter{apertureID: aperture.ID}
	if writer.ApertureID() != aperture.ID.Hex() {
		t.Fatalf("writer Aperture=%q, want %q", writer.ApertureID(), aperture.ID.Hex())
	}
	payload := Payload{ID: bson.NewObjectID(), ApertureID: writer.apertureID}
	decision := DecisionRecord{ID: bson.NewObjectID(), ApertureID: writer.apertureID, PayloadID: payload.ID}
	if payload.ApertureID != aperture.ID || decision.ApertureID != aperture.ID || decision.PayloadID != payload.ID {
		t.Fatalf("lineage mismatch: payload=%+v decision=%+v", payload, decision)
	}
}

func TestMongoWriterCloseShutsCurrentApertureThenDisconnects(t *testing.T) {
	repository := &fakeApertureRepository{}
	openedAt := time.Date(2026, 9, 3, 15, 0, 0, 0, time.UTC)
	aperture, err := createProcessAperture(context.Background(), repository, MongoConfig{}, openedAt)
	if err != nil {
		t.Fatal(err)
	}
	disconnected := false
	writer := &MongoWriter{
		apertures: repository, apertureID: aperture.ID,
		disconnect: func(context.Context) error {
			if repository.apertures[0].Status != "SHUT" {
				t.Fatal("MongoDB disconnected before current Aperture was SHUT")
			}
			disconnected = true
			return nil
		},
	}
	if err := writer.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	closed := repository.apertures[0]
	if !disconnected || closed.ID != aperture.ID || closed.SequenceNum != aperture.SequenceNum || !closed.Open.Equal(openedAt) {
		t.Fatalf("immutable lifecycle fields changed: before=%+v after=%+v", aperture, closed)
	}
	if closed.Status != "SHUT" || closed.Shut == nil || closed.Shut.Before(openedAt) {
		t.Fatalf("Aperture not orderly SHUT: %+v", closed)
	}
}

type recordingWriter struct {
	mu        sync.Mutex
	order     []string
	release   chan struct{}
	started   chan struct{}
	startOnce sync.Once
}

func (w *recordingWriter) record(value string) {
	w.startOnce.Do(func() { close(w.started) })
	if w.release != nil {
		<-w.release
	}
	w.mu.Lock()
	w.order = append(w.order, value)
	w.mu.Unlock()
}

func (w *recordingWriter) WriteBar(_ context.Context, bar domain.Bar) error {
	w.record("bar:" + bar.MarketSnapshotID)
	return nil
}
func (w *recordingWriter) WriteDecision(_ context.Context, event domain.DecisionEvent, _ *adaptive.PipelineOutputs) error {
	w.record("decision:" + event.MarketSnapshotID)
	return nil
}
func (w *recordingWriter) WritePrice(_ context.Context, event domain.PriceEvent) error {
	w.record("price:" + event.MarketSnapshotID)
	return nil
}
func (w *recordingWriter) Close(context.Context) error { return nil }

func TestAsyncStoreIsBoundedNonblockingAndFlushesInOrder(t *testing.T) {
	writer := &recordingWriter{release: make(chan struct{}), started: make(chan struct{})}
	store := NewAsyncStore(writer, 1)
	if !store.CaptureBar(domain.Bar{MarketSnapshotID: "one"}) {
		t.Fatal("first capture rejected")
	}
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	if !store.CaptureDecision(domain.DecisionEvent{MarketSnapshotID: "one"}, nil) {
		t.Fatal("queued decision rejected")
	}
	started := time.Now()
	if store.CapturePrice(domain.PriceEvent{MarketSnapshotID: "one"}) {
		t.Fatal("capture should report a full queue")
	}
	if time.Since(started) > 50*time.Millisecond {
		t.Fatal("full queue capture blocked science")
	}
	if store.Health().Dropped != 1 {
		t.Fatalf("dropped=%d", store.Health().Dropped)
	}
	close(writer.release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := store.Close(ctx); err != nil {
		t.Fatal(err)
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if len(writer.order) != 2 || writer.order[0] != "bar:one" || writer.order[1] != "decision:one" {
		t.Fatalf("write order=%v", writer.order)
	}
}
