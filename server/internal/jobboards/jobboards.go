// Package jobboards checks a company's job board against its provider's
// public API, so an agent's guess about where a company lists roles is
// confirmed before it is stored.
package jobboards

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

var ErrUnsupportedProvider = errors.New("this provider's job boards cannot be verified yet")

// ErrRateLimited means the provider asked for fewer requests, answering 429.
var ErrRateLimited = errors.New("the provider is limiting requests")

const (
	Greenhouse = "greenhouse"
	Lever      = "lever"
	Ashby      = "ashby"
	Workable   = "workable"
)

const (
	requestTimeout = 10 * time.Second
	userAgent      = "job-search-hub/0.1"
)

// Verification is what a provider confirmed about a board. OpenPostingCount is
// nil when the board exists but its postings could not be counted.
type Verification struct {
	Verified         bool   `json:"verified"`
	OpenPostingCount *int   `json:"open_posting_count,omitempty"`
	BoardURL         string `json:"board_url,omitempty"`
}

// Verifier calls the providers' public posting APIs. The base URLs are fields
// so tests can point them at a local server.
type Verifier struct {
	HTTPClient        *http.Client
	GreenhouseAPIBase string
	LeverAPIBase      string
	AshbyAPIBase      string
	AshbyBoardBase    string
	WorkableAPIBase   string
	HimalayasAPIBase  string
	// EightfoldAPIBase is empty for each tenant's own site; tests set it.
	EightfoldAPIBase string
	// SearchTerms are what a large employer's board is searched by, rather
	// than read whole: the owner's criteria.
	SearchTerms           func(ctx context.Context) []string
	eightfoldDescriptions eightfoldDescriptions
}

func NewVerifier() *Verifier {
	return &Verifier{
		HTTPClient:        &http.Client{Timeout: requestTimeout},
		GreenhouseAPIBase: "https://boards-api.greenhouse.io",
		LeverAPIBase:      "https://api.lever.co",
		AshbyAPIBase:      "https://api.ashbyhq.com",
		AshbyBoardBase:    "https://jobs.ashbyhq.com",
		WorkableAPIBase:   "https://apply.workable.com",
		HimalayasAPIBase:  "https://himalayas.app",
	}
}

// Verify reports whether the board exists and how many postings it has open.
// A board the provider does not know is unverified, not an error.
func (verifier *Verifier) Verify(ctx context.Context, provider, boardToken string) (Verification, error) {
	escapedToken := url.PathEscape(boardToken)
	boardURL := GetBoardURL(provider, boardToken)
	var apiURL string
	switch provider {
	case Greenhouse:
		apiURL = verifier.GreenhouseAPIBase + "/v1/boards/" + escapedToken + "/jobs"
	case Lever:
		apiURL = verifier.LeverAPIBase + "/v0/postings/" + escapedToken + "?mode=json"
	case Ashby:
		apiURL = verifier.AshbyAPIBase + "/posting-api/job-board/" + escapedToken
	case Workable:
		apiURL = verifier.WorkableAPIBase + "/api/v1/widget/accounts/" + escapedToken
	case Eightfold:
		return verifier.verifyEightfold(ctx, boardToken)
	default:
		return Verification{}, ErrUnsupportedProvider
	}

	body, found, err := verifier.fetch(ctx, provider, apiURL)
	if err != nil {
		return Verification{}, err
	}
	if !found {
		if provider == Ashby {
			return verifier.verifyAshbyBoardPage(ctx, boardToken, boardURL)
		}
		return Verification{}, nil
	}
	count, err := countPostings(provider, body)
	if err != nil {
		return Verification{}, fmt.Errorf("read the %s board %q: %w", provider, boardToken, err)
	}
	return Verification{Verified: true, OpenPostingCount: &count, BoardURL: boardURL}, nil
}

// GetBoardURL returns the board's public page on Greenhouse, Lever, Ashby or
// Workable, and "" for any other provider.
func GetBoardURL(provider, boardToken string) string {
	escapedToken := url.PathEscape(boardToken)
	switch provider {
	case Greenhouse:
		return "https://job-boards.greenhouse.io/" + escapedToken
	case Lever:
		return "https://jobs.lever.co/" + escapedToken
	case Ashby:
		return "https://jobs.ashbyhq.com/" + escapedToken
	case Workable:
		return "https://apply.workable.com/" + escapedToken + "/"
	}
	return ""
}

// verifyAshbyBoardPage covers Ashby customers who turn the posting API off.
// Their public board page answers 200 for any name, but only a real board
// embeds its own slug, so the slug's presence confirms the board. Its
// postings cannot be counted from the page.
func (verifier *Verifier) verifyAshbyBoardPage(ctx context.Context, boardToken, boardURL string) (Verification, error) {
	page, found, err := verifier.fetch(ctx, Ashby, verifier.AshbyBoardBase+"/"+url.PathEscape(boardToken))
	if err != nil || !found {
		return Verification{}, err
	}
	slug, err := json.Marshal(boardToken)
	if err != nil {
		return Verification{}, err
	}
	if !bytes.Contains(page, append([]byte(`"hostedJobsPageSlug":`), slug...)) {
		return Verification{}, nil
	}
	return Verification{Verified: true, BoardURL: boardURL}, nil
}

// fetch returns the body of a 200, or found=false for a 404.
func (verifier *Verifier) fetch(ctx context.Context, provider, fetchURL string) ([]byte, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return nil, false, err
	}
	request.Header.Set("User-Agent", userAgent)

	response, err := verifier.HTTPClient.Do(request)
	if err != nil {
		return nil, false, fmt.Errorf("reach %s: %w", provider, err)
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusOK:
		body, err := io.ReadAll(response.Body)
		return body, err == nil, err
	case http.StatusNotFound:
		return nil, false, nil
	case http.StatusTooManyRequests:
		return nil, false, fmt.Errorf("%w: %s answered 429", ErrRateLimited, provider)
	default:
		return nil, false, fmt.Errorf("%s answered %d", provider, response.StatusCode)
	}
}

// countPostings reads the posting list out of each provider's payload shape:
// Lever answers with a bare array, Greenhouse and Ashby with {"jobs": [...]}.
func countPostings(provider string, body []byte) (int, error) {
	if provider == Lever {
		var postings []json.RawMessage
		err := json.Unmarshal(body, &postings)
		return len(postings), err
	}
	var board struct {
		Jobs []json.RawMessage `json:"jobs"`
	}
	err := json.Unmarshal(body, &board)
	return len(board.Jobs), err
}
