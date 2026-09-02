package semantics

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

type Contract struct {
	Name              string `json:"name"`
	Version           string `json:"version"`
	Date              string `json:"date"`
	Status            string `json:"status"`
	PersistencePolicy string `json:"persistence_policy"`
}

type Source struct {
	GoFile           string `json:"go_file"`
	GoSymbol         string `json:"go_symbol"`
	ProtoEnumOrField string `json:"proto_enum_or_field"`
	TestReference    string `json:"test_reference"`
}

type UI struct {
	Tooltip              string `json:"tooltip"`
	PopoverTitle         string `json:"popover_title"`
	PopoverBody          string `json:"popover_body"`
	ShowScientificDetail bool   `json:"show_scientific_detail"`
}

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

type Document struct {
	SemanticContract Contract `json:"semantic_contract"`
	Terms            []Term   `json:"terms"`
}

type Dictionary struct {
	doc  Document
	byID map[string]Term
}

func (d *Dictionary) Document() Document {
	if d == nil {
		return Document{}
	}
	return d.doc
}

func (d *Dictionary) Version() string {
	if d == nil {
		return ""
	}
	return d.doc.SemanticContract.Version
}

func (d *Dictionary) Contract() Contract {
	if d == nil {
		return Contract{}
	}
	return d.doc.SemanticContract
}

func (d *Dictionary) TermCount() int {
	if d == nil {
		return 0
	}
	return len(d.doc.Terms)
}

func (d *Dictionary) Term(id string) (Term, bool) {
	if d == nil {
		return Term{}, false
	}
	t, ok := d.byID[id]
	return t, ok
}

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
