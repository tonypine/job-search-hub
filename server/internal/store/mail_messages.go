package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Mail directions.
const (
	MailReceived = "received"
	MailSent     = "sent"
)

// Rules a message is matched by.
const (
	MatchedByPerson            = "person"
	MatchedByDomain            = "domain"
	MatchedByThread            = "thread"
	MatchedByApplicantTracking = "applicant_tracking"
)

// MailMessage is one message of the owner's mail, as Gmail announced it, and
// the company and person it was matched to. MatchedBy is empty until a rule
// matches it.
type MailMessage struct {
	ID             uuid.UUID  `json:"id"`
	GmailMessageID string     `json:"gmail_message_id"`
	ThreadID       string     `json:"thread_id"`
	Direction      string     `json:"direction"`
	Sender         string     `json:"sender"`
	Recipients     string     `json:"recipients"`
	Subject        string     `json:"subject"`
	SentAt         time.Time  `json:"sent_at"`
	LabelIDs       []string   `json:"label_ids"`
	RecordedAt     time.Time  `json:"recorded_at"`
	CompanyID      *uuid.UUID `json:"company_id,omitempty"`
	PersonID       *uuid.UUID `json:"person_id,omitempty"`
	MatchedBy      string     `json:"matched_by,omitempty"`
}

const mailMessageColumns = `id, gmail_message_id, thread_id, direction, sender, recipients, subject, sent_at, label_ids, recorded_at,
	company_id, person_id, matched_by`

func scanMailMessage(row pgx.Row) (MailMessage, error) {
	var message MailMessage
	err := row.Scan(&message.ID, &message.GmailMessageID, &message.ThreadID, &message.Direction, &message.Sender, &message.Recipients,
		&message.Subject, &message.SentAt, &message.LabelIDs, &message.RecordedAt, &message.CompanyID, &message.PersonID, &message.MatchedBy)
	return message, err
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
		RETURNING `+mailMessageColumns,
		input.GmailMessageID, input.ThreadID, input.Direction, input.Sender, input.Recipients, input.Subject, input.SentAt, labelIDs)
	if err != nil {
		return MailMessage{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return MailMessage{}, false, rows.Err()
	}
	message, err = scanMailMessage(rows)
	return message, err == nil, err
}

// IsMailMessageRecorded reports whether a Gmail message is already stored.
func (s *Store) IsMailMessageRecorded(ctx context.Context, gmailMessageID string) (bool, error) {
	var recorded bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM mail_messages WHERE gmail_message_id = $1)`, gmailMessageID).Scan(&recorded)
	return recorded, err
}

// MailFilter narrows a listing to one company's mail, or to mail no rule
// has matched yet.
type MailFilter struct {
	CompanyID     *uuid.UUID
	UnmatchedOnly bool
	Limit         int
}

// ListMailMessages returns the newest messages first.
func (s *Store) ListMailMessages(ctx context.Context, filter MailFilter) ([]MailMessage, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+mailMessageColumns+` FROM mail_messages
		WHERE ($1::uuid IS NULL OR company_id = $1) AND (NOT $2 OR matched_by = '')
		ORDER BY sent_at DESC LIMIT $3`, filter.CompanyID, filter.UnmatchedOnly, filter.Limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (MailMessage, error) { return scanMailMessage(row) })
}

// MailMatch is what a message is about and the rule that said so.
type MailMatch struct {
	CompanyID uuid.UUID
	PersonID  *uuid.UUID
	MatchedBy string
}

func (s *Store) SaveMailMatch(ctx context.Context, messageID uuid.UUID, match MailMatch) error {
	_, err := s.pool.Exec(ctx, `UPDATE mail_messages SET company_id = $2, person_id = $3, matched_by = $4 WHERE id = $1`,
		messageID, match.CompanyID, match.PersonID, match.MatchedBy)
	return err
}

// MailDirectory is what the hub knows that mail can be matched against:
// every company, the people with an email, and the company each matched
// thread is about.
type MailDirectory struct {
	Companies       []Company
	PeopleWithEmail []Person
	ThreadCompanies map[string]uuid.UUID
}

func (s *Store) GetMailDirectory(ctx context.Context) (MailDirectory, error) {
	directory := MailDirectory{ThreadCompanies: map[string]uuid.UUID{}}
	rows, err := s.pool.Query(ctx, `SELECT `+companyColumns+` FROM companies ORDER BY created_at`)
	if err != nil {
		return MailDirectory{}, err
	}
	if directory.Companies, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Company, error) { return scanCompany(row) }); err != nil {
		return MailDirectory{}, err
	}
	rows, err = s.pool.Query(ctx, `SELECT `+personColumns+` FROM people WHERE email <> ''`)
	if err != nil {
		return MailDirectory{}, err
	}
	if directory.PeopleWithEmail, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Person, error) { return scanPerson(row) }); err != nil {
		return MailDirectory{}, err
	}
	rows, err = s.pool.Query(ctx, `
		SELECT DISTINCT ON (thread_id) thread_id, company_id FROM mail_messages
		WHERE company_id IS NOT NULL ORDER BY thread_id, sent_at`)
	if err != nil {
		return MailDirectory{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var threadID string
		var companyID uuid.UUID
		if err := rows.Scan(&threadID, &companyID); err != nil {
			return MailDirectory{}, err
		}
		directory.ThreadCompanies[threadID] = companyID
	}
	return directory, rows.Err()
}
