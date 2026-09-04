// Package tooling builds, validates, and audits the canonical semantic
// contract and its repository evidence.
package tooling

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"quantram/internal/semantics"
)

// FindingKind classifies the relationship between an observed token and the
// semantic catalog.
type FindingKind string

const (
	// FindingKnown identifies an unambiguous cataloged token.
	FindingKnown FindingKind = "KNOWN"
	// FindingMissing identifies an observed token with no exact catalog term.
	FindingMissing FindingKind = "MISSING"
	// FindingOrphaned identifies catalog source evidence absent from the repo.
	FindingOrphaned FindingKind = "ORPHANED"
	// FindingPresentationOnly identifies a render-only catalog alias.
	FindingPresentationOnly FindingKind = "PRESENTATION_ONLY"
	// FindingReview identifies an English token shared by multiple semantic IDs.
	FindingReview FindingKind = "REVIEW"
)

// Candidate is one executable, proto, viewer, or caller-supplied audit token.
type Candidate struct {
	Token  string
	Source string
}

// Finding records one classified token or source-evidence observation.
type Finding struct {
	Kind   FindingKind
	ID     string
	Token  string
	Source string
	Detail string
}

// AuditReport is the deterministically ordered set of semantic audit findings.
type AuditReport struct {
	Findings []Finding
}

// ByKind returns findings with the requested classification.
func (r AuditReport) ByKind(kind FindingKind) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Kind == kind {
			out = append(out, f)
		}
	}
	return out
}

var constString = regexp.MustCompile(`=\s+"([A-Za-z][A-Za-z0-9_ ]*)"`)
var protoEnum = regexp.MustCompile(`^\s+([A-Z][A-Z0-9_]+) = [0-9]+;`)

// Audit classifies discovered and supplied tokens, then verifies each local
// catalog source file and symbol. It reports evidence gaps without modifying
// the catalog or source tree.
func Audit(repoRoot string, extra []Candidate) (AuditReport, error) {
	doc := Catalog()
	if err := semantics.Validate(doc); err != nil {
		return AuditReport{}, err
	}
	byToken := map[string][]semantics.Term{}
	byID := map[string]semantics.Term{}
	for _, t := range doc.Terms {
		byID[t.ID] = t
		byToken[t.Term] = append(byToken[t.Term], t)
		for _, alias := range t.CompatibilityAliases {
			byToken[alias] = append(byToken[alias], t)
		}
	}

	cands, err := collectCandidates(repoRoot)
	if err != nil {
		return AuditReport{}, err
	}
	cands = append(cands, extra...)

	var report AuditReport
	seenToken := map[string]struct{}{}
	for _, c := range cands {
		if ignoreAuditToken(c.Token) {
			continue
		}
		key := c.Token + "|" + c.Source
		if _, ok := seenToken[key]; ok {
			continue
		}
		seenToken[key] = struct{}{}
		if t, ok := byID[c.Token]; ok {
			kind := FindingKnown
			if t.PresentationOnly {
				kind = FindingPresentationOnly
			}
			report.Findings = append(report.Findings, Finding{
				Kind:   kind,
				ID:     t.ID,
				Token:  c.Token,
				Source: c.Source,
			})
			continue
		}
		matches := byToken[c.Token]
		if len(matches) == 0 {
			// A missing finding concerns observed vocabulary, not a broken source
			// reference from an existing catalog term.
			report.Findings = append(report.Findings, Finding{
				Kind:   FindingMissing,
				Token:  c.Token,
				Source: c.Source,
				Detail: "executable or viewer token has no semantic term with this exact name",
			})
			continue
		}
		kind := FindingKnown
		id := matches[0].ID
		if matches[0].PresentationOnly {
			kind = FindingPresentationOnly
		}
		if len(matches) > 1 {
			// Shared display vocabulary is not guessed: callers must review the
			// distinct semantic identities.
			kind = FindingReview
			ids := make([]string, 0, len(matches))
			for _, m := range matches {
				ids = append(ids, m.ID)
			}
			report.Findings = append(report.Findings, Finding{
				Kind:   kind,
				ID:     strings.Join(ids, ","),
				Token:  c.Token,
				Source: c.Source,
				Detail: "same English token maps to multiple semantic IDs",
			})
			continue
		}
		report.Findings = append(report.Findings, Finding{
			Kind:   kind,
			ID:     id,
			Token:  c.Token,
			Source: c.Source,
		})
	}

	for _, t := range doc.Terms {
		if t.Source.GoFile == "" || t.Source.GoSymbol == "" {
			continue
		}
		if strings.HasPrefix(t.Source.GoFile, "quantram-dashboard/") {
			continue
		}
		path := filepath.Join(repoRoot, filepath.FromSlash(t.Source.GoFile))
		body, err := os.ReadFile(path)
		if err != nil {
			// Orphaned findings run in the opposite direction from missing ones:
			// the catalog term exists, but its claimed implementation does not.
			report.Findings = append(report.Findings, Finding{
				Kind:   FindingOrphaned,
				ID:     t.ID,
				Token:  t.Term,
				Source: t.Source.GoFile,
				Detail: "go_file not found",
			})
			continue
		}
		symbol := t.Source.GoSymbol
		if i := strings.LastIndex(symbol, "."); i >= 0 {
			symbol = symbol[i+1:]
		}
		if symbol == "" || !strings.Contains(string(body), symbol) {
			report.Findings = append(report.Findings, Finding{
				Kind:   FindingOrphaned,
				ID:     t.ID,
				Token:  t.Term,
				Source: t.Source.GoFile + ":" + t.Source.GoSymbol,
				Detail: "go_symbol not found in go_file",
			})
		}
	}

	sort.Slice(report.Findings, func(i, j int) bool {
		if report.Findings[i].Kind != report.Findings[j].Kind {
			return report.Findings[i].Kind < report.Findings[j].Kind
		}
		return report.Findings[i].Token < report.Findings[j].Token
	})
	return report, nil
}

func collectCandidates(repoRoot string) ([]Candidate, error) {
	var out []Candidate
	files := []string{
		"internal/domain/bar.go",
		"internal/domain/decision.go",
		"internal/domain/price.go",
		"internal/domain/health.go",
		"internal/domain/continuity.go",
		"internal/domain/quality.go",
		"api/proto/quantram/v1/quantram.proto",
	}
	for _, rel := range files {
		path := filepath.Join(repoRoot, filepath.FromSlash(rel))
		body, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		text := string(body)
		if strings.HasSuffix(rel, ".proto") {
			for _, line := range strings.Split(text, "\n") {
				m := protoEnum.FindStringSubmatch(line)
				if m == nil || strings.Contains(m[1], "UNSPECIFIED") {
					continue
				}
				out = append(out, Candidate{Token: protoTail(m[1]), Source: rel})
			}
			continue
		}
		for _, m := range constString.FindAllStringSubmatch(text, -1) {
			out = append(out, Candidate{Token: m[1], Source: rel})
		}
	}
	for _, id := range viewerSemanticIDs() {
		out = append(out, Candidate{Token: id, Source: "quantram-dashboard/src/lib/semantics.ts"})
	}
	return out, nil
}

func protoTail(name string) string {
	prefixes := []string{
		"SKIP_REASON_", "PRICING_SKIP_REASON_", "PRICING_STATUS_",
		"QUALITY_STATUS_", "FEED_STATE_", "COMPONENT_STATE_",
		"PATH_DIRECTION_", "EMITTER_POSITION_", "MODEL_STATUS_",
		"INSTRUMENT_TYPE_", "SIDE_",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return strings.TrimPrefix(name, prefix)
		}
	}
	return name
}

func ignoreAuditToken(token string) bool {
	switch token {
	case "", "STOCK", "ETF", "INDEX", "1Min", "ALPACA_IEX", "ALPACA_TEST":
		return true
	}
	if strings.Contains(token, "UNSPECIFIED") {
		return true
	}
	return false
}

func viewerSemanticIDs() []string {
	return []string{
		"ADAPTIVE_BUY", "ADAPTIVE_SELL", "ADAPTIVE_HOLD",
		"ADAPTIVE_LONG", "ADAPTIVE_SHORT", "ADAPTIVE_FLAT",
		"PRICE_HOLDING", "PRICE_ON_TIME", "PRICE_DELAYED",
		"VIEWER_ADAPTIVE_HOLDING", "VIEWER_PRICE_HOLDING",
	}
}
