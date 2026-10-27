// Package migrations holds the SQL schema, applied in order by goose.
package migrations

import "embed"

// FS is every migration, embedded so the binary can migrate a database with
// nothing beside it.
//
//go:embed *.sql
var FS embed.FS
