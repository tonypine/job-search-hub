package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrArtifactNotFound = errors.New("artifact not found")

// MaximumArtifactSize is the largest file the hub keeps.
const MaximumArtifactSize = 10 << 20

// Kinds of artifact: what a file the owner uploaded is.
const (
	ArtifactResume          = "resume"
	ArtifactCompanyDocument = "company_document"
	ArtifactSavedPage       = "saved_page"
	ArtifactOther           = "other"
)

var artifactKinds = map[string]bool{ArtifactResume: true, ArtifactCompanyDocument: true, ArtifactSavedPage: true, ArtifactOther: true}

// Artifact is a file the owner uploaded as context for the agents, about
// themselves (no company) or about one company. Its content is read apart.
type Artifact struct {
	ID          uuid.UUID  `json:"id"`
	CompanyID   *uuid.UUID `json:"company_id,omitempty"`
	Kind        string     `json:"kind"`
	Name        string     `json:"name"`
	ContentType string     `json:"content_type"`
	Size        int        `json:"size"`
	SHA256      string     `json:"sha256"`
	CreatedAt   time.Time  `json:"created_at"`
	// TextLength is how much text was read from the file; TextError says
	// why none could be, and both are empty until it is read.
	TextLength int    `json:"text_length"`
	TextError  string `json:"text_error,omitempty"`
}

// HasReadText says whether reading the file's text was tried.
func (artifact Artifact) HasReadText() bool {
	return artifact.TextLength > 0 || artifact.TextError != ""
}

// NewArtifact is a file being uploaded.
type NewArtifact struct {
	CompanyID   *uuid.UUID
	Kind        string
	Name        string
	ContentType string
	Content     []byte
	// Text is the file's text, or TextError why none could be read.
	Text      string
	TextError string
}

const artifactColumns = `id, company_id, kind, name, content_type, size, encode(sha256, 'hex'), created_at,
	coalesce(length(text), 0), text_error`

func scanArtifact(row pgx.Row) (Artifact, error) {
	var artifact Artifact
	err := row.Scan(&artifact.ID, &artifact.CompanyID, &artifact.Kind, &artifact.Name, &artifact.ContentType, &artifact.Size,
		&artifact.SHA256, &artifact.CreatedAt, &artifact.TextLength, &artifact.TextError)
	if errors.Is(err, pgx.ErrNoRows) {
		return Artifact{}, ErrArtifactNotFound
	}
	return artifact, err
}

// SaveArtifact stores an uploaded file. The same content uploaded again is
// stored once: created is false and the stored artifact is returned.
func (s *Store) SaveArtifact(ctx context.Context, actor Actor, input NewArtifact) (artifact Artifact, created bool, err error) {
	switch {
	case !artifactKinds[input.Kind]:
		return Artifact{}, false, errors.New("kind must be resume, company_document, saved_page or other")
	case strings.TrimSpace(input.Name) == "":
		return Artifact{}, false, errors.New("a file needs a name")
	case len(input.Content) == 0:
		return Artifact{}, false, errors.New("the file is empty")
	case len(input.Content) > MaximumArtifactSize:
		return Artifact{}, false, errors.New("files over 10 MB are refused")
	}
	sum := sha256.Sum256(input.Content)
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		artifact, err = scanArtifact(tx.QueryRow(ctx, `
			INSERT INTO artifacts (company_id, kind, name, content_type, size, sha256, content, text, text_error)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9)
			ON CONFLICT (sha256) DO NOTHING
			RETURNING `+artifactColumns,
			input.CompanyID, input.Kind, strings.TrimSpace(input.Name), input.ContentType, len(input.Content), sum[:], input.Content,
			input.Text, input.TextError))
		if errors.Is(err, ErrArtifactNotFound) {
			artifact, err = scanArtifact(tx.QueryRow(ctx, `SELECT `+artifactColumns+` FROM artifacts WHERE sha256 = $1`, sum[:]))
			return err
		}
		if err != nil {
			return err
		}
		created = true
		return insertChange(ctx, tx, actor, change{
			entityType: "artifact", entityID: artifact.ID, operation: "create",
			after: map[string]any{"kind": artifact.Kind, "name": artifact.Name, "size": artifact.Size, "sha256": hex.EncodeToString(sum[:])},
		})
	})
	return artifact, created, err
}

// ListArtifacts returns a company's files, or the owner's own when companyID
// is nil, newest first.
func (s *Store) ListArtifacts(ctx context.Context, companyID *uuid.UUID) ([]Artifact, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+artifactColumns+` FROM artifacts
		WHERE company_id IS NOT DISTINCT FROM $1
		ORDER BY created_at DESC`, companyID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Artifact, error) { return scanArtifact(row) })
}

// DeleteArtifact removes a file and records that it was removed.
func (s *Store) DeleteArtifact(ctx context.Context, actor Actor, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		artifact, err := scanArtifact(tx.QueryRow(ctx, `DELETE FROM artifacts WHERE id = $1 RETURNING `+artifactColumns, id))
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{
			entityType: "artifact", entityID: artifact.ID, operation: "delete",
			before: map[string]any{"kind": artifact.Kind, "name": artifact.Name, "size": artifact.Size},
		})
	})
}

// SaveArtifactText stores the text read from a file stored before its text
// was read, or why none could be.
func (s *Store) SaveArtifactText(ctx context.Context, id uuid.UUID, text, textError string) (Artifact, error) {
	return scanArtifact(s.pool.QueryRow(ctx, `
		UPDATE artifacts SET text = NULLIF($2, ''), text_error = $3 WHERE id = $1
		RETURNING `+artifactColumns, id, text, textError))
}

// ArtifactText is a file's text, as agents read it.
type ArtifactText struct {
	Artifact
	Text string `json:"text"`
}

// GetArtifactText returns a file's text; its raw content is never handed out.
func (s *Store) GetArtifactText(ctx context.Context, id uuid.UUID) (ArtifactText, error) {
	var text ArtifactText
	var stored *string
	row := s.pool.QueryRow(ctx, `SELECT `+artifactColumns+`, text FROM artifacts WHERE id = $1`, id)
	err := row.Scan(&text.ID, &text.CompanyID, &text.Kind, &text.Name, &text.ContentType, &text.Size, &text.SHA256, &text.CreatedAt,
		&text.TextLength, &text.TextError, &stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return ArtifactText{}, ErrArtifactNotFound
	}
	if stored != nil {
		text.Text = *stored
	}
	return text, err
}

// ListArtifactTexts returns the text of a company's files, or the owner's own
// when companyID is nil, oldest first, leaving out files without text.
func (s *Store) ListArtifactTexts(ctx context.Context, companyID *uuid.UUID) ([]ArtifactText, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+artifactColumns+`, text FROM artifacts
		WHERE company_id IS NOT DISTINCT FROM $1 AND text IS NOT NULL
		ORDER BY created_at`, companyID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ArtifactText, error) {
		var text ArtifactText
		err := row.Scan(&text.ID, &text.CompanyID, &text.Kind, &text.Name, &text.ContentType, &text.Size, &text.SHA256, &text.CreatedAt,
			&text.TextLength, &text.TextError, &text.Text)
		return text, err
	})
}
