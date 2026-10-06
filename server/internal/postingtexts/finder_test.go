package postingtexts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

var now = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

var criteria = store.JobCriteria{Roles: []string{"Senior Front-End Engineer"}, ExcludedRoleTerms: []string{"Sales"}}

var fullText = strings.Repeat("Build the web app with React and TypeScript. ", 20)

// fakeHub keeps alert jobs and what the finder records about them.
type fakeHub struct {
	jobs     []store.AlertJobAwaitingText
	used     int
	texts    map[uuid.UUID]string
	reasons  map[uuid.UUID]string
	searched map[uuid.UUID]bool
	failed   map[uuid.UUID]bool
}

func newFakeHub(used int, jobs ...store.AlertJobAwaitingText) *fakeHub {
	return &fakeHub{jobs: jobs, used: used, texts: map[uuid.UUID]string{}, reasons: map[uuid.UUID]string{}, searched: map[uuid.UUID]bool{},
		failed: map[uuid.UUID]bool{}}
}

func (hub *fakeHub) GetJobCriteria(context.Context) (store.SavedJobCriteria, error) {
	return store.SavedJobCriteria{Criteria: criteria}, nil
}

func (hub *fakeHub) ListAlertJobsAwaitingText(context.Context) ([]store.AlertJobAwaitingText, error) {
	return hub.jobs, nil
}

func (hub *fakeHub) CountPostingTextSearchesSince(_ context.Context, since time.Time) (int, error) {
	if !since.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		return 0, fmt.Errorf("counted since %v, want the month's start", since)
	}
	return hub.used, nil
}

func (hub *fakeHub) SavePostingText(_ context.Context, _ store.Actor, jobID uuid.UUID, text, _ string, _ time.Time) error {
	hub.texts[jobID], hub.searched[jobID] = text, true
	return nil
}

func (hub *fakeHub) RecordPostingTextMissing(_ context.Context, jobID uuid.UUID, reason string, searchedAt *time.Time) error {
	hub.reasons[jobID] = reason
	if searchedAt != nil {
		hub.searched[jobID] = true
	}
	return nil
}

func (hub *fakeHub) RecordPostingTextSearchFailed(_ context.Context, jobID uuid.UUID, reason string, _ time.Time) error {
	hub.reasons[jobID], hub.searched[jobID], hub.failed[jobID] = reason, true, true
	return nil
}

// fakeSearch answers every search with its postings, but for the first ones,
// which fail with its failures in turn.
type fakeSearch struct {
	postings []Posting
	failures []error
	queries  []string
}

func (search *fakeSearch) Search(_ context.Context, query, country string) ([]Posting, error) {
	search.queries = append(search.queries, query+" in "+country)
	if len(search.failures) >= len(search.queries) {
		return nil, search.failures[len(search.queries)-1]
	}
	return search.postings, nil
}

func alertJob(title, companyName string, age time.Duration) store.AlertJobAwaitingText {
	return store.AlertJobAwaitingText{
		Job: store.Job{
			ID: uuid.New(), Source: store.JobSourceGlassdoor, Title: title, URL: "https://www.glassdoor.com.br/job-listing/j?jl=1",
			FirstSeenAt: now.Add(-age),
		},
		CompanyName: companyName,
	}
}

func newFinder(hub *fakeHub, search *fakeSearch) *Finder {
	finder := &Finder{hub: hub, monthlySearches: 100, now: func() time.Time { return now }}
	if search != nil {
		finder.search = search
	}
	return finder
}

func TestAnAlertJobNoBoardHadGetsItsTextFromGoogleForJobs(t *testing.T) {
	job := alertJob("Engenheiro de Software Sênior Front-end", "Acme", 2*24*time.Hour)
	hub := newFakeHub(0, job)
	search := &fakeSearch{postings: []Posting{
		{Title: "Engenheiro de Software Sênior Front-end", EmployerName: "Globex", Description: fullText},
		{Title: "Engenheiro de Software Senior Front-End", EmployerName: "Acme do Brasil Ltda", Description: fullText},
	}}

	summary, err := newFinder(hub, search).FindOnce(context.Background())

	if err != nil || summary != (PassSummary{Found: 1}) {
		t.Fatalf("summary %+v, err %v", summary, err)
	}
	if hub.texts[job.Job.ID] != fullText {
		t.Errorf("text = %q, want the posting's", hub.texts[job.Job.ID])
	}
	if len(search.queries) != 1 || search.queries[0] != "Engenheiro de Software Sênior Front-end Acme in br" {
		t.Errorf("queries = %q, want the title and company, in Brazil", search.queries)
	}
}

func TestAnAlertJobWithoutItsTextSaysWhy(t *testing.T) {
	waiting := alertJob("Senior Frontend Engineer", "Acme", time.Hour)
	ruledOut := alertJob("Sales Engineer", "Acme", 2*24*time.Hour)
	searched := alertJob("Senior Frontend Engineer", "Globex", 3*24*time.Hour)
	searched.Searched = true
	failed := alertJob("Senior Frontend Engineer", "Hooli", 3*24*time.Hour)
	failed.Searched, failed.SearchFailed = true, true
	ready := alertJob("Senior Frontend Engineer", "Initech", 2*24*time.Hour)
	ready.Job.Description = "Build the web app."

	for _, test := range []struct {
		name   string
		used   int
		search *fakeSearch
		job    store.AlertJobAwaitingText
		reason string
	}{
		{"its board still has time", 0, &fakeSearch{}, waiting, "the Glassdoor alert gave no text; looking for the posting on its company's board"},
		{"its title rules it out", 0, &fakeSearch{}, ruledOut, "the Glassdoor alert gave no text, and its title rules it out, so its posting wasn't looked for"},
		{"it was searched", 0, &fakeSearch{}, searched, "the Glassdoor alert gave no text, and neither a board found nor Google for Jobs lists the posting"},
		{"its search failed", 0, &fakeSearch{}, failed,
			"the Glassdoor alert gave no text, no board found lists it, and the Google for Jobs search for it failed"},
		{"the search is off", 0, nil, ready,
			"the Glassdoor alert gave only a snippet, no board found lists it, and the Google for Jobs search is off: it needs a JSearch key"},
		{"the month's searches are spent", 100, &fakeSearch{}, ready,
			"the Glassdoor alert gave only a snippet, no board found lists it, and this month's Google for Jobs searches are spent"},
	} {
		t.Run(test.name, func(t *testing.T) {
			hub := newFakeHub(test.used, test.job)

			if _, err := newFinder(hub, test.search).FindOnce(context.Background()); err != nil {
				t.Fatal(err)
			}

			if hub.reasons[test.job.Job.ID] != test.reason || hub.searched[test.job.Job.ID] {
				t.Errorf("reason %q, searched now %v", hub.reasons[test.job.Job.ID], hub.searched[test.job.Job.ID])
			}
			if test.search != nil && len(test.search.queries) > 0 {
				t.Errorf("searched %q; no request was due", test.search.queries)
			}
		})
	}
}

func TestAPostingGoogleForJobsDoesNotListIsSearchedOnce(t *testing.T) {
	job := alertJob("Senior Frontend Engineer", "Acme", 2*24*time.Hour)
	hub := newFakeHub(0, job)
	search := &fakeSearch{postings: []Posting{
		{Title: "Senior Backend Engineer", EmployerName: "Acme", Description: fullText},
		{Title: "Senior Frontend Engineer", EmployerName: "Acme", Description: "Too short to be the posting."},
	}}

	summary, err := newFinder(hub, search).FindOnce(context.Background())

	if err != nil || summary != (PassSummary{NotFound: 1}) || !hub.searched[job.Job.ID] {
		t.Fatalf("summary %+v, searched %v, err %v", summary, hub.searched[job.Job.ID], err)
	}
	if want := "the Glassdoor alert gave no text, and neither a board found nor Google for Jobs lists the posting"; hub.reasons[job.Job.ID] != want {
		t.Errorf("reason = %q", hub.reasons[job.Job.ID])
	}
}

func TestARefusedSearchStopsThePassWithoutCountingIt(t *testing.T) {
	first, second := alertJob("Senior Frontend Engineer", "Acme", 2*24*time.Hour), alertJob("Senior Frontend Engineer", "Globex", 2*24*time.Hour)
	hub := newFakeHub(0, first, second)
	search := &fakeSearch{failures: []error{fmt.Errorf("%w: it answered 429", ErrSearchRefused)}}

	if _, err := newFinder(hub, search).FindOnce(context.Background()); err == nil {
		t.Fatal("the refusal was not reported")
	}
	if len(search.queries) != 1 || hub.searched[first.Job.ID] || hub.reasons[first.Job.ID] != "" {
		t.Errorf("queries %q, searched %v, reason %q", search.queries, hub.searched[first.Job.ID], hub.reasons[first.Job.ID])
	}
}

func TestASearchThatNeverReachesJSearchStopsThePassWithoutCountingIt(t *testing.T) {
	first, second := alertJob("Senior Frontend Engineer", "Acme", 2*24*time.Hour), alertJob("Senior Frontend Engineer", "Globex", 2*24*time.Hour)
	hub := newFakeHub(99, first, second)
	search := &fakeSearch{failures: []error{fmt.Errorf("%w: dial tcp: lookup jsearch.p.rapidapi.com: no such host", ErrSearchUnreachable)}}
	finder := newFinder(hub, search)

	if _, err := finder.FindOnce(context.Background()); !errors.Is(err, ErrSearchUnreachable) {
		t.Fatalf("err = %v, want the unreachable search reported", err)
	}
	if len(search.queries) != 1 || hub.searched[first.Job.ID] || hub.failed[first.Job.ID] || hub.reasons[first.Job.ID] != "" {
		t.Errorf("queries %q, searched %v, failed %v, reason %q", search.queries, hub.searched[first.Job.ID], hub.failed[first.Job.ID], hub.reasons[first.Job.ID])
	}

	// Nothing was recorded, so the month's last search is left for the next pass, which tries the job again.
	summary, err := finder.FindOnce(context.Background())

	if err != nil || summary != (PassSummary{NotFound: 1}) || !hub.searched[first.Job.ID] {
		t.Fatalf("next pass: summary %+v, searched %v, err %v", summary, hub.searched[first.Job.ID], err)
	}
}

func TestAFailedSearchCountsAndThePassGoesOn(t *testing.T) {
	first, second := alertJob("Senior Frontend Engineer", "Acme", 2*24*time.Hour), alertJob("Senior Frontend Engineer", "Globex", 2*24*time.Hour)
	third := alertJob("Senior Frontend Engineer", "Initech", 2*24*time.Hour)
	hub := newFakeHub(98, first, second, third)
	search := &fakeSearch{
		postings: []Posting{{Title: "Senior Frontend Engineer", EmployerName: "Globex", Description: fullText}},
		failures: []error{errors.New("JSearch answered 500")},
	}

	summary, err := newFinder(hub, search).FindOnce(context.Background())

	if err != nil || summary != (PassSummary{Found: 1, Failed: 1}) {
		t.Fatalf("summary %+v, err %v", summary, err)
	}
	if !hub.searched[first.Job.ID] || !hub.failed[first.Job.ID] ||
		hub.reasons[first.Job.ID] != "the Glassdoor alert gave no text, no board found lists it, and the Google for Jobs search for it failed" {
		t.Errorf("first: searched %v, failed %v, reason %q", hub.searched[first.Job.ID], hub.failed[first.Job.ID], hub.reasons[first.Job.ID])
	}
	if hub.texts[second.Job.ID] != fullText {
		t.Errorf("second's text = %q, want the posting's", hub.texts[second.Job.ID])
	}
	if len(search.queries) != 2 || hub.searched[third.Job.ID] ||
		!strings.Contains(hub.reasons[third.Job.ID], "this month's Google for Jobs searches are spent") {
		t.Errorf("queries %q, third's reason %q; the failed search should count toward the month's", search.queries, hub.reasons[third.Job.ID])
	}
}

func TestACanceledSearchStopsThePassWithoutCountingIt(t *testing.T) {
	job := alertJob("Senior Frontend Engineer", "Acme", 2*24*time.Hour)
	hub := newFakeHub(0, job)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	search := &fakeSearch{failures: []error{context.Canceled}}

	if _, err := newFinder(hub, search).FindOnce(ctx); err == nil {
		t.Fatal("the canceled search was not reported")
	}
	if hub.searched[job.Job.ID] || hub.reasons[job.Job.ID] != "" {
		t.Errorf("searched %v, reason %q", hub.searched[job.Job.ID], hub.reasons[job.Job.ID])
	}
}

func TestTheMonthsSearchesStopAtItsQuota(t *testing.T) {
	first, second := alertJob("Senior Frontend Engineer", "Acme", 2*24*time.Hour), alertJob("Senior Frontend Engineer", "Globex", 2*24*time.Hour)
	hub := newFakeHub(99, first, second)
	search := &fakeSearch{}

	if _, err := newFinder(hub, search).FindOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(search.queries) != 1 || !hub.searched[first.Job.ID] || hub.searched[second.Job.ID] ||
		!strings.Contains(hub.reasons[second.Job.ID], "this month's Google for Jobs searches are spent") {
		t.Errorf("queries %q, second's reason %q", search.queries, hub.reasons[second.Job.ID])
	}
}

func TestAReasonAlreadyStatedIsNotWrittenAgain(t *testing.T) {
	job := alertJob("Senior Frontend Engineer", "Acme", time.Hour)
	job.Job.TextMissingReason = "the Glassdoor alert gave no text; looking for the posting on its company's board"
	hub := newFakeHub(0, job)

	if _, err := newFinder(hub, nil).FindOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, written := hub.reasons[job.Job.ID]; written {
		t.Error("the same reason was written again")
	}
}

func TestTheCountryComesFromThePostingsSite(t *testing.T) {
	for postingURL, want := range map[string]string{
		"https://www.glassdoor.com.br/job-listing/j?jl=1": "br",
		"https://br.indeed.com/viewjob?jk=1":              "br",
		"https://uk.indeed.com/viewjob?jk=1":              "gb",
		"https://www.linkedin.com/jobs/view/1/":           "",
	} {
		if got := getCountry(postingURL); got != want {
			t.Errorf("%s: %q, want %q", postingURL, got, want)
		}
	}
}
