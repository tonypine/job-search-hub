// Package boarddiscovery finds company boards in bulk from Common Crawl's
// index of public web pages, and keeps the boards whose titles name one of
// the owner's roles; the poller then reads them daily. The index answers
// slowly and often times out, so each pass reads a few pages and the next
// one resumes.
package boarddiscovery

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/drain"
	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	// maximumIndexPagesPerPass bounds the index pages one pass reads per URL
	// pattern; each holds a few thousand URLs.
	maximumIndexPagesPerPass = 2
	// maximumChecksPerProvider bounds the tokens one pass checks per
	// provider, one request each.
	maximumChecksPerProvider = 150
	indexAttempts            = 3
	indexRetryPause          = 30 * time.Second
	indexTimeout             = 3 * time.Minute
	userAgent                = "job-search-hub/0.1"
)

// boardIndex is where an index lists a provider's boards, and how a listed
// URL names its board.
type boardIndex struct {
	provider   string
	urlPattern string
	// matchType is "domain" for boards on their own subdomain.
	matchType string
	getToken  func(pageURL *url.URL) string
}

var boardIndexes = []boardIndex{
	{provider: jobboards.Greenhouse, urlPattern: "job-boards.greenhouse.io/*", getToken: getGreenhouseToken},
	{provider: jobboards.Greenhouse, urlPattern: "boards.greenhouse.io/*", getToken: getGreenhouseToken},
	{provider: jobboards.Lever, urlPattern: "jobs.lever.co/*", getToken: getFirstPathSegment},
	{provider: jobboards.Ashby, urlPattern: "jobs.ashbyhq.com/*", getToken: getFirstPathSegment},
	{provider: jobboards.Recruitee, urlPattern: "recruitee.com", matchType: "domain", getToken: getRecruiteeToken},
}

// checkedProviders are the providers whose discovered tokens are checked:
// each answers a board's titles in one request.
var checkedProviders = []string{jobboards.Greenhouse, jobboards.Lever, jobboards.Ashby, jobboards.Recruitee}

type boardReader interface {
	ListPostingTitles(ctx context.Context, provider, boardToken string) ([]string, error)
	FetchBoardCompanyName(ctx context.Context, provider, boardToken string) (string, error)
}

type Discoverer struct {
	hub    *store.Store
	boards boardReader
	// IndexBase is Common Crawl's index server; tests point it elsewhere.
	IndexBase string
	// RequestPause spaces the requests to each provider.
	RequestPause time.Duration
	// IndexRetryPause waits between attempts at an index page.
	IndexRetryPause time.Duration
	httpClient      *http.Client
}

func New(hub *store.Store, boards boardReader) *Discoverer {
	return &Discoverer{
		hub: hub, boards: boards, IndexBase: "https://index.commoncrawl.org", RequestPause: 500 * time.Millisecond,
		IndexRetryPause: indexRetryPause, httpClient: &http.Client{Timeout: indexTimeout},
	}
}

// PassSummary counts one discovery pass.
type PassSummary struct {
	IndexPagesRead int
	TokensFound    int
	Checked        int
	Kept           int
	Failed         int
}

// Run discovers once at start and then every interval, until ctx ends.
func (discoverer *Discoverer) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if !drain.IsDraining(ctx) {
			summary, err := discoverer.DiscoverOnce(ctx)
			if err != nil {
				slog.Error("board discovery pass stopped", "error", err)
			} else if summary != (PassSummary{}) {
				slog.Info("board discovery pass done", "index pages read", summary.IndexPagesRead, "tokens found", summary.TokensFound,
					"checked", summary.Checked, "kept", summary.Kept, "failed", summary.Failed)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// DiscoverOnce reads the next pages of the latest crawl's index for each
// provider, then checks the titles of tokens not checked yet. An index page
// that keeps failing is logged and left for the next pass.
func (discoverer *Discoverer) DiscoverOnce(ctx context.Context) (PassSummary, error) {
	var summary PassSummary
	crawlID, indexURL, err := discoverer.getLatestIndex(ctx)
	if err != nil {
		slog.Warn("board index unavailable", "error", err)
	} else {
		for _, index := range boardIndexes {
			pagesRead, tokensFound, err := discoverer.readIndexPages(ctx, crawlID, indexURL, index)
			summary.IndexPagesRead += pagesRead
			summary.TokensFound += tokensFound
			if ctx.Err() != nil {
				return summary, ctx.Err()
			}
			if err != nil {
				slog.Warn("board index page failed", "pattern", index.urlPattern, "error", err)
			}
		}
	}
	criteria, err := discoverer.hub.GetJobCriteria(ctx)
	if err != nil {
		return summary, err
	}
	var lock sync.Mutex
	var group sync.WaitGroup
	var failure error
	for _, provider := range checkedProviders {
		group.Go(func() {
			checks, err := discoverer.checkTokens(ctx, provider, criteria.Criteria)
			lock.Lock()
			defer lock.Unlock()
			summary.Checked += checks.Checked
			summary.Kept += checks.Kept
			summary.Failed += checks.Failed
			if err != nil {
				failure = err
			}
		})
	}
	group.Wait()
	return summary, failure
}

// getLatestIndex returns the newest crawl's id and its index's address.
func (discoverer *Discoverer) getLatestIndex(ctx context.Context) (string, string, error) {
	body, err := discoverer.get(ctx, discoverer.IndexBase+"/collinfo.json")
	if err != nil {
		return "", "", err
	}
	var crawls []struct {
		ID     string `json:"id"`
		CDXAPI string `json:"cdx-api"`
	}
	if err := json.Unmarshal(body, &crawls); err != nil {
		return "", "", err
	}
	if len(crawls) == 0 {
		return "", "", errors.New("the index lists no crawl")
	}
	indexURL := crawls[0].CDXAPI
	if !strings.HasPrefix(indexURL, discoverer.IndexBase) {
		indexURL = discoverer.IndexBase + "/" + crawls[0].ID + "-index"
	}
	return crawls[0].ID, indexURL, nil
}

// readIndexPages reads up to maximumIndexPagesPerPass unread pages of the
// index for the pattern, and stores the board tokens they list.
func (discoverer *Discoverer) readIndexPages(ctx context.Context, crawlID, indexURL string, index boardIndex) (int, int, error) {
	query := url.Values{"url": {index.urlPattern}, "output": {"json"}}
	if index.matchType != "" {
		query.Set("matchType", index.matchType)
	}
	var pageCount struct {
		Pages int `json:"pages"`
	}
	countQuery := url.Values{"showNumPages": {"true"}}
	for key, values := range query {
		countQuery[key] = values
	}
	body, err := discoverer.getWithRetries(ctx, indexURL+"?"+countQuery.Encode())
	if err != nil {
		return 0, 0, err
	}
	if err := json.Unmarshal(body, &pageCount); err != nil {
		return 0, 0, fmt.Errorf("read the page count: %w", err)
	}
	read, err := discoverer.hub.ListReadIndexPages(ctx, crawlID, index.urlPattern)
	if err != nil {
		return 0, 0, err
	}
	pagesRead, tokensFound := 0, 0
	for page := 0; page < pageCount.Pages && pagesRead < maximumIndexPagesPerPass; page++ {
		if read[page] {
			continue
		}
		pageQuery := url.Values{"fl": {"url"}, "page": {fmt.Sprint(page)}}
		for key, values := range query {
			pageQuery[key] = values
		}
		body, err := discoverer.getWithRetries(ctx, indexURL+"?"+pageQuery.Encode())
		if err != nil {
			return pagesRead, tokensFound, err
		}
		newTokens, err := discoverer.hub.SaveDiscoveredBoardTokens(ctx, index.provider, readBoardTokens(body, index))
		if err != nil {
			return pagesRead, tokensFound, err
		}
		if err := discoverer.hub.RecordIndexPage(ctx, crawlID, index.urlPattern, page); err != nil {
			return pagesRead, tokensFound, err
		}
		pagesRead++
		tokensFound += newTokens
	}
	return pagesRead, tokensFound, nil
}

// readBoardTokens returns the distinct board tokens an index page lists, one
// JSON line per URL.
func readBoardTokens(page []byte, index boardIndex) []string {
	seen := map[string]bool{}
	var boardTokens []string
	scanner := bufio.NewScanner(bytes.NewReader(page))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var line struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(scanner.Bytes(), &line) != nil {
			continue
		}
		pageURL, err := url.Parse(line.URL)
		if err != nil {
			continue
		}
		boardToken := strings.ToLower(index.getToken(pageURL))
		if validToken.MatchString(boardToken) && !seen[boardToken] {
			seen[boardToken] = true
			boardTokens = append(boardTokens, boardToken)
		}
	}
	return boardTokens
}

var validToken = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,80}$`)

func getFirstPathSegment(pageURL *url.URL) string {
	segment, _, _ := strings.Cut(strings.TrimPrefix(pageURL.Path, "/"), "/")
	return segment
}

// getGreenhouseToken reads a board's token from its page, or from an
// embedded board's "for" parameter.
func getGreenhouseToken(pageURL *url.URL) string {
	if segment := getFirstPathSegment(pageURL); segment != "embed" {
		return segment
	}
	return pageURL.Query().Get("for")
}

func getRecruiteeToken(pageURL *url.URL) string {
	subdomain, found := strings.CutSuffix(pageURL.Hostname(), ".recruitee.com")
	if !found || strings.Contains(subdomain, ".") || subdomain == "www" || subdomain == "app" {
		return ""
	}
	return subdomain
}

// TokenChecks counts one provider's token checks.
type TokenChecks struct {
	Checked int
	Kept    int
	Failed  int
}

// checkTokens reads the titles of up to maximumChecksPerProvider of the
// provider's unchecked tokens, one request at a time, and keeps a board
// whose titles name one of the roles. A provider limiting requests ends the
// provider's checks for the pass.
func (discoverer *Discoverer) checkTokens(ctx context.Context, provider string, criteria store.JobCriteria) (TokenChecks, error) {
	var checks TokenChecks
	boardTokens, err := discoverer.hub.ListUncheckedBoardTokens(ctx, provider, maximumChecksPerProvider)
	if err != nil {
		return checks, err
	}
	for _, boardToken := range boardTokens {
		if err := waitFor(ctx, discoverer.RequestPause); err != nil {
			return checks, err
		}
		titles, err := discoverer.boards.ListPostingTitles(ctx, provider, boardToken)
		switch {
		case errors.Is(err, jobboards.ErrRateLimited):
			checks.Failed++
			return checks, nil
		case errors.Is(err, jobboards.ErrPostingAPIOff):
			titles, err = nil, nil
		case err != nil:
			checks.Failed++
			continue
		}
		checks.Checked++
		var boardID *uuid.UUID
		if namesARole(titles, criteria) {
			board, err := discoverer.keepBoard(ctx, provider, boardToken, len(titles))
			if err != nil {
				return checks, err
			}
			boardID = &board.ID
			checks.Kept++
		}
		if err := discoverer.hub.RecordBoardTokenCheck(ctx, provider, boardToken, boardID); err != nil {
			return checks, err
		}
	}
	return checks, nil
}

// keepBoard stores a discovered board under the company name its provider
// publishes, or one made from its token.
func (discoverer *Discoverer) keepBoard(ctx context.Context, provider, boardToken string, openPostingCount int) (store.JobBoard, error) {
	companyName, err := discoverer.boards.FetchBoardCompanyName(ctx, provider, boardToken)
	if err != nil || companyName == "" {
		companyName = getNameFromToken(boardToken)
	}
	return discoverer.hub.SaveFoundJobBoard(ctx, store.Actor{Kind: store.ActorSystem}, store.FoundJobBoardInput{
		CompanyName: companyName, Provider: provider, BoardToken: boardToken, BoardURL: jobboards.GetBoardURL(provider, boardToken),
		OpenPostingCount: openPostingCount, FoundBy: store.JobBoardFoundByDiscovery,
	})
}

// namesARole reports whether any title names one of the roles.
func namesARole(titles []string, criteria store.JobCriteria) bool {
	for _, title := range titles {
		if jobfit.CouldFit(store.JobPosting{Title: title}, criteria) {
			return true
		}
	}
	return false
}

// getNameFromToken spells a board token as a company name: "acme-labs"
// becomes "Acme Labs".
func getNameFromToken(boardToken string) string {
	words := strings.FieldsFunc(boardToken, func(character rune) bool { return character == '-' || character == '_' })
	for index, word := range words {
		words[index] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

// getWithRetries reads an index address, trying again after a pause when
// the index times out or fails, as it often does.
func (discoverer *Discoverer) getWithRetries(ctx context.Context, address string) ([]byte, error) {
	var lastErr error
	for attempt := range indexAttempts {
		if attempt > 0 {
			if err := waitFor(ctx, discoverer.IndexRetryPause); err != nil {
				return nil, err
			}
		}
		body, err := discoverer.get(ctx, address)
		if err == nil {
			return body, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (discoverer *Discoverer) get(ctx context.Context, address string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", userAgent)
	response, err := discoverer.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("reach the index: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the index answered %d", response.StatusCode)
	}
	return io.ReadAll(response.Body)
}

func waitFor(ctx context.Context, pause time.Duration) error {
	timer := time.NewTimer(pause)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
