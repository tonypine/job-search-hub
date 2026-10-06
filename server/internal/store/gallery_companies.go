package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// GalleryCompany is a company startups.gallery lists as remote: its name,
// site and careers link, and the board that link names, if any.
type GalleryCompany struct {
	Slug       string     `json:"slug"`
	Name       string     `json:"name"`
	Website    string     `json:"website,omitempty"`
	CareersURL string     `json:"careers_url,omitempty"`
	Provider   string     `json:"provider,omitempty"`
	BoardToken string     `json:"board_token,omitempty"`
	JobBoardID *uuid.UUID `json:"job_board_id,omitempty"`
}

// GalleryPage is what a company's page on startups.gallery links to.
type GalleryPage struct {
	Website    string
	CareersURL string
	Provider   string
	BoardToken string
}

const galleryCompanyColumns = `gallery_companies.slug, gallery_companies.name, gallery_companies.website, gallery_companies.careers_url,
	gallery_companies.provider, gallery_companies.board_token, gallery_companies.job_board_id`

func scanGalleryCompany(row pgx.CollectableRow) (GalleryCompany, error) {
	var company GalleryCompany
	err := row.Scan(&company.Slug, &company.Name, &company.Website, &company.CareersURL, &company.Provider, &company.BoardToken, &company.JobBoardID)
	return company, err
}

// GetLastGalleryListRead returns when startups.gallery's remote list was
// last read, or nil when it never was.
func (s *Store) GetLastGalleryListRead(ctx context.Context) (*time.Time, error) {
	var readAt *time.Time
	err := s.pool.QueryRow(ctx, `SELECT max(read_at) FROM gallery_list_reads`).Scan(&readAt)
	return readAt, err
}

// RecordGalleryListRead records a read of the remote list and how many
// companies it listed.
func (s *Store) RecordGalleryListRead(ctx context.Context, listed int) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO gallery_list_reads (listed) VALUES ($1)`, listed)
	return err
}

// SaveListedGalleryCompanies stores the companies the remote list named, by
// slug, and returns how many were new.
func (s *Store) SaveListedGalleryCompanies(ctx context.Context, companies []GalleryCompany) (int, error) {
	stored := 0
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, company := range companies {
			tag, err := tx.Exec(ctx, `INSERT INTO gallery_companies (slug, name) VALUES ($1, $2) ON CONFLICT DO NOTHING`, company.Slug, company.Name)
			if err != nil {
				return err
			}
			stored += int(tag.RowsAffected())
		}
		return nil
	})
	return stored, err
}

// ListUnreadGalleryPages returns up to limit of the listed companies whose
// page wasn't read yet, oldest first.
func (s *Store) ListUnreadGalleryPages(ctx context.Context, limit int) ([]GalleryCompany, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+galleryCompanyColumns+` FROM gallery_companies WHERE page_read_at IS NULL ORDER BY found_at, slug LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanGalleryCompany)
}

// RecordGalleryPage stores what a company's page links to.
func (s *Store) RecordGalleryPage(ctx context.Context, slug string, page GalleryPage) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE gallery_companies SET website = $2, careers_url = $3, provider = $4, board_token = $5, page_read_at = now()
		WHERE slug = $1`, slug, page.Website, page.CareersURL, page.Provider, page.BoardToken)
	return err
}

// ListUnkeptGalleryBoards returns up to limit of the listed companies whose
// careers link names a board not kept yet, the longest unchecked first.
func (s *Store) ListUnkeptGalleryBoards(ctx context.Context, limit int) ([]GalleryCompany, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+galleryCompanyColumns+` FROM gallery_companies
		WHERE board_token <> '' AND job_board_id IS NULL
		ORDER BY checked_at NULLS FIRST, found_at, slug LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanGalleryCompany)
}

// RecordGalleryBoardCheck records that a listed company's board titles were
// checked, with the board kept for it, or nil when none was.
func (s *Store) RecordGalleryBoardCheck(ctx context.Context, slug string, jobBoardID *uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE gallery_companies SET checked_at = now(), job_board_id = $2 WHERE slug = $1`, slug, jobBoardID)
	return err
}

// ListGalleryCompaniesNotInHub returns the listed companies with a kept
// board that the hub doesn't hold, by the board or by name, and the owner
// never worked at.
func (s *Store) ListGalleryCompaniesNotInHub(ctx context.Context) ([]GalleryCompany, error) {
	var companies []GalleryCompany
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		companyIDs, err := getCompanyIDsByName(ctx, tx)
		if err != nil {
			return err
		}
		formerEmployers, err := getFormerEmployerNames(ctx, tx)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT `+galleryCompanyColumns+`
			FROM gallery_companies JOIN job_boards ON job_boards.id = gallery_companies.job_board_id
			WHERE job_boards.company_id IS NULL
			ORDER BY gallery_companies.found_at DESC, gallery_companies.slug`)
		if err != nil {
			return err
		}
		listed, err := pgx.CollectRows(rows, scanGalleryCompany)
		if err != nil {
			return err
		}
		for _, company := range listed {
			key := NormalizeCompanyName(company.Name)
			if _, inHub := companyIDs[key]; inHub || isFormerEmployer(key, formerEmployers) {
				continue
			}
			companies = append(companies, company)
		}
		return nil
	})
	return companies, err
}
