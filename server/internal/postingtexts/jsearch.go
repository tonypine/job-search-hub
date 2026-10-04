// Package postingtexts finds the full text of the postings mail alerts list
// with no more than a snippet, and states why when it can't. A job's
// company's board gives the best text, so the board search goes first; a
// job still without its text a day later is looked up on Google for Jobs,
// through JSearch, once. Alert sites' own pages can't be read: Glassdoor
// answers the server with Cloudflare's challenge.
package postingtexts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultJSearchURL is JSearch on RapidAPI; OpenWeb Ninja serves the same
// API under its own address and key.
const DefaultJSearchURL = "https://jsearch.p.rapidapi.com"

// ErrSearchRefused is a key JSearch doesn't accept, or a quota it says is
// spent: no search can succeed until the owner acts.
var ErrSearchRefused = errors.New("JSearch refused the search")

// Posting is one posting Google for Jobs lists, as JSearch gives it.
type Posting struct {
	Title        string `json:"job_title"`
	EmployerName string `json:"employer_name"`
	Description  string `json:"job_description"`
	ApplyLink    string `json:"job_apply_link"`
}

// JSearch searches Google for Jobs' postings through the JSearch API.
type JSearch struct {
	HTTPClient *http.Client
	BaseURL    string
	APIKey     string
}

func NewJSearch(baseURL, apiKey string) *JSearch {
	return &JSearch{HTTPClient: &http.Client{Timeout: 30 * time.Second}, BaseURL: strings.TrimSuffix(baseURL, "/"), APIKey: apiKey}
}

// Search returns the first page of postings matching query, in country
// when it isn't empty: a two-letter code such as "br".
func (search *JSearch) Search(ctx context.Context, query, country string) ([]Posting, error) {
	parameters := url.Values{"query": {query}, "page": {"1"}, "num_pages": {"1"}, "date_posted": {"all"}}
	if country != "" {
		parameters.Set("country", country)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, search.BaseURL+"/search?"+parameters.Encode(), nil)
	if err != nil {
		return nil, err
	}
	// RapidAPI takes its key and the API's host; OpenWeb Ninja its own key.
	if strings.HasSuffix(request.URL.Hostname(), ".rapidapi.com") {
		request.Header.Set("X-RapidAPI-Key", search.APIKey)
		request.Header.Set("X-RapidAPI-Host", request.URL.Hostname())
	} else {
		request.Header.Set("X-API-Key", search.APIKey)
	}
	response, err := search.HTTPClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("reach JSearch: %w", err)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		return nil, fmt.Errorf("%w: it answered %d", ErrSearchRefused, response.StatusCode)
	default:
		return nil, fmt.Errorf("JSearch answered %d", response.StatusCode)
	}
	var page struct {
		Status string    `json:"status"`
		Data   []Posting `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(&page); err != nil {
		return nil, fmt.Errorf("read JSearch's answer: %w", err)
	}
	if page.Status != "OK" {
		return nil, fmt.Errorf("JSearch answered status %q", page.Status)
	}
	return page.Data, nil
}
