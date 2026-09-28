// Package jobboards checks a company's job board against its provider's
// public API, so an agent's guess about where a company lists roles is
// confirmed before it is stored.
package jobboards

import (
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

const (
	Greenhouse = "greenhouse"
	Lever      = "lever"
	Ashby      = "ashby"
)

const (
	requestTimeout = 10 * time.Second
	userAgent      = "job-search-hub/0.1"
)

type Verification struct {
	Verified         bool   `json:"verified"`
	OpenPostingCount int    `json:"open_posting_count"`
	BoardURL         string `json:"board_url,omitempty"`
}

// Verifier calls the providers' public posting APIs. The API base URLs are
// fields so tests can point them at a local server.
type Verifier struct {
	HTTPClient        *http.Client
	GreenhouseAPIBase string
	LeverAPIBase      string
	AshbyAPIBase      string
}

func NewVerifier() *Verifier {
	return &Verifier{
		HTTPClient:        &http.Client{Timeout: requestTimeout},
		GreenhouseAPIBase: "https://boards-api.greenhouse.io",
		LeverAPIBase:      "https://api.lever.co",
		AshbyAPIBase:      "https://api.ashbyhq.com",
	}
}

// Verify reports whether the board exists and how many postings it has open.
// A board the provider does not know is unverified, not an error.
func (verifier *Verifier) Verify(ctx context.Context, provider, boardToken string) (Verification, error) {
	escapedToken := url.PathEscape(boardToken)
	var apiURL, boardURL string
	switch provider {
	case Greenhouse:
		apiURL = verifier.GreenhouseAPIBase + "/v1/boards/" + escapedToken + "/jobs"
		boardURL = "https://job-boards.greenhouse.io/" + escapedToken
	case Lever:
		apiURL = verifier.LeverAPIBase + "/v0/postings/" + escapedToken + "?mode=json"
		boardURL = "https://jobs.lever.co/" + escapedToken
	case Ashby:
		apiURL = verifier.AshbyAPIBase + "/posting-api/job-board/" + escapedToken
		boardURL = "https://jobs.ashbyhq.com/" + escapedToken
	default:
		return Verification{}, ErrUnsupportedProvider
	}

	body, found, err := verifier.fetch(ctx, provider, apiURL)
	if err != nil || !found {
		return Verification{}, err
	}
	count, err := countPostings(provider, body)
	if err != nil {
		return Verification{}, fmt.Errorf("read the %s board %q: %w", provider, boardToken, err)
	}
	return Verification{Verified: true, OpenPostingCount: count, BoardURL: boardURL}, nil
}

// fetch returns the body of a 200, or found=false for a 404.
func (verifier *Verifier) fetch(ctx context.Context, provider, apiURL string) ([]byte, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, false, err
	}
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Accept", "application/json")

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
