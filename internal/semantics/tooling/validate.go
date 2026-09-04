package tooling

import (
	"os"

	"quantram/internal/semantics"
)

// ValidatePath validates a semantic JSON artifact at path.
func ValidatePath(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = semantics.Parse(raw)
	return err
}

// ValidateCatalog validates the curated authoring document.
func ValidateCatalog() error {
	return semantics.Validate(Catalog())
}
