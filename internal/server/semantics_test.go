package server

import (
	"context"
	"testing"

	quantramv1 "quantram/gen/quantram/v1"
	"quantram/internal/ingestion"
	"quantram/internal/semantics"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSemanticServiceUnavailableWithoutDictionary(t *testing.T) {
	s := New(ingestion.NewPipeline(nil, nil, "TEST", []string{"AAPL"}), nil)
	_, err := s.GetTerm(context.Background(), &quantramv1.GetSemanticTermRequest{Id: "ADAPTIVE_HOLD"})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("want Unavailable, got %v", err)
	}
}

func TestSemanticServiceGetListContract(t *testing.T) {
	dict, err := semantics.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	s := New(ingestion.NewPipeline(nil, nil, "TEST", []string{"AAPL"}), nil)
	s.SetSemantics(dict)

	term, err := s.GetTerm(context.Background(), &quantramv1.GetSemanticTermRequest{Id: "ADAPTIVE_HOLD"})
	if err != nil {
		t.Fatal(err)
	}
	if term.GetId() != "ADAPTIVE_HOLD" || term.GetTerm() != "HOLD" || term.GetSemanticContractVersion() != "1.0" {
		t.Fatalf("GetTerm %+v", term)
	}
	if term.GetGoSymbol() == "" {
		t.Fatal("expected go_symbol metadata")
	}

	_, err = s.GetTerm(context.Background(), &quantramv1.GetSemanticTermRequest{Id: "NOT_A_REAL_TERM"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("unknown want NotFound, got %v", err)
	}

	listed, err := s.ListTerms(context.Background(), &quantramv1.ListSemanticTermsRequest{
		Component: "PRICE_ENGINE",
		Type:      "STATE",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.GetTerms()) == 0 {
		t.Fatal("expected PRICE_ENGINE STATE terms")
	}
	for _, item := range listed.GetTerms() {
		if item.GetComponent() != "PRICE_ENGINE" || item.GetType() != "STATE" {
			t.Fatalf("filter leaked %+v", item)
		}
	}

	contract, err := s.GetSemanticContract(context.Background(), &quantramv1.GetSemanticContractRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if contract.GetVersion() != "1.0" || contract.GetTermCount() == 0 {
		t.Fatalf("contract %+v", contract)
	}
	if contract.GetTermCount() != listed.GetContract().GetTermCount() && listed.GetContract().GetTermCount() != contract.GetTermCount() {
		// ListTerms returns full contract term_count, not the filtered length.
	}
	if listed.GetContract().GetTermCount() != contract.GetTermCount() {
		t.Fatalf("filtered list must still report full contract count: list=%d contract=%d", listed.GetContract().GetTermCount(), contract.GetTermCount())
	}
}

func TestSemanticQueryDoesNotRequireModelHost(t *testing.T) {
	dict, err := semantics.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	s := New(ingestion.NewPipeline(nil, nil, "TEST", []string{"AAPL"}), nil)
	s.SetSemantics(dict)
	if err := s.StreamDecisions(&quantramv1.StreamDecisionsRequest{}, nil); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("model remains off after semantic load, got %v", err)
	}
	holding, err := s.GetTerm(context.Background(), &quantramv1.GetSemanticTermRequest{Id: "PRICE_HOLDING"})
	if err != nil {
		t.Fatal(err)
	}
	if !holding.GetPresentationOnly() || holding.GetPersistencePolicy() != "RENDER_ONLY" {
		t.Fatalf("PRICE_HOLDING must be presentation-only RENDER_ONLY, got %+v", holding)
	}
	proj, err := s.GetTerm(context.Background(), &quantramv1.GetSemanticTermRequest{Id: "PRICE_STATUS_PROJECTION_FAILURE"})
	if err != nil {
		t.Fatal(err)
	}
	if proj.GetTerm() != "PROJECTION_FAILURE" {
		t.Fatalf("canonical projection term %q", proj.GetTerm())
	}
	stale, err := s.GetTerm(context.Background(), &quantramv1.GetSemanticTermRequest{Id: "QUALITY_STALE"})
	if err != nil {
		t.Fatal(err)
	}
	if stale.GetLifecycleStatus() != "RESERVED" || stale.GetLivePathProven() {
		t.Fatalf("QUALITY_STALE %+v", stale)
	}
	contract, err := s.GetSemanticContract(context.Background(), &quantramv1.GetSemanticContractRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if contract.GetPersistencePolicy() != "PERSIST_CANONICAL_RENDER_PRESENTATION" {
		t.Fatalf("contract policy %q", contract.GetPersistencePolicy())
	}
}
