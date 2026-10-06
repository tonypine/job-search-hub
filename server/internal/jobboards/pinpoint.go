package jobboards

import (
	"context"
	"encoding/json"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/textextract"
)

const Pinpoint = "pinpoint"

// fetchPinpointPostings reads a Pinpoint board's postings, each with its
// text.
func (verifier *Verifier) fetchPinpointPostings(ctx context.Context, boardToken string) ([]store.JobPosting, error) {
	base, err := getTenantBase(verifier.PinpointAPIBase, boardToken, "pinpointhq.com")
	if err != nil {
		return nil, err
	}
	body, found, err := verifier.fetch(ctx, Pinpoint, base+"/postings.json")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrPostingAPIOff
	}
	var answer struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return nil, err
	}
	return mapEach(answer.Data, parsePinpointPosting)
}

func parsePinpointPosting(raw json.RawMessage) (store.JobPosting, bool, error) {
	var posting struct {
		ID                       string      `json:"id"`
		Title                    string      `json:"title"`
		URL                      string      `json:"url"`
		Description              string      `json:"description"`
		KeyResponsibilitiesTitle string      `json:"key_responsibilities_header"`
		KeyResponsibilities      string      `json:"key_responsibilities"`
		SkillsTitle              string      `json:"skills_knowledge_expertise_header"`
		Skills                   string      `json:"skills_knowledge_expertise"`
		BenefitsTitle            string      `json:"benefits_header"`
		Benefits                 string      `json:"benefits"`
		WorkplaceType            string      `json:"workplace_type"`
		EmploymentType           string      `json:"employment_type_text"`
		CompensationMinimum      looseNumber `json:"compensation_minimum"`
		CompensationMaximum      looseNumber `json:"compensation_maximum"`
		CompensationCurrency     string      `json:"compensation_currency"`
		CompensationFrequency    string      `json:"compensation_frequency"`
		CompensationVisible      bool        `json:"compensation_visible"`
		Location                 struct {
			Name     string `json:"name"`
			City     string `json:"city"`
			Province string `json:"province"`
		} `json:"location"`
		Job struct {
			Department struct {
				Name string `json:"name"`
			} `json:"department"`
		} `json:"job"`
	}
	if err := json.Unmarshal(raw, &posting); err != nil {
		return store.JobPosting{}, false, err
	}
	job := store.JobPosting{
		ExternalID: posting.ID, Title: posting.Title, URL: posting.URL,
		Description: joinSections(textextract.ConvertHTMLToMarkdown(posting.Description),
			formatSection(posting.KeyResponsibilitiesTitle, textextract.ConvertHTMLToMarkdown(posting.KeyResponsibilities)),
			formatSection(posting.SkillsTitle, textextract.ConvertHTMLToMarkdown(posting.Skills)),
			formatSection(posting.BenefitsTitle, textextract.ConvertHTMLToMarkdown(posting.Benefits))),
		Location: joinNonEmpty(", ", posting.Location.City, posting.Location.Province),
	}
	if job.Location == "" {
		job.Location = posting.Location.Name
	}
	job.WorkplaceType = pinpointWorkplaceTypes[posting.WorkplaceType]
	job.EmploymentType = posting.EmploymentType
	job.Department = posting.Job.Department.Name
	minimum, maximum := posting.CompensationMinimum.value, posting.CompensationMaximum.value
	if posting.CompensationVisible && (minimum != nil || maximum != nil) {
		low, high := getBothBounds(minimum, maximum)
		job.Pay = makePay([]store.PayRange{{Min: low, Max: high, Currency: posting.CompensationCurrency,
			Interval: convertPayIntervalToUnit(posting.CompensationFrequency)}}, "")
	}
	return job, posting.ID != "" && posting.Title != "", nil
}

// pinpointWorkplaceTypes spell Pinpoint's workplace types the way other
// boards do.
var pinpointWorkplaceTypes = map[string]string{"remote": "Remote", "hybrid": "Hybrid", "onsite": "On-site"}
