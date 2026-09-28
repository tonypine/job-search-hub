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
		apiURL = verifier.GreenhouseAPIBase + "/v1/boards/" + escapedToken + "/jobs?content=true"
	case Lever:
		apiURL = verifier.LeverAPIBase + "/v0/postings/" + escapedToken + "?mode=json"
	case Ashby:
		apiURL = verifier.AshbyAPIBase + "/posting-api/job-board/" + escapedToken
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
		ID          int64  `json:"id"`
		Title       string `json:"title"`
		AbsoluteURL string `json:"absolute_url"`
		Content     string `json:"content"`
		Location    struct {
			Name string `json:"name"`
		} `json:"location"`
	}
	if err := json.Unmarshal(raw, &job); err != nil {
		return store.JobPosting{}, false, err
	}
	return store.JobPosting{
		ExternalID:  strconv.FormatInt(job.ID, 10),
		Title:       job.Title,
		Location:    job.Location.Name,
		URL:         job.AbsoluteURL,
		Description: convertHTMLToText(html.UnescapeString(job.Content)),
	}, true, nil
}

func parseLeverPosting(raw json.RawMessage) (store.JobPosting, bool, error) {
	var posting struct {
		ID               string `json:"id"`
		Text             string `json:"text"`
		HostedURL        string `json:"hostedUrl"`
		WorkplaceType    string `json:"workplaceType"`
		DescriptionPlain string `json:"descriptionPlain"`
		AdditionalPlain  string `json:"additionalPlain"`
		Categories       struct {
			Location string `json:"location"`
		} `json:"categories"`
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
	return store.JobPosting{
		ExternalID:    posting.ID,
		Title:         posting.Text,
		Location:      posting.Categories.Location,
		WorkplaceType: posting.WorkplaceType,
		URL:           posting.HostedURL,
		Description:   strings.TrimSpace(strings.Join(sections, "\n\n")),
	}, true, nil
}

// parseAshbyPosting reports an unlisted posting as not open: Ashby hides it
// from the company's own board.
func parseAshbyPosting(raw json.RawMessage) (store.JobPosting, bool, error) {
	var job struct {
		ID               string `json:"id"`
		Title            string `json:"title"`
		Location         string `json:"location"`
		WorkplaceType    string `json:"workplaceType"`
		JobURL           string `json:"jobUrl"`
		DescriptionPlain string `json:"descriptionPlain"`
		IsListed         bool   `json:"isListed"`
	}
	if err := json.Unmarshal(raw, &job); err != nil {
		return store.JobPosting{}, false, err
	}
	return store.JobPosting{
		ExternalID:    job.ID,
		Title:         job.Title,
		Location:      job.Location,
		WorkplaceType: job.WorkplaceType,
		URL:           job.JobURL,
		Description:   job.DescriptionPlain,
	}, job.IsListed, nil
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
