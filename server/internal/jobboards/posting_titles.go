package jobboards

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/url"
)

// ListPostingTitles returns the titles of a board's open postings from its
// provider's list alone, without their text: enough to tell whose board it
// is, in one request, where FetchPostings may make one per posting.
func (verifier *Verifier) ListPostingTitles(ctx context.Context, provider, boardToken string) ([]string, error) {
	listURL, err := verifier.getPostingListURL(provider, boardToken)
	if err != nil {
		return nil, err
	}
	body, found, err := verifier.fetch(ctx, provider, listURL)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrPostingAPIOff
	}
	titles, err := readPostingTitles(provider, body)
	// BambooHR and Personio send an unknown name to their own site, whose
	// page isn't a list.
	if err != nil && (provider == BambooHR || provider == Personio) {
		return nil, ErrPostingAPIOff
	}
	return titles, err
}

func (verifier *Verifier) getPostingListURL(provider, boardToken string) (string, error) {
	escapedToken := url.PathEscape(boardToken)
	switch provider {
	case Greenhouse:
		return verifier.GreenhouseAPIBase + "/v1/boards/" + escapedToken + "/jobs", nil
	case Lever:
		return verifier.LeverAPIBase + "/v0/postings/" + escapedToken + "?mode=json", nil
	case Ashby:
		return verifier.AshbyAPIBase + "/posting-api/job-board/" + escapedToken, nil
	case Workable:
		return verifier.WorkableAPIBase + "/api/v1/widget/accounts/" + escapedToken, nil
	case SmartRecruiters:
		return verifier.SmartRecruitersAPIBase + "/v1/companies/" + escapedToken + "/postings?limit=100", nil
	}
	var domain, path string
	var base string
	switch provider {
	case Recruitee:
		base, domain, path = verifier.RecruiteeAPIBase, "recruitee.com", "/api/offers/"
	case BambooHR:
		base, domain, path = verifier.BambooHRAPIBase, "bamboohr.com", "/careers/list"
	case Personio:
		base, domain, path = verifier.PersonioAPIBase, "jobs.personio.com", "/xml"
	case Pinpoint:
		base, domain, path = verifier.PinpointAPIBase, "pinpointhq.com", "/postings.json"
	default:
		return "", ErrUnsupportedProvider
	}
	tenantBase, err := getTenantBase(base, boardToken, domain)
	return tenantBase + path, err
}

func readPostingTitles(provider string, body []byte) ([]string, error) {
	type titled struct {
		Title string `json:"title"`
	}
	collect := func(count int, title func(int) string) []string {
		titles := make([]string, 0, count)
		for index := range count {
			titles = append(titles, title(index))
		}
		return titles
	}
	switch provider {
	case Lever:
		var postings []struct {
			Text string `json:"text"`
		}
		err := json.Unmarshal(body, &postings)
		return collect(len(postings), func(index int) string { return postings[index].Text }), err
	case Recruitee:
		var answer struct {
			Offers []titled `json:"offers"`
		}
		err := json.Unmarshal(body, &answer)
		return collect(len(answer.Offers), func(index int) string { return answer.Offers[index].Title }), err
	case BambooHR:
		var answer struct {
			Result []struct {
				Name string `json:"jobOpeningName"`
			} `json:"result"`
		}
		err := json.Unmarshal(body, &answer)
		return collect(len(answer.Result), func(index int) string { return answer.Result[index].Name }), err
	case SmartRecruiters:
		var answer struct {
			Content []struct {
				Name string `json:"name"`
			} `json:"content"`
		}
		err := json.Unmarshal(body, &answer)
		return collect(len(answer.Content), func(index int) string { return answer.Content[index].Name }), err
	case Personio:
		var feed struct {
			XMLName   xml.Name `xml:"workzag-jobs"`
			Positions []struct {
				Name string `xml:"name"`
			} `xml:"position"`
		}
		err := xml.Unmarshal(body, &feed)
		return collect(len(feed.Positions), func(index int) string { return feed.Positions[index].Name }), err
	case Pinpoint:
		var answer struct {
			Data []titled `json:"data"`
		}
		err := json.Unmarshal(body, &answer)
		return collect(len(answer.Data), func(index int) string { return answer.Data[index].Title }), err
	default:
		var answer struct {
			Jobs []titled `json:"jobs"`
		}
		err := json.Unmarshal(body, &answer)
		return collect(len(answer.Jobs), func(index int) string { return answer.Jobs[index].Title }), err
	}
}
