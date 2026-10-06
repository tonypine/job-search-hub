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

// Mail classes: what kind of mail a received message is.
const (
	MailHumanReply              = "human_reply"
	MailApplicationConfirmation = "application_confirmation"
	MailRejection               = "rejection"
	MailInterviewInvite         = "interview_invite"
	MailRecruiterOutreach       = "recruiter_outreach"
	MailJobAlert                = "job_alert"
	MailNoise                   = "noise"
)

// Who classified a message.
const (
	ClassifiedByRule  = "rule"
	ClassifiedByModel = "model"
)

// MailMessage is one message of the owner's mail, as Gmail announced it, the
// company and person it was matched to, and its class. MatchedBy and
// Classification are empty until a rule, or the model, sets them.
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
	Classification string     `json:"classification,omitempty"`
	ClassifiedBy   string     `json:"classified_by,omitempty"`
	// ClassificationReason is the model's one-sentence reason, or the rule's.
	ClassificationReason string     `json:"classification_reason,omitempty"`
	ClassifiedAt         *time.Time `json:"classified_at,omitempty"`
	// Action is what the hub did about the message; ActedAt is set once it
	// acted, or found nothing to do.
	Action  string     `json:"action,omitempty"`
	ActedAt *time.Time `json:"acted_at,omitempty"`
}

const mailMessageColumns = `id, gmail_message_id, thread_id, direction, sender, recipients, subject, sent_at, label_ids, recorded_at,
	company_id, person_id, matched_by, classification, classified_by, classification_reason, classified_at, action, acted_at`

func scanMailMessage(row pgx.Row) (MailMessage, error) {
	var message MailMessage
	err := row.Scan(&message.ID, &message.GmailMessageID, &message.ThreadID, &message.Direction, &message.Sender, &message.Recipients,
		&message.Subject, &message.SentAt, &message.LabelIDs, &message.RecordedAt, &message.CompanyID, &message.PersonID, &message.MatchedBy,
		&message.Classification, &message.ClassifiedBy, &message.ClassificationReason, &message.ClassifiedAt, &message.Action, &message.ActedAt)
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
// has matched yet, and can leave some classes out. Messages not classified
// yet are never left out.
type MailFilter struct {
	CompanyID       *uuid.UUID
	UnmatchedOnly   bool
	LeaveOutClasses []string
	Limit           int
}

// ListMailMessages returns the newest messages first.
func (s *Store) ListMailMessages(ctx context.Context, filter MailFilter) ([]MailMessage, error) {
	leaveOut := filter.LeaveOutClasses
	if leaveOut == nil {
		leaveOut = []string{}
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+mailMessageColumns+` FROM mail_messages
		WHERE ($1::uuid IS NULL OR company_id = $1) AND (NOT $2 OR matched_by = '') AND classification <> ALL($4::text[])
		ORDER BY sent_at DESC LIMIT $3`, filter.CompanyID, filter.UnmatchedOnly, filter.Limit, leaveOut)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (MailMessage, error) { return scanMailMessage(row) })
}

// CountCompanyMailOfClasses counts a company's messages in any of the
// classes, such as the ones its page leaves out.
func (s *Store) CountCompanyMailOfClasses(ctx context.Context, companyID uuid.UUID, classes []string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM mail_messages WHERE company_id = $1 AND classification = ANY($2::text[])`,
		companyID, classes).Scan(&count)
	return count, err
}

// MailMatch is what a message is about and the rule that said so.
type MailMatch struct {
	CompanyID uuid.UUID
	PersonID  *uuid.UUID
	MatchedBy string
}

// SaveMailMatch ties a message to what it is about. A class given without
// that knowledge is cleared, so the message is read again.
func (s *Store) SaveMailMatch(ctx context.Context, messageID uuid.UUID, match MailMatch) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE mail_messages SET company_id = $2, person_id = $3, matched_by = $4,
			classification = '', classified_by = '', classification_reason = '', classification_prompt_id = NULL, classified_at = NULL
		WHERE id = $1`,
		messageID, match.CompanyID, match.PersonID, match.MatchedBy)
	return err
}

// ListMailAwaitingClassification returns received messages not classified
// yet, newest first, so fresh mail is read before old.
func (s *Store) ListMailAwaitingClassification(ctx context.Context, limit int) ([]MailMessage, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+mailMessageColumns+` FROM mail_messages
		WHERE classification = '' AND direction = 'received' ORDER BY sent_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (MailMessage, error) { return scanMailMessage(row) })
}

// MailClassification is a message's class, who gave it, and why; PromptID
// is the mail_triage version the model read, nil for a rule.
type MailClassification struct {
	Class        string
	ClassifiedBy string
	Reason       string
	PromptID     *uuid.UUID
}

func (s *Store) SaveMailClassification(ctx context.Context, messageID uuid.UUID, classification MailClassification) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE mail_messages SET classification = $2, classified_by = $3, classification_reason = $4, classification_prompt_id = $5,
			classified_at = now()
		WHERE id = $1`,
		messageID, classification.Class, classification.ClassifiedBy, classification.Reason, classification.PromptID)
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

// ListMailAwaitingAction returns, oldest first, the messages the hub hasn't
// acted on that may call for it: mail sent to a known company, and received
// mail of a class that moves a card or brings news.
func (s *Store) ListMailAwaitingAction(ctx context.Context, limit int) ([]MailMessage, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+mailMessageColumns+` FROM mail_messages
		WHERE acted_at IS NULL AND (
			(direction = 'sent' AND company_id IS NOT NULL) OR
			(direction = 'received' AND classification IN ('human_reply', 'application_confirmation', 'rejection', 'interview_invite', 'recruiter_outreach')))
		ORDER BY sent_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (MailMessage, error) { return scanMailMessage(row) })
}

// StartsItsThread reports whether the message is the first the hub has of
// its thread, as one the owner wrote rather than answered is.
func (s *Store) StartsItsThread(ctx context.Context, message MailMessage) (bool, error) {
	var answers bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM mail_messages WHERE thread_id = $1 AND id <> $2 AND sent_at <= $3)`,
		message.ThreadID, message.ID, message.SentAt).Scan(&answers)
	return !answers, err
}

// MarkMailActed records what the hub did about a message, so it acts once.
func (s *Store) MarkMailActed(ctx context.Context, messageID uuid.UUID, action string) error {
	_, err := s.pool.Exec(ctx, `UPDATE mail_messages SET acted_at = now(), action = $2 WHERE id = $1`, messageID, action)
	return err
}

// SaveMailCompanyFromItsSender ties a message to a company created from its
// own sender, keeping its class, which was given knowing the message.
func (s *Store) SaveMailCompanyFromItsSender(ctx context.Context, messageID, companyID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE mail_messages SET company_id = $2, matched_by = 'domain' WHERE id = $1`, messageID, companyID)
	return err
}
