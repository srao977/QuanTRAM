package semantics

import (
	"fmt"
	"strings"
)

func Validate(doc Document) error {
	return validate(doc)
}

func validate(doc Document) error {
	if strings.TrimSpace(doc.SemanticContract.Name) == "" {
		return fmt.Errorf("semantics: contract name is required")
	}
	if strings.TrimSpace(doc.SemanticContract.Version) == "" {
		return fmt.Errorf("semantics: contract version is required")
	}
	if len(doc.Terms) == 0 {
		return fmt.Errorf("semantics: no terms")
	}
	seen := make(map[string]struct{}, len(doc.Terms))
	for i, t := range doc.Terms {
		if t.ID == "" {
			return fmt.Errorf("semantics: term %d missing id", i)
		}
		if _, ok := seen[t.ID]; ok {
			return fmt.Errorf("semantics: duplicate id %s", t.ID)
		}
		seen[t.ID] = struct{}{}
		if t.Term == "" || t.Display == "" || t.PlainMeaning == "" || t.ScientificMeaning == "" {
			return fmt.Errorf("semantics: %s missing required meaning fields", t.ID)
		}
		if t.UI.Tooltip == "" || t.UI.PopoverTitle == "" || t.UI.PopoverBody == "" {
			return fmt.Errorf("semantics: %s missing UI text", t.ID)
		}
		if _, ok := allowedTypes[t.Type]; !ok {
			return fmt.Errorf("semantics: %s invalid type %q", t.ID, t.Type)
		}
		if _, ok := allowedComponents[t.Component]; !ok {
			return fmt.Errorf("semantics: %s invalid component %q", t.ID, t.Component)
		}
		if _, ok := allowedLifecycle[t.LifecycleStatus]; !ok {
			return fmt.Errorf("semantics: %s invalid lifecycle_status %q", t.ID, t.LifecycleStatus)
		}
		if _, ok := allowedPersistence[t.PersistencePolicy]; !ok {
			return fmt.Errorf("semantics: %s invalid persistence_policy %q", t.ID, t.PersistencePolicy)
		}
		if strings.HasPrefix(t.ScientificMeaning, "UNRESOLVED") {
			return fmt.Errorf("semantics: %s scientific_meaning is UNRESOLVED", t.ID)
		}
		if t.PresentationOnly && len(t.CanonicalSourceIDs) == 0 {
			return fmt.Errorf("semantics: %s presentation alias missing canonical_source_ids", t.ID)
		}
	}
	if doc.SemanticContract.PersistencePolicy != "PERSIST_CANONICAL_RENDER_PRESENTATION" {
		return fmt.Errorf("semantics: contract persistence_policy must be PERSIST_CANONICAL_RENDER_PRESENTATION")
	}
	for _, t := range doc.Terms {
		for _, rel := range t.RelatedTerms {
			if _, ok := seen[rel]; !ok {
				return fmt.Errorf("semantics: %s related term %s does not resolve", t.ID, rel)
			}
		}
		for _, rel := range t.CanonicalSourceIDs {
			if _, ok := seen[rel]; !ok {
				return fmt.Errorf("semantics: %s canonical source %s does not resolve", t.ID, rel)
			}
		}
	}
	return nil
}
