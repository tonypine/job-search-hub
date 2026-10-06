package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/buildinfo"
	"github.com/tonypine/job-search-hub/server/migrations"
)

func TestVersionHandlerAnswersTheBuildsVersionCommitAndNewestMigration(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewVersionHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/version", nil))

	var body VersionResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); recorder.Code != http.StatusOK || err != nil {
		t.Fatalf("%d %s", recorder.Code, recorder.Body.String())
	}
	if body.Version != buildinfo.Version() || body.Commit != buildinfo.Commit() || body.NewestMigration != migrations.Newest() {
		t.Fatalf("body = %+v", body)
	}
}
