package jobboards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// ErrPostingAPIOff means the board exists but its provider does not serve its
// postings, as with Ashby customers who turn the posting API off.
var ErrPostingAPIOff = errors.New("the board's posting API is off")

// FetchPostings lists the board's open postings through its provider's
// public API.
func (verifier *Verifier) FetchPostings(ctx context.Context, provider, boardToken string) ([]store.JobPosting, error) {
	escapedToken := url.PathEscape(boardToken)
	var apiURL string
	switch provider {
	case Greenhouse:
		apiURL = verifier.GreenhouseAPIBase + "/v1/boards/" + escapedToken + "/jobs?content=true&pay_transparency=true"
	case Lever:
		apiURL = verifier.LeverAPIBase + "/v0/postings/" + escapedToken + "?mode=json"
	case Ashby:
		apiURL = verifier.AshbyAPIBase + "/posting-api/job-board/" + escapedToken + "?includeCompensation=true"
	default:
		return nil, ErrUnsupportedProvider
	}

	body, found, err := verifier.fetch(ctx, provider, apiURL)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrPostingAPIOff
	}
	postings, err := parsePostings(provider, body)
	if err != nil {
		return nil, fmt.Errorf("read the %s board %q: %w", provider, boardToken, err)
	}
	return postings, nil
}

func parsePostings(provider string, body []byte) ([]store.JobPosting, error) {
	switch provider {
	case Greenhouse:
		var board struct {
			Jobs []json.RawMessage `json:"jobs"`
		}
		if err := json.Unmarshal(body, &board); err != nil {
			return nil, err
		}
		return mapEach(board.Jobs, parseGreenhousePosting)
	case Lever:
		var postings []json.RawMessage
		if err := json.Unmarshal(body, &postings); err != nil {
			return nil, err
		}
		return mapEach(postings, parseLeverPosting)
	default:
		var board struct {
			Jobs []json.RawMessage `json:"jobs"`
		}
		if err := json.Unmarshal(body, &board); err != nil {
			return nil, err
		}
		return mapEach(board.Jobs, parseAshbyPosting)
	}
}

// mapEach parses each raw posting, keeping those the parser says are open.
func mapEach(raws []json.RawMessage, parse func(json.RawMessage) (store.JobPosting, bool, error)) ([]store.JobPosting, error) {
	postings := make([]store.JobPosting, 0, len(raws))
	for _, raw := range raws {
		posting, isOpen, err := parse(raw)
		if err != nil {
			return nil, err
		}
		if !isOpen {
			continue
		}
		posting.Raw = raw
		postings = append(postings, posting)
	}
	return postings, nil
}

func parseGreenhousePosting(raw json.RawMessage) (store.JobPosting, bool, error) {
	var job struct {
		ID             int64      `json:"id"`
		Title          string     `json:"title"`
		AbsoluteURL    string     `json:"absolute_url"`
		Content        string     `json:"content"`
		FirstPublished *time.Time `json:"first_published"`
		Location       struct {
			Name string `json:"name"`
		} `json:"location"`
		PayInputRanges []struct {
			MinCents     float64 `json:"min_cents"`
			MaxCents     float64 `json:"max_cents"`
			CurrencyType string  `json:"currency_type"`
			Title        string  `json:"title"`
		} `json:"pay_input_ranges"`
		Departments []struct {
			Name string `json:"name"`
		} `json:"departments"`
		Offices []struct {
			Name string `json:"name"`
		} `json:"offices"`
		Metadata []struct {
			Name  string          `json:"name"`
			Value json.RawMessage `json:"value"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &job); err != nil {
		return store.JobPosting{}, false, err
	}
	posting := store.JobPosting{
		ExternalID:  strconv.FormatInt(job.ID, 10),
		Title:       job.Title,
		Location:    job.Location.Name,
		URL:         job.AbsoluteURL,
		Description: convertHTMLToText(html.UnescapeString(job.Content)),
	}
	posting.PublishedAt = job.FirstPublished
	var ranges []store.PayRange
	for _, payRange := range job.PayInputRanges {
		// Greenhouse publishes cents and does not say the interval.
		ranges = append(ranges, store.PayRange{
			Label: payRange.Title, Min: payRange.MinCents / 100, Max: payRange.MaxCents / 100, Currency: payRange.CurrencyType,
		})
	}
	posting.Pay = makePay(ranges, "")
	if len(job.Departments) > 0 {
		posting.Department = job.Departments[0].Name
	}
	for _, office := range job.Offices {
		posting.OtherLocations = appendOtherLocation(posting.OtherLocations, job.Location.Name, office.Name)
	}
	for _, field := range job.Metadata {
		var value string
		if field.Name == "Employment Type" && json.Unmarshal(field.Value, &value) == nil {
			posting.EmploymentType = value
		}
	}
	return posting, true, nil
}

func parseLeverPosting(raw json.RawMessage) (store.JobPosting, bool, error) {
	var posting struct {
		ID               string `json:"id"`
		Text             string `json:"text"`
		HostedURL        string `json:"hostedUrl"`
		WorkplaceType    string `json:"workplaceType"`
		DescriptionPlain string `json:"descriptionPlain"`
		AdditionalPlain  string `json:"additionalPlain"`
		CreatedAt        int64  `json:"createdAt"`
		Categories       struct {
			Location     string   `json:"location"`
			AllLocations []string `json:"allLocations"`
			Commitment   string   `json:"commitment"`
			Department   string   `json:"department"`
			Team         string   `json:"team"`
		} `json:"categories"`
		SalaryRange *struct {
			Min      float64 `json:"min"`
			Max      float64 `json:"max"`
			Currency string  `json:"currency"`
			Interval string  `json:"interval"`
		} `json:"salaryRange"`
		Lists []struct {
			Text    string `json:"text"`
			Content string `json:"content"`
		} `json:"lists"`
	}
	if err := json.Unmarshal(raw, &posting); err != nil {
		return store.JobPosting{}, false, err
	}
	sections := []string{posting.DescriptionPlain}
	for _, list := range posting.Lists {
		sections = append(sections, list.Text+"\n"+convertHTMLToText(list.Content))
	}
	sections = append(sections, posting.AdditionalPlain)
	parsed := store.JobPosting{
		ExternalID:    posting.ID,
		Title:         posting.Text,
		Location:      posting.Categories.Location,
		WorkplaceType: posting.WorkplaceType,
		URL:           posting.HostedURL,
		Description:   strings.TrimSpace(strings.Join(sections, "\n\n")),
	}
	parsed.EmploymentType = posting.Categories.Commitment
	parsed.Department = posting.Categories.Department
	if parsed.Department == "" {
		parsed.Department = posting.Categories.Team
	}
	for _, location := range posting.Categories.AllLocations {
		parsed.OtherLocations = appendOtherLocation(parsed.OtherLocations, posting.Categories.Location, location)
	}
	if posting.CreatedAt > 0 {
		createdAt := time.UnixMilli(posting.CreatedAt).UTC()
		parsed.PublishedAt = &createdAt
	}
	if salary := posting.SalaryRange; salary != nil {
		parsed.Pay = makePay([]store.PayRange{{
			Min: salary.Min, Max: salary.Max, Currency: salary.Currency, Interval: convertPayIntervalToUnit(salary.Interval),
		}}, "")
	}
	return parsed, true, nil
}

// parseAshbyPosting reports an unlisted posting as not open: Ashby hides it
// from the company's own board.
func parseAshbyPosting(raw json.RawMessage) (store.JobPosting, bool, error) {
	var job struct {
		ID                 string     `json:"id"`
		Title              string     `json:"title"`
		Location           string     `json:"location"`
		WorkplaceType      string     `json:"workplaceType"`
		JobURL             string     `json:"jobUrl"`
		DescriptionPlain   string     `json:"descriptionPlain"`
		IsListed           bool       `json:"isListed"`
		EmploymentType     string     `json:"employmentType"`
		Department         string     `json:"department"`
		PublishedAt        *time.Time `json:"publishedAt"`
		SecondaryLocations []struct {
			Location string `json:"location"`
		} `json:"secondaryLocations"`
		Compensation *struct {
			CompensationTierSummary string `json:"compensationTierSummary"`
			CompensationTiers       []struct {
				Title      *string `json:"title"`
				Components []struct {
					CompensationType string   `json:"compensationType"`
					Interval         string   `json:"interval"`
					CurrencyCode     *string  `json:"currencyCode"`
					MinValue         *float64 `json:"minValue"`
					MaxValue         *float64 `json:"maxValue"`
				} `json:"components"`
			} `json:"compensationTiers"`
		} `json:"compensation"`
	}
	if err := json.Unmarshal(raw, &job); err != nil {
		return store.JobPosting{}, false, err
	}
	posting := store.JobPosting{
		ExternalID:    job.ID,
		Title:         job.Title,
		Location:      job.Location,
		WorkplaceType: job.WorkplaceType,
		URL:           job.JobURL,
		Description:   job.DescriptionPlain,
	}
	posting.EmploymentType = ashbyEmploymentTypes[job.EmploymentType]
	posting.Department = job.Department
	posting.PublishedAt = job.PublishedAt
	for _, secondary := range job.SecondaryLocations {
		posting.OtherLocations = appendOtherLocation(posting.OtherLocations, job.Location, secondary.Location)
	}
	if compensation := job.Compensation; compensation != nil {
		var ranges []store.PayRange
		for _, tier := range compensation.CompensationTiers {
			for _, component := range tier.Components {
				if component.CompensationType != "Salary" || component.CurrencyCode == nil || (component.MinValue == nil && component.MaxValue == nil) {
					continue
				}
				payRange := store.PayRange{Currency: *component.CurrencyCode, Interval: convertPayIntervalToUnit(component.Interval)}
				if tier.Title != nil {
					payRange.Label = *tier.Title
				}
				payRange.Min, payRange.Max = getBothBounds(component.MinValue, component.MaxValue)
				ranges = append(ranges, payRange)
			}
		}
		posting.Pay = makePay(ranges, compensation.CompensationTierSummary)
	}
	return posting, job.IsListed, nil
}

// ashbyEmploymentTypes spells Ashby's employment types the way Greenhouse and
// Lever boards usually do.
var ashbyEmploymentTypes = map[string]string{
	"FullTime": "Full-time", "PartTime": "Part-time", "Intern": "Intern", "Contract": "Contract", "Temporary": "Temporary",
}

// makePay returns nil when a posting publishes neither a range nor a summary.
func makePay(ranges []store.PayRange, summary string) *store.Pay {
	if len(ranges) == 0 && summary == "" {
		return nil
	}
	return &store.Pay{Ranges: ranges, Summary: summary}
}

// getBothBounds fills a range that publishes one bound with that bound.
func getBothBounds(minimum, maximum *float64) (float64, float64) {
	switch {
	case minimum == nil:
		return *maximum, *maximum
	case maximum == nil:
		return *minimum, *minimum
	default:
		return *minimum, *maximum
	}
}

// payIntervalWords map the words boards use for a pay period to its unit:
// Ashby's "1 YEAR", Lever's "per-year-salary", Himalayas' "annual".
var payIntervalWords = []struct{ word, unit string }{
	{"year", "year"}, {"annual", "year"}, {"month", "month"}, {"week", "week"}, {"daily", "day"}, {"day", "day"}, {"hour", "hour"},
}

// convertPayIntervalToUnit reads a board's interval as year, month, week,
// day or hour, or "" when it names none.
func convertPayIntervalToUnit(interval string) string {
	lowered := strings.ToLower(interval)
	for _, candidate := range payIntervalWords {
		if strings.Contains(lowered, candidate.word) {
			return candidate.unit
		}
	}
	return ""
}

// appendOtherLocation adds a location unless it is the posting's main one or
// already listed; boards repeat the main location among the others.
func appendOtherLocation(otherLocations []string, mainLocation, location string) []string {
	location = strings.TrimSpace(location)
	if location == "" || strings.EqualFold(location, strings.TrimSpace(mainLocation)) {
		return otherLocations
	}
	for _, listed := range otherLocations {
		if strings.EqualFold(listed, location) {
			return otherLocations
		}
	}
	return append(otherLocations, location)
}

var (
	blockEndTag   = regexp.MustCompile(`(?i)</(p|div|li|h[1-6]|ul|ol)>|<br\s*/?>`)
	anyTag        = regexp.MustCompile(`<[^>]*>`)
	blankLineRuns = regexp.MustCompile(`\n{3,}`)
)

// convertHTMLToText keeps a description readable as plain text: block ends
// become line breaks, other tags are dropped, entities are decoded.
func convertHTMLToText(markup string) string {
	withBreaks := blockEndTag.ReplaceAllString(markup, "\n")
	text := html.UnescapeString(anyTag.ReplaceAllString(withBreaks, ""))
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		lines[index] = strings.TrimSpace(line)
	}
	return strings.TrimSpace(blankLineRuns.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}
