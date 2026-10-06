package jobboards

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/textextract"
)

const RemoteOK = "remoteok"

// remoteOKPostingLifetime is how long a Remote OK posting stays open after
// the feed last listed it: the feed never says when one closes.
const remoteOKPostingLifetime = 30 * 24 * time.Hour

// FetchRemoteOKPostings reads Remote OK's feed for a tag, such as "react":
// its newest remote postings so tagged, each with its text. The feed's first
// element is its terms of use.
func (verifier *Verifier) FetchRemoteOKPostings(ctx context.Context, tag string) ([]store.JobPosting, error) {
	body, found, err := verifier.fetch(ctx, RemoteOK, verifier.RemoteOKAPIBase+"/api?"+url.Values{"tag": {tag}}.Encode())
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("remote ok has no feed at %s", verifier.RemoteOKAPIBase)
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(body, &elements); err != nil {
		return nil, err
	}
	if len(elements) > 0 {
		elements = elements[1:]
	}
	postings, err := mapEach(elements, parseRemoteOKPosting)
	expiresAt := time.Now().Add(remoteOKPostingLifetime)
	for index := range postings {
		postings[index].ExpiresAt = &expiresAt
	}
	return postings, err
}

func parseRemoteOKPosting(raw json.RawMessage) (store.JobPosting, bool, error) {
	var job struct {
		ID          looseNumber `json:"id"`
		Company     string      `json:"company"`
		Position    string      `json:"position"`
		Location    string      `json:"location"`
		Description string      `json:"description"`
		URL         string      `json:"url"`
		Date        string      `json:"date"`
		SalaryMin   looseNumber `json:"salary_min"`
		SalaryMax   looseNumber `json:"salary_max"`
	}
	if err := json.Unmarshal(raw, &job); err != nil {
		return store.JobPosting{}, false, err
	}
	if job.ID.value == nil {
		return store.JobPosting{}, false, nil
	}
	posting := store.JobPosting{
		ExternalID: fmt.Sprintf("%.0f", *job.ID.value), CompanyName: job.Company, Title: job.Position, Location: job.Location,
		WorkplaceType: "Remote", URL: job.URL, Description: textextract.ConvertHTMLToMarkdown(job.Description),
	}
	if published, err := time.Parse(time.RFC3339, job.Date); err == nil {
		posting.PublishedAt = &published
	}
	minimum, maximum := job.SalaryMin.value, job.SalaryMax.value
	if minimum != nil && *minimum == 0 {
		minimum = nil
	}
	if maximum != nil && *maximum == 0 {
		maximum = nil
	}
	// Remote OK publishes pay as yearly US dollars.
	if minimum != nil || maximum != nil {
		low, high := getBothBounds(minimum, maximum)
		posting.Pay = makePay([]store.PayRange{{Min: low, Max: high, Currency: "USD", Interval: "year"}}, "")
	}
	return posting, posting.Title != "" && posting.CompanyName != "" && posting.URL != "", nil
}
