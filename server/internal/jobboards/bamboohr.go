package jobboards

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/textextract"
)

const BambooHR = "bamboohr"

type bambooHRLocation struct {
	City     string `json:"city"`
	State    string `json:"state"`
	Province string `json:"province"`
	Country  string `json:"country"`
}

func (location bambooHRLocation) describe() string {
	return joinNonEmpty(", ", location.City, location.State, location.Province, location.Country)
}

// fetchBambooHRPostings reads a BambooHR careers site: its list of openings,
// then each opening's details for the text. BambooHR redirects an unknown
// name to its own site, whose page isn't the list, so that's no board.
func (verifier *Verifier) fetchBambooHRPostings(ctx context.Context, boardToken string) ([]store.JobPosting, error) {
	base, err := getTenantBase(verifier.BambooHRAPIBase, boardToken, "bamboohr.com")
	if err != nil {
		return nil, err
	}
	body, found, err := verifier.fetch(ctx, BambooHR, base+"/careers/list")
	if err != nil {
		return nil, err
	}
	var list struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if !found || json.Unmarshal(body, &list) != nil {
		return nil, ErrPostingAPIOff
	}
	postings := make([]store.JobPosting, 0, len(list.Result))
	for _, opening := range list.Result[:min(len(list.Result), maximumDetailedPostings)] {
		detail, found, err := verifier.fetch(ctx, BambooHR, base+"/careers/"+opening.ID+"/detail")
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		posting, isOpen, err := parseBambooHRPosting(opening.ID, detail)
		if err != nil {
			return nil, fmt.Errorf("read the bamboohr opening %s: %w", opening.ID, err)
		}
		if isOpen {
			posting.Raw = detail
			postings = append(postings, posting)
		}
	}
	return postings, nil
}

func parseBambooHRPosting(id string, detail []byte) (store.JobPosting, bool, error) {
	var answer struct {
		Result struct {
			JobOpening struct {
				Name                  string           `json:"jobOpeningName"`
				Status                string           `json:"jobOpeningStatus"`
				ShareURL              string           `json:"jobOpeningShareUrl"`
				Description           string           `json:"description"`
				DepartmentLabel       string           `json:"departmentLabel"`
				EmploymentStatusLabel string           `json:"employmentStatusLabel"`
				Location              bambooHRLocation `json:"location"`
				ATSLocation           bambooHRLocation `json:"atsLocation"`
				IsRemote              *bool            `json:"isRemote"`
				LocationType          string           `json:"locationType"`
				DatePosted            string           `json:"datePosted"`
			} `json:"jobOpening"`
		} `json:"result"`
	}
	if err := json.Unmarshal(detail, &answer); err != nil {
		return store.JobPosting{}, false, err
	}
	opening := answer.Result.JobOpening
	posting := store.JobPosting{
		ExternalID: id, Title: opening.Name, URL: opening.ShareURL, Description: textextract.ConvertHTMLToMarkdown(opening.Description),
		Location: opening.Location.describe(),
	}
	if posting.Location == "" {
		posting.Location = opening.ATSLocation.describe()
	}
	// BambooHR doesn't document locationType. Across seven boards, 1 came
	// without a city, as remote openings do, 2 with one, as hybrid ones do,
	// and 0 was rare and is left unread.
	switch {
	case opening.IsRemote != nil && *opening.IsRemote, opening.LocationType == "1":
		posting.WorkplaceType = "Remote"
	case opening.LocationType == "2":
		posting.WorkplaceType = "Hybrid"
	}
	posting.EmploymentType = opening.EmploymentStatusLabel
	posting.Department = opening.DepartmentLabel
	if published, err := time.Parse("2006-01-02", opening.DatePosted); err == nil {
		posting.PublishedAt = &published
	}
	isOpen := opening.Name != "" && (opening.Status == "" || opening.Status == "Open")
	return posting, isOpen, nil
}
