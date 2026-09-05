package volume

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestA12NoPricingDependency(t *testing.T) {
	t.Log("invariant: internal/volume must not import internal/pricing")
	assertNoProductionImport(t, packageDir(t), `"quantram/internal/pricing"`)
}

func TestA13NoAdaptiveDependency(t *testing.T) {
	t.Log("invariant: internal/volume must not import Adaptive scientific implementation")
	assertNoProductionImport(t, packageDir(t), `"quantram/internal/adaptive"`)
}

func TestA14NoProductionEntityHardcoding(t *testing.T) {
	t.Log("invariant: no production ticker hardcoding in Volume production sources")
	forbidden := []string{"SPY", "AAPL", "MSFT", "NVDA"}
	err := filepath.WalkDir(packageDir(t), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(body)
		for _, tok := range forbidden {
			if strings.Contains(text, `"`+tok+`"`) || strings.Contains(text, `"`+strings.ToLower(tok)+`"`) {
				t.Errorf("%s hardcodes %s", path, tok)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNoImport(t, filepath.Join(repoRoot(t), "internal", "domain"), `"quantram/internal/volume"`, false)
	domainVol := filepath.Join(repoRoot(t), "internal", "domain", "volume.go")
	body, err := os.ReadFile(domainVol)
	if err != nil {
		t.Fatal(err)
	}
	for _, tok := range forbidden {
		if strings.Contains(string(body), `"`+tok+`"`) {
			t.Errorf("domain/volume.go hardcodes %s", tok)
		}
	}
}

func TestA16NoRuntimeIntegration(t *testing.T) {
	t.Log("invariant: modelhost and ingestion must not import internal/volume in Phase A")
	root := repoRoot(t)
	assertNoImport(t, filepath.Join(root, "internal", "modelhost"), `"quantram/internal/volume"`, false)
	assertNoImport(t, filepath.Join(root, "internal", "ingestion"), `"quantram/internal/volume"`, false)
}

func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	return filepath.Clean(filepath.Join(packageDir(t), "..", ".."))
}

func assertNoProductionImport(t *testing.T, dir, spec string) {
	t.Helper()
	assertNoImport(t, dir, spec, true)
}

func assertNoImport(t *testing.T, dir, spec string, skipTests bool) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if skipTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(body), spec) {
			t.Errorf("%s imports %s", path, spec)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
