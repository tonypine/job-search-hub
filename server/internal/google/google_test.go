package google

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

// startFakeGoogle answers the token endpoint and the few API calls the hub
// makes. A refresh token named "expired" is refused as Google refuses one.
func startFakeGoogle(t *testing.T) (*Client, *store.Store) {
	t.Helper()
	routes := http.NewServeMux()
	routes.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Form.Get("grant_type") == "authorization_code" && r.Form.Get("code") == "good-code" && r.Form.Get("code_verifier") != "":
			fmt.Fprint(w, `{"access_token":"access-1","refresh_token":"refresh-1","expires_in":3600,"token_type":"Bearer"}`)
		case r.Form.Get("grant_type") == "refresh_token" && r.Form.Get("refresh_token") == "expired":
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`)
		case r.Form.Get("grant_type") == "refresh_token":
			fmt.Fprint(w, `{"access_token":"access-2","expires_in":3600,"token_type":"Bearer"}`)
		default:
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"invalid_request"}`)
		}
	})
	routes.HandleFunc("GET /gmail/v1/users/me/profile", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"emailAddress":"owner@example.com"}`)
	})
	routes.HandleFunc("GET /gmail/v1/users/me/labels", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"labels":[{"id":"INBOX"},{"id":"SENT"},{"id":"Label_1"}]}`)
	})
	routes.HandleFunc("GET /calendar/v3/users/me/calendarList", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"items":[{"id":"primary"}]}`)
	})
	routes.HandleFunc("GET /gmail/v1/users/me/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "from:greenhouse.io" || r.URL.Query().Get("maxResults") != "5" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"messages":[{"id":"m1","threadId":"t1"}]}`)
	})
	routes.HandleFunc("GET /gmail/v1/users/me/messages/m1", func(w http.ResponseWriter, r *http.Request) {
		headers := `"headers":[{"name":"From","value":"Acme Recruiting <no-reply@greenhouse.io>"},{"name":"Subject","value":"Thank you for applying"}]`
		if r.URL.Query().Get("format") == "metadata" {
			fmt.Fprintf(w, `{"id":"m1","threadId":"t1","labelIds":["INBOX"],"snippet":"We&#39;ve received your application","internalDate":"1790600000000","payload":{"mimeType":"multipart/alternative",%s}}`, headers)
			return
		}
		// "Hi Tony," and an HTML alternative; the plain part wins.
		fmt.Fprintf(w, `{"id":"m1","threadId":"t1","internalDate":"1790600000000","payload":{"mimeType":"multipart/alternative",%s,"parts":[
			{"mimeType":"text/html","body":{"data":"PHA-SGkgPGI-VG9ueTwvYj4sPC9wPg"}},
			{"mimeType":"text/plain","body":{"data":"SGkgVG9ueSw"}}]}}`, headers)
	})
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)

	hub := store.New(testdatabase.New(t))
	clientJSON := fmt.Sprintf(`{"installed":{"client_id":"id-1","client_secret":"secret-1","auth_uri":"%s/auth","token_uri":"%s/token","redirect_uris":["http://localhost"]}}`, server.URL, server.URL)
	client, err := NewClient([]byte(clientJSON), "http://localhost:8090/v1/google/callback", hub)
	if err != nil {
		t.Fatal(err)
	}
	client.gmailBase, client.calendarBase = server.URL, server.URL
	return client, hub
}

func TestASignInStoresTheGrantAndTheCheckReadsMailAndCalendars(t *testing.T) {
	client, hub := startFakeGoogle(t)
	ctx := context.Background()

	signInURL, err := url.Parse(client.StartSignIn())
	if err != nil {
		t.Fatal(err)
	}
	query := signInURL.Query()
	if query.Get("access_type") != "offline" || query.Get("prompt") != "consent" || query.Get("code_challenge_method") != "S256" ||
		query.Get("redirect_uri") != "http://localhost:8090/v1/google/callback" || !strings.Contains(query.Get("scope"), "gmail.readonly") {
		t.Fatalf("sign-in URL = %s", signInURL)
	}

	connection, err := client.FinishSignIn(ctx, query.Get("state"), "good-code")
	if err != nil || connection.Email != "owner@example.com" {
		t.Fatalf("finish = %+v, %v", connection, err)
	}
	stored, _ := hub.GetGoogleConnection(ctx)
	if stored.RefreshToken != "refresh-1" || len(stored.Scopes) != 2 {
		t.Fatalf("stored = %+v", stored)
	}
	if _, err := client.FinishSignIn(ctx, query.Get("state"), "good-code"); !errors.Is(err, ErrUnknownSignIn) {
		t.Fatalf("a replayed state: err = %v", err)
	}

	result, err := client.Check(ctx)
	if err != nil || result.Email != "owner@example.com" || result.LabelCount != 3 || result.CalendarCount != 1 {
		t.Fatalf("check = %+v, %v", result, err)
	}
}

func TestARefusedGrantAsksForASignInAgain(t *testing.T) {
	client, hub := startFakeGoogle(t)
	ctx := context.Background()
	if _, err := hub.SaveGoogleConnection(ctx, store.Actor{Kind: store.ActorOwner}, "owner@example.com", "expired", Scopes); err != nil {
		t.Fatal(err)
	}

	if _, err := client.Check(ctx); !errors.Is(err, ErrReconnectNeeded) {
		t.Fatalf("check with an expired grant: err = %v", err)
	}
	connection, _ := hub.GetGoogleConnection(ctx)
	if connection.NeedsReconnectSince == nil || !strings.Contains(connection.LastError, "expired") {
		t.Fatalf("connection = %+v; want it marked for a new sign-in", connection)
	}
	if _, err := client.Check(ctx); !errors.Is(err, ErrReconnectNeeded) {
		t.Fatalf("a second check: err = %v", err)
	}
}

func TestGmailIsSearchedAndReadAsText(t *testing.T) {
	client, hub := startFakeGoogle(t)
	ctx := context.Background()
	if _, err := hub.SaveGoogleConnection(ctx, store.Actor{Kind: store.ActorOwner}, "owner@example.com", "refresh-1", Scopes); err != nil {
		t.Fatal(err)
	}

	messages, err := client.SearchMessages(ctx, "from:greenhouse.io", 5)
	if err != nil || len(messages) != 1 {
		t.Fatalf("search = %+v, %v", messages, err)
	}
	found := messages[0]
	if found.Subject != "Thank you for applying" || found.From != "Acme Recruiting <no-reply@greenhouse.io>" || found.ThreadID != "t1" ||
		found.Snippet != "We've received your application" || found.Date.Unix() != 1790600000 {
		t.Fatalf("summary = %+v", found)
	}

	message, err := client.GetMessage(ctx, "m1")
	if err != nil || message.Text != "Hi Tony," || message.Subject != "Thank you for applying" {
		t.Fatalf("message = %+v, %v", message, err)
	}
	if text := convertHTMLToText("<p>Hi <b>Tony</b>,</p><style>x{}</style><div>Next steps</div>"); text != "Hi Tony,\nNext steps" {
		t.Fatalf("html text = %q", text)
	}
}
