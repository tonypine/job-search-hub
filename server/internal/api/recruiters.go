package api

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/jobfit"
	"github.com/tonypine/job-search-hub/server/internal/prompts"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	// maximumConversationMessages is far above any one LinkedIn conversation.
	maximumConversationMessages = 1000
)

// recruiterConversation is a conversation a recruiter started, with the open
// jobs at the company they hired for, and how many of those fit.
type recruiterConversation struct {
	store.LinkedInConversation
	CompanyID   *uuid.UUID `json:"company_id,omitempty"`
	OpenJobs    int        `json:"open_jobs"`
	FittingJobs int        `json:"fitting_jobs"`
}

type recruitersResponse struct {
	Recruiters []recruiterConversation `json:"recruiters"`
}

// maximumOpeningsInDraft keeps a draft's context to the roles worth naming.
const maximumOpeningsInDraft = 8

type replyPromptResponse struct {
	Prompt  string `json:"prompt"`
	Version int    `json:"version"`
}

// RegisterRecruiterRoutes adds the owner-only list of recruiters who wrote
// to the owner on LinkedIn, flagged by what their company has open now.
func RegisterRecruiterRoutes(routes *http.ServeMux, hub *store.Store, rateSource exchangeRateSource, requireOwner func(http.Handler) http.Handler) {
	routes.Handle("GET /v1/recruiters", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conversations, err := hub.ListRecruiterConversations(r.Context())
		if err != nil {
			writeStoreError(w, err)
			return
		}
		openings, err := getOpeningsByCompany(r.Context(), hub, rateSource)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		recruiters := make([]recruiterConversation, 0, len(conversations))
		for _, conversation := range conversations {
			recruiter := recruiterConversation{LinkedInConversation: conversation}
			if found, ok := openings[store.NormalizeCompanyName(conversation.HiringCompany)]; ok && conversation.HiringCompany != "" {
				recruiter.CompanyID, recruiter.OpenJobs, recruiter.FittingJobs = found.companyID, found.open, found.fitting
			}
			recruiters = append(recruiters, recruiter)
		}
		writeJSON(w, http.StatusOK, recruitersResponse{Recruiters: recruiters})
	})))

	routes.Handle("GET /v1/recruiters/{id}/reply-prompt", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		conversation, err := hub.GetLinkedInConversation(r.Context(), id)
		if errors.Is(err, store.ErrConversationNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		if err != nil {
			writeStoreError(w, err)
			return
		}
		messages, err := hub.ListConversationMessages(r.Context(), id, maximumConversationMessages)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		openings, err := getOpeningsByCompany(r.Context(), hub, rateSource)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		jobs := []opening{}
		if conversation.HiringCompany != "" {
			jobs = pickOpeningsForDraft(openings[store.NormalizeCompanyName(conversation.HiringCompany)].jobs)
		}
		rendered, err := prompts.RenderRecruiterReply(r.Context(), hub, conversation, messages, jobs)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, replyPromptResponse{Prompt: rendered.Body, Version: rendered.Version})
	})))

	routes.Handle("GET /v1/linkedin/conversations/{id}/messages", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		messages, err := hub.ListConversationMessages(r.Context(), id, maximumConversationMessages)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, conversationMessagesResponse{Messages: messages})
	})))
}

type conversationMessagesResponse struct {
	Messages []store.LinkedInMessage `json:"messages"`
}

// pickOpeningsForDraft keeps the roles a draft may name: good fits first,
// then unclear ones, never poor ones.
func pickOpeningsForDraft(jobs []opening) []opening {
	picked := []opening{}
	for _, level := range []jobfit.Level{jobfit.LevelGood, jobfit.LevelUnclear} {
		for _, job := range jobs {
			if job.Fit == level && len(picked) < maximumOpeningsInDraft {
				picked = append(picked, job)
			}
		}
	}
	return picked
}
