// Package hiringthread reads Hacker News' monthly "Who is hiring?" thread
// into the jobs feed. Code keeps the comments that mention remote work and
// the owner's stack or search terms; the local model reads each kept comment
// into its company and roles, through the hiring_thread prompt.
package hiringthread

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/wordmatch"
)

const (
	// threadLifetime is how long the jobs a thread gave stay open: a
	// month's thread is replaced by the next.
	threadLifetime      = 45 * 24 * time.Hour
	maximumAnswerTokens = 1024
	requestTimeout      = 30 * time.Second
	userAgent           = "job-search-hub/0.1"
	threadTitlePrefix   = "Ask HN: Who is hiring?"
)

type modelClient interface {
	CompleteJSON(ctx context.Context, request chatcompletions.JSONRequest) (chatcompletions.Answer, error)
}

type Reader struct {
	hub    *store.Store
	client modelClient
	// AlgoliaBase is Hacker News' search API; tests point it elsewhere.
	AlgoliaBase string
	httpClient  *http.Client
}

func NewReader(hub *store.Store, client modelClient) *Reader {
	return &Reader{hub: hub, client: client, AlgoliaBase: "https://hn.algolia.com", httpClient: &http.Client{Timeout: requestTimeout}}
}

// PassSummary counts one pass over the thread's comments not read yet.
type PassSummary struct {
	// Read comments went through the model; LeftOut ones mention neither
	// remote work nor the stack, and were left unread.
	Read    int
	LeftOut int
	Stored  int
	Failed  int
}

// Run reads once at start and then every interval, until ctx ends.
func (reader *Reader) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		summary, err := reader.ReadOnce(ctx)
		if err != nil {
			slog.Error("hiring thread pass stopped", "error", err)
		} else if summary != (PassSummary{}) {
			slog.Info("hiring thread pass done", "read", summary.Read, "left out", summary.LeftOut, "stored", summary.Stored, "failed", summary.Failed)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Thread is a month's "Who is hiring?" story.
type Thread struct {
	ID        int64
	Title     string
	CreatedAt time.Time
}

// Comment is a top-level comment of the thread: one company's posting.
type Comment struct {
	ID        int64
	Text      string
	CreatedAt time.Time
}

// ReadOnce reads the latest thread's comments not read yet. Without a
// hiring_thread prompt there is nothing to read them with. A comment the
// model fails on is logged, counted and left for the next pass; an
// unreachable model server stops the pass.
func (reader *Reader) ReadOnce(ctx context.Context) (PassSummary, error) {
	prompt, err := reader.hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindHiringThread)
	if errors.Is(err, store.ErrAgentPromptNotFound) {
		return PassSummary{}, nil
	}
	if err != nil {
		return PassSummary{}, err
	}
	saved, err := reader.hub.GetJobCriteria(ctx)
	if err != nil {
		return PassSummary{}, err
	}
	thread, err := reader.findLatestThread(ctx)
	if err != nil {
		return PassSummary{}, err
	}
	comments, err := reader.listComments(ctx, thread.ID)
	if err != nil {
		return PassSummary{}, err
	}
	read, err := reader.hub.ListReadHiringComments(ctx, thread.ID)
	if err != nil {
		return PassSummary{}, err
	}
	var summary PassSummary
	for _, comment := range comments {
		if read[comment.ID] {
			continue
		}
		if !mentionsRemoteWorkAndTheStack(comment.Text, saved.Criteria) {
			summary.LeftOut++
			if err := reader.hub.RecordHiringComment(ctx, thread.ID, comment.ID, 0, nil); err != nil {
				return summary, err
			}
			continue
		}
		postings, err := reader.readComment(ctx, thread, comment, prompt)
		if errors.Is(err, chatcompletions.ErrUnreachable) || ctx.Err() != nil {
			return summary, err
		}
		if err != nil {
			summary.Failed++
			slog.Warn("hiring thread comment failed", "comment", comment.ID, "error", err)
			continue
		}
		postings = slices.DeleteFunc(postings, func(posting store.JobPosting) bool { return !jobfit.CouldFit(posting, saved.Criteria) })
		if _, err := reader.hub.SyncFeedJobs(ctx, store.Actor{Kind: store.ActorSystem}, store.JobSourceHackerNews, postings, time.Now()); err != nil {
			return summary, err
		}
		summary.Read++
		summary.Stored += len(postings)
		if err := reader.hub.RecordHiringComment(ctx, thread.ID, comment.ID, len(postings), &prompt.ID); err != nil {
			return summary, err
		}
	}
	return summary, nil
}

// readComment asks the model for the comment's company and roles, and
// returns a posting per role.
func (reader *Reader) readComment(ctx context.Context, thread Thread, comment Comment, prompt store.AgentPrompt) ([]store.JobPosting, error) {
	answer, err := reader.client.CompleteJSON(ctx, chatcompletions.JSONRequest{
		System: prompt.Body, User: comment.Text, SchemaName: store.AgentPromptKindHiringThread, Schema: prompt.ResultSchema,
		Examples: prompt.Examples, MaxTokens: maximumAnswerTokens,
		Task: chatcompletions.TaskLabel{PromptID: &prompt.ID, PromptVersion: prompt.Version},
	})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		IsJobPosting bool   `json:"is_job_posting"`
		Company      string `json:"company"`
		Roles        []struct {
			Title         string `json:"title"`
			Location      string `json:"location"`
			WorkplaceType string `json:"workplace_type"`
		} `json:"roles"`
	}
	if err := json.Unmarshal(answer.Object, &parsed); err != nil {
		return nil, fmt.Errorf("read the model's answer: %w", err)
	}
	if !parsed.IsJobPosting || strings.TrimSpace(parsed.Company) == "" {
		return nil, nil
	}
	expiresAt := thread.CreatedAt.Add(threadLifetime)
	publishedAt := comment.CreatedAt
	var postings []store.JobPosting
	for index, role := range parsed.Roles {
		if strings.TrimSpace(role.Title) == "" {
			continue
		}
		postings = append(postings, store.JobPosting{
			ExternalID: fmt.Sprintf("%d-%d", comment.ID, index), CompanyName: strings.TrimSpace(parsed.Company), Title: strings.TrimSpace(role.Title),
			Location: strings.TrimSpace(role.Location), WorkplaceType: role.WorkplaceType, Description: comment.Text,
			URL: "https://news.ycombinator.com/item?id=" + strconv.FormatInt(comment.ID, 10), ExpiresAt: &expiresAt,
			BoardFacts: store.BoardFacts{PublishedAt: &publishedAt},
		})
	}
	return postings, nil
}

// mentionsRemoteWorkAndTheStack reports whether a comment mentions remote
// work and one of the search terms or technologies, as whole words.
func mentionsRemoteWorkAndTheStack(text string, criteria store.JobCriteria) bool {
	normalized := strings.ReplaceAll(wordmatch.Normalize(text), "-", " ")
	if !wordmatch.Contains(normalized, "remote") {
		return false
	}
	terms := append(slices.Clone(criteria.SearchTerms), criteria.Technologies...)
	return slices.ContainsFunc(terms, func(term string) bool {
		return wordmatch.Contains(normalized, strings.ReplaceAll(wordmatch.Normalize(term), "-", " "))
	})
}

// findLatestThread returns the newest "Who is hiring?" story.
func (reader *Reader) findLatestThread(ctx context.Context) (Thread, error) {
	var answer struct {
		Hits []struct {
			ObjectID  string `json:"objectID"`
			Title     string `json:"title"`
			CreatedAt string `json:"created_at"`
		} `json:"hits"`
	}
	query := url.Values{"tags": {"story,author_whoishiring"}, "hitsPerPage": {"10"}}
	if err := reader.getJSON(ctx, reader.AlgoliaBase+"/api/v1/search_by_date?"+query.Encode(), &answer); err != nil {
		return Thread{}, err
	}
	for _, hit := range answer.Hits {
		if !strings.HasPrefix(hit.Title, threadTitlePrefix) {
			continue
		}
		id, err := strconv.ParseInt(hit.ObjectID, 10, 64)
		if err != nil {
			return Thread{}, err
		}
		createdAt, err := time.Parse(time.RFC3339, hit.CreatedAt)
		if err != nil {
			return Thread{}, err
		}
		return Thread{ID: id, Title: hit.Title, CreatedAt: createdAt}, nil
	}
	return Thread{}, errors.New("no Who is hiring? thread among whoishiring's latest stories")
}

var anyTag = regexp.MustCompile(`<[^>]*>`)

// listComments returns the thread's top-level comments, as plain text.
func (reader *Reader) listComments(ctx context.Context, threadID int64) ([]Comment, error) {
	var item struct {
		Children []struct {
			ID        int64  `json:"id"`
			Text      string `json:"text"`
			CreatedAt string `json:"created_at"`
		} `json:"children"`
	}
	if err := reader.getJSON(ctx, fmt.Sprintf("%s/api/v1/items/%d", reader.AlgoliaBase, threadID), &item); err != nil {
		return nil, err
	}
	comments := make([]Comment, 0, len(item.Children))
	for _, child := range item.Children {
		withBreaks := strings.NewReplacer("<p>", "\n\n", "<br>", "\n").Replace(child.Text)
		text := strings.TrimSpace(html.UnescapeString(anyTag.ReplaceAllString(withBreaks, "")))
		if text == "" {
			continue
		}
		createdAt, _ := time.Parse(time.RFC3339, child.CreatedAt)
		comments = append(comments, Comment{ID: child.ID, Text: text, CreatedAt: createdAt})
	}
	return comments, nil
}

func (reader *Reader) getJSON(ctx context.Context, address string, into any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", userAgent)
	response, err := reader.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("reach hacker news: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("hacker news answered %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, into)
}
