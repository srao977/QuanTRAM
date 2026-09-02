package semantics

import (
	"strings"
	"testing"
)

func TestLoadEmbedded(t *testing.T) {
	dict, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	if dict.Version() != ContractVersion {
		t.Fatalf("version %q want %q", dict.Version(), ContractVersion)
	}
	if dict.Contract().Status != "INITIAL_CANONICAL_BASELINE" {
		t.Fatalf("status %q", dict.Contract().Status)
	}
	if dict.Contract().PersistencePolicy != "PERSIST_CANONICAL_RENDER_PRESENTATION" {
		t.Fatalf("persistence %q", dict.Contract().PersistencePolicy)
	}
	if dict.TermCount() == 0 {
		t.Fatal("no terms")
	}
	seen := map[string]struct{}{}
	for _, term := range dict.List("", "") {
		if _, ok := seen[term.ID]; ok {
			t.Fatalf("duplicate id %s", term.ID)
		}
		seen[term.ID] = struct{}{}
		if term.Term == "" || term.Display == "" || term.PlainMeaning == "" || term.ScientificMeaning == "" {
			t.Fatalf("%s missing required fields", term.ID)
		}
		if term.UI.Tooltip == "" || term.UI.PopoverTitle == "" || term.UI.PopoverBody == "" {
			t.Fatalf("%s missing UI text", term.ID)
		}
		for _, rel := range term.RelatedTerms {
			if _, ok := dict.Term(rel); !ok {
				t.Fatalf("%s related %s does not resolve", term.ID, rel)
			}
		}
	}
	hold, ok := dict.Term("ADAPTIVE_HOLD")
	if !ok || hold.Term != "HOLD" || hold.Component != "ADAPTIVE_EMITTER" {
		t.Fatalf("ADAPTIVE_HOLD %+v", hold)
	}
	holding, ok := dict.Term("PRICE_HOLDING")
	if !ok || holding.Component != "VIEWER" || !holding.PresentationOnly {
		t.Fatalf("PRICE_HOLDING must stay distinct from ADAPTIVE_HOLD, got %+v", holding)
	}
}

func TestQualityReservedClassification(t *testing.T) {
	dict, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"QUALITY_STALE", "QUALITY_DEGRADED"} {
		term, ok := dict.Term(id)
		if !ok {
			t.Fatalf("missing %s", id)
		}
		if term.LifecycleStatus != "RESERVED" {
			t.Fatalf("%s lifecycle %q want RESERVED", id, term.LifecycleStatus)
		}
		if term.LivePathProven {
			t.Fatalf("%s live path must not be proven", id)
		}
		if strings.HasPrefix(term.ScientificMeaning, "UNRESOLVED") {
			t.Fatalf("%s still UNRESOLVED", id)
		}
	}
}

func TestProjectionFailureIsCanonicalNotRK45Science(t *testing.T) {
	dict, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	term, ok := dict.Term("PRICE_STATUS_PROJECTION_FAILURE")
	if !ok {
		t.Fatal("missing PRICE_STATUS_PROJECTION_FAILURE")
	}
	if term.Term != "PROJECTION_FAILURE" {
		t.Fatalf("canonical term %q", term.Term)
	}
	foundAlias := false
	for _, alias := range term.CompatibilityAliases {
		if alias == "RK45_FAILURE" {
			foundAlias = true
		}
	}
	if !foundAlias {
		t.Fatal("RK45_FAILURE must remain a compatibility alias")
	}
	if strings.Contains(term.ScientificMeaning, "production uses RK45") || strings.Contains(strings.ToLower(term.UI.Tooltip), "ran rk45") {
		t.Fatalf("must not claim RK45 is production science: %s", term.ScientificMeaning)
	}
	if !strings.Contains(term.ScientificMeaning, "EXPM") {
		t.Fatal("scientific meaning must name EXPM")
	}
}

func TestPresentationAliasesResolveCanonicalSources(t *testing.T) {
	dict, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	holding, _ := dict.Term("PRICE_HOLDING")
	if holding.PersistencePolicy != "RENDER_ONLY" {
		t.Fatalf("PRICE_HOLDING persistence %q", holding.PersistencePolicy)
	}
	foundAmber := false
	for _, id := range holding.CanonicalSourceIDs {
		src, ok := dict.Term(id)
		if !ok {
			t.Fatalf("PRICE_HOLDING source %s missing", id)
		}
		if src.PresentationOnly {
			t.Fatalf("canonical source %s must not be presentation-only", id)
		}
		if id == "PRICE_COLOR_AMBER" {
			foundAmber = true
		}
	}
	if !foundAmber {
		t.Fatal("PRICE_HOLDING must resolve to PRICE_COLOR_AMBER")
	}
}

func TestParseRejectsInvalidTypeAndDuplicate(t *testing.T) {
	valid, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	raw := `{
	  "semantic_contract": {"name":"x","version":"9.9","date":"2026-09-02","status":"TEST","persistence_policy":"PERSIST_CANONICAL_RENDER_PRESENTATION"},
	  "terms": [{
	    "id":"BAD","term":"BAD","display":"Bad","type":"NOT_A_TYPE","component":"PRICE_ENGINE",
	    "plain_meaning":"p","scientific_meaning":"s","interpretation":"i",
	    "does_not_mean":[],"related_terms":[],
	    "lifecycle_status":"ACTIVE","presentation_only":false,"canonical_source_ids":[],"compatibility_aliases":[],"live_path_proven":true,"persistence_policy":"PERSIST",
	    "source":{"go_file":"","go_symbol":"","proto_enum_or_field":"","test_reference":""},
	    "ui":{"tooltip":"t","popover_title":"p","popover_body":"b","show_scientific_detail":false}
	  }]
	}`
	if _, err := Parse([]byte(raw)); err == nil || !strings.Contains(err.Error(), "invalid type") {
		t.Fatalf("want invalid type, got %v", err)
	}
	dup := `{
	  "semantic_contract": {"name":"x","version":"9.9","date":"2026-09-02","status":"TEST","persistence_policy":"PERSIST_CANONICAL_RENDER_PRESENTATION"},
	  "terms": [
	    {"id":"A","term":"A","display":"A","type":"STATE","component":"RUNTIME","plain_meaning":"p","scientific_meaning":"s","interpretation":"i","does_not_mean":[],"related_terms":["MISSING"],"lifecycle_status":"ACTIVE","presentation_only":false,"canonical_source_ids":[],"compatibility_aliases":[],"live_path_proven":true,"persistence_policy":"PERSIST","source":{"go_file":"","go_symbol":"","proto_enum_or_field":"","test_reference":""},"ui":{"tooltip":"t","popover_title":"p","popover_body":"b","show_scientific_detail":false}},
	    {"id":"A","term":"A","display":"A","type":"STATE","component":"RUNTIME","plain_meaning":"p","scientific_meaning":"s","interpretation":"i","does_not_mean":[],"related_terms":[],"lifecycle_status":"ACTIVE","presentation_only":false,"canonical_source_ids":[],"compatibility_aliases":[],"live_path_proven":true,"persistence_policy":"PERSIST","source":{"go_file":"","go_symbol":"","proto_enum_or_field":"","test_reference":""},"ui":{"tooltip":"t","popover_title":"p","popover_body":"b","show_scientific_detail":false}}
	  ]
	}`
	if _, err := Parse([]byte(dup)); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("want duplicate, got %v", err)
	}
	missing := `{
	  "semantic_contract": {"name":"x","version":"9.9","date":"2026-09-02","status":"TEST","persistence_policy":"PERSIST_CANONICAL_RENDER_PRESENTATION"},
	  "terms": [{
	    "id":"A","term":"A","display":"A","type":"STATE","component":"RUNTIME",
	    "plain_meaning":"p","scientific_meaning":"s","interpretation":"i",
	    "does_not_mean":[],"related_terms":["MISSING"],
	    "lifecycle_status":"ACTIVE","presentation_only":false,"canonical_source_ids":[],"compatibility_aliases":[],"live_path_proven":true,"persistence_policy":"PERSIST",
	    "source":{"go_file":"","go_symbol":"","proto_enum_or_field":"","test_reference":""},
	    "ui":{"tooltip":"t","popover_title":"p","popover_body":"b","show_scientific_detail":false}
	  }]
	}`
	if _, err := Parse([]byte(missing)); err == nil || !strings.Contains(err.Error(), "does not resolve") {
		t.Fatalf("want related resolve error, got %v", err)
	}
	badComp := `{
	  "semantic_contract": {"name":"x","version":"9.9","date":"2026-09-02","status":"TEST","persistence_policy":"PERSIST_CANONICAL_RENDER_PRESENTATION"},
	  "terms": [{
	    "id":"A","term":"A","display":"A","type":"STATE","component":"NOT_A_COMPONENT",
	    "plain_meaning":"p","scientific_meaning":"s","interpretation":"i",
	    "does_not_mean":[],"related_terms":[],
	    "lifecycle_status":"ACTIVE","presentation_only":false,"canonical_source_ids":[],"compatibility_aliases":[],"live_path_proven":true,"persistence_policy":"PERSIST",
	    "source":{"go_file":"","go_symbol":"","proto_enum_or_field":"","test_reference":""},
	    "ui":{"tooltip":"t","popover_title":"p","popover_body":"b","show_scientific_detail":false}
	  }]
	}`
	if _, err := Parse([]byte(badComp)); err == nil || !strings.Contains(err.Error(), "invalid component") {
		t.Fatalf("want invalid component, got %v", err)
	}
	_ = valid
}

func TestListFilters(t *testing.T) {
	dict, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	price := dict.List("PRICE_ENGINE", "")
	if len(price) == 0 {
		t.Fatal("expected PRICE_ENGINE terms")
	}
	for _, term := range price {
		if term.Component != "PRICE_ENGINE" {
			t.Fatalf("filter leaked %s", term.ID)
		}
	}
	states := dict.List("", "DECISION")
	if len(states) == 0 {
		t.Fatal("expected DECISION terms")
	}
	both := dict.List("ADAPTIVE_EMITTER", "DECISION")
	if len(both) < 3 {
		t.Fatalf("BUY/SELL/HOLD expected, got %d", len(both))
	}
}
