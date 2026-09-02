package tooling

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"quantram/internal/semantics"
	"quantram/internal/semantics/catalog"
)

var errDocumentsDiffer = errors.New("semantics: generated JSON differs from checked-in contract")

func Catalog() semantics.Document {
	return catalog.V1()
}

func Build() ([]byte, error) {
	return Encode(Catalog())
}

func WriteCanonical(path string) error {
	raw, err := Build()
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func CheckCanonical(path string) error {
	want, err := Build()
	if err != nil {
		return err
	}
	got, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(want, got) {
		return fmt.Errorf("%w: %s", errDocumentsDiffer, path)
	}
	return nil
}
