package jobboards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/textextract"
)

const Gupy = "gupy"

// gupyTalentPool is the type of a Gupy listing that collects résumés for
// later rather than hiring now.
const gupyTalentPool = "vacancy_type_talent_pool"

// nextData is the data a Next.js page renders from, as Gupy's career pages
// embed their jobs.
var nextData = regexp.MustCompile(`(?s)<script id="__NEXT_DATA__"[^>]*>(.*?)</script>`)

type gupyListedJob struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// listGupyJobs reads the open jobs a Gupy career page lists, leaving out
// talent pools. A page without a job list isn't a career page.
func (verifier *Verifier) listGupyJobs(ctx context.Context, base string) ([]gupyListedJob, error) {
	page, found, err := verifier.fetch(ctx, Gupy, base+"/")
	if err != nil {
		return nil, err
	}
	var data struct {
		Props struct {
			PageProps struct {
				Jobs *[]gupyListedJob `json:"jobs"`
			} `json:"pageProps"`
		} `json:"props"`
	}
	if !found || readNextData(page, &data) != nil || data.Props.PageProps.Jobs == nil {
		return nil, ErrPostingAPIOff
	}
	var jobs []gupyListedJob
	for _, job := range *data.Props.PageProps.Jobs {
		if job.ID != 0 && job.Title != "" && job.Type != gupyTalentPool {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// fetchGupyPostings reads a Gupy career page's jobs, then each job's page for
// its text.
func (verifier *Verifier) fetchGupyPostings(ctx context.Context, boardToken string) ([]store.JobPosting, error) {
	base, err := getTenantBase(verifier.GupyBase, boardToken, "gupy.io")
	if err != nil {
		return nil, err
	}
	listed, err := verifier.listGupyJobs(ctx, base)
	if err != nil {
		return nil, err
	}
	postings := make([]store.JobPosting, 0, len(listed))
	for _, job := range listed[:min(len(listed), maximumDetailedPostings)] {
		jobURL := fmt.Sprintf("%s/jobs/%d", base, job.ID)
		page, found, err := verifier.fetch(ctx, Gupy, jobURL)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		posting, isOpen, err := parseGupyJobPage(jobURL, page)
		if err != nil {
			return nil, fmt.Errorf("read the gupy job %d: %w", job.ID, err)
		}
		if isOpen {
			postings = append(postings, posting)
		}
	}
	return postings, nil
}

func parseGupyJobPage(jobURL string, page []byte) (store.JobPosting, bool, error) {
	var data struct {
		Props struct {
			PageProps struct {
				Job json.RawMessage `json:"job"`
			} `json:"pageProps"`
		} `json:"props"`
	}
	if err := readNextData(page, &data); err != nil {
		return store.JobPosting{}, false, err
	}
	var job struct {
		ID                  looseNumber `json:"id"`
		Name                string      `json:"name"`
		Description         string      `json:"description"`
		Responsibilities    string      `json:"responsibilities"`
		Prerequisites       string      `json:"prerequisites"`
		RelevantExperiences string      `json:"relevantExperiences"`
		AddressCity         string      `json:"addressCity"`
		AddressState        string      `json:"addressState"`
		AddressCountry      string      `json:"addressCountry"`
		WorkplaceType       string      `json:"workplaceType"`
		JobType             string      `json:"jobType"`
		Status              string      `json:"status"`
		PublishedAt         string      `json:"publishedAt"`
	}
	if err := json.Unmarshal(data.Props.PageProps.Job, &job); err != nil {
		return store.JobPosting{}, false, err
	}
	if job.ID.value == nil {
		return store.JobPosting{}, false, errors.New("the job page names no job")
	}
	posting := store.JobPosting{
		ExternalID: fmt.Sprintf("%.0f", *job.ID.value), Title: strings.TrimSpace(job.Name), URL: jobURL,
		Description: joinSections(textextract.ConvertHTMLToMarkdown(job.Description), textextract.ConvertHTMLToMarkdown(job.Responsibilities),
			textextract.ConvertHTMLToMarkdown(job.Prerequisites), textextract.ConvertHTMLToMarkdown(job.RelevantExperiences)),
		Location:      joinNonEmpty(", ", job.AddressCity, job.AddressState, job.AddressCountry),
		WorkplaceType: gupyWorkplaceTypes[job.WorkplaceType],
	}
	if published, err := time.Parse(time.RFC3339, job.PublishedAt); err == nil {
		posting.PublishedAt = &published
	}
	isOpen := posting.Title != "" && job.JobType != gupyTalentPool && (job.Status == "" || job.Status == "published")
	return posting, isOpen, nil
}

func readNextData(page []byte, into any) error {
	match := nextData.FindSubmatch(page)
	if match == nil {
		return errors.New("the page has no Next.js data")
	}
	return json.Unmarshal(match[1], into)
}

// gupyWorkplaceTypes spell Gupy's workplace types the way other boards do.
var gupyWorkplaceTypes = map[string]string{"remote": "Remote", "hybrid": "Hybrid", "on-site": "On-site"}
