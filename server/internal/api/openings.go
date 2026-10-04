package api

import (
	"context"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// openJobsPageSize is the page open jobs are read by.
const openJobsPageSize = 500

// companyOpenings are the open jobs at one company, by its normalized name.
type companyOpenings struct {
	companyID *uuid.UUID
	open      int
	fitting   int
	jobs      []opening
}

// opening is an open job as a draft names it.
type opening struct {
	Title string       `json:"title"`
	URL   string       `json:"url"`
	Fit   jobfit.Level `json:"fit"`
}

// getOpeningsByCompany counts every open job, and the ones judged a good
// fit, by the normalized name of its company.
func getOpeningsByCompany(ctx context.Context, hub *store.Store, rateSource exchangeRateSource) (map[string]companyOpenings, error) {
	openings := map[string]companyOpenings{}
	err := walkOpenJobs(ctx, hub, rateSource, func(item store.JobListItem, level jobfit.Level) {
		if item.CompanyName != nil {
			key := store.NormalizeCompanyName(*item.CompanyName)
			openings[key] = countOpening(openings[key], item, level)
		}
	})
	if err != nil {
		return nil, err
	}
	return openings, nil
}

// countOpening adds an open job to a company's openings.
func countOpening(counted companyOpenings, item store.JobListItem, level jobfit.Level) companyOpenings {
	counted.companyID = item.Job.CompanyID
	counted.open++
	if level == jobfit.LevelGood {
		counted.fitting++
	}
	counted.jobs = append(counted.jobs, opening{Title: item.Job.Title, URL: item.Job.URL, Fit: level})
	return counted
}

// walkOpenJobs calls visit with every open job and how well it fits, a page
// at a time.
func walkOpenJobs(ctx context.Context, hub *store.Store, rateSource exchangeRateSource, visit func(store.JobListItem, jobfit.Level)) error {
	criteria, rates, err := jobfit.ReadInputs(ctx, hub, rateSource)
	if err != nil {
		return err
	}
	for offset := 0; ; offset += openJobsPageSize {
		jobs, total, err := hub.ListJobs(ctx, store.JobFilter{Status: store.JobStatusOpen, Limit: openJobsPageSize, Offset: offset})
		if err != nil {
			return err
		}
		for _, item := range jobs {
			visit(item, jobfit.Judge(item.Job, item.Facts, criteria, rates).Level)
		}
		if offset+len(jobs) >= total || len(jobs) == 0 {
			return nil
		}
	}
}
