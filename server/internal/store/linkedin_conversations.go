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
