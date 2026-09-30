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
	criteria, rates, err := jobfit.ReadInputs(ctx, hub, rateSource)
	if err != nil {
		return nil, err
	}
	openings := map[string]companyOpenings{}
	for offset := 0; ; offset += openJobsPageSize {
		jobs, total, err := hub.ListJobs(ctx, store.JobFilter{Status: store.JobStatusOpen, Limit: openJobsPageSize, Offset: offset})
		if err != nil {
			return nil, err
		}
		for _, item := range jobs {
			if item.CompanyName == nil {
				continue
			}
			key := store.NormalizeCompanyName(*item.CompanyName)
			counted := openings[key]
			counted.companyID = item.Job.CompanyID
			counted.open++
			level := jobfit.Judge(item.Job, item.Facts, criteria, rates).Level
			if level == jobfit.LevelGood {
				counted.fitting++
			}
			counted.jobs = append(counted.jobs, opening{Title: item.Job.Title, URL: item.Job.URL, Fit: level})
			openings[key] = counted
		}
		if offset+len(jobs) >= total || len(jobs) == 0 {
			return openings, nil
		}
	}
}
