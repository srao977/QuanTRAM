// Tests in this file verify server composition and coordinated lifecycle order
// without opening listeners, feeds, or MongoDB connections.
package main

import (
	"context"
	"sync"
	"testing"

	"quantram/internal/config"
)

type lifecycleRecorder struct {
	mu    sync.Mutex
	order []string
}

func (r *lifecycleRecorder) add(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.order = append(r.order, event)
}

func (r *lifecycleRecorder) index(event string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index, recorded := range r.order {
		if recorded == event {
			return index
		}
	}
	return -1
}

type recordingGRPC struct {
	recorder *lifecycleRecorder
}

func (g recordingGRPC) GracefulStop() { g.recorder.add("grpc-stopped") }
func (g recordingGRPC) Stop()         { g.recorder.add("grpc-forced") }

type recordingSnapshot struct {
	recorder *lifecycleRecorder
}

func (s recordingSnapshot) FinalEvaluate(context.Context) error {
	s.recorder.add("snapshot-final")
	return nil
}

type recordingPersistence struct {
	recorder *lifecycleRecorder
}

func (p recordingPersistence) Drain(context.Context) error {
	p.recorder.add("persistence-drained")
	return nil
}

func (p recordingPersistence) Close(context.Context) error {
	p.recorder.add("persistence-closed")
	return nil
}

func TestNewPersistenceLeavesMongoDisabled(t *testing.T) {
	store, snapshots, err := newPersistence(config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if store != nil || snapshots != nil {
		t.Fatalf("disabled persistence returned store=%v snapshots=%v", store, snapshots)
	}
}

func TestShutdownRuntimeCoordinatesProducersSnapshotAndPersistence(t *testing.T) {
	recorder := &lifecycleRecorder{}
	producerCtx, cancelProducers := context.WithCancel(context.Background())
	snapshotCtx, cancelSnapshot := context.WithCancel(context.Background())

	var producerWG sync.WaitGroup
	producerWG.Add(1)
	go func() {
		defer producerWG.Done()
		<-producerCtx.Done()
		recorder.add("producer-stopped")
	}()

	var snapshotWG sync.WaitGroup
	snapshotWG.Add(1)
	go func() {
		defer snapshotWG.Done()
		<-snapshotCtx.Done()
		recorder.add("snapshot-stopped")
	}()

	err := shutdownRuntime(
		recordingGRPC{recorder}, cancelProducers, &producerWG,
		recordingSnapshot{recorder}, cancelSnapshot, &snapshotWG,
		recordingPersistence{recorder},
	)
	if err != nil {
		t.Fatal(err)
	}

	producerStopped := recorder.index("producer-stopped")
	grpcStopped := recorder.index("grpc-stopped")
	persistenceDrained := recorder.index("persistence-drained")
	finalSnapshot := recorder.index("snapshot-final")
	snapshotStopped := recorder.index("snapshot-stopped")
	persistenceClosed := recorder.index("persistence-closed")
	if producerStopped < 0 || grpcStopped < 0 || persistenceDrained <= producerStopped || persistenceDrained <= grpcStopped || finalSnapshot <= persistenceDrained || snapshotStopped <= finalSnapshot || persistenceClosed <= snapshotStopped {
		t.Fatalf("shutdown order=%v", recorder.order)
	}
}
