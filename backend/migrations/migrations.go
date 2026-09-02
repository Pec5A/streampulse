// Package migrations embeds the raw SQL migration files so the API binary
// can apply them itself at startup — no separate migrate CLI or manual
// psql step, which was the missing piece keeping `docker compose up` from
// actually being a working deploy (see internal/infrastructure/persistence
// for the runner).
package migrations

import "embed"

//go:embed *.up.sql
var FS embed.FS
