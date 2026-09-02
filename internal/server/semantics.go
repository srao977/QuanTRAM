package server

import (
	"context"

	quantramv1 "quantram/gen/quantram/v1"
	"quantram/internal/semantics"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) SetSemantics(dict *semantics.Dictionary) {
	s.semantics = dict
}

func (s *Server) GetTerm(_ context.Context, req *quantramv1.GetSemanticTermRequest) (*quantramv1.SemanticTerm, error) {
	dict, err := s.requireSemantics()
	if err != nil {
		return nil, err
	}
	id := req.GetId()
	term, ok := dict.Term(id)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "unknown semantic id %s", id)
	}
	return toProtoSemanticTerm(term, dict.Version()), nil
}

func (s *Server) ListTerms(_ context.Context, req *quantramv1.ListSemanticTermsRequest) (*quantramv1.ListSemanticTermsResponse, error) {
	dict, err := s.requireSemantics()
	if err != nil {
		return nil, err
	}
	terms := dict.List(req.GetComponent(), req.GetType())
	out := make([]*quantramv1.SemanticTerm, 0, len(terms))
	for _, term := range terms {
		out = append(out, toProtoSemanticTerm(term, dict.Version()))
	}
	return &quantramv1.ListSemanticTermsResponse{
		Contract: toProtoSemanticContract(dict),
		Terms:    out,
	}, nil
}

func (s *Server) GetSemanticContract(context.Context, *quantramv1.GetSemanticContractRequest) (*quantramv1.SemanticContractInfo, error) {
	dict, err := s.requireSemantics()
	if err != nil {
		return nil, err
	}
	return toProtoSemanticContract(dict), nil
}

func (s *Server) requireSemantics() (*semantics.Dictionary, error) {
	if s.semantics == nil {
		return nil, status.Error(codes.Unavailable, "semantic dictionary unavailable")
	}
	return s.semantics, nil
}

func toProtoSemanticContract(dict *semantics.Dictionary) *quantramv1.SemanticContractInfo {
	c := dict.Contract()
	return &quantramv1.SemanticContractInfo{
		Name:              c.Name,
		Version:           c.Version,
		Date:              c.Date,
		Status:            c.Status,
		TermCount:         uint32(dict.TermCount()),
		PersistencePolicy: c.PersistencePolicy,
	}
}

func toProtoSemanticTerm(term semantics.Term, version string) *quantramv1.SemanticTerm {
	return &quantramv1.SemanticTerm{
		Id:                      term.ID,
		Term:                    term.Term,
		Display:                 term.Display,
		Type:                    term.Type,
		Component:               term.Component,
		PlainMeaning:            term.PlainMeaning,
		ScientificMeaning:       term.ScientificMeaning,
		Interpretation:          term.Interpretation,
		DoesNotMean:             append([]string(nil), term.DoesNotMean...),
		RelatedTerms:            append([]string(nil), term.RelatedTerms...),
		Tooltip:                 term.UI.Tooltip,
		PopoverTitle:            term.UI.PopoverTitle,
		PopoverBody:             term.UI.PopoverBody,
		ShowScientificDetail:    term.UI.ShowScientificDetail,
		SemanticContractVersion: version,
		GoSymbol:                term.Source.GoSymbol,
		ProtoEnumOrField:        term.Source.ProtoEnumOrField,
		PresentationOnly:        term.PresentationOnly,
		LifecycleStatus:         term.LifecycleStatus,
		CanonicalSourceIds:      append([]string(nil), term.CanonicalSourceIDs...),
		CompatibilityAliases:    append([]string(nil), term.CompatibilityAliases...),
		LivePathProven:          term.LivePathProven,
		PersistencePolicy:       term.PersistencePolicy,
	}
}
