package api

import (
	"net/http"

	"github.com/tonypine/job-search-hub/server/internal/buildinfo"
	"github.com/tonypine/job-search-hub/server/migrations"
)

// VersionResponse is the server's build, as hub-server --version prints it.
type VersionResponse struct {
	Version         string `json:"version"`
	Commit          string `json:"commit"`
	NewestMigration int64  `json:"newest_migration"`
}

// CurrentVersion is this build's version, commit and newest migration.
func CurrentVersion() VersionResponse {
	return VersionResponse{Version: buildinfo.Version(), Commit: buildinfo.Commit(), NewestMigration: migrations.Newest()}
}

// NewVersionHandler answers with the server's version. It needs no token,
// so a client can tell which hub it reaches before it pairs.
func NewVersionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, CurrentVersion())
	})
}
