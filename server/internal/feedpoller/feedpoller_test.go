package feedpoller_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/feedpoller"
	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

type fakeFeed struct {
	results  map[string][]string
	searches []jobboards.HimalayasSearch
	remoteOK []store.JobPosting
	tags     []string
}

func (feed *fakeFeed) FetchRemoteOKPostings(_ context.Context, tag string) ([]store.JobPosting, error) {
	feed.tags = append(feed.tags, tag)
	return feed.remoteOK, nil
}

func (feed *fakeFeed) SearchHimalayas(_ context.Context, search jobboards.HimalayasSearch) ([]store.JobPosting, error) {
	feed.searches = append(feed.searches, search)
	ids, found := feed.results[search.Term]
	if !found {
		return nil, errors.New("himalayas answered 500")
	}
	var postings []store.JobPosting
	for _, id := range ids {
		postings = append(postings, store.JobPosting{ExternalID: id, CompanyName: "Acme", Title: "Engineer " + id, URL: "https://himalayas.app/j/" + id})
	}
	return postings, nil
}

func TestAPollSearchesEachTermAndStoresEachPostingOnce(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	if _, err := hub.SaveJobCriteria(ctx, store.Actor{Kind: store.ActorOwner}, store.JobCriteria{
		SearchTerms: []string{"react", "typescript", "broken"}, HomeCountry: "Brazil",
	}); err != nil {
		t.Fatal(err)
	}
	feed := &fakeFeed{results: map[string][]string{"react": {"1", "2"}, "typescript": {"2", "3"}}}

	result, err := feedpoller.New(hub, feed).PollOnce(ctx)
	if err != nil || result.Created != 3 || result.Seen != 3 {
		t.Fatalf("result = %+v, %v; want three postings, the failed search skipped", result, err)
	}
	if len(feed.searches) != 3 || feed.searches[0].Country != "Brazil" || feed.searches[0].MaximumPages == 0 ||
		time.Since(feed.searches[0].PublishedAfter) < 13*24*time.Hour {
		t.Fatalf("searches = %+v", feed.searches)
	}
}

func TestAPollWithoutSearchTermsSearchesNothing(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	feed := &fakeFeed{}

	if _, err := feedpoller.New(hub, feed).PollOnce(context.Background()); !errors.Is(err, feedpoller.ErrNoSearchCriteria) || len(feed.searches) != 0 {
		t.Fatalf("err = %v, %d searches", err, len(feed.searches))
	}
}

func TestARemoteOKPollStoresThePostingsThatCouldFit(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	if _, err := hub.SaveJobCriteria(ctx, store.Actor{Kind: store.ActorOwner}, store.JobCriteria{
		Roles: []string{"Frontend Engineer"}, ExcludedRoleTerms: []string{"Sales"}, IneligibleLocationTerms: []string{"US only"},
		SearchTerms: []string{"react", "Full Stack"},
	}); err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	posting := func(id, title, location string) store.JobPosting {
		return store.JobPosting{ExternalID: id, CompanyName: "Acme", Title: title, Location: location, WorkplaceType: "Remote",
			URL: "https://remoteok.com/remote-jobs/" + id, Description: "Build.", ExpiresAt: &expiresAt}
	}
	old := posting("4", "Frontend Engineer", "")
	publishedAt := time.Now().Add(-40 * 24 * time.Hour)
	old.PublishedAt = &publishedAt
	feed := &fakeFeed{remoteOK: []store.JobPosting{
		posting("1", "Senior Frontend Engineer", ""), posting("2", "Sales Representative", ""), posting("3", "Frontend Engineer", "US only"), old,
	}}

	result, err := feedpoller.New(hub, feed).PollRemoteOK(ctx)
	if err != nil || result.Created != 1 || result.Closed != 0 || result.Dropped != 3 {
		t.Fatalf("result = %+v, %v; want the recent frontend role stored and the other three dropped", result, err)
	}
	if fmt.Sprint(feed.tags) != "[react full-stack]" {
		t.Errorf("tags = %v", feed.tags)
	}
	items, _, _ := hub.ListJobs(ctx, store.JobFilter{})
	if len(items) != 1 || items[0].Job.Source != store.JobSourceRemoteOK {
		t.Fatalf("jobs = %+v", items)
	}
}
