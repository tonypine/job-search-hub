package google

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ErrHistoryTooOld means Gmail no longer keeps changes that far back.
var ErrHistoryTooOld = errors.New("gmail no longer keeps changes that far back")

// historyPageSize is the most changes Gmail lists in one page.
const historyPageSize = 500

// MailboxWatch is Gmail's promise to announce the mailbox's changes to a
// Pub/Sub topic until it expires. HistoryID is the mailbox's state when the
// watch was made.
type MailboxWatch struct {
	HistoryID string
	ExpiresAt time.Time
}

// WatchMailbox asks Gmail to announce every change of the mailbox to topic,
// a full topic name such as "projects/p/topics/t". Asking again renews it.
func (client *Client) WatchMailbox(ctx context.Context, topic string) (MailboxWatch, error) {
	httpClient, _, err := client.getAuthorizedClient(ctx)
	if err != nil {
		return MailboxWatch{}, err
	}
	var answer struct {
		HistoryID  string `json:"historyId"`
		Expiration string `json:"expiration"`
	}
	request := map[string]string{"topicName": topic}
	if err := client.sendGoogle(ctx, httpClient, http.MethodPost, client.gmailBase+"/gmail/v1/users/me/watch", request, &answer); err != nil {
		return MailboxWatch{}, err
	}
	milliseconds, err := strconv.ParseInt(answer.Expiration, 10, 64)
	if err != nil {
		return MailboxWatch{}, errors.New("gmail's watch named no expiration")
	}
	return MailboxWatch{HistoryID: answer.HistoryID, ExpiresAt: time.UnixMilli(milliseconds).UTC()}, nil
}

// MailboxChanges are the messages added to the mailbox since a history ID,
// oldest first, and the history ID the next reading resumes from.
type MailboxChanges struct {
	AddedMessageIDs []string
	HistoryID       string
}

// ListAddedMessages returns the messages added after startHistoryID.
func (client *Client) ListAddedMessages(ctx context.Context, startHistoryID string) (MailboxChanges, error) {
	httpClient, _, err := client.getAuthorizedClient(ctx)
	if err != nil {
		return MailboxChanges{}, err
	}
	changes := MailboxChanges{HistoryID: startHistoryID}
	listed := map[string]bool{}
	pageToken := ""
	for {
		query := url.Values{
			"startHistoryId": {startHistoryID}, "historyTypes": {"messageAdded"}, "maxResults": {strconv.Itoa(historyPageSize)},
		}
		if pageToken != "" {
			query.Set("pageToken", pageToken)
		}
		var page struct {
			History []struct {
				MessagesAdded []struct {
					Message struct {
						ID string `json:"id"`
					} `json:"message"`
				} `json:"messagesAdded"`
			} `json:"history"`
			HistoryID     string `json:"historyId"`
			NextPageToken string `json:"nextPageToken"`
		}
		err := client.callGoogle(ctx, httpClient, client.gmailBase+"/gmail/v1/users/me/history?"+query.Encode(), &page)
		var status *StatusError
		if errors.As(err, &status) && status.Status == http.StatusNotFound {
			return MailboxChanges{}, ErrHistoryTooOld
		}
		if err != nil {
			return MailboxChanges{}, err
		}
		for _, record := range page.History {
			for _, added := range record.MessagesAdded {
				if id := added.Message.ID; id != "" && !listed[id] {
					listed[id] = true
					changes.AddedMessageIDs = append(changes.AddedMessageIDs, id)
				}
			}
		}
		if page.HistoryID != "" {
			changes.HistoryID = page.HistoryID
		}
		if page.NextPageToken == "" {
			return changes, nil
		}
		pageToken = page.NextPageToken
	}
}
