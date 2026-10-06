// Package startupsgallery reads startups.gallery's list of remote companies
// once a week, and keeps the boards of the ones whose careers link names a
// board with a title in the owner's roles; the poller then reads them daily
// and the Companies page suggests the ones with a fitting job. Only each
// company's name, site and careers link are kept: never the site's
// descriptions, tags or pictures. robots.txt is read first, every request
// names the hub, and the requests are spaced.
package startupsgallery

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/drain"
	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	// readInterval is how often the list is read.
	readInterval = 7 * 24 * time.Hour
	listPath     = "/categories/work-type/remote"
	// maximumPagesPerPass bounds the company pages one pass reads; the list
	// names about fifteen.
	maximumPagesPerPass = 30
	// maximumBoardChecksPerPass bounds the boards one pass checks: new ones,
	// and earlier ones whose titles named no role yet.
	maximumBoardChecksPerPass = 200
	requestTimeout            = 30 * time.Second
	maximumPageSize           = 4 << 20
	// UserAgent names the hub and where to read about it.
	UserAgent = "job-search-hub/0.1 (+https://github.com/tonypine/job-search-hub)"
)

type boardReader interface {
	ListPostingTitles(ctx context.Context, provider, boardToken string) ([]string, error)
}

type Reader struct {
	hub    *store.Store
	boards boardReader
	// SiteBase is startups.gallery; tests point it elsewhere.
	SiteBase string
	// RequestPause spaces the requests to the site, and to each board.
	RequestPause time.Duration
	// Now tells the time; tests move it.
	Now        func() time.Time
	httpClient *http.Client
}

func NewReader(hub *store.Store, boards boardReader) *Reader {
	return &Reader{
		hub: hub, boards: boards, SiteBase: "https://startups.gallery", RequestPause: 5 * time.Second, Now: time.Now,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

// PassSummary counts one weekly pass.
type PassSummary struct {
	Listed          int
	New             int
	PagesRead       int
	PagesDisallowed int
	Checked         int
	Kept            int
	Failed          int
	Disallowed      bool
}

// Run checks every interval whether a week has passed since the last read,
// and reads the list when it has, until ctx ends.
func (reader *Reader) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if !drain.IsDraining(ctx) {
			summary, err := reader.ReadOnce(ctx)
			if err != nil {
				slog.Error("startups.gallery pass stopped", "error", err)
			} else if summary != (PassSummary{}) {
				slog.Info("startups.gallery pass done", "listed", summary.Listed, "new", summary.New, "pages read", summary.PagesRead,
					"checked", summary.Checked, "kept", summary.Kept, "failed", summary.Failed, "disallowed", summary.Disallowed)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ReadOnce reads the remote list when a week has passed since the last
// read: robots.txt first, then the list, then the page of each company not
// read before. It then checks the titles of the boards those pages name, and
// keeps a board whose titles name one of the roles. A page that fails is
// left for next week's pass.
func (reader *Reader) ReadOnce(ctx context.Context) (PassSummary, error) {
	var summary PassSummary
	lastRead, err := reader.hub.GetLastGalleryListRead(ctx)
	if err != nil {
		return summary, err
	}
	if lastRead != nil && reader.Now().Sub(*lastRead) < readInterval {
		return summary, nil
	}
	robots, err := reader.readRobots(ctx)
	if err != nil {
		return summary, err
	}
	// The read counts from here: whatever happens next, the site is asked
	// again only next week.
	if !robots.allows(listPath) {
		summary.Disallowed = true
		return summary, reader.hub.RecordGalleryListRead(ctx, 0)
	}
	if err := reader.pause(ctx); err != nil {
		return summary, err
	}
	list, err := reader.get(ctx, listPath)
	if err != nil {
		return summary, errors.Join(fmt.Errorf("read the remote list: %w", err), reader.hub.RecordGalleryListRead(ctx, 0))
	}
	listed := readListedCompanies(list)
	summary.Listed = len(listed)
	if err := reader.hub.RecordGalleryListRead(ctx, len(listed)); err != nil {
		return summary, err
	}
	if summary.New, err = reader.hub.SaveListedGalleryCompanies(ctx, listed); err != nil {
		return summary, err
	}
	if err := reader.readCompanyPages(ctx, robots, &summary); err != nil {
		return summary, err
	}
	return summary, reader.checkBoards(ctx, &summary)
}

// readRobots reads the site's robots.txt. A missing file allows everything;
// one the site fails to serve stops the pass, as if it disallowed it.
func (reader *Reader) readRobots(ctx context.Context) (robotsRules, error) {
	body, status, err := reader.fetch(ctx, "/robots.txt")
	switch {
	case err != nil:
		return robotsRules{}, fmt.Errorf("read robots.txt: %w", err)
	case status >= 400 && status < 500:
		return robotsRules{}, nil
	case status != http.StatusOK:
		return robotsRules{}, fmt.Errorf("robots.txt answered %d", status)
	}
	return readRobots(body, UserAgent), nil
}

// readCompanyPages reads the page of each listed company not read before,
// for its site and careers link.
func (reader *Reader) readCompanyPages(ctx context.Context, robots robotsRules, summary *PassSummary) error {
	unread, err := reader.hub.ListUnreadGalleryPages(ctx, maximumPagesPerPass)
	if err != nil {
		return err
	}
	for _, company := range unread {
		path := "/companies/" + company.Slug
		if !robots.allows(path) {
			summary.PagesDisallowed++
			continue
		}
		if err := reader.pause(ctx); err != nil {
			return err
		}
		page, err := reader.get(ctx, path)
		if err != nil {
			slog.Warn("startups.gallery company page failed", "slug", company.Slug, "error", err)
			summary.Failed++
			continue
		}
		if err := reader.hub.RecordGalleryPage(ctx, company.Slug, readCompanyPage(page)); err != nil {
			return err
		}
		summary.PagesRead++
	}
	return nil
}

// checkBoards reads the titles of the boards the companies' careers links
// name and that aren't kept yet, and keeps a board whose titles name one of
// the roles. A provider limiting requests ends the checks for the pass.
func (reader *Reader) checkBoards(ctx context.Context, summary *PassSummary) error {
	saved, err := reader.hub.GetJobCriteria(ctx)
	if err != nil {
		return err
	}
	companies, err := reader.hub.ListUnkeptGalleryBoards(ctx, maximumBoardChecksPerPass)
	if err != nil {
		return err
	}
	for _, company := range companies {
		if err := reader.pause(ctx); err != nil {
			return err
		}
		titles, err := reader.boards.ListPostingTitles(ctx, company.Provider, company.BoardToken)
		switch {
		case errors.Is(err, jobboards.ErrRateLimited):
			summary.Failed++
			return nil
		case errors.Is(err, jobboards.ErrPostingAPIOff):
			titles = nil
		case err != nil:
			summary.Failed++
			continue
		}
		summary.Checked++
		var boardID *uuid.UUID
		if namesARole(titles, saved.Criteria) {
			board, err := reader.hub.SaveFoundJobBoard(ctx, store.Actor{Kind: store.ActorSystem}, store.FoundJobBoardInput{
				CompanyName: company.Name, Provider: company.Provider, BoardToken: company.BoardToken,
				BoardURL: jobboards.GetBoardURL(company.Provider, company.BoardToken), OpenPostingCount: len(titles), FoundBy: store.JobBoardFoundByDiscovery,
			})
			if err != nil {
				return err
			}
			boardID = &board.ID
			summary.Kept++
		}
		if err := reader.hub.RecordGalleryBoardCheck(ctx, company.Slug, boardID); err != nil {
			return err
		}
	}
	return nil
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

var (
	companyLink = regexp.MustCompile(`(?s)<a\b[^>]*\bhref="(?:\.\./\.\./|/|https://startups\.gallery/)companies/([a-z0-9][a-z0-9-]*)"[^>]*>(.*?)</a>`)
	heading     = regexp.MustCompile(`(?s)<h3\b[^>]*>(.*?)</h3>`)
	anyLink     = regexp.MustCompile(`(?s)<a\b[^>]*\bhref="(https?://[^"]+)"[^>]*>(.*?)</a>`)
	anyTag      = regexp.MustCompile(`<[^>]*>`)
)

// readListedCompanies returns each company the list links to, once, by its
// slug and the name its card shows.
func readListedCompanies(page []byte) []store.GalleryCompany {
	seen := map[string]bool{}
	var companies []store.GalleryCompany
	for _, match := range companyLink.FindAllSubmatch(page, -1) {
		slug := string(match[1])
		name := heading.FindSubmatch(match[2])
		if seen[slug] || name == nil {
			continue
		}
		if text := getText(name[1]); text != "" {
			seen[slug] = true
			companies = append(companies, store.GalleryCompany{Slug: slug, Name: text})
		}
	}
	return companies
}

// readCompanyPage reads a company's page for its site ("Visit Website") and
// careers link ("View Jobs"), and the board that link, or else any posting
// the page links to, names.
func readCompanyPage(page []byte) store.GalleryPage {
	var read store.GalleryPage
	for _, match := range anyLink.FindAllSubmatch(page, -1) {
		address := html.UnescapeString(string(match[1]))
		switch strings.ToLower(getText(match[2])) {
		case "visit website":
			if read.Website == "" {
				read.Website = address
			}
		case "view jobs":
			if read.CareersURL == "" {
				read.CareersURL = address
			}
		}
	}
	if provider, boardToken, ok := jobboards.ParseBoardURL(read.CareersURL); ok {
		read.Provider, read.BoardToken = provider, strings.ToLower(boardToken)
		return read
	}
	for _, match := range anyLink.FindAllSubmatch(page, -1) {
		if provider, boardToken, ok := jobboards.ParseBoardURL(html.UnescapeString(string(match[1]))); ok {
			read.Provider, read.BoardToken = provider, strings.ToLower(boardToken)
			return read
		}
	}
	return read
}

// getText returns an element's text, without tags and with spaces folded.
func getText(markup []byte) string {
	return strings.Join(strings.Fields(html.UnescapeString(anyTag.ReplaceAllString(string(markup), " "))), " ")
}

func (reader *Reader) get(ctx context.Context, path string) ([]byte, error) {
	body, status, err := reader.fetch(ctx, path)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("the site answered %d", status)
	}
	return body, nil
}

func (reader *Reader) fetch(ctx context.Context, path string) ([]byte, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, reader.SiteBase+(&url.URL{Path: path}).EscapedPath(), nil)
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("User-Agent", UserAgent)
	response, err := reader.httpClient.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumPageSize))
	return body, response.StatusCode, err
}

func (reader *Reader) pause(ctx context.Context) error {
	timer := time.NewTimer(reader.RequestPause)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
