package main

import (
	"testing"

	"quantram/internal/config"
)

func TestNewPersistenceLeavesMongoDisabled(t *testing.T) {
	store, snapshots, err := newPersistence(config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if store != nil || snapshots != nil {
		t.Fatalf("disabled persistence returned store=%v snapshots=%v", store, snapshots)
	}
}
