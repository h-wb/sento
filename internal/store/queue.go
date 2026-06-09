package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type QueueItem struct {
	ID            int64           `json:"id"`
	SeriesID      int64           `json:"seriesId"`
	Protocol      string          `json:"protocol"` // torrent|source
	ExternalID    string          `json:"externalId"`
	Title         string          `json:"title"`
	Status        string          `json:"status"`
	ExpectedSlots json.RawMessage `json:"expectedSlots"`
	Error         string          `json:"error,omitempty"`
	CreatedAt     string          `json:"createdAt"`
	UpdatedAt     string          `json:"updatedAt,omitempty"`
}

const (
	QueueQueued      = "queued"
	QueueDownloading = "downloading"
	QueueImporting   = "importing"
	QueueImported    = "imported"
	QueueFailed      = "failed"
)

// ExpectedSlot mirrors file_slot_links coordinates for pending grabs.
type ExpectedSlot struct {
	Ledger       string `json:"ledger"`
	CollectionID int64  `json:"collectionId"`
	Number       int64  `json:"number"`
}

func (s *Store) AddQueueItem(ctx context.Context, q QueueItem) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO queue_items (series_id, protocol, external_id, title, status, expected_slots_json)
		VALUES (?,?,?,?,?,?)`,
		q.SeriesID, q.Protocol, q.ExternalID, q.Title, orDefault(q.Status, QueueQueued),
		string(q.ExpectedSlots))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateQueueItem(ctx context.Context, id int64, status, errMsg string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE queue_items SET status = ?, error = ?, updated_at = datetime('now') WHERE id = ?`,
		status, errMsg, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListQueueItems(ctx context.Context, activeOnly bool) ([]QueueItem, error) {
	q := `SELECT id, series_id, protocol, external_id, title, status,
	             expected_slots_json, COALESCE(error,''), created_at, COALESCE(updated_at,'')
	      FROM queue_items`
	if activeOnly {
		q += ` WHERE status IN ('queued','downloading','importing')`
	}
	q += ` ORDER BY created_at DESC LIMIT 200`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []QueueItem
	for rows.Next() {
		var item QueueItem
		var slots string
		if err := rows.Scan(&item.ID, &item.SeriesID, &item.Protocol, &item.ExternalID,
			&item.Title, &item.Status, &slots, &item.Error, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.ExpectedSlots = json.RawMessage(slots)
		out = append(out, item)
	}
	return out, rows.Err()
}

// PendingSlotKeys returns slot keys currently held by active queue items of
// a series (these slots are "pending", not "wanted").
func (s *Store) PendingSlotKeys(ctx context.Context, seriesID int64) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT expected_slots_json FROM queue_items
		WHERE series_id = ? AND status IN ('queued','downloading','importing')`, seriesID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var slots []ExpectedSlot
		if err := json.Unmarshal([]byte(raw), &slots); err != nil {
			continue
		}
		for _, sl := range slots {
			out[SlotKey(sl.Ledger, sl.CollectionID, sl.Number)] = true
		}
	}
	return out, rows.Err()
}

func (s *Store) SetSeriesPath(ctx context.Context, seriesID int64, path string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE series SET path = ? WHERE id = ?`, path, seriesID)
	return err
}

// SeriesTitles returns the primary title (raw_json carries aliases; the
// primary is enough for anchoring v1).
func (s *Store) SeriesTitles(ctx context.Context, seriesID int64) ([]string, error) {
	var title string
	err := s.db.QueryRowContext(ctx, `SELECT title FROM series WHERE id = ?`, seriesID).Scan(&title)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return []string{title}, nil
}
