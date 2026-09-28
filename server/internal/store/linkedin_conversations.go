package store

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// NewLinkedInMessage is one message of the owner's LinkedIn export.
type NewLinkedInMessage struct {
	ConversationID       string
	ConversationTitle    string
	SenderName           string
	SenderProfileURL     string
	RecipientProfileURLs []string
	SentAt               time.Time
	Subject              string
	Content              string
	Folder               string
}

// NewLinkedInInvitation is one open connection request, sent or received.
type NewLinkedInInvitation struct {
	Direction  string
	FromName   string
	ToName     string
	InviterURL string
	InviteeURL string
	SentAt     *time.Time
	Message    string
}

// MessagesImport counts what an import of messages.csv stored.
type MessagesImport struct {
	Conversations int `json:"conversations"`
	Messages      int `json:"messages"`
	// ConnectionsWithHistory is how many connections the owner has
	// exchanged messages with.
	ConnectionsWithHistory int `json:"connections_with_history"`
}

// ImportLinkedInMessages stores each conversation, replacing its messages
// when it was imported before, then refreshes every connection's history.
// The owner is the one person in every conversation, so is found as the
// most frequent participant.
func (s *Store) ImportLinkedInMessages(ctx context.Context, actor Actor, messages []NewLinkedInMessage) (MessagesImport, error) {
	ownerURL := findOwnerProfileURL(messages)
	conversations := map[string][]NewLinkedInMessage{}
	for _, message := range messages {
		conversations[message.ConversationID] = append(conversations[message.ConversationID], message)
	}
	result := MessagesImport{Conversations: len(conversations), Messages: len(messages)}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for linkedInID, thread := range conversations {
			sort.Slice(thread, func(a, b int) bool { return thread[a].SentAt.Before(thread[b].SentAt) })
			first, last := thread[0], thread[len(thread)-1]
			ownerWrote := false
			for _, message := range thread {
				ownerWrote = ownerWrote || normalizeProfileURL(message.SenderProfileURL) == ownerURL
			}
			var conversationID uuid.UUID
			err := tx.QueryRow(ctx, `
				INSERT INTO linkedin_conversations (linkedin_id, title, started_by_url, started_by_owner, owner_wrote, message_count,
					first_message_at, last_message_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (linkedin_id) DO UPDATE SET
					title = EXCLUDED.title, started_by_url = EXCLUDED.started_by_url, started_by_owner = EXCLUDED.started_by_owner,
					owner_wrote = EXCLUDED.owner_wrote, message_count = EXCLUDED.message_count,
					first_message_at = EXCLUDED.first_message_at, last_message_at = EXCLUDED.last_message_at, imported_at = now()
				RETURNING id`,
				linkedInID, first.ConversationTitle, first.SenderProfileURL, normalizeProfileURL(first.SenderProfileURL) == ownerURL,
				ownerWrote, len(thread), first.SentAt, last.SentAt).Scan(&conversationID)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `DELETE FROM linkedin_messages WHERE conversation_id = $1`, conversationID); err != nil {
				return err
			}
			rows := make([][]any, 0, len(thread))
			for _, message := range thread {
				recipients := message.RecipientProfileURLs
				if recipients == nil {
					recipients = []string{}
				}
				rows = append(rows, []any{conversationID, message.SenderName, message.SenderProfileURL, recipients, message.SentAt,
					message.Subject, message.Content, message.Folder})
			}
			if _, err := tx.CopyFrom(ctx, pgx.Identifier{"linkedin_messages"},
				[]string{"conversation_id", "sender_name", "sender_profile_url", "recipient_profile_urls", "sent_at", "subject", "content", "folder"},
				pgx.CopyFromRows(rows)); err != nil {
				return err
			}
		}
		withHistory, err := refreshConnectionHistory(ctx, tx)
		if err != nil {
			return err
		}
		result.ConnectionsWithHistory = withHistory
		return insertChange(ctx, tx, actor, change{entityType: "linkedin_messages", entityID: uuid.New(), operation: "import", after: result})
	})
	return result, err
}

// InvitationsImport counts what an import of Invitations.csv stored.
type InvitationsImport struct {
	Incoming int `json:"incoming"`
	Outgoing int `json:"outgoing"`
}

// ImportLinkedInInvitations stores each open invitation once, by who sent it
// to whom.
func (s *Store) ImportLinkedInInvitations(ctx context.Context, actor Actor, invitations []NewLinkedInInvitation) (InvitationsImport, error) {
	var result InvitationsImport
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, invitation := range invitations {
			if _, err := tx.Exec(ctx, `
				INSERT INTO linkedin_invitations (direction, from_name, to_name, inviter_url, invitee_url, sent_at, message)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (inviter_url, invitee_url) DO UPDATE SET
					direction = EXCLUDED.direction, from_name = EXCLUDED.from_name, to_name = EXCLUDED.to_name,
					sent_at = EXCLUDED.sent_at, message = EXCLUDED.message`,
				invitation.Direction, invitation.FromName, invitation.ToName, invitation.InviterURL, invitation.InviteeURL,
				invitation.SentAt, invitation.Message); err != nil {
				return err
			}
			if invitation.Direction == "incoming" {
				result.Incoming++
			} else {
				result.Outgoing++
			}
		}
		return insertChange(ctx, tx, actor, change{entityType: "linkedin_invitations", entityID: uuid.New(), operation: "import", after: result})
	})
	return result, err
}

// refreshConnectionHistory recounts, for every connection, the conversations
// they were in, their messages, when they first and last talked, and whether
// they ever wrote first. It returns how many connections have any history.
func refreshConnectionHistory(ctx context.Context, tx pgx.Tx) (int, error) {
	if _, err := tx.Exec(ctx, `
		UPDATE connections SET conversation_count = 0, message_count = 0, first_message_at = NULL, last_message_at = NULL,
			they_wrote_first = false`); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `
		WITH participants AS (
			SELECT DISTINCT conversations.id AS conversation_id, lower(rtrim(url, '/')) AS url
			FROM linkedin_conversations AS conversations
			JOIN linkedin_messages AS messages ON messages.conversation_id = conversations.id,
			LATERAL unnest(array_append(messages.recipient_profile_urls, messages.sender_profile_url)) AS url
			WHERE url <> ''
		), history AS (
			SELECT participants.url, count(*) AS conversations, sum(conversations.message_count) AS messages,
				min(conversations.first_message_at) AS first_at, max(conversations.last_message_at) AS last_at,
				bool_or(lower(rtrim(conversations.started_by_url, '/')) = participants.url) AS wrote_first
			FROM participants JOIN linkedin_conversations AS conversations ON conversations.id = participants.conversation_id
			GROUP BY participants.url
		)
		UPDATE connections SET conversation_count = history.conversations, message_count = history.messages,
			first_message_at = history.first_at, last_message_at = history.last_at, they_wrote_first = history.wrote_first
		FROM history WHERE lower(rtrim(connections.profile_url, '/')) = history.url`)
	return int(tag.RowsAffected()), err
}

// findOwnerProfileURL returns the participant found in the most messages,
// as sender or recipient: in the owner's own export, that is the owner.
func findOwnerProfileURL(messages []NewLinkedInMessage) string {
	counts := map[string]int{}
	for _, message := range messages {
		seen := map[string]bool{}
		for _, url := range append([]string{message.SenderProfileURL}, message.RecipientProfileURLs...) {
			if url = normalizeProfileURL(url); url != "" && !seen[url] {
				seen[url] = true
				counts[url]++
			}
		}
	}
	owner, most := "", 0
	for url, count := range counts {
		if count > most || (count == most && url < owner) {
			owner, most = url, count
		}
	}
	return owner
}

func normalizeProfileURL(url string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(url)), "/")
}

// LinkedInConversation is one conversation someone else started, with how
// it was classified and, for a recruiter, whom they hired for.
type LinkedInConversation struct {
	ID                   uuid.UUID  `json:"id"`
	Title                string     `json:"title,omitempty"`
	StartedByName        string     `json:"started_by_name"`
	StartedByURL         string     `json:"started_by_url"`
	OwnerWrote           bool       `json:"owner_wrote"`
	MessageCount         int        `json:"message_count"`
	FirstMessageAt       *time.Time `json:"first_message_at,omitempty"`
	LastMessageAt        *time.Time `json:"last_message_at,omitempty"`
	Classification       string     `json:"classification,omitempty"`
	ClassificationReason string     `json:"classification_reason,omitempty"`
	HiringCompany        string     `json:"hiring_company,omitempty"`
	Role                 string     `json:"role,omitempty"`
	IsAgency             bool       `json:"is_agency"`
	// StarterPosition is the starter's current position, when they are a
	// connection.
	StarterPosition *string `json:"starter_position,omitempty"`
	StarterCompany  *string `json:"starter_company,omitempty"`
}

const linkedInConversationColumns = `conversations.id, conversations.title,
	COALESCE((SELECT sender_name FROM linkedin_messages WHERE conversation_id = conversations.id ORDER BY sent_at LIMIT 1), ''),
	conversations.started_by_url, conversations.owner_wrote, conversations.message_count, conversations.first_message_at,
	conversations.last_message_at, conversations.classification, conversations.classification_reason, conversations.hiring_company,
	conversations.role, conversations.is_agency, NULLIF(connections.position, ''), NULLIF(connections.company_name, '')`

const linkedInConversationJoin = `linkedin_conversations AS conversations
	LEFT JOIN connections ON lower(rtrim(connections.profile_url, '/')) = lower(rtrim(conversations.started_by_url, '/'))`

func scanLinkedInConversation(row pgx.Row) (LinkedInConversation, error) {
	var conversation LinkedInConversation
	err := row.Scan(&conversation.ID, &conversation.Title, &conversation.StartedByName, &conversation.StartedByURL, &conversation.OwnerWrote,
		&conversation.MessageCount, &conversation.FirstMessageAt, &conversation.LastMessageAt, &conversation.Classification,
		&conversation.ClassificationReason, &conversation.HiringCompany, &conversation.Role, &conversation.IsAgency,
		&conversation.StarterPosition, &conversation.StarterCompany)
	return conversation, err
}

// ListConversationsAwaitingClassification returns the conversations others
// started that aren't classified yet, the latest first.
func (s *Store) ListConversationsAwaitingClassification(ctx context.Context, limit int) ([]LinkedInConversation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+linkedInConversationColumns+` FROM `+linkedInConversationJoin+`
		WHERE NOT conversations.started_by_owner AND conversations.classification = ''
		ORDER BY conversations.last_message_at DESC NULLS LAST LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (LinkedInConversation, error) { return scanLinkedInConversation(row) })
}

// ListRecruiterConversations returns the conversations recruiters started,
// the latest first.
func (s *Store) ListRecruiterConversations(ctx context.Context) ([]LinkedInConversation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+linkedInConversationColumns+` FROM `+linkedInConversationJoin+`
		WHERE conversations.classification = 'recruiter_outreach'
		ORDER BY conversations.last_message_at DESC NULLS LAST`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (LinkedInConversation, error) { return scanLinkedInConversation(row) })
}

// LinkedInMessage is one message of a conversation.
type LinkedInMessage struct {
	SenderName string    `json:"sender_name"`
	SentAt     time.Time `json:"sent_at"`
	Subject    string    `json:"subject,omitempty"`
	Content    string    `json:"content"`
}

// ListConversationMessages returns a conversation's messages, oldest first.
func (s *Store) ListConversationMessages(ctx context.Context, conversationID uuid.UUID, limit int) ([]LinkedInMessage, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT sender_name, sent_at, subject, content FROM linkedin_messages WHERE conversation_id = $1 ORDER BY sent_at LIMIT $2`,
		conversationID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (LinkedInMessage, error) {
		var message LinkedInMessage
		err := row.Scan(&message.SenderName, &message.SentAt, &message.Subject, &message.Content)
		return message, err
	})
}

// ConversationClassification is a conversation's class, who gave it and
// why, and for a recruiter the company and role.
type ConversationClassification struct {
	Class         string
	ClassifiedBy  string
	Reason        string
	PromptID      *uuid.UUID
	HiringCompany string
	Role          string
	IsAgency      bool
}

func (s *Store) SaveConversationClassification(ctx context.Context, conversationID uuid.UUID, classification ConversationClassification) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE linkedin_conversations SET classification = $2, classified_by = $3, classification_reason = $4,
			classification_prompt_id = $5, hiring_company = $6, role = $7, is_agency = $8, classified_at = now()
		WHERE id = $1`,
		conversationID, classification.Class, classification.ClassifiedBy, classification.Reason, classification.PromptID,
		classification.HiringCompany, classification.Role, classification.IsAgency)
	return err
}
