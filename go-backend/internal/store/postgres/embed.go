package postgres

import _ "embed"

//go:embed sql/schema.sql
var EmbeddedSchema string

//go:embed sql/data.sql
var EmbeddedSeedData string
