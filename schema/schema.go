package schema

import _ "embed"

// Asset is the schema used by the shared validator.
//
//go:embed asset.schema.json
var Asset string
