package store

import (
	"context"
	"encoding/json"
)

// Fixtures for the tests of this and other packages; nothing else calls them.

func (s *Store) AddBlocklist(ctx context.Context, protocol, key, reason string) error {
	_, err := s.db.ExecContext(ctx, s.q(`
		INSERT INTO blocklist (protocol, key, reason) VALUES (?,?,?)
		ON CONFLICT(protocol, key) DO UPDATE SET reason = excluded.reason`),
		protocol, key, reason)
	return err
}

func (s *Store) ListBlocklist(ctx context.Context) ([]BlocklistEntry, error) {
	return list[BlocklistEntry](ctx, s, `
		SELECT id, protocol, key, COALESCE(reason,'') AS reason, created_at, title FROM blocklist ORDER BY id DESC`)
}

func (s *Store) AddQueueItem(ctx context.Context, q QueueItem) (int64, error) {
	return s.insert(ctx, `
		INSERT INTO queue_items (series_id, protocol, external_id, title, status, books_json)
		VALUES (?,?,?,?,?,?) RETURNING id`,
		q.SeriesID, q.Protocol, q.ExternalID, q.Title, orDefault(q.Status, QueueQueued),
		string(q.Books))
}

func (s *Store) PendingBooks(ctx context.Context, seriesID int64) (map[string]bool, error) {
	states, err := s.PendingBookStates(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for key := range states {
		out[key] = true
	}
	return out, nil
}

func (s *Store) SetAssetFacts(ctx context.Context, id int64, f AssetFacts, hash string) error {
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	// A different fingerprint was not read from the archive the signature describes.
	return s.touch(ctx, `UPDATE files SET facts_json=?,hash_stat=CASE WHEN COALESCE(hash,'')=? THEN hash_stat ELSE '' END,hash=? WHERE id=?`, string(raw), hash, hash, id)
}
