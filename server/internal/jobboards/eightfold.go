package jobboards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// Eightfold boards are large employers' career sites at
// <tenant>.eightfold.ai. The board token is "<tenant>/<company domain>",
// since the site's API needs both.
const Eightfold = "eightfold"

const (
	// eightfoldPageSize is how many positions a search answers with.
	eightfoldPageSize = 10
	// eightfoldPagesPerTerm bounds one search term's reading.
	eightfoldPagesPerTerm = 5
)

// eightfoldDescriptions keeps each position's description once read: a
// description takes a request of its own, and a large employer lists
// hundreds of positions.
type eightfoldDescriptions struct {
	mutex sync.Mutex
	texts map[string]string
}

func (cache *eightfoldDescriptions) get(id string) (string, bool) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	text, found := cache.texts[id]
	return text, found
}

func (cache *eightfoldDescriptions) put(id, text string) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	if cache.texts == nil {
		cache.texts = map[string]string{}
	}
	cache.texts[id] = text
}

type eightfoldBoard struct {
	tenant string
	domain string
}

func parseEightfoldToken(boardToken string) (eightfoldBoard, error) {
	tenant, domain, found := strings.Cut(boardToken, "/")
	if !found || tenant == "" || domain == "" {
		return eightfoldBoard{}, errors.New(`an Eightfold board token is "<tenant>/<company domain>", e.g. "acme/acme.com"`)
	}
	return eightfoldBoard{tenant: tenant, domain: domain}, nil
}

// getEightfoldBase is the tenant's site, unless tests point every tenant at
// one server.
func (verifier *Verifier) getEightfoldBase(board eightfoldBoard) string {
	if verifier.EightfoldAPIBase != "" {
		return verifier.EightfoldAPIBase
	}
	return "https://" + url.PathEscape(board.tenant) + ".eightfold.ai"
}

type eightfoldPosition struct {
	ID                 int64    `json:"id"`
	Name               string   `json:"name"`
	Locations          []string `json:"locations"`
	PostedTs           int64    `json:"postedTs"`
	Department         string   `json:"department"`
	WorkLocationOption string   `json:"workLocationOption"`
	PositionURL        string   `json:"positionUrl"`
}

type eightfoldSearch struct {
	Data struct {
		Count     int                 `json:"count"`
		Positions []eightfoldPosition `json:"positions"`
	} `json:"data"`
}

func (verifier *Verifier) searchEightfold(ctx context.Context, board eightfoldBoard, query string, start int) (eightfoldSearch, bool, error) {
	search := url.Values{"domain": {board.domain}, "query": {query}, "location": {""}, "start": {strconv.Itoa(start)}}
	body, found, err := verifier.fetch(ctx, Eightfold, verifier.getEightfoldBase(board)+"/api/pcsx/search?"+search.Encode())
	if err != nil || !found {
		return eightfoldSearch{}, found, err
	}
	var answer eightfoldSearch
	if err := json.Unmarshal(body, &answer); err != nil {
		return eightfoldSearch{}, false, fmt.Errorf("read Eightfold's search: %w", err)
	}
	return answer, true, nil
}

func (verifier *Verifier) verifyEightfold(ctx context.Context, boardToken string) (Verification, error) {
	board, err := parseEightfoldToken(boardToken)
	if err != nil {
		return Verification{}, err
	}
	answer, found, err := verifier.searchEightfold(ctx, board, "", 0)
	if err != nil || !found {
		return Verification{}, err
	}
	count := answer.Data.Count
	return Verification{Verified: true, OpenPostingCount: &count, BoardURL: verifier.getEightfoldBase(board) + "/careers?domain=" + url.QueryEscape(board.domain)}, nil
}

// fetchEightfoldPostings reads the positions the owner's search terms find,
// not all of a large employer's, with each one's description.
func (verifier *Verifier) fetchEightfoldPostings(ctx context.Context, boardToken string) ([]store.JobPosting, error) {
	board, err := parseEightfoldToken(boardToken)
	if err != nil {
		return nil, err
	}
	terms := []string{""}
	if verifier.SearchTerms != nil {
		if owned := verifier.SearchTerms(ctx); len(owned) > 0 {
			terms = owned
		}
	}
	seen := map[int64]bool{}
	var postings []store.JobPosting
	for _, term := range terms {
		for page := range eightfoldPagesPerTerm {
			answer, found, err := verifier.searchEightfold(ctx, board, term, page*eightfoldPageSize)
			if err != nil {
				return nil, err
			}
			if !found {
				return nil, ErrPostingAPIOff
			}
			for _, position := range answer.Data.Positions {
				if seen[position.ID] {
					continue
				}
				seen[position.ID] = true
				postings = append(postings, verifier.convertEightfoldPosition(ctx, board, position))
			}
			if len(answer.Data.Positions) < eightfoldPageSize {
				break
			}
		}
	}
	return postings, nil
}

func (verifier *Verifier) convertEightfoldPosition(ctx context.Context, board eightfoldBoard, position eightfoldPosition) store.JobPosting {
	id := strconv.FormatInt(position.ID, 10)
	posting := store.JobPosting{
		ExternalID: id, Title: position.Name,
		URL:         verifier.getEightfoldBase(board) + position.PositionURL + "?domain=" + url.QueryEscape(board.domain),
		Description: verifier.getEightfoldDescription(ctx, board, id),
	}
	if len(position.Locations) > 0 {
		posting.Location = position.Locations[0]
		posting.OtherLocations = position.Locations[1:]
	}
	switch position.WorkLocationOption {
	case "remote", "remote_global", "remote_local":
		posting.WorkplaceType = "Remote"
	case "hybrid":
		posting.WorkplaceType = "Hybrid"
	case "onsite":
		posting.WorkplaceType = "On-site"
	}
	posting.Department = position.Department
	if position.PostedTs > 0 {
		published := time.Unix(position.PostedTs, 0).UTC()
		posting.PublishedAt = &published
	}
	return posting
}

// getEightfoldDescription reads a position's description once; one it can't
// read is left empty and tried again on the next poll.
func (verifier *Verifier) getEightfoldDescription(ctx context.Context, board eightfoldBoard, id string) string {
	if text, found := verifier.eightfoldDescriptions.get(board.tenant + "/" + id); found {
		return text
	}
	details := url.Values{"position_id": {id}, "domain": {board.domain}}
	body, found, err := verifier.fetch(ctx, Eightfold, verifier.getEightfoldBase(board)+"/api/pcsx/position_details?"+details.Encode())
	if err != nil || !found {
		return ""
	}
	var answer struct {
		Data struct {
			JobDescription string `json:"jobDescription"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &answer) != nil {
		return ""
	}
	text := convertHTMLToText(html.UnescapeString(answer.Data.JobDescription))
	verifier.eightfoldDescriptions.put(board.tenant+"/"+id, text)
	return text
}
