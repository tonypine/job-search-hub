package api

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"slices"
	"strconv"

	"github.com/tonypine/job-search-hub/server/internal/google"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

type googleStatusResponse struct {
	// Configured is false when the hub has no OAuth client file.
	Configured bool                    `json:"configured"`
	Connection *store.GoogleConnection `json:"connection,omitempty"`
	// MissingScopes are what the hub now asks for that the stored sign-in
	// didn't grant; connecting again grants them.
	MissingScopes []string `json:"missing_scopes,omitempty"`
}

type gmailMessagesResponse struct {
	Messages []google.MessageSummary `json:"messages"`
}

type googleSignInResponse struct {
	URL string `json:"url"`
}

// RegisterGoogleRoutes adds the routes that connect the owner's Google
// account. connector is nil when the hub has no OAuth client file. The
// callback is the one route without the owner token: Google sends the
// owner's browser there, and the sign-in's one-time state guards it.
func RegisterGoogleRoutes(routes *http.ServeMux, hub *store.Store, connector *google.Client, requireOwner func(http.Handler) http.Handler) {
	notConfigured := errorResponse{Error: "Google isn't set up: give the hub its OAuth client file (HUB_GOOGLE_OAUTH_CLIENT_FILE)"}

	routes.Handle("GET /v1/google", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := googleStatusResponse{Configured: connector != nil}
		connection, err := hub.GetGoogleConnection(r.Context())
		switch {
		case err == nil:
			status.Connection = &connection
			for _, scope := range google.Scopes {
				if !slices.Contains(connection.Scopes, scope) {
					status.MissingScopes = append(status.MissingScopes, scope)
				}
			}
		case !errors.Is(err, store.ErrGoogleNotConnected):
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, status)
	})))

	routes.Handle("POST /v1/google/sign-in", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if connector == nil {
			writeJSON(w, http.StatusServiceUnavailable, notConfigured)
			return
		}
		writeJSON(w, http.StatusOK, googleSignInResponse{URL: connector.StartSignIn()})
	})))

	routes.HandleFunc("GET /v1/google/callback", func(w http.ResponseWriter, r *http.Request) {
		if connector == nil {
			writeCallbackPage(w, http.StatusServiceUnavailable, notConfigured.Error)
			return
		}
		query := r.URL.Query()
		if refusal := query.Get("error"); refusal != "" {
			writeCallbackPage(w, http.StatusBadRequest, "Google did not connect: "+refusal)
			return
		}
		connection, err := connector.FinishSignIn(r.Context(), query.Get("state"), query.Get("code"))
		if err != nil {
			writeCallbackPage(w, http.StatusBadRequest, "Google did not connect: "+err.Error())
			return
		}
		writeCallbackPage(w, http.StatusOK, fmt.Sprintf("Connected to Google as %s. You can close this tab.", connection.Email))
	})

	routes.Handle("GET /v1/google/gmail/messages", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if connector == nil {
			writeJSON(w, http.StatusServiceUnavailable, notConfigured)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("max"))
		messages, err := connector.SearchMessages(r.Context(), r.URL.Query().Get("q"), limit)
		if err == nil {
			writeJSON(w, http.StatusOK, gmailMessagesResponse{Messages: messages})
			return
		}
		writeGoogleResult(w, nil, err)
	})))

	routes.Handle("GET /v1/google/gmail/messages/{id}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if connector == nil {
			writeJSON(w, http.StatusServiceUnavailable, notConfigured)
			return
		}
		message, err := connector.GetMessage(r.Context(), r.PathValue("id"))
		writeGoogleResult(w, message, err)
	})))

	routes.Handle("GET /v1/google/check", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if connector == nil {
			writeJSON(w, http.StatusServiceUnavailable, notConfigured)
			return
		}
		result, err := connector.Check(r.Context())
		writeGoogleResult(w, result, err)
	})))
}

// writeGoogleResult answers a Google read: the result, or the status its
// failure stands for.
func writeGoogleResult(w http.ResponseWriter, result any, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, result)
	case errors.Is(err, store.ErrGoogleNotConnected):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Google is not connected yet"})
	case errors.Is(err, google.ErrReconnectNeeded):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: err.Error()})
	}
}

func writeCallbackPage(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>Job Search Hub</title>
<body style="font: 16px -apple-system, sans-serif; margin: 3em; color-scheme: light dark"><p>%s</p></body>`, html.EscapeString(message))
}
