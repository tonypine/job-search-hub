package linkedinexport

import (
	"errors"
	"io"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// ErrNotJobsFile means the file lacks the columns of Job Applications.csv
// or Saved Jobs.csv.
var ErrNotJobsFile = errors.New(`not a LinkedIn Job Applications.csv or Saved Jobs.csv: no "Job Url" column`)

// ParseJobApplications reads Jobs/Job Applications.csv: the jobs the owner
// applied to through LinkedIn. A row without a URL, a title or a date can't
// become a job and is skipped.
func ParseJobApplications(file io.Reader) (jobs []store.NewLinkedInJob, skipped int, err error) {
	return parseJobs(file, store.LinkedInJobApplied, "Application Date")
}

// ParseSavedJobs reads Jobs/Saved Jobs.csv: the jobs the owner saved.
func ParseSavedJobs(file io.Reader) (jobs []store.NewLinkedInJob, skipped int, err error) {
	return parseJobs(file, store.LinkedInJobSaved, "Saved Date")
}

func parseJobs(file io.Reader, kind, dateColumn string) (jobs []store.NewLinkedInJob, skipped int, err error) {
	rows, columns, err := readTable(file, "Job Url")
	if errors.Is(err, errMissingColumn) {
		return nil, 0, ErrNotJobsFile
	}
	if err != nil {
		return nil, 0, err
	}
	for _, row := range rows {
		field := columns.reader(row)
		at, err := time.Parse(invitationDateLayout, field(dateColumn))
		if field("Job Url") == "" || field("Job Title") == "" || err != nil {
			skipped++
			continue
		}
		jobs = append(jobs, store.NewLinkedInJob{
			Kind: kind, At: at, URL: field("Job Url"), Title: field("Job Title"), CompanyName: field("Company Name"),
		})
	}
	return jobs, skipped, nil
}
