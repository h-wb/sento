package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// JSONText is raw JSON that scans from a TEXT column (json.RawMessage
// itself has no sql.Scanner).
type JSONText []byte

func (j *JSONText) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*j = nil
	case string:
		*j = JSONText(v)
	case []byte:
		*j = append(JSONText(nil), v...)
	default:
		return fmt.Errorf("cannot scan %T into JSONText", src)
	}
	return nil
}

func (j JSONText) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

type QueueItem struct {
	ID            int64    `json:"id"`
	SeriesID      int64    `json:"seriesId"`
	Protocol      string   `json:"protocol"` // torrent|source
	ExternalID    string   `json:"externalId"`
	Title         string   `json:"title"`
	Status        string   `json:"status"`
	ExpectedSlots JSONText `json:"expectedSlots" db:"expected_slots_json" tstype:"ExpectedSlot[]"`
	Error         string   `json:"error,omitempty"`
	CreatedAt     string   `json:"createdAt"`
	UpdatedAt     string   `json:"updatedAt,omitempty"`
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
	return s.touch(ctx, `
		UPDATE queue_items SET status = ?, error = ?, updated_at = datetime('now') WHERE id = ?`,
		status, errMsg, id)
}

func (s *Store) ListQueueItems(ctx context.Context, activeOnly bool) ([]QueueItem, error) {
	q := `SELECT id, series_id, protocol, external_id, title, status, expected_slots_json,
	             COALESCE(error,'') AS error, created_at,
	             COALESCE(updated_at,'') AS updated_at
	      FROM queue_items`
	if activeOnly {
		q += ` WHERE status IN ('queued','downloading','importing')`
	}
	q += ` ORDER BY created_at DESC LIMIT 200`
	return list[QueueItem](ctx, s, q)
}

// PendingSlotKeys returns slot keys currently held by active queue items of
// a series (these slots are "pending", not "wanted").
func (s *Store) PendingSlotKeys(ctx context.Context, seriesID int64) (map[string]bool, error) {
	raws, err := list[string](ctx, s, `
		SELECT expected_slots_json FROM queue_items
		WHERE series_id = ? AND status IN ('queued','downloading','importing')`, seriesID)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, raw := range raws {
		var slots []ExpectedSlot
		if err := json.Unmarshal([]byte(raw), &slots); err != nil {
			continue
		}
		for _, sl := range slots {
			out[SlotKey(sl.Ledger, sl.CollectionID, sl.Number)] = true
		}
	}
	return out, nil
}

func (s *Store) SetSeriesPath(ctx context.Context, seriesID int64, path string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE series SET path = ? WHERE id = ?`, path, seriesID)
	return err
}

// SeriesTitles returns the primary title plus every alias from the cached
// provider payload — anchoring and source binding match releases named in
// any language (Japscan lists French titles, raws use kana, …).
func (s *Store) SeriesTitles(ctx context.Context, seriesID int64) ([]string, error) {
	row, err := get[struct {
		Title   string
		RawJSON string `db:"raw_json"`
	}](ctx, s, `SELECT title, COALESCE(raw_json,'') AS raw_json FROM series WHERE id = ?`, seriesID)
	if err != nil {
		return nil, err
	}
	titles := []string{row.Title}
	add := func(t string) {
		t = strings.TrimSpace(t)
		if t == "" {
			return
		}
		for _, ex := range titles {
			if strings.EqualFold(ex, t) {
				return
			}
		}
		titles = append(titles, t)
	}
	var raw struct {
		NativeTitle    string `json:"native_title"`
		RomanizedTitle string `json:"romanized_title"`
		Titles         []struct {
			Title string `json:"title"`
		} `json:"titles"`
		Secondary map[string][]struct {
			Title string `json:"title"`
		} `json:"secondary_titles"`
	}
	if json.Unmarshal([]byte(row.RawJSON), &raw) == nil {
		add(raw.NativeTitle)
		add(raw.RomanizedTitle)
		for _, t := range raw.Titles {
			add(t.Title)
		}
		for _, list := range raw.Secondary {
			for _, t := range list {
				add(t.Title)
			}
		}
	}
	return titles, nil
}
