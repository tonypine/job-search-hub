package store

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tonypine/job-search-hub/server/internal/wordmatch"
)

var (
	ErrApplicationAnswerNotFound = errors.New("application answer not found")
	ErrDuplicateQuestion         = errors.New("that question already has an answer")
)

// Where an answer came from.
const (
	AnswerSourceOwner    = "owner"
	AnswerSourceLinkedIn = "linkedin"
)

// ApplicationAnswer is the owner's answer to a question application forms
// ask; an empty answer is a question still to answer.
type ApplicationAnswer struct {
	ID        uuid.UUID `json:"id"`
	Question  string    `json:"question"`
	Answer    string    `json:"answer"`
	Source    string    `json:"source"`
	UpdatedAt time.Time `json:"updated_at"`
}

const applicationAnswerColumns = `id, question, answer, source, updated_at`

func scanApplicationAnswer(row pgx.Row) (ApplicationAnswer, error) {
	var answer ApplicationAnswer
	err := row.Scan(&answer.ID, &answer.Question, &answer.Answer, &answer.Source, &answer.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ApplicationAnswer{}, ErrApplicationAnswerNotFound
	}
	return answer, err
}

// getQuestionKey is a question without case, accents or punctuation, so
// "Notice period?" and "notice period" are the same question.
func getQuestionKey(question string) string {
	letters := strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return character
		}
		return ' '
	}, wordmatch.Normalize(question))
	return strings.Join(strings.Fields(letters), " ")
}

// ListApplicationAnswers returns every answer, in the order they were added.
func (s *Store) ListApplicationAnswers(ctx context.Context) ([]ApplicationAnswer, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+applicationAnswerColumns+` FROM application_answers ORDER BY created_at, question`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ApplicationAnswer, error) { return scanApplicationAnswer(row) })
}

// SaveApplicationAnswer answers a question: it updates the answer with id,
// or, without one, the answer to the same question, adding it when there is
// none.
func (s *Store) SaveApplicationAnswer(ctx context.Context, actor Actor, id *uuid.UUID, question, answer string) (ApplicationAnswer, error) {
	question, answer = strings.TrimSpace(question), strings.TrimSpace(answer)
	if question == "" {
		return ApplicationAnswer{}, errors.New("an answer needs its question")
	}
	var saved ApplicationAnswer
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		if id != nil {
			saved, err = scanApplicationAnswer(tx.QueryRow(ctx, `
				UPDATE application_answers SET question = $2, question_key = $3, answer = $4, updated_at = now()
				WHERE id = $1 RETURNING `+applicationAnswerColumns, *id, question, getQuestionKey(question), answer))
		} else {
			saved, err = scanApplicationAnswer(tx.QueryRow(ctx, `
				INSERT INTO application_answers (question, question_key, answer, source) VALUES ($1, $2, $3, $4)
				ON CONFLICT (question_key) DO UPDATE SET answer = EXCLUDED.answer, updated_at = now()
				RETURNING `+applicationAnswerColumns, question, getQuestionKey(question), answer, AnswerSourceOwner))
		}
		if isUniqueViolation(err) {
			return ErrDuplicateQuestion
		}
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "application_answer", entityID: saved.ID, operation: "save",
			after: map[string]string{"question": saved.Question, "answer": saved.Answer},
		})
	})
	return saved, err
}

// DeleteApplicationAnswer removes an answer.
func (s *Store) DeleteApplicationAnswer(ctx context.Context, actor Actor, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		deleted, err := scanApplicationAnswer(tx.QueryRow(ctx, `DELETE FROM application_answers WHERE id = $1 RETURNING `+applicationAnswerColumns, id))
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "application_answer", entityID: deleted.ID, operation: "delete",
			before: map[string]string{"question": deleted.Question, "answer": deleted.Answer},
		})
	})
}

// NewApplicationAnswer is an answer read from an export.
type NewApplicationAnswer struct {
	Question string
	Answer   string
}

// AnswersImport counts what an import of saved answers added.
type AnswersImport struct {
	Added           int `json:"added"`
	AlreadyAnswered int `json:"already_answered"`
}

// ImportApplicationAnswers adds the answers to questions the library doesn't
// hold yet; an answer the owner already has is never replaced.
func (s *Store) ImportApplicationAnswers(ctx context.Context, actor Actor, answers []NewApplicationAnswer) (AnswersImport, error) {
	var result AnswersImport
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, answer := range answers {
			question := strings.TrimSpace(answer.Question)
			saved, err := scanApplicationAnswer(tx.QueryRow(ctx, `
				INSERT INTO application_answers (question, question_key, answer, source) VALUES ($1, $2, $3, $4)
				ON CONFLICT (question_key) DO NOTHING
				RETURNING `+applicationAnswerColumns, question, getQuestionKey(question), strings.TrimSpace(answer.Answer), AnswerSourceLinkedIn))
			if errors.Is(err, ErrApplicationAnswerNotFound) {
				result.AlreadyAnswered++
				continue
			}
			if err != nil {
				return err
			}
			result.Added++
			if err := insertChange(ctx, tx, actor, change{
				entityType: "application_answer", entityID: saved.ID, operation: "import", after: map[string]string{"question": saved.Question},
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}
