package jobboards

import (
	"context"
	"encoding/xml"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/textextract"
)

const Personio = "personio"

type personioPosition struct {
	ID                string   `xml:"id"`
	Name              string   `xml:"name"`
	Office            string   `xml:"office"`
	AdditionalOffices []string `xml:"additionalOffices>office"`
	Department        string   `xml:"department"`
	Schedule          string   `xml:"schedule"`
	CreatedAt         string   `xml:"createdAt"`
	JobDescriptions   []struct {
		Name  string `xml:"name"`
		Value string `xml:"value"`
	} `xml:"jobDescriptions>jobDescription"`
	Salary *struct {
		Min          float64 `xml:"min"`
		Max          float64 `xml:"max"`
		CurrencyCode string  `xml:"currencyCode"`
		Type         string  `xml:"type"`
	} `xml:"salaryInformation"`
}

// fetchPersonioPostings reads a Personio board's XML feed, which lists every
// position with its text.
func (verifier *Verifier) fetchPersonioPostings(ctx context.Context, boardToken string) ([]store.JobPosting, error) {
	base, err := getTenantBase(verifier.PersonioAPIBase, boardToken, "jobs.personio.com")
	if err != nil {
		return nil, err
	}
	body, found, err := verifier.fetch(ctx, Personio, base+"/xml")
	if err != nil {
		return nil, err
	}
	var feed struct {
		XMLName   xml.Name           `xml:"workzag-jobs"`
		Positions []personioPosition `xml:"position"`
	}
	if !found || xml.Unmarshal(body, &feed) != nil {
		return nil, ErrPostingAPIOff
	}
	postings := make([]store.JobPosting, 0, len(feed.Positions))
	for _, position := range feed.Positions {
		if position.ID == "" || position.Name == "" {
			continue
		}
		postings = append(postings, convertPersonioPosition(base, position))
	}
	return postings, nil
}

func convertPersonioPosition(base string, position personioPosition) store.JobPosting {
	var sections []string
	for _, description := range position.JobDescriptions {
		sections = append(sections, formatSection(description.Name, textextract.ConvertHTMLToMarkdown(description.Value)))
	}
	posting := store.JobPosting{
		ExternalID: position.ID, Title: position.Name, URL: base + "/job/" + position.ID, Description: joinSections(sections...),
		Location: position.Office,
	}
	for _, office := range position.AdditionalOffices {
		posting.OtherLocations = appendOtherLocation(posting.OtherLocations, posting.Location, office)
	}
	for _, office := range append([]string{position.Office}, position.AdditionalOffices...) {
		if strings.EqualFold(strings.TrimSpace(office), "remote") {
			posting.WorkplaceType = "Remote"
		}
	}
	posting.EmploymentType = personioSchedules[position.Schedule]
	posting.Department = position.Department
	if created, err := time.Parse(time.RFC3339, position.CreatedAt); err == nil {
		posting.PublishedAt = &created
	}
	if salary := position.Salary; salary != nil && (salary.Min > 0 || salary.Max > 0) {
		posting.Pay = makePay([]store.PayRange{{Min: salary.Min, Max: salary.Max, Currency: salary.CurrencyCode,
			Interval: convertPayIntervalToUnit(salary.Type)}}, "")
	}
	return posting
}

// personioSchedules spell Personio's schedules the way other boards do.
var personioSchedules = map[string]string{"full-time": "Full-time", "part-time": "Part-time", "full-or-part-time": "Full-time"}
