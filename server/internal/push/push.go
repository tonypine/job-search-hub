// Package push sends the hub's updates to the paired phones through Firebase
// Cloud Messaging, with the service account of the hub's Firebase project.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	messagingScope = "https://www.googleapis.com/auth/firebase.messaging"
	defaultBaseURL = "https://fcm.googleapis.com"
	sendTimeout    = 20 * time.Second
	// maximumBodyLength keeps a message well inside FCM's 4 KB data limit.
	maximumBodyLength = 1000
)

// ErrTokenGone is FCM saying the token no longer reaches an app: it was
// unregistered, or it belongs to another Firebase project.
var ErrTokenGone = errors.New("FCM no longer delivers to this token")

// Message is what a phone gets: the update's text and what it's about. The
// phone builds the notification from it.
type Message struct {
	UpdateID  string
	Title     string
	Body      string
	JobID     string
	CompanyID string
}

// Sender sends messages to phones through FCM's v1 API.
type Sender struct {
	projectID string
	tokens    oauth2.TokenSource
	client    *http.Client
	baseURL   string
}

// NewSender reads a Firebase service account key.
func NewSender(ctx context.Context, serviceAccountJSON []byte) (*Sender, error) {
	credentials, err := google.CredentialsFromJSONWithType(ctx, serviceAccountJSON, google.ServiceAccount, messagingScope)
	if err != nil {
		return nil, fmt.Errorf("read the Firebase service account: %w", err)
	}
	if credentials.ProjectID == "" {
		return nil, errors.New("the Firebase service account names no project")
	}
	return &Sender{projectID: credentials.ProjectID, tokens: credentials.TokenSource, client: http.DefaultClient, baseURL: defaultBaseURL}, nil
}

// ProjectID is the Firebase project the sender sends through.
func (sender *Sender) ProjectID() string {
	return sender.projectID
}

type sendRequest struct {
	Message fcmMessage `json:"message"`
}

type fcmMessage struct {
	Token   string            `json:"token"`
	Data    map[string]string `json:"data"`
	Android fcmAndroid        `json:"android"`
}

type fcmAndroid struct {
	Priority string `json:"priority"`
}

type fcmErrorResponse struct {
	Error struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Details []struct {
			ErrorCode string `json:"errorCode"`
		} `json:"details"`
	} `json:"error"`
}

// Send sends one message to the app holding pushToken, as a high-priority
// data message the app always shows.
func (sender *Sender) Send(ctx context.Context, pushToken string, message Message) error {
	accessToken, err := sender.tokens.Token()
	if err != nil {
		return fmt.Errorf("get an FCM access token: %w", err)
	}
	data := map[string]string{"update_id": message.UpdateID, "title": message.Title}
	for key, value := range map[string]string{"body": truncate(message.Body), "job_id": message.JobID, "company_id": message.CompanyID} {
		if value != "" {
			data[key] = value
		}
	}
	payload, err := json.Marshal(sendRequest{Message: fcmMessage{Token: pushToken, Data: data, Android: fcmAndroid{Priority: "HIGH"}}})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, sender.baseURL+"/v1/projects/"+sender.projectID+"/messages:send", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	accessToken.SetAuthHeader(request)
	response, err := sender.client.Do(request)
	if err != nil {
		return fmt.Errorf("send to FCM: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	var failure fcmErrorResponse
	json.Unmarshal(body, &failure)
	for _, detail := range failure.Error.Details {
		if detail.ErrorCode == "UNREGISTERED" || detail.ErrorCode == "SENDER_ID_MISMATCH" {
			return ErrTokenGone
		}
	}
	return fmt.Errorf("FCM answered %d %s: %s", response.StatusCode, failure.Error.Status, failure.Error.Message)
}

func truncate(body string) string {
	runes := []rune(body)
	if len(runes) <= maximumBodyLength {
		return body
	}
	return string(runes[:maximumBodyLength]) + "…"
}
