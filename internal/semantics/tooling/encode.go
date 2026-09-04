package tooling

import (
	"bytes"
	"encoding/json"

	"quantram/internal/semantics"
)

// CanonicalJSONPath is the checked-in runtime publication artifact.
const CanonicalJSONPath = "internal/semantics/data/quantram_semantics_v1.json"

type publishedDocument struct {
	SemanticContract semantics.Contract `json:"semantic_contract"`
	Terms            []publishedTerm    `json:"terms"`
}

// publishedTerm field order is the deterministic V1 encoder contract.
type publishedTerm struct {
	ID                   string           `json:"id"`
	Term                 string           `json:"term"`
	Display              string           `json:"display"`
	Type                 string           `json:"type"`
	Component            string           `json:"component"`
	PlainMeaning         string           `json:"plain_meaning"`
	ScientificMeaning    string           `json:"scientific_meaning"`
	DoesNotMean          []string         `json:"does_not_mean"`
	RelatedTerms         []string         `json:"related_terms"`
	Interpretation       string           `json:"interpretation"`
	CompatibilityAliases []string         `json:"compatibility_aliases"`
	CanonicalSourceIDs   []string         `json:"canonical_source_ids"`
	PresentationOnly     bool             `json:"presentation_only"`
	PersistencePolicy    string           `json:"persistence_policy"`
	LivePathProven       bool             `json:"live_path_proven"`
	LifecycleStatus      string           `json:"lifecycle_status"`
	Source               semantics.Source `json:"source"`
	UI                   semantics.UI     `json:"ui"`
}

// Encode validates and emits deterministic indented JSON with non-nil list
// fields and a trailing newline.
func Encode(doc semantics.Document) ([]byte, error) {
	if err := semantics.Validate(doc); err != nil {
		return nil, err
	}
	out := publishedDocument{
		SemanticContract: doc.SemanticContract,
		Terms:            make([]publishedTerm, 0, len(doc.Terms)),
	}
	for _, t := range doc.Terms {
		out.Terms = append(out.Terms, publishedTerm{
			ID:                   t.ID,
			Term:                 t.Term,
			Display:              t.Display,
			Type:                 t.Type,
			Component:            t.Component,
			PlainMeaning:         t.PlainMeaning,
			ScientificMeaning:    t.ScientificMeaning,
			DoesNotMean:          nonNil(t.DoesNotMean),
			RelatedTerms:         nonNil(t.RelatedTerms),
			Interpretation:       t.Interpretation,
			CompatibilityAliases: nonNil(t.CompatibilityAliases),
			CanonicalSourceIDs:   nonNil(t.CanonicalSourceIDs),
			PresentationOnly:     t.PresentationOnly,
			PersistencePolicy:    t.PersistencePolicy,
			LivePathProven:       t.LivePathProven,
			LifecycleStatus:      t.LifecycleStatus,
			Source:               t.Source,
			UI:                   t.UI,
		})
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	raw = append(raw, '\n')
	return raw, nil
}

// DocumentsEqual compares documents by their canonical encoded form.
func DocumentsEqual(a, b semantics.Document) error {
	left, err := Encode(a)
	if err != nil {
		return err
	}
	right, err := Encode(b)
	if err != nil {
		return err
	}
	if !bytes.Equal(left, right) {
		return errDocumentsDiffer
	}
	return nil
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
