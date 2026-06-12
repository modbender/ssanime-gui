// Package migrations embeds the goose SQL migration files so the binary can run
// them on boot without shipping the .sql files alongside.
package migrations

import "embed"

// FS holds the embedded goose migration files.
//
//go:embed *.sql
var FS embed.FS
