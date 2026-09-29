package mcptools

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

const (
	maximumCareersPageJobs        = 300
	maximumCareersPageDescription = 40_000
)

type recordCompanyJobsInput struct {
	CompanyID uuid.UUID        `json:"company_id"`
	Jobs      []careersPageJob `json:"jobs" jsonschema:"every open role the careers page lists now, the whole list: a role recorded before and left out now is closed"`
}

type careersPageJob struct {
	Title          string `json:"title"`
	URL            string `json:"url" jsonschema:"the role's own page, where the candidate reads it and applies"`
	Location       string `json:"location,omitempty"`
	WorkplaceType  string `json:"workplace_type,omitempty" jsonschema:"Remote, Hybrid or On-site, when the page says"`
	EmploymentType string `json:"employment_type,omitempty" jsonschema:"e.g. Full-time or Contract, when the page says"`
	Description    string `json:"description,omitempty" jsonschema:"the role's text as its page shows it, in plain text; empty when only the list was read"`
}

func addCareersPageTools(server *mcp.Server, hub *store.Store) {
	addTool(server, &mcp.Tool{
		Name: "record_company_jobs",
		Description: "Record the open roles a company's careers page lists, for a company whose roles sit on no job board the hub reads " +
			"(set_job_board covers those). Pass the whole list each time: roles recorded before and missing now are closed, and a role " +
			"the hub already lists for the company from another source is left out. Never pass linkedin.com links.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input recordCompanyJobsInput) (*mcp.CallToolResult, store.CareersPageSyncResult, error) {
		actor, err := tokens.GetActor(ctx)
		if err != nil {
			return nil, store.CareersPageSyncResult{}, err
		}
		postings, err := convertCareersPageJobs(input.Jobs)
		if err != nil {
			return nil, store.CareersPageSyncResult{}, err
		}
		result, err := hub.SyncCareersPageJobs(ctx, actor, input.CompanyID, postings, time.Now())
		return nil, result, err
	})
}

// convertCareersPageJobs checks each role the agent read and turns it into a
// posting known by its link.
func convertCareersPageJobs(jobs []careersPageJob) ([]store.JobPosting, error) {
	if len(jobs) > maximumCareersPageJobs {
		return nil, fmt.Errorf("at most %d roles at once; a careers page listing more is best read through its job board", maximumCareersPageJobs)
	}
	seen := map[string]bool{}
	postings := make([]store.JobPosting, 0, len(jobs))
	for index, job := range jobs {
		title, link := strings.TrimSpace(job.Title), strings.TrimSpace(job.URL)
		parsed, err := url.Parse(link)
		switch {
		case title == "":
			return nil, fmt.Errorf("role %d has no title", index+1)
		case err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "":
			return nil, fmt.Errorf("role %q needs its page's full link", title)
		case strings.HasSuffix(strings.ToLower(parsed.Hostname()), "linkedin.com"):
			return nil, errors.New("linkedin.com links are never recorded; use the role's page on the company's own site")
		}
		if seen[link] {
			continue
		}
		seen[link] = true
		description := job.Description
		if len(description) > maximumCareersPageDescription {
			description = strings.ToValidUTF8(description[:maximumCareersPageDescription], "")
		}
		postings = append(postings, store.JobPosting{
			ExternalID: link, Title: title, URL: link, Location: strings.TrimSpace(job.Location),
			WorkplaceType: strings.TrimSpace(job.WorkplaceType), Description: strings.TrimSpace(description),
			BoardFacts: store.BoardFacts{EmploymentType: strings.TrimSpace(job.EmploymentType)},
		})
	}
	return postings, nil
}
