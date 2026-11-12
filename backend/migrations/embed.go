// Package migrations embeds the SQL migration files in this directory so
// cmd/migrate can read them from the compiled binary — no filesystem path
// resolution is needed at runtime, which matters inside the Docker image
// where the working directory differs from a local `go run`.
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS
