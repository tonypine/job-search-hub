package jobboards

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

const Recruitee = "recruitee"

// fetchRecruiteePostings reads a Recruitee board's offers, each with its full
// text.
func (verifier *Verifier) fetchRecruiteePostings(ctx context.Context, boardToken string) ([]store.JobPosting, error) {
	base, err := getTenantBase(verifier.RecruiteeAPIBase, boardToken, "recruitee.com")
	if err != nil {
		return nil, err
	}
	body, found, err := verifier.fetch(ctx, Recruitee, base+"/api/offers/")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrPostingAPIOff
	}
	var answer struct {
		Offers []json.RawMessage `json:"offers"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return nil, err
	}
	return mapEach(answer.Offers, parseRecruiteePosting)
}

func parseRecruiteePosting(raw json.RawMessage) (store.JobPosting, bool, error) {
	var offer struct {
		ID                 int64  `json:"id"`
		Title              string `json:"title"`
		CareersURL         string `json:"careers_url"`
		Description        string `json:"description"`
		Requirements       string `json:"requirements"`
		Status             string `json:"status"`
		Remote             bool   `json:"remote"`
		Hybrid             bool   `json:"hybrid"`
		EmploymentTypeCode string `json:"employment_type_code"`
		Department         string `json:"department"`
		PublishedAt        string `json:"published_at"`
		Locations          []struct {
			City    string `json:"city"`
			State   string `json:"state"`
			Country string `json:"country"`
		} `json:"locations"`
		Salary struct {
			Min      looseNumber `json:"min"`
			Max      looseNumber `json:"max"`
			Period   string      `json:"period"`
			Currency string      `json:"currency"`
		} `json:"salary"`
	}
	if err := json.Unmarshal(raw, &offer); err != nil {
		return store.JobPosting{}, false, err
	}
	posting := store.JobPosting{
		ExternalID: strconv.FormatInt(offer.ID, 10), Title: offer.Title, URL: offer.CareersURL,
		Description: joinSections(convertHTMLToText(offer.Description), convertHTMLToText(offer.Requirements)),
	}
	for _, location := range offer.Locations {
		place := joinNonEmpty(", ", strings.TrimSpace(location.City), location.State, location.Country)
		if posting.Location == "" {
			posting.Location = place
			continue
		}
		posting.OtherLocations = appendOtherLocation(posting.OtherLocations, posting.Location, place)
	}
	switch {
	case offer.Remote:
		posting.WorkplaceType = "Remote"
	case offer.Hybrid:
		posting.WorkplaceType = "Hybrid"
	}
	posting.EmploymentType = recruiteeEmploymentTypes[offer.EmploymentTypeCode]
	posting.Department = offer.Department
	if published, err := time.Parse("2006-01-02 15:04:05 MST", offer.PublishedAt); err == nil {
		posting.PublishedAt = &published
	}
	if offer.Salary.Min.value != nil || offer.Salary.Max.value != nil {
		minimum, maximum := getBothBounds(offer.Salary.Min.value, offer.Salary.Max.value)
		posting.Pay = makePay([]store.PayRange{{Min: minimum, Max: maximum, Currency: offer.Salary.Currency,
			Interval: convertPayIntervalToUnit(offer.Salary.Period)}}, "")
	}
	isOpen := offer.ID != 0 && offer.Title != "" && (offer.Status == "" || offer.Status == "published")
	return posting, isOpen, nil
}

// recruiteeEmploymentTypes spell Recruitee's codes the way other boards do.
var recruiteeEmploymentTypes = map[string]string{
	"fulltime": "Full-time", "fulltime_permanent": "Full-time", "fulltime_fixed_term": "Full-time", "parttime": "Part-time",
	"parttime_permanent": "Part-time", "parttime_fixed_term": "Part-time", "contract": "Contract", "freelance": "Contract",
	"internship": "Intern", "temporary": "Temporary",
}
