package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Mail directions.
const (
	MailReceived = "received"
	MailSent     = "sent"
)

// MailMessage is one message of the owner's mail, as Gmail announced it.
type MailMessage struct {
	ID             uuid.UUID `json:"id"`
	GmailMessageID string    `json:"gmail_message_id"`
	ThreadID       string    `json:"thread_id"`
	Direction      string    `json:"direction"`
	Sender         string    `json:"sender"`
	Recipients     string    `json:"recipients"`
	Subject        string    `json:"subject"`
	SentAt         time.Time `json:"sent_at"`
	LabelIDs       []string  `json:"label_ids"`
	RecordedAt     time.Time `json:"recorded_at"`
}

type NewMailMessage struct {
	GmailMessageID string
	ThreadID       string
	Direction      string
	Sender         string
	Recipients     string
	Subject        string
	SentAt         time.Time
	LabelIDs       []string
}

// RecordMailMessage stores a message once; created is false when it was
// already recorded.
func (s *Store) RecordMailMessage(ctx context.Context, input NewMailMessage) (message MailMessage, created bool, err error) {
	labelIDs := input.LabelIDs
	if labelIDs == nil {
		labelIDs = []string{}
	}
	rows, err := s.pool.Query(ctx, `
		INSERT INTO mail_messages (gmail_message_id, thread_id, direction, sender, recipients, subject, sent_at, label_ids)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (gmail_message_id) DO NOTHING
		RETURNING id, gmail_message_id, thread_id, direction, sender, recipients, subject, sent_at, label_ids, recorded_at`,
		input.GmailMessageID, input.ThreadID, input.Direction, input.Sender, input.Recipients, input.Subject, input.SentAt, labelIDs)
	if err != nil {
		return MailMessage{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return MailMessage{}, false, rows.Err()
	}
	err = rows.Scan(&message.ID, &message.GmailMessageID, &message.ThreadID, &message.Direction, &message.Sender, &message.Recipients,
		&message.Subject, &message.SentAt, &message.LabelIDs, &message.RecordedAt)
	return message, err == nil, err
}

// ListMailMessages returns the newest messages first.
func (s *Store) ListMailMessages(ctx context.Context, limit int) ([]MailMessage, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, gmail_message_id, thread_id, direction, sender, recipients, subject, sent_at, label_ids, recorded_at
		FROM mail_messages ORDER BY sent_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := []MailMessage{}
	for rows.Next() {
		var message MailMessage
		if err := rows.Scan(&message.ID, &message.GmailMessageID, &message.ThreadID, &message.Direction, &message.Sender,
			&message.Recipients, &message.Subject, &message.SentAt, &message.LabelIDs, &message.RecordedAt); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}
