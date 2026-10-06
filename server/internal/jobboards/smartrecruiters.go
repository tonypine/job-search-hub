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

const SmartRecruiters = "smartrecruiters"

// fetchSmartRecruitersPostings reads a SmartRecruiters company's postings,
// then each posting's job ad for the text. SmartRecruiters answers any name
// with a list, empty for a company it doesn't know.
func (verifier *Verifier) fetchSmartRecruitersPostings(ctx context.Context, boardToken string) ([]store.JobPosting, error) {
	companyURL := verifier.SmartRecruitersAPIBase + "/v1/companies/" + url.PathEscape(boardToken) + "/postings"
	body, found, err := verifier.fetch(ctx, SmartRecruiters, companyURL+"?limit=100")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrPostingAPIOff
	}
	var list struct {
		Content []struct {
			ID string `json:"id"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	postings := make([]store.JobPosting, 0, len(list.Content))
	for _, listed := range list.Content[:min(len(list.Content), maximumDetailedPostings)] {
		detail, found, err := verifier.fetch(ctx, SmartRecruiters, companyURL+"/"+url.PathEscape(listed.ID))
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		posting, isOpen, err := parseSmartRecruitersPosting(detail)
		if err != nil {
			return nil, fmt.Errorf("read the smartrecruiters posting %s: %w", listed.ID, err)
		}
		if isOpen {
			posting.Raw = detail
			postings = append(postings, posting)
		}
	}
	return postings, nil
}

type smartRecruitersSection struct {
	Text string `json:"text"`
}

func parseSmartRecruitersPosting(detail []byte) (store.JobPosting, bool, error) {
	var posting struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		PostingURL string `json:"postingUrl"`
		Active     *bool  `json:"active"`
		Location   struct {
			FullLocation string `json:"fullLocation"`
			City         string `json:"city"`
			Region       string `json:"region"`
			Country      string `json:"country"`
			Remote       bool   `json:"remote"`
			Hybrid       bool   `json:"hybrid"`
		} `json:"location"`
		TypeOfEmployment struct {
			Label string `json:"label"`
		} `json:"typeOfEmployment"`
		Department struct {
			Label string `json:"label"`
		} `json:"department"`
		ReleasedDate string `json:"releasedDate"`
		JobAd        struct {
			Sections struct {
				CompanyDescription    smartRecruitersSection `json:"companyDescription"`
				JobDescription        smartRecruitersSection `json:"jobDescription"`
				Qualifications        smartRecruitersSection `json:"qualifications"`
				AdditionalInformation smartRecruitersSection `json:"additionalInformation"`
			} `json:"sections"`
		} `json:"jobAd"`
	}
	if err := json.Unmarshal(detail, &posting); err != nil {
		return store.JobPosting{}, false, err
	}
	sections := posting.JobAd.Sections
	job := store.JobPosting{
		ExternalID: posting.ID, Title: posting.Name, URL: posting.PostingURL,
		Description: joinSections(textextract.ConvertHTMLToMarkdown(sections.CompanyDescription.Text), textextract.ConvertHTMLToMarkdown(sections.JobDescription.Text),
			textextract.ConvertHTMLToMarkdown(sections.Qualifications.Text), textextract.ConvertHTMLToMarkdown(sections.AdditionalInformation.Text)),
		Location: posting.Location.FullLocation,
	}
	if job.Location == "" {
		job.Location = joinNonEmpty(", ", posting.Location.City, posting.Location.Region, posting.Location.Country)
	}
	switch {
	case posting.Location.Remote:
		job.WorkplaceType = "Remote"
	case posting.Location.Hybrid:
		job.WorkplaceType = "Hybrid"
	}
	job.EmploymentType = posting.TypeOfEmployment.Label
	job.Department = posting.Department.Label
	if released, err := time.Parse(time.RFC3339, posting.ReleasedDate); err == nil {
		job.PublishedAt = &released
	}
	isOpen := posting.ID != "" && posting.Name != "" && (posting.Active == nil || *posting.Active)
	return job, isOpen, nil
}
