package jobboards

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// PostingReference names one posting on one provider's board.
type PostingReference struct {
	Provider   string
	BoardToken string
	PostingID  string
}

// ParsePostingURL recognizes a posting's public URL on Greenhouse, Lever or
// Ashby. ok is false for any other URL.
func ParsePostingURL(raw string) (PostingReference, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return PostingReference{}, false
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	host := strings.ToLower(parsed.Hostname())
	switch {
	case (host == "job-boards.greenhouse.io" || host == "boards.greenhouse.io") && len(segments) == 3 && segments[1] == "jobs":
		return PostingReference{Provider: Greenhouse, BoardToken: segments[0], PostingID: segments[2]}, true
	case host == "jobs.lever.co" && len(segments) >= 2:
		return PostingReference{Provider: Lever, BoardToken: segments[0], PostingID: segments[1]}, true
	case host == "jobs.ashbyhq.com" && len(segments) >= 2:
		return PostingReference{Provider: Ashby, BoardToken: segments[0], PostingID: segments[1]}, true
	default:
		return PostingReference{}, false
	}
}

// FetchPosting reads one posting from its provider's public API. Ashby serves
// no single-posting endpoint, so its board is read and the posting found in it.
func (verifier *Verifier) FetchPosting(ctx context.Context, reference PostingReference) (store.JobPosting, error) {
	escapedToken := url.PathEscape(reference.BoardToken)
	escapedID := url.PathEscape(reference.PostingID)
	switch reference.Provider {
	case Greenhouse:
		return verifier.fetchOnePosting(ctx, Greenhouse, verifier.GreenhouseAPIBase+"/v1/boards/"+escapedToken+"/jobs/"+escapedID, parseGreenhousePosting)
	case Lever:
		return verifier.fetchOnePosting(ctx, Lever, verifier.LeverAPIBase+"/v0/postings/"+escapedToken+"/"+escapedID+"?mode=json", parseLeverPosting)
	case Ashby:
		postings, err := verifier.FetchPostings(ctx, Ashby, reference.BoardToken)
		if err != nil {
			return store.JobPosting{}, err
		}
		for _, posting := range postings {
			if posting.ExternalID == reference.PostingID {
				return posting, nil
			}
		}
		return store.JobPosting{}, fmt.Errorf("ashby board %q has no open posting %q", reference.BoardToken, reference.PostingID)
	default:
		return store.JobPosting{}, ErrUnsupportedProvider
	}
}

func (verifier *Verifier) fetchOnePosting(ctx context.Context, provider, fetchURL string, parse func(json.RawMessage) (store.JobPosting, bool, error)) (store.JobPosting, error) {
	body, found, err := verifier.fetch(ctx, provider, fetchURL)
	if err != nil {
		return store.JobPosting{}, err
	}
	if !found {
		return store.JobPosting{}, fmt.Errorf("%s has no such posting", provider)
	}
	posting, _, err := parse(body)
	if err != nil {
		return store.JobPosting{}, err
	}
	posting.Raw = body
	return posting, nil
}
