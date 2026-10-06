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
	// AppVersion is the version of the app the phone last called the hub
	// with; nil until it sends one.
	AppVersion *string `json:"app_version,omitempty"`
}

const deviceColumns = `id, name, created_at, last_seen_at, revoked_at, app_version`

func scanDevice(row pgx.Row) (Device, error) {
	var device Device
	err := row.Scan(&device.ID, &device.Name, &device.CreatedAt, &device.LastSeenAt, &device.RevokedAt, &device.AppVersion)
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
// token hashes to tokenHash, and notes that it was seen, with appVersion, the
// version of the app it calls from, when it sends one.
func (s *Store) GetActiveDeviceByTokenHash(ctx context.Context, tokenHash []byte, appVersion string) (Device, error) {
	device, err := scanDevice(s.pool.QueryRow(ctx, `SELECT `+deviceColumns+` FROM devices WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash))
	if err != nil {
		return Device{}, err
	}
	updated := appVersion != "" && (device.AppVersion == nil || *device.AppVersion != appVersion)
	if updated || device.LastSeenAt == nil || time.Since(*device.LastSeenAt) > deviceSeenInterval {
		if appVersion == "" {
			device, err = scanDevice(s.pool.QueryRow(ctx, `UPDATE devices SET last_seen_at = now() WHERE id = $1 RETURNING `+deviceColumns, device.ID))
		} else {
			device, err = scanDevice(s.pool.QueryRow(ctx, `UPDATE devices SET last_seen_at = now(), app_version = $2 WHERE id = $1 RETURNING `+deviceColumns, device.ID, appVersion))
		}
		if err != nil {
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

// SetDevicePushToken records the token FCM gave the device's app. A token
// moves to the newest device that registers it, as when a phone is paired
// again.
func (s *Store) SetDevicePushToken(ctx context.Context, deviceID uuid.UUID, pushToken string) error {
	pushToken = strings.TrimSpace(pushToken)
	if pushToken == "" {
		return errors.New("the push token is empty")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE devices SET push_token = NULL WHERE push_token = $1 AND id <> $2`, pushToken, deviceID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE devices SET push_token = $2 WHERE id = $1 AND revoked_at IS NULL`, deviceID, pushToken)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrDeviceNotFound
		}
		return err
	})
}

// ListDevicePushTokens returns the push tokens of the paired, unrevoked
// devices.
func (s *Store) ListDevicePushTokens(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT push_token FROM devices WHERE push_token IS NOT NULL AND revoked_at IS NULL ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// ForgetDevicePushToken drops a token FCM no longer delivers to.
func (s *Store) ForgetDevicePushToken(ctx context.Context, pushToken string) error {
	_, err := s.pool.Exec(ctx, `UPDATE devices SET push_token = NULL WHERE push_token = $1`, pushToken)
	return err
}
