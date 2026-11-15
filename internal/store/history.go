package store

import (
	"context"
	"encoding/json"
)

// Decision is one history event (grab/import/failure) — the explainable
// record of everything Sento did autonomously.
type Decision struct {
	ID        int64    `json:"id"`
	SeriesID  int64    `json:"seriesId"`
	Body      JSONText `json:"body" db:"body_json" tstype:"DecisionBody"`
	CreatedAt string   `json:"createdAt"`
}

// DecisionBody is the JSON payload of one history event.
type DecisionBody struct {
	Event    string `json:"event"` // grabbed | imported | imported_after_review | import_failed | download_failed | download_retry | download_needs_review | blocklisted | editions_grouped | listing_switched_off
	Ledger   string `json:"ledger,omitempty"`
	Title    string `json:"title,omitempty"` // release title
	Provider string `json:"provider,omitempty"`
	Site     string `json:"site,omitempty"`
	Key      string `json:"key,omitempty"` // external id (infohash / chapter id)
	// "books" is the name in decisions already stored.
	Books []BookRef `json:"slots,omitempty"`
	Error string    `json:"error,omitempty"`
	// Note says what happened when it is not an error.
	Note  string `json:"note,omitempty"`
	Force bool   `json:"force,omitempty"`
	// By says who chose a grabbed release: "schedule" for Sento's own
	// searches, "you" for a pick in the search dialog.
	By string `json:"by,omitempty" tstype:"'schedule' | 'you'"`
}

func (s *Store) AddDecision(ctx context.Context, seriesID int64, body DecisionBody) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		s.q(`INSERT INTO decisions (series_id, body_json) VALUES (?,?)`), seriesID, string(raw))
	return err
}

// PruneDecisions deletes history beyond the newest keep (per series and
// overall), so the decisions table does not grow without bound.
func (s *Store) PruneDecisions(ctx context.Context, keep int) (int64, error) {
	res, err := s.db.ExecContext(ctx, s.q(`
		DELETE FROM decisions WHERE id NOT IN (
			SELECT id FROM decisions ORDER BY id DESC LIMIT ?
		)`), keep)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ListDecisions returns recent history, newest first (seriesID 0 = all).
func (s *Store) ListDecisions(ctx context.Context, seriesID int64, limit int) ([]Decision, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `SELECT id, COALESCE(series_id,0) AS series_id, body_json, created_at FROM decisions`
	args := []any{}
	if seriesID != 0 {
		q += ` WHERE series_id = ?`
		args = append(args, seriesID)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	return list[Decision](ctx, s, q, args...)
}
