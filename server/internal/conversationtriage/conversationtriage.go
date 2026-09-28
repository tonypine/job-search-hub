// Package conversationtriage sorts the LinkedIn conversations others started
// with the owner, finding the recruiters among them, through the local model
// and the editable linkedin_conversation prompt.
package conversationtriage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	// maximumConversationsPerPass bounds one pass; the next picks up the rest.
	maximumConversationsPerPass = 200
	// messagesRead are the first messages the model reads: where a
	// conversation says what it is about.
	messagesRead         = 4
	maximumMessageLength = 1500
	maximumAnswerTokens  = 512
)

type modelClient interface {
	CompleteJSON(ctx context.Context, request chatcompletions.JSONRequest) (json.RawMessage, error)
}

type Classifier struct {
	hub       *store.Store
	client    modelClient
	modelName string
	nudges    chan struct{}
}

func NewClassifier(hub *store.Store, client modelClient, modelName string) *Classifier {
	return &Classifier{hub: hub, client: client, modelName: modelName, nudges: make(chan struct{}, 1)}
}

// Nudge asks for a pass now, as after an import.
func (classifier *Classifier) Nudge() {
	select {
	case classifier.nudges <- struct{}{}:
	default:
	}
}

// PassSummary counts one pass over the conversations awaiting a class.
type PassSummary struct {
	ByRule  int
	ByModel int
	Failed  int
}

// Run classifies at start, then every interval and on each nudge, until ctx
// ends.
func (classifier *Classifier) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		summary, err := classifier.ClassifyOnce(ctx)
		if err != nil {
			slog.Error("conversation triage pass stopped", "error", err, "by rule", summary.ByRule, "by model", summary.ByModel)
		} else if summary != (PassSummary{}) {
			slog.Info("conversation triage pass done", "by rule", summary.ByRule, "by model", summary.ByModel, "failed", summary.Failed)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-classifier.nudges:
		}
	}
}

// ClassifyOnce classifies the conversations awaiting a class, the latest
// first. A conversation the model fails on is logged, counted and left for
// the next pass; an unreachable model server stops the pass.
func (classifier *Classifier) ClassifyOnce(ctx context.Context) (PassSummary, error) {
	conversations, err := classifier.hub.ListConversationsAwaitingClassification(ctx, maximumConversationsPerPass)
	if err != nil || len(conversations) == 0 {
		return PassSummary{}, err
	}
	prompt, err := classifier.hub.GetLatestAgentPrompt(ctx, store.AgentPromptKindLinkedInConversation)
	if err != nil {
		return PassSummary{}, fmt.Errorf("read the linkedin_conversation prompt: %w", err)
	}
	var summary PassSummary
	for _, conversation := range conversations {
		messages, err := classifier.hub.ListConversationMessages(ctx, conversation.ID, messagesRead)
		if err != nil {
			return summary, err
		}
		if !hasText(messages) {
			if err := classifier.hub.SaveConversationClassification(ctx, conversation.ID, store.ConversationClassification{
				Class: "other", ClassifiedBy: store.ClassifiedByRule, Reason: "no text to read",
			}); err != nil {
				return summary, err
			}
			summary.ByRule++
			continue
		}
		classification, err := classifier.classifyByModel(ctx, conversation, messages, prompt)
		if errors.Is(err, chatcompletions.ErrUnreachable) || ctx.Err() != nil {
			return summary, err
		}
		if err == nil {
			err = classifier.hub.SaveConversationClassification(ctx, conversation.ID, classification)
		}
		if err != nil {
			summary.Failed++
			slog.Warn("conversation triage failed", "conversation", conversation.ID, "error", err)
			continue
		}
		summary.ByModel++
	}
	return summary, nil
}

func (classifier *Classifier) classifyByModel(
	ctx context.Context, conversation store.LinkedInConversation, messages []store.LinkedInMessage, prompt store.AgentPrompt,
) (store.ConversationClassification, error) {
	answer, err := classifier.client.CompleteJSON(ctx, chatcompletions.JSONRequest{
		Model: classifier.modelName, System: prompt.Body, User: formatConversationText(conversation, messages),
		SchemaName: store.AgentPromptKindLinkedInConversation, Schema: prompt.ResultSchema, MaxTokens: maximumAnswerTokens,
	})
	if err != nil {
		return store.ConversationClassification{}, err
	}
	var parsed struct {
		Class    string `json:"class"`
		Company  string `json:"company"`
		Role     string `json:"role"`
		IsAgency bool   `json:"is_agency"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(answer, &parsed); err != nil {
		return store.ConversationClassification{}, fmt.Errorf("read the model's answer: %w", err)
	}
	role := strings.TrimSpace(parsed.Role)
	if slices.Contains(classes, role) {
		role = ""
	}
	return store.ConversationClassification{
		Class: parsed.Class, ClassifiedBy: store.ClassifiedByModel, Reason: parsed.Reason, PromptID: &prompt.ID,
		HiringCompany: strings.TrimSpace(parsed.Company), Role: role, IsAgency: parsed.IsAgency,
	}, nil
}

// classes are the prompt's classes; a small model sometimes echoes one as
// the role.
var classes = []string{"recruiter_outreach", "known_person", "sales_pitch", "other"}

// formatConversationText is what the model reads: who started it, whether
// the owner answered, and its first messages.
func formatConversationText(conversation store.LinkedInConversation, messages []store.LinkedInMessage) string {
	var text strings.Builder
	if conversation.Title != "" {
		fmt.Fprintf(&text, "Title: %s\n", conversation.Title)
	}
	starter := conversation.StartedByName
	if conversation.StarterPosition != nil {
		starter += ", " + *conversation.StarterPosition
		if conversation.StarterCompany != nil {
			starter += " at " + *conversation.StarterCompany
		}
	}
	fmt.Fprintf(&text, "Started by: %s\nI answered: %t\n\nFirst messages:\n", starter, conversation.OwnerWrote)
	for _, message := range messages {
		content := message.Content
		if len(content) > maximumMessageLength {
			content = strings.ToValidUTF8(content[:maximumMessageLength], "") + "…"
		}
		fmt.Fprintf(&text, "\n[%s] %s:", message.SentAt.Format("2006-01-02"), message.SenderName)
		if message.Subject != "" {
			fmt.Fprintf(&text, " (subject: %s)", message.Subject)
		}
		fmt.Fprintf(&text, "\n%s\n", content)
	}
	return text.String()
}

func hasText(messages []store.LinkedInMessage) bool {
	for _, message := range messages {
		if strings.TrimSpace(message.Content) != "" || strings.TrimSpace(message.Subject) != "" {
			return true
		}
	}
	return false
}
