package postingtexts

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/wordmatch"
)

const (
	// boardSearchWait is how long an alert job waits for its company's board
	// to give its text before Google for Jobs is asked: the board search
	// reaches a company within a pass or two, unless a provider is limiting
	// requests.
	boardSearchWait = 24 * time.Hour
	// maximumSearchesPerPass bounds one pass; the next picks up the rest.
	maximumSearchesPerPass = 20
	// DefaultMonthlySearches keeps within JSearch's free plans, which allow
	// 100 to 200 requests a month.
	DefaultMonthlySearches = 100
)

// textFinder is who the change log says gave a job its text; the found
// posting is its source.
var textFinder = store.Actor{Kind: store.ActorSystem}

type searcher interface {
	Search(ctx context.Context, query, country string) ([]Posting, error)
}

type jobStore interface {
	GetJobCriteria(ctx context.Context) (store.SavedJobCriteria, error)
	ListAlertJobsAwaitingText(ctx context.Context) ([]store.AlertJobAwaitingText, error)
	CountPostingTextSearchesSince(ctx context.Context, since time.Time) (int, error)
	SavePostingText(ctx context.Context, actor store.Actor, jobID uuid.UUID, text, sourceURL string, searchedAt time.Time) error
	RecordPostingTextMissing(ctx context.Context, jobID uuid.UUID, reason string, searchedAt *time.Time) error
	RecordPostingTextSearchFailed(ctx context.Context, jobID uuid.UUID, reason string, searchedAt time.Time) error
}

type Finder struct {
	hub jobStore
	// search is nil when no JSearch key is set, which leaves Google for Jobs
	// unasked.
	search searcher
	// monthlySearches is how many requests a calendar month (UTC) may make.
	monthlySearches int
	now             func() time.Time
}

// New returns a finder that asks search for the texts no board gave, up to
// monthlySearches times a month; a nil search leaves it to state reasons.
func New(hub *store.Store, search *JSearch, monthlySearches int) *Finder {
	finder := &Finder{hub: hub, monthlySearches: monthlySearches, now: time.Now}
	if search != nil {
		finder.search = search
	}
	return finder
}

// PassSummary counts the Google for Jobs searches of one pass.
type PassSummary struct {
	Found    int
	NotFound int
	Failed   int
}

// Run looks once at start and then every interval, until ctx ends.
func (finder *Finder) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		summary, err := finder.FindOnce(ctx)
		if err != nil {
			slog.Error("posting text pass stopped", "error", err, "found", summary.Found, "not found", summary.NotFound, "failed", summary.Failed)
		} else if summary != (PassSummary{}) {
			slog.Info("posting texts searched", "found", summary.Found, "not found", summary.NotFound, "failed", summary.Failed)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// FindOnce goes over the open alert jobs with no more than a snippet,
// newest first. Each one past its board's wait is searched on Google for
// Jobs once, while the month's searches last; every other one is given the
// reason it has no text yet. A refused search, or one that never reaches
// JSearch, stops the pass, and the next one tries again; any other failed
// search counts toward the month's, and its job isn't searched again.
func (finder *Finder) FindOnce(ctx context.Context) (PassSummary, error) {
	saved, err := finder.hub.GetJobCriteria(ctx)
	if err != nil {
		return PassSummary{}, err
	}
	jobs, err := finder.hub.ListAlertJobsAwaitingText(ctx)
	if err != nil {
		return PassSummary{}, err
	}
	now := finder.now()
	monthStart := time.Date(now.UTC().Year(), now.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	used, err := finder.hub.CountPostingTextSearchesSince(ctx, monthStart)
	if err != nil {
		return PassSummary{}, err
	}
	var summary PassSummary
	searches := 0
	for _, awaiting := range jobs {
		job := awaiting.Job
		alert := describeAlert(job)
		var reason string
		switch {
		case jobfit.IsRoleRuledOut(job, saved.Criteria):
			reason = alert + ", and its title rules it out, so its posting wasn't looked for"
		case awaiting.SearchFailed:
			reason = alert + ", no board found lists it, and the Google for Jobs search for it failed"
		case awaiting.Searched:
			reason = alert + ", and neither a board found nor Google for Jobs lists the posting"
		case now.Sub(job.FirstSeenAt) < boardSearchWait:
			reason = alert + "; looking for the posting on its company's board"
		case finder.search == nil:
			reason = alert + ", no board found lists it, and the Google for Jobs search is off: it needs a JSearch key"
		case used >= finder.monthlySearches:
			reason = alert + ", no board found lists it, and this month's Google for Jobs searches are spent"
		case strings.TrimSpace(awaiting.CompanyName) == "":
			reason = alert + ", and names no company to look the posting up by"
		case searches == maximumSearchesPerPass:
			continue
		default:
			searches++
			posting, found, err := finder.searchText(ctx, awaiting)
			if err != nil && (errors.Is(err, ErrSearchRefused) || errors.Is(err, ErrSearchUnreachable) || ctx.Err() != nil) {
				return summary, err
			}
			used++
			if err != nil {
				slog.Warn("posting text search failed", "job", job.ID, "error", err)
				summary.Failed++
				reason = alert + ", no board found lists it, and the Google for Jobs search for it failed"
				if err := finder.hub.RecordPostingTextSearchFailed(ctx, job.ID, reason, now); err != nil {
					return summary, err
				}
				continue
			}
			if found {
				if err := finder.hub.SavePostingText(ctx, textFinder, job.ID, posting.Description, posting.ApplyLink, now); err != nil {
					return summary, err
				}
				summary.Found++
				continue
			}
			summary.NotFound++
			reason = alert + ", and neither a board found nor Google for Jobs lists the posting"
			if err := finder.hub.RecordPostingTextMissing(ctx, job.ID, reason, &now); err != nil {
				return summary, err
			}
			continue
		}
		if reason != job.TextMissingReason {
			if err := finder.hub.RecordPostingTextMissing(ctx, job.ID, reason, nil); err != nil {
				return summary, err
			}
		}
	}
	return summary, nil
}

// searchText asks Google for Jobs for the job's title at its company, and
// returns the posting of that same title and company, when one has more
// than a snippet of text.
func (finder *Finder) searchText(ctx context.Context, awaiting store.AlertJobAwaitingText) (Posting, bool, error) {
	postings, err := finder.search.Search(ctx, awaiting.Job.Title+" "+awaiting.CompanyName, getCountry(awaiting.Job.URL))
	if err != nil {
		return Posting{}, false, err
	}
	for _, posting := range postings {
		if getTitleKey(posting.Title) == getTitleKey(awaiting.Job.Title) && isSameCompany(posting.EmployerName, awaiting.CompanyName) &&
			len(strings.TrimSpace(posting.Description)) >= store.AlertSnippetLength {
			return posting, true, nil
		}
	}
	return Posting{}, false, nil
}

// alertSenders name the sites whose alerts list the jobs.
var alertSenders = map[string]string{
	store.JobSourceGlassdoor: "Glassdoor", store.JobSourceIndeed: "Indeed", store.JobSourceLinkedIn: "LinkedIn",
}

// describeAlert says what the job's alert gave of its posting.
func describeAlert(job store.Job) string {
	gave := "only a snippet"
	if strings.TrimSpace(job.Description) == "" {
		gave = "no text"
	}
	return "the " + alertSenders[job.Source] + " alert gave " + gave
}

// getCountry returns the country a posting's site is for, from its address:
// "br" for glassdoor.com.br and br.indeed.com, and empty when it doesn't say.
func getCountry(postingURL string) string {
	parsed, err := url.Parse(postingURL)
	if err != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	country := ""
	if index := strings.LastIndex(host, "."); index >= 0 && len(host)-index == 3 {
		country = host[index+1:]
	} else if first, _, found := strings.Cut(host, "."); found && len(first) == 2 {
		country = first
	}
	// Sites name the United Kingdom "uk"; JSearch takes its ISO code.
	if country == "uk" {
		return "gb"
	}
	return country
}

var nonTitleCharacters = regexp.MustCompile(`[^a-z0-9]+`)

// getTitleKey compares titles without case, accents or punctuation.
func getTitleKey(title string) string {
	return nonTitleCharacters.ReplaceAllString(wordmatch.Normalize(title), "")
}

// isSameCompany reports whether two names are one company's, as "Acme" and
// "Acme do Brasil Tecnologia Ltda": equal once normalized, or one the
// other's first words.
func isSameCompany(a, b string) bool {
	a, b = store.NormalizeCompanyName(a), store.NormalizeCompanyName(b)
	if a == "" || b == "" {
		return false
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	return a == b || strings.HasPrefix(b, a+" ")
}
