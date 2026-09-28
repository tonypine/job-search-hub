// Package google signs the owner in to Google with the hub's own OAuth
// client, reads Gmail and Calendar read-only with the stored grant, and lends
// that grant to the Pub/Sub client that hears Gmail's changes.
package google

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// Scopes are what the hub asks for: reading mail and calendars, and hearing
// Gmail announce mailbox changes through a Pub/Sub subscription.
var Scopes = []string{
	"https://www.googleapis.com/auth/gmail.readonly", "https://www.googleapis.com/auth/calendar.readonly",
	"https://www.googleapis.com/auth/pubsub",
}

// ErrReconnectNeeded means Google refused the stored grant: expired, as a
// "Testing" app's grants do after 7 days, or revoked.
var ErrReconnectNeeded = errors.New("google refused the stored sign-in; connect again")

// ErrUnknownSignIn means a callback named no sign-in this server started.
var ErrUnknownSignIn = errors.New("this sign-in was not started here or has expired; start it again")

const (
	signInLifetime = 10 * time.Minute
	requestTimeout = 30 * time.Second
)

type Client struct {
	config       *oauth2.Config
	hub          *store.Store
	gmailBase    string
	calendarBase string

	mutex   sync.Mutex
	pending map[string]pendingSignIn
}

type pendingSignIn struct {
	verifier  string
	startedAt time.Time
}

// NewClient reads the OAuth client file Google issued for a desktop app;
// Google sends a finished sign-in to redirectURL.
func NewClient(clientJSON []byte, redirectURL string, hub *store.Store) (*Client, error) {
	var file struct {
		Installed *struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
			AuthURI      string `json:"auth_uri"`
			TokenURI     string `json:"token_uri"`
		} `json:"installed"`
	}
	if err := json.Unmarshal(clientJSON, &file); err != nil {
		return nil, fmt.Errorf("read the Google OAuth client: %w", err)
	}
	if file.Installed == nil || file.Installed.ClientID == "" || file.Installed.TokenURI == "" {
		return nil, errors.New("the Google OAuth client file is not a desktop app's")
	}
	config := &oauth2.Config{
		ClientID: file.Installed.ClientID, ClientSecret: file.Installed.ClientSecret, RedirectURL: redirectURL, Scopes: Scopes,
		Endpoint: oauth2.Endpoint{AuthURL: file.Installed.AuthURI, TokenURL: file.Installed.TokenURI, AuthStyle: oauth2.AuthStyleInParams},
	}
	return &Client{
		config: config, hub: hub, gmailBase: "https://gmail.googleapis.com", calendarBase: "https://www.googleapis.com",
		pending: map[string]pendingSignIn{},
	}, nil
}

// StartSignIn returns the Google consent URL for a new sign-in. It asks for a
// refresh token every time, so connecting again always replaces it.
func (client *Client) StartSignIn() string {
	state := rand.Text()
	verifier := oauth2.GenerateVerifier()
	client.mutex.Lock()
	defer client.mutex.Unlock()
	for key, pending := range client.pending {
		if time.Since(pending.startedAt) > signInLifetime {
			delete(client.pending, key)
		}
	}
	client.pending[state] = pendingSignIn{verifier: verifier, startedAt: time.Now()}
	return client.config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.S256ChallengeOption(verifier))
}

// FinishSignIn trades the callback's code for a grant and stores it, with
// the address it belongs to.
func (client *Client) FinishSignIn(ctx context.Context, state, code string) (store.GoogleConnection, error) {
	client.mutex.Lock()
	pending, found := client.pending[state]
	delete(client.pending, state)
	client.mutex.Unlock()
	if !found || time.Since(pending.startedAt) > signInLifetime {
		return store.GoogleConnection{}, ErrUnknownSignIn
	}
	token, err := client.config.Exchange(ctx, code, oauth2.VerifierOption(pending.verifier))
	if err != nil {
		return store.GoogleConnection{}, fmt.Errorf("trade the sign-in code with Google: %w", err)
	}
	if token.RefreshToken == "" {
		return store.GoogleConnection{}, errors.New("google granted no refresh token")
	}
	var profile struct {
		EmailAddress string `json:"emailAddress"`
	}
	if err := sendJSON(ctx, client.config.Client(ctx, token), http.MethodGet, client.gmailBase+"/gmail/v1/users/me/profile", nil, &profile); err != nil {
		return store.GoogleConnection{}, err
	}
	return client.hub.SaveGoogleConnection(ctx, store.Actor{Kind: store.ActorOwner}, profile.EmailAddress, token.RefreshToken, Scopes)
}

// CheckResult proves the grant works: whose it is, and how many labels and
// calendars it can see.
type CheckResult struct {
	Email         string `json:"email"`
	LabelCount    int    `json:"label_count"`
	CalendarCount int    `json:"calendar_count"`
}

func (client *Client) Check(ctx context.Context) (CheckResult, error) {
	httpClient, connection, err := client.getAuthorizedClient(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	var labels struct {
		Labels []json.RawMessage `json:"labels"`
	}
	if err := client.callGoogle(ctx, httpClient, client.gmailBase+"/gmail/v1/users/me/labels", &labels); err != nil {
		return CheckResult{}, err
	}
	var calendars struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := client.callGoogle(ctx, httpClient, client.calendarBase+"/calendar/v3/users/me/calendarList", &calendars); err != nil {
		return CheckResult{}, err
	}
	return CheckResult{Email: connection.Email, LabelCount: len(labels.Labels), CalendarCount: len(calendars.Items)}, nil
}

// GetTokenSource gives access tokens for the stored grant, for Google
// clients that take a token source. ctx bounds every refresh it makes.
func (client *Client) GetTokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	connection, err := client.hub.GetGoogleConnection(ctx)
	if err != nil {
		return nil, err
	}
	if connection.NeedsReconnectSince != nil {
		return nil, ErrReconnectNeeded
	}
	return client.config.TokenSource(ctx, &oauth2.Token{RefreshToken: connection.RefreshToken}), nil
}

func (client *Client) getAuthorizedClient(ctx context.Context) (*http.Client, store.GoogleConnection, error) {
	connection, err := client.hub.GetGoogleConnection(ctx)
	if err != nil {
		return nil, store.GoogleConnection{}, err
	}
	if connection.NeedsReconnectSince != nil {
		return nil, store.GoogleConnection{}, ErrReconnectNeeded
	}
	httpClient := client.config.Client(ctx, &oauth2.Token{RefreshToken: connection.RefreshToken})
	httpClient.Timeout = requestTimeout
	return httpClient, connection, nil
}

// callGoogle reads a Google API.
func (client *Client) callGoogle(ctx context.Context, httpClient *http.Client, apiURL string, into any) error {
	return client.sendGoogle(ctx, httpClient, http.MethodGet, apiURL, nil, into)
}

// sendGoogle calls a Google API, sending body as JSON when there is one. A
// grant Google refuses is recorded, so the owner is asked to connect again.
func (client *Client) sendGoogle(ctx context.Context, httpClient *http.Client, method, apiURL string, body, into any) error {
	err := sendJSON(ctx, httpClient, method, apiURL, body, into)
	var refused *oauth2.RetrieveError
	if errors.As(err, &refused) && refused.ErrorCode == "invalid_grant" {
		if markErr := client.hub.MarkGoogleConnectionRefused(ctx, refused.ErrorDescription); markErr != nil {
			return markErr
		}
		return ErrReconnectNeeded
	}
	return err
}

// StatusError is a Google API's answer other than 200 OK.
type StatusError struct {
	Status int
	Body   string
}

func (err *StatusError) Error() string {
	return fmt.Sprintf("google answered %d: %s", err.Status, err.Body)
}

func sendJSON(ctx context.Context, httpClient *http.Client, method, apiURL string, body, into any) error {
	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, apiURL, requestBody)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := httpClient.Do(request)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return urlErr.Err
		}
		return err
	}
	defer response.Body.Close()
	answer, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return &StatusError{Status: response.StatusCode, Body: string(answer)}
	}
	return json.Unmarshal(answer, into)
}
