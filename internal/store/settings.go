package store

import (
	"context"
	"database/sql"
	"errors"
)

// GetSetting returns one settings document (ErrNotFound when unset).
func (s *Store) GetSetting(ctx context.Context, key string) ([]byte, error) {
	var v []byte
	err := s.db.QueryRowContext(ctx,
		s.q(`SELECT value_json FROM settings WHERE key = ?`), key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, err
}

// PutSetting upserts one settings document.
func (s *Store) PutSetting(ctx context.Context, key string, value []byte) error {
	_, err := s.db.ExecContext(ctx, s.q(`
		INSERT INTO settings (key, value_json, updated_at) VALUES (?,?,?)
		ON CONFLICT(key) DO UPDATE SET
			value_json = excluded.value_json,
			updated_at = excluded.updated_at`),
		key, value, utcNow())
	return err
}
