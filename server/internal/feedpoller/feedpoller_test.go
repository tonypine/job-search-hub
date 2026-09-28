package feedpoller_test

import (
	"context"
	"errors"
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
