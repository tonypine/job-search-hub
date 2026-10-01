package hiringthread

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

var owner = store.Actor{Kind: store.ActorOwner}

// fakeModel answers each comment by the company it names first.
type fakeModel struct {
	answers map[string]string
	asked   []string
}

func (model *fakeModel) CompleteJSON(_ context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error) {
	model.asked = append(model.asked, request.User)
	for company, answer := range model.answers {
		if strings.HasPrefix(request.User, company) {
			return chatcompletions.Answer{Object: json.RawMessage(answer)}, nil
		}
	}
	return chatcompletions.Answer{}, fmt.Errorf("no answer for %q", request.User)
}

func startHackerNews(t *testing.T) string {
	t.Helper()
	routes := http.NewServeMux()
	routes.HandleFunc("GET /api/v1/search_by_date", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tags") != "story,author_whoishiring" {
			http.Error(w, "unexpected tags", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"hits":[
			{"objectID":"200","title":"Ask HN: Who wants to be hired? (October 2026)","created_at":"2026-10-01T15:02:07Z"},
			{"objectID":"201","title":"Ask HN: Who is hiring? (October 2026)","created_at":"2026-10-01T15:02:07Z"},
			{"objectID":"100","title":"Ask HN: Who is hiring? (September 2026)","created_at":"2026-09-01T15:01:17Z"}]}`)
	})
	routes.HandleFunc("GET /api/v1/items/201", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":201,"children":[
			{"id":301,"created_at":"2026-10-01T15:05:00Z","text":"Acme | Senior Frontend Engineer, Office Manager | REMOTE (Americas) | React, TypeScript<p>Apply at acme.example/jobs"},
			{"id":302,"created_at":"2026-10-01T15:06:00Z","text":"Globex | Rust Engineer | ONSITE in Berlin"},
			{"id":303,"created_at":"2026-10-01T15:07:00Z","text":"Initech &amp; Co | We are a remote-first React shop, more soon."},
			{"id":304,"created_at":"2026-10-01T15:08:00Z","text":""}]}`)
	})
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	return server.URL
}

func TestAPassReadsTheThreadsMatchingCommentsIntoJobs(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	if _, err := hub.SaveJobCriteria(ctx, owner, store.JobCriteria{
		Roles: []string{"Frontend Engineer"}, SearchTerms: []string{"react"}, Technologies: []string{"TypeScript"},
		EligibleLocationTerms: []string{"Americas"},
	}); err != nil {
		t.Fatal(err)
	}
	model := &fakeModel{answers: map[string]string{
		"Acme": `{"is_job_posting":true,"company":"Acme","roles":[{"title":"Senior Frontend Engineer","location":"Americas","workplace_type":"Remote"},
			{"title":"Office Manager","location":"New York","workplace_type":"On-site"}]}`,
		"Initech": `{"is_job_posting":false,"company":"","roles":[]}`,
	}}
	reader := NewReader(hub, model)
	reader.AlgoliaBase = startHackerNews(t)

	summary, err := reader.ReadOnce(ctx)
	if err != nil || summary != (PassSummary{Read: 2, LeftOut: 1, Stored: 1}) {
		t.Fatalf("summary = %+v, %v; want Acme and Initech read, Globex left out, and Acme's frontend role stored", summary, err)
	}
	items, _, err := hub.ListJobs(ctx, store.JobFilter{})
	if err != nil || len(items) != 1 {
		t.Fatalf("jobs = %+v, %v", items, err)
	}
	job := items[0].Job
	if job.Source != store.JobSourceHackerNews || job.Title != "Senior Frontend Engineer" || job.URL != "https://news.ycombinator.com/item?id=301" ||
		!strings.Contains(job.Description, "Apply at acme.example/jobs") || job.WorkplaceType != "Remote" {
		t.Errorf("job = %+v", job)
	}
	if !strings.Contains(model.asked[0], "Acme | Senior Frontend Engineer") || strings.Contains(model.asked[0], "<p>") {
		t.Errorf("the model read %q; want the comment as text", model.asked[0])
	}

	if summary, err := reader.ReadOnce(ctx); err != nil || summary != (PassSummary{}) || len(model.asked) != 2 {
		t.Errorf("second pass = %+v, %v, asked %d; want nothing read again", summary, err, len(model.asked))
	}
}
