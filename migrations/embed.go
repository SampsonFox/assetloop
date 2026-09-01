package migrations

import "embed"

// FS contains the matching SQLite and PostgreSQL forward migrations.
//
//go:embed sqlite/*.sql postgres/*.sql
var FS embed.FS
