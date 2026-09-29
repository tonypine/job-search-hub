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
}

// NewArtifact is a file being uploaded.
type NewArtifact struct {
	CompanyID   *uuid.UUID
	Kind        string
	Name        string
	ContentType string
	Content     []byte
}

const artifactColumns = `id, company_id, kind, name, content_type, size, encode(sha256, 'hex'), created_at`

func scanArtifact(row pgx.Row) (Artifact, error) {
	var artifact Artifact
	err := row.Scan(&artifact.ID, &artifact.CompanyID, &artifact.Kind, &artifact.Name, &artifact.ContentType, &artifact.Size,
		&artifact.SHA256, &artifact.CreatedAt)
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
			INSERT INTO artifacts (company_id, kind, name, content_type, size, sha256, content)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (sha256) DO NOTHING
			RETURNING `+artifactColumns,
			input.CompanyID, input.Kind, strings.TrimSpace(input.Name), input.ContentType, len(input.Content), sum[:], input.Content))
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
