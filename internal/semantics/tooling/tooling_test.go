package tooling

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quantram/internal/semantics"
)

func TestCatalogMatchesBaselineSemantically(t *testing.T) {
	baseline, err := semantics.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Version() != "1.0" || baseline.TermCount() != 107 {
		t.Fatalf("baseline version=%s count=%d", baseline.Version(), baseline.TermCount())
	}
	got := Catalog()
	if err := semantics.Validate(got); err != nil {
		t.Fatal(err)
	}
	if got.SemanticContract.Version != "1.0" || len(got.Terms) != 107 {
		t.Fatalf("catalog version=%s count=%d", got.SemanticContract.Version, len(got.Terms))
	}
	if err := semanticEqual(baseline.Document(), got); err != nil {
		t.Fatal(err)
	}
}

func TestDeterministicBuild(t *testing.T) {
	a, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("build is not deterministic")
	}
	sum := sha256.Sum256(a)
	t.Log("built sha256", hex.EncodeToString(sum[:]))
}

func TestBuildCheckMatchesCanonicalAfterEncode(t *testing.T) {
	built, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	checked, err := os.ReadFile(absJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(built, checked) {
		t.Fatalf("generated JSON differs from checked-in contract; run go run ./cmd/quantram-semantics build")
	}
}

func TestValidateRejectsInvalidCatalogShape(t *testing.T) {
	doc := Catalog()
	doc.Terms[0].ID = doc.Terms[1].ID
	if err := semantics.Validate(doc); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("want duplicate, got %v", err)
	}
	doc = Catalog()
	doc.Terms[0].RelatedTerms = []string{"NOT_A_REAL_RELATED"}
	if err := semantics.Validate(doc); err == nil || !strings.Contains(err.Error(), "does not resolve") {
		t.Fatalf("want related resolve, got %v", err)
	}
	doc = Catalog()
	doc.Terms[0].Component = "NOT_A_COMPONENT"
	if err := semantics.Validate(doc); err == nil || !strings.Contains(err.Error(), "invalid component") {
		t.Fatalf("want component, got %v", err)
	}
	doc = Catalog()
	doc.Terms[0].Type = "NOT_A_TYPE"
	if err := semantics.Validate(doc); err == nil || !strings.Contains(err.Error(), "invalid type") {
		t.Fatalf("want type, got %v", err)
	}
	doc = Catalog()
	for i := range doc.Terms {
		if doc.Terms[i].PresentationOnly {
			doc.Terms[i].CanonicalSourceIDs = nil
			if err := semantics.Validate(doc); err == nil {
				t.Fatal("presentation alias must require canonical_source_ids")
			}
			return
		}
	}
	t.Fatal("expected a presentation alias")
}

func TestAuditSyntheticMissingAndOrphan(t *testing.T) {
	root := repoRoot(t)
	report, err := Audit(root, []Candidate{{Token: "SYNTHETIC_MISSING_TOKEN_XYZ", Source: "testdata"}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range report.ByKind(FindingMissing) {
		if f.Token == "SYNTHETIC_MISSING_TOKEN_XYZ" {
			found = true
		}
	}
	if !found {
		t.Fatal("audit must report synthetic missing token")
	}

	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "ghost.go"), []byte("package ghost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := Catalog()
	doc.Terms = append([]semantics.Term(nil), doc.Terms...)
	doc.Terms[0].Source.GoFile = "ghost.go"
	doc.Terms[0].Source.GoSymbol = "DoesNotExist"
	// Use a tiny isolated audit of orphan via file miss on catalog source.
	// Real catalog sources are checked against this repo; a missing file is orphaned.
	report, err = Audit(tmp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.ByKind(FindingOrphaned)) == 0 {
		t.Fatal("audit must report orphaned sources when go_file is absent")
	}
}

func semanticEqual(a, b semantics.Document) error {
	if a.SemanticContract != b.SemanticContract {
		return errDocumentsDiffer
	}
	if len(a.Terms) != len(b.Terms) {
		return errDocumentsDiffer
	}
	index := map[string]semantics.Term{}
	for _, t := range b.Terms {
		index[t.ID] = t
	}
	for _, left := range a.Terms {
		right, ok := index[left.ID]
		if !ok {
			return errDocumentsDiffer
		}
		lb, _ := json.Marshal(normalize(left))
		rb, _ := json.Marshal(normalize(right))
		if !bytes.Equal(lb, rb) {
			return errDocumentsDiffer
		}
	}
	return nil
}

func normalize(t semantics.Term) semantics.Term {
	if t.DoesNotMean == nil {
		t.DoesNotMean = []string{}
	}
	if t.RelatedTerms == nil {
		t.RelatedTerms = []string{}
	}
	if t.CompatibilityAliases == nil {
		t.CompatibilityAliases = []string{}
	}
	if t.CanonicalSourceIDs == nil {
		t.CanonicalSourceIDs = []string{}
	}
	return t
}

func absJSON(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), CanonicalJSONPath)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
