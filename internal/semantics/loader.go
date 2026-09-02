package semantics

import (
	"encoding/json"
	"fmt"
	"os"
)

func LoadEmbedded() (*Dictionary, error) {
	return Parse(embeddedJSON)
}

func LoadFile(path string) (*Dictionary, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

func Parse(raw []byte) (*Dictionary, error) {
	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("semantics: parse: %w", err)
	}
	if err := validate(doc); err != nil {
		return nil, err
	}
	byID := make(map[string]Term, len(doc.Terms))
	for _, t := range doc.Terms {
		byID[t.ID] = t
	}
	return &Dictionary{doc: doc, byID: byID}, nil
}
