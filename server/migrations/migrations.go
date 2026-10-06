// Package migrations embeds the hub's schema, applied in order by goose.
package migrations

import (
	"embed"
	"io/fs"
	"strconv"
	"strings"
)

//go:embed *.sql
var Files embed.FS

// Newest is the version of the newest migration embedded, the one a database
// this build migrates ends at.
func Newest() int64 {
	names, err := fs.Glob(Files, "*.sql")
	if err != nil {
		panic(err)
	}
	var newest int64
	for _, name := range names {
		number, _, _ := strings.Cut(name, "_")
		version, err := strconv.ParseInt(number, 10, 64)
		if err != nil {
			panic("migration " + name + " doesn't start with its version")
		}
		newest = max(newest, version)
	}
	return newest
}
