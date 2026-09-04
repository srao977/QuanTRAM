// Package semantics defines, validates, and serves the canonical QuanTRAM
// semantic contract independently of its authoring and transport adapters.
package semantics

// ContractVersion is the semantic contract version embedded in this binary.
const ContractVersion = "1.0"

var allowedTypes = map[string]struct{}{
	"NOUN": {}, "VERB": {}, "STATE": {}, "MEASURE": {}, "EVENT": {},
	"REASON": {}, "DECISION": {}, "STATUS": {}, "CLASSIFICATION": {},
}

var allowedComponents = map[string]struct{}{
	"INGESTION": {}, "RUNTIME": {}, "D01": {}, "D02": {}, "D04": {},
	"ADAPTIVE_EMITTER": {}, "PRICE_ENGINE": {}, "VOLUME_ENGINE": {},
	"MODEL_HOST": {}, "OPERATIONS": {}, "VIEWER": {},
}

var allowedLifecycle = map[string]struct{}{
	"ACTIVE": {}, "RESERVED": {}, "TEST_ONLY": {}, "LEGACY": {}, "UNREACHABLE": {},
}

var allowedPersistence = map[string]struct{}{
	"PERSIST": {}, "RENDER_ONLY": {},
}

// Contract identifies one published semantic dictionary and its persistence
// policy.
type Contract struct {
	Name              string `json:"name"`
	Version           string `json:"version"`
	Date              string `json:"date"`
	Status            string `json:"status"`
	PersistencePolicy string `json:"persistence_policy"`
}

// Source records implementation and test evidence for a semantic term.
type Source struct {
	GoFile           string `json:"go_file"`
	GoSymbol         string `json:"go_symbol"`
	ProtoEnumOrField string `json:"proto_enum_or_field"`
	TestReference    string `json:"test_reference"`
}

// UI contains presentation text published with a semantic term.
type UI struct {
	Tooltip              string `json:"tooltip"`
	PopoverTitle         string `json:"popover_title"`
	PopoverBody          string `json:"popover_body"`
	ShowScientificDetail bool   `json:"show_scientific_detail"`
}

// Term is one canonical concept, including its exclusions, relationships,
// lifecycle classification, and source evidence.
type Term struct {
	ID                   string   `json:"id"`
	Term                 string   `json:"term"`
	Display              string   `json:"display"`
	Type                 string   `json:"type"`
	Component            string   `json:"component"`
	PlainMeaning         string   `json:"plain_meaning"`
	ScientificMeaning    string   `json:"scientific_meaning"`
	Interpretation       string   `json:"interpretation"`
	DoesNotMean          []string `json:"does_not_mean"`
	RelatedTerms         []string `json:"related_terms"`
	Source               Source   `json:"source"`
	UI                   UI       `json:"ui"`
	LifecycleStatus      string   `json:"lifecycle_status"`
	PresentationOnly     bool     `json:"presentation_only"`
	CanonicalSourceIDs   []string `json:"canonical_source_ids"`
	CompatibilityAliases []string `json:"compatibility_aliases"`
	LivePathProven       bool     `json:"live_path_proven"`
	PersistencePolicy    string   `json:"persistence_policy"`
}

// Document is a complete semantic contract and its ordered terms.
type Document struct {
	SemanticContract Contract `json:"semantic_contract"`
	Terms            []Term   `json:"terms"`
}

// Dictionary is an immutable validated Document with ID lookup indexing.
type Dictionary struct {
	doc  Document
	byID map[string]Term
}

// Document returns the validated semantic document.
func (d *Dictionary) Document() Document {
	if d == nil {
		return Document{}
	}
	return d.doc
}

// Version returns the loaded contract version.
func (d *Dictionary) Version() string {
	if d == nil {
		return ""
	}
	return d.doc.SemanticContract.Version
}

// Contract returns the loaded contract metadata.
func (d *Dictionary) Contract() Contract {
	if d == nil {
		return Contract{}
	}
	return d.doc.SemanticContract
}

// TermCount returns the number of loaded canonical and presentation terms.
func (d *Dictionary) TermCount() int {
	if d == nil {
		return 0
	}
	return len(d.doc.Terms)
}

// Term looks up a term by canonical semantic ID.
func (d *Dictionary) Term(id string) (Term, bool) {
	if d == nil {
		return Term{}, false
	}
	t, ok := d.byID[id]
	return t, ok
}

// List returns terms matching optional exact component and type filters while
// preserving document order.
func (d *Dictionary) List(component, typ string) []Term {
	if d == nil {
		return nil
	}
	out := make([]Term, 0, len(d.doc.Terms))
	for _, t := range d.doc.Terms {
		if component != "" && t.Component != component {
			continue
		}
		if typ != "" && t.Type != typ {
			continue
		}
		out = append(out, t)
	}
	return out
}
