package jobboards

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/textextract"
)

// Himalayas is the remote-jobs feed searched for postings open to the owner.
const Himalayas = "himalayas"

// listedLocationLimit is how many countries a posting's location names
// before it reads as a count, with the full list kept as its other locations.
const listedLocationLimit = 3

// HimalayasSearch is one search of the feed: postings matching Term that can
// hire from Country, published after PublishedAfter, newest first, reading at
// most MaximumPages pages of 20.
type HimalayasSearch struct {
	Term           string
	Country        string
	PublishedAfter time.Time
	MaximumPages   int
}

// SearchHimalayas returns the search's postings. Paging stops at the first
// page with nothing published after PublishedAfter, or at MaximumPages.
func (verifier *Verifier) SearchHimalayas(ctx context.Context, search HimalayasSearch) ([]store.JobPosting, error) {
	var postings []store.JobPosting
	for page := 1; page <= search.MaximumPages; page++ {
		query := url.Values{"q": {search.Term}, "country": {search.Country}, "sort": {"recent"}, "page": {strconv.Itoa(page)}}
		body, found, err := verifier.fetch(ctx, Himalayas, verifier.HimalayasAPIBase+"/jobs/api/search?"+query.Encode())
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("himalayas has no search API at %s", verifier.HimalayasAPIBase)
		}
		var answer struct {
			Jobs []json.RawMessage `json:"jobs"`
		}
		if err := json.Unmarshal(body, &answer); err != nil {
			return nil, fmt.Errorf("read the himalayas search for %q: %w", search.Term, err)
		}
		recentOnPage := 0
		for _, raw := range answer.Jobs {
			posting, err := parseHimalayasPosting(raw)
			if err != nil {
				return nil, fmt.Errorf("read a himalayas posting: %w", err)
			}
			if posting.PublishedAt == nil || posting.PublishedAt.Before(search.PublishedAfter) {
				continue
			}
			recentOnPage++
			postings = append(postings, posting)
		}
		if len(answer.Jobs) == 0 || recentOnPage == 0 {
			break
		}
	}
	return postings, nil
}

var himalayasEmploymentTypes = map[string]string{
	"Full Time": "Full-time", "Part Time": "Part-time", "Contractor": "Contract", "Intern": "Intern", "Temporary": "Temporary",
}

func parseHimalayasPosting(raw json.RawMessage) (store.JobPosting, error) {
	var job struct {
		GUID                 string   `json:"guid"`
		Title                string   `json:"title"`
		CompanyName          string   `json:"companyName"`
		ApplicationLink      string   `json:"applicationLink"`
		Description          string   `json:"description"`
		EmploymentType       string   `json:"employmentType"`
		MinSalary            *float64 `json:"minSalary"`
		MaxSalary            *float64 `json:"maxSalary"`
		Currency             string   `json:"currency"`
		SalaryPeriod         string   `json:"salaryPeriod"`
		LocationRestrictions []string `json:"locationRestrictions"`
		PubDate              int64    `json:"pubDate"`
		ExpiryDate           int64    `json:"expiryDate"`
	}
	if err := json.Unmarshal(raw, &job); err != nil {
		return store.JobPosting{}, err
	}
	posting := store.JobPosting{
		ExternalID:    job.GUID,
		CompanyName:   job.CompanyName,
		Title:         job.Title,
		WorkplaceType: "Remote",
		URL:           job.ApplicationLink,
		Description:   textextract.ConvertHTMLToMarkdown(job.Description),
		Raw:           raw,
	}
	switch restrictions := job.LocationRestrictions; {
	case len(restrictions) == 0:
		posting.Location = "Worldwide"
	case len(restrictions) <= listedLocationLimit:
		posting.Location = strings.Join(restrictions, ", ")
	default:
		posting.Location = fmt.Sprintf("%d countries", len(restrictions))
		posting.OtherLocations = restrictions
	}
	if employmentType, known := himalayasEmploymentTypes[job.EmploymentType]; known {
		posting.EmploymentType = employmentType
	} else {
		posting.EmploymentType = job.EmploymentType
	}
	if (job.MinSalary != nil || job.MaxSalary != nil) && job.Currency != "" {
		payRange := store.PayRange{Currency: job.Currency, Interval: convertPayIntervalToUnit(job.SalaryPeriod)}
		payRange.Min, payRange.Max = getBothBounds(job.MinSalary, job.MaxSalary)
		posting.Pay = makePay([]store.PayRange{payRange}, "")
	}
	if job.PubDate > 0 {
		publishedAt := time.Unix(job.PubDate, 0).UTC()
		posting.PublishedAt = &publishedAt
	}
	if job.ExpiryDate > 0 {
		expiresAt := time.Unix(job.ExpiryDate, 0).UTC()
		posting.ExpiresAt = &expiresAt
	}
	return posting, nil
}
