package google

import (
	"context"
	"encoding/base64"
	"errors"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// maximumSearchResults bounds one search; each result costs a request.
const maximumSearchResults = 100

// ErrMessageNotFound means Gmail has no such message, e.g. one deleted since
// it was announced.
var ErrMessageNotFound = errors.New("gmail has no such message")

// MessageSummary is one message as a search lists it.
type MessageSummary struct {
	ID       string    `json:"id"`
	ThreadID string    `json:"thread_id"`
	From     string    `json:"from"`
	To       string    `json:"to,omitempty"`
	Cc       string    `json:"cc,omitempty"`
	Subject  string    `json:"subject"`
	Date     time.Time `json:"date"`
	Snippet  string    `json:"snippet"`
	LabelIDs []string  `json:"label_ids,omitempty"`
}

// Message is one message with its text.
type Message struct {
	MessageSummary
	Text string `json:"text"`
}

type gmailMessage struct {
	ID           string    `json:"id"`
	ThreadID     string    `json:"threadId"`
	LabelIDs     []string  `json:"labelIds"`
	Snippet      string    `json:"snippet"`
	InternalDate string    `json:"internalDate"`
	Payload      gmailPart `json:"payload"`
}

type gmailPart struct {
	MimeType string `json:"mimeType"`
	Headers  []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"headers"`
	Body struct {
		Data string `json:"data"`
	} `json:"body"`
	Parts []gmailPart `json:"parts"`
}

// SearchMessages returns up to limit messages matching a Gmail query, such
// as "newer_than:30d from:greenhouse.io", newest first.
func (client *Client) SearchMessages(ctx context.Context, query string, limit int) ([]MessageSummary, error) {
	if limit <= 0 || limit > maximumSearchResults {
		limit = maximumSearchResults
	}
	httpClient, _, err := client.getAuthorizedClient(ctx)
	if err != nil {
		return nil, err
	}
	search := url.Values{"q": {query}, "maxResults": {strconv.Itoa(limit)}}
	var listed struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := client.callGoogle(ctx, httpClient, client.gmailBase+"/gmail/v1/users/me/messages?"+search.Encode(), &listed); err != nil {
		return nil, err
	}
	summaries := make([]MessageSummary, 0, len(listed.Messages))
	for _, listedMessage := range listed.Messages {
		summary, err := client.getMessageSummary(ctx, httpClient, listedMessage.ID)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

// GetMessageSummary returns one message's headers, labels and snippet,
// without its text.
func (client *Client) GetMessageSummary(ctx context.Context, id string) (MessageSummary, error) {
	httpClient, _, err := client.getAuthorizedClient(ctx)
	if err != nil {
		return MessageSummary{}, err
	}
	return client.getMessageSummary(ctx, httpClient, id)
}

func (client *Client) getMessageSummary(ctx context.Context, httpClient *http.Client, id string) (MessageSummary, error) {
	metadata := url.Values{"format": {"metadata"}, "metadataHeaders": {"From", "To", "Cc", "Subject"}}
	var message gmailMessage
	err := client.callGoogle(ctx, httpClient, client.gmailBase+"/gmail/v1/users/me/messages/"+url.PathEscape(id)+"?"+metadata.Encode(), &message)
	var status *StatusError
	if errors.As(err, &status) && status.Status == http.StatusNotFound {
		return MessageSummary{}, ErrMessageNotFound
	}
	if err != nil {
		return MessageSummary{}, err
	}
	return summarize(message), nil
}

// GetMessage returns one message with its text: the plain-text part when it
// has one, otherwise its HTML part without the markup.
func (client *Client) GetMessage(ctx context.Context, id string) (Message, error) {
	httpClient, _, err := client.getAuthorizedClient(ctx)
	if err != nil {
		return Message{}, err
	}
	var message gmailMessage
	if err := client.callGoogle(ctx, httpClient, client.gmailBase+"/gmail/v1/users/me/messages/"+url.PathEscape(id)+"?format=full", &message); err != nil {
		return Message{}, err
	}
	text := findPartText(message.Payload, "text/plain")
	if text == "" {
		text = convertHTMLToText(findPartText(message.Payload, "text/html"))
	}
	return Message{MessageSummary: summarize(message), Text: strings.TrimSpace(text)}, nil
}

func summarize(message gmailMessage) MessageSummary {
	summary := MessageSummary{ID: message.ID, ThreadID: message.ThreadID, Snippet: html.UnescapeString(message.Snippet), LabelIDs: message.LabelIDs}
	for _, header := range message.Payload.Headers {
		switch strings.ToLower(header.Name) {
		case "from":
			summary.From = header.Value
		case "to":
			summary.To = header.Value
		case "cc":
			summary.Cc = header.Value
		case "subject":
			summary.Subject = header.Value
		}
	}
	if milliseconds, err := strconv.ParseInt(message.InternalDate, 10, 64); err == nil {
		summary.Date = time.UnixMilli(milliseconds).UTC()
	}
	return summary
}

// findPartText returns the decoded body of the first part of mimeType,
// searching nested parts depth first.
func findPartText(part gmailPart, mimeType string) string {
	if part.MimeType == mimeType && part.Body.Data != "" {
		decoded, err := base64.URLEncoding.DecodeString(padBase64(part.Body.Data))
		if err != nil {
			return ""
		}
		return string(decoded)
	}
	for _, child := range part.Parts {
		if text := findPartText(child, mimeType); text != "" {
			return text
		}
	}
	return ""
}

func padBase64(data string) string {
	if remainder := len(data) % 4; remainder != 0 {
		return data + strings.Repeat("=", 4-remainder)
	}
	return data
}

var (
	lineBreakTags = regexp.MustCompile(`(?i)<br\s*/?>|</(p|div|li|tr|h[1-6])>`)
	hiddenBlocks  = regexp.MustCompile(`(?is)<(style|script|head)[^>]*>.*?</(style|script|head)>`)
	anyMarkup     = regexp.MustCompile(`<[^>]*>`)
	blankRuns     = regexp.MustCompile(`\n\s*\n\s*\n+`)
)

func convertHTMLToText(markup string) string {
	text := hiddenBlocks.ReplaceAllString(markup, "")
	text = lineBreakTags.ReplaceAllString(text, "\n")
	text = html.UnescapeString(anyMarkup.ReplaceAllString(text, ""))
	return strings.TrimSpace(blankRuns.ReplaceAllString(text, "\n\n"))
}

// ListMessageIDs returns the IDs of every message matching a Gmail query,
// newest first.
func (client *Client) ListMessageIDs(ctx context.Context, query string) ([]string, error) {
	httpClient, _, err := client.getAuthorizedClient(ctx)
	if err != nil {
		return nil, err
	}
	var ids []string
	pageToken := ""
	for {
		search := url.Values{"q": {query}, "maxResults": {"500"}}
		if pageToken != "" {
			search.Set("pageToken", pageToken)
		}
		var page struct {
			Messages []struct {
				ID string `json:"id"`
			} `json:"messages"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := client.callGoogle(ctx, httpClient, client.gmailBase+"/gmail/v1/users/me/messages?"+search.Encode(), &page); err != nil {
			return nil, err
		}
		for _, message := range page.Messages {
			ids = append(ids, message.ID)
		}
		if page.NextPageToken == "" {
			return ids, nil
		}
		pageToken = page.NextPageToken
	}
}
