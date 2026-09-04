package semantics

import _ "embed"

// embeddedJSON is the generated publication artifact consumed at runtime; the
// catalog package remains the authoring source.
//
//go:embed data/quantram_semantics_v1.json
var embeddedJSON []byte
