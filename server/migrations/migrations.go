// Package migrations embeds the hub's schema, applied in order by goose.
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS
