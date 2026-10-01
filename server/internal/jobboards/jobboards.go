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
	"slices"
	"strconv"
	"strings"
	"sync"
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
	// requestTimeout leaves room for a large board's full text, which
	// Greenhouse took over 10 seconds to send for 96 postings.
	requestTimeout = 30 * time.Second
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
	// SmartRecruitersAPIBase serves every company's postings; the others are
	// empty for each board's own site, as acme.recruitee.com, and tests set
	// them.
	SmartRecruitersAPIBase string
	RecruiteeAPIBase       string
	BambooHRAPIBase        string
	PersonioAPIBase        string
	PinpointAPIBase        string
	GupyBase               string
	RemoteOKAPIBase        string
	// SearchTerms are what a large employer's board is searched by, rather
	// than read whole: the owner's criteria.
	SearchTerms           func(ctx context.Context) []string
	eightfoldDescriptions eightfoldDescriptions
	// limits are when each provider that answered 429 may be asked again,
	// from its Retry-After.
	limits providerLimits
}

// providerLimits keeps, per provider, when it may be asked again.
type providerLimits struct {
	lock  sync.Mutex
	until map[string]time.Time
}

func (limits *providerLimits) getUntil(provider string) time.Time {
	limits.lock.Lock()
	defer limits.lock.Unlock()
	return limits.until[provider]
}

func (limits *providerLimits) setUntil(provider string, until time.Time) {
	limits.lock.Lock()
	defer limits.lock.Unlock()
	if limits.until == nil {
		limits.until = map[string]time.Time{}
	}
	limits.until[provider] = until
}

// defaultRetryAfter is how long a provider that answers 429 without a
// Retry-After is left alone.
const defaultRetryAfter = 15 * time.Minute

// getRetryAfter reads a Retry-After header, in seconds or as a date.
func getRetryAfter(header string, now time.Time) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(header); err == nil && date.After(now) {
		return date.Sub(now)
	}
	return defaultRetryAfter
}

func NewVerifier() *Verifier {
	return &Verifier{
		HTTPClient:             &http.Client{Timeout: requestTimeout},
		GreenhouseAPIBase:      "https://boards-api.greenhouse.io",
		LeverAPIBase:           "https://api.lever.co",
		AshbyAPIBase:           "https://api.ashbyhq.com",
		AshbyBoardBase:         "https://jobs.ashbyhq.com",
		WorkableAPIBase:        "https://apply.workable.com",
		HimalayasAPIBase:       "https://himalayas.app",
		SmartRecruitersAPIBase: "https://api.smartrecruiters.com",
		RemoteOKAPIBase:        "https://remoteok.com",
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
	case Recruitee, BambooHR, SmartRecruiters, Personio, Pinpoint, Gupy:
		return verifier.verifyByPostings(ctx, provider, boardToken)
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

// GetBoardURL returns the board's public page, or "" for a provider without
// one per board.
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
	case SmartRecruiters:
		return "https://jobs.smartrecruiters.com/" + escapedToken
	}
	if !tenantName.MatchString(boardToken) {
		return ""
	}
	switch provider {
	case Recruitee:
		return "https://" + boardToken + ".recruitee.com/"
	case BambooHR:
		return "https://" + boardToken + ".bamboohr.com/careers"
	case Personio:
		return "https://" + boardToken + ".jobs.personio.com/"
	case Pinpoint:
		return "https://" + boardToken + ".pinpointhq.com/"
	case Gupy:
		return "https://" + boardToken + ".gupy.io/"
	}
	return ""
}

// verifyByPostings confirms a board by reading its postings. SmartRecruiters
// lists nothing for a company it doesn't know, so an empty list there
// confirms nothing.
func (verifier *Verifier) verifyByPostings(ctx context.Context, provider, boardToken string) (Verification, error) {
	postings, err := verifier.FetchPostings(ctx, provider, boardToken)
	if errors.Is(err, ErrPostingAPIOff) || err == nil && provider == SmartRecruiters && len(postings) == 0 {
		return Verification{}, nil
	}
	if err != nil {
		return Verification{}, err
	}
	count := len(postings)
	return Verification{Verified: true, OpenPostingCount: &count, BoardURL: GetBoardURL(provider, boardToken)}, nil
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

// redirectingProviders send a name they don't know to their own site, so a
// redirect there means no board.
var redirectingProviders = []string{BambooHR, Personio}

// fetch returns the body of a 200, or found=false for a 404 or, on a
// redirecting provider, a redirect. A provider that answered 429 isn't asked
// again until its Retry-After has passed.
func (verifier *Verifier) fetch(ctx context.Context, provider, fetchURL string) ([]byte, bool, error) {
	if until := verifier.limits.getUntil(provider); time.Now().Before(until) {
		return nil, false, fmt.Errorf("%w: %s, until %s", ErrRateLimited, provider, until.Format(time.RFC3339))
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return nil, false, err
	}
	request.Header.Set("User-Agent", userAgent)

	client := verifier.HTTPClient
	if slices.Contains(redirectingProviders, provider) {
		withoutRedirects := *verifier.HTTPClient
		withoutRedirects.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		client = &withoutRedirects
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, false, fmt.Errorf("reach %s: %w", provider, err)
	}
	defer response.Body.Close()

	switch {
	case response.StatusCode == http.StatusOK:
		body, err := io.ReadAll(response.Body)
		return body, err == nil, err
	case response.StatusCode == http.StatusNotFound, response.StatusCode >= 300 && response.StatusCode < 400:
		return nil, false, nil
	case response.StatusCode == http.StatusTooManyRequests:
		retryAfter := getRetryAfter(response.Header.Get("Retry-After"), time.Now())
		verifier.limits.setUntil(provider, time.Now().Add(retryAfter))
		return nil, false, fmt.Errorf("%w: %s answered 429, retry in %s", ErrRateLimited, provider, retryAfter.Round(time.Second))
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
