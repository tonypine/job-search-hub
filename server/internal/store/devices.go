package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrDeviceNotFound = errors.New("device not found")

// Device is a phone paired with the hub. Its token is shown once, when it is
// paired; the hub keeps only the token's hash.
type Device struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

const deviceColumns = `id, name, created_at, last_seen_at, revoked_at`

func scanDevice(row pgx.Row) (Device, error) {
	var device Device
	err := row.Scan(&device.ID, &device.Name, &device.CreatedAt, &device.LastSeenAt, &device.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrDeviceNotFound
	}
	return device, err
}

// CreateDevice pairs a phone under the hash of its new token.
func (s *Store) CreateDevice(ctx context.Context, actor Actor, name string, tokenHash []byte) (Device, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Device{}, errors.New("a phone needs a name, e.g. Tony's phone")
	}
	var device Device
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		device, err = scanDevice(tx.QueryRow(ctx, `INSERT INTO devices (name, token_hash) VALUES ($1, $2) RETURNING `+deviceColumns, name, tokenHash))
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{entityType: "device", entityID: device.ID, operation: "pair", after: map[string]string{"name": name}})
	})
	return device, err
}

// deviceSeenInterval is how often a device's last use is written: a phone
// calls the hub many times a minute.
const deviceSeenInterval = time.Minute

// GetActiveDeviceByTokenHash returns the paired, unrevoked device whose
// token hashes to tokenHash, and notes that it was seen.
func (s *Store) GetActiveDeviceByTokenHash(ctx context.Context, tokenHash []byte) (Device, error) {
	device, err := scanDevice(s.pool.QueryRow(ctx, `SELECT `+deviceColumns+` FROM devices WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash))
	if err != nil {
		return Device{}, err
	}
	if device.LastSeenAt == nil || time.Since(*device.LastSeenAt) > deviceSeenInterval {
		if _, err := s.pool.Exec(ctx, `UPDATE devices SET last_seen_at = now() WHERE id = $1`, device.ID); err != nil {
			return Device{}, err
		}
	}
	return device, nil
}

// ListDevices returns the paired phones, revoked ones included, newest first.
func (s *Store) ListDevices(ctx context.Context) ([]Device, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+deviceColumns+` FROM devices ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Device, error) { return scanDevice(row) })
}

// RevokeDevice stops a device's token from working.
func (s *Store) RevokeDevice(ctx context.Context, actor Actor, id uuid.UUID) (Device, error) {
	var device Device
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		device, err = scanDevice(tx.QueryRow(ctx, `
			UPDATE devices SET revoked_at = coalesce(revoked_at, now()) WHERE id = $1 RETURNING `+deviceColumns, id))
		if err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{entityType: "device", entityID: device.ID, operation: "revoke"})
	})
	return device, err
}
