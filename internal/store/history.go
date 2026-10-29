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
	Event    string `json:"event"` // grabbed | imported | imported_after_review | import_failed | download_failed | blocklisted | editions_grouped
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

// BlocklistEntry bans one release (by protocol + external key) from ever
// being grabbed again.
type BlocklistEntry struct {
	ID        int64  `json:"id"`
	Protocol  string `json:"protocol"`
	Key       string `json:"key"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"createdAt"`
}

func (s *Store) AddBlocklist(ctx context.Context, protocol, key, reason string) error {
	_, err := s.db.ExecContext(ctx, s.q(`
		INSERT INTO blocklist (protocol, key, reason) VALUES (?,?,?)
		ON CONFLICT(protocol, key) DO UPDATE SET reason = excluded.reason`),
		protocol, key, reason)
	return err
}

func (s *Store) ListBlocklist(ctx context.Context) ([]BlocklistEntry, error) {
	return list[BlocklistEntry](ctx, s, `
		SELECT id, protocol, key, reason, created_at FROM blocklist ORDER BY id DESC LIMIT 500`)
}

func (s *Store) DeleteBlocklist(ctx context.Context, id int64) error {
	return s.touch(ctx, `DELETE FROM blocklist WHERE id = ?`, id)
}

// BlockedKeys returns the active ban set keyed "protocol/key".
func (s *Store) BlockedKeys(ctx context.Context) (map[string]bool, error) {
	rows, err := list[BlocklistEntry](ctx, s, `SELECT id, protocol, key, COALESCE(reason,'') AS reason, created_at FROM blocklist`)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, e := range rows {
		out[e.Protocol+"/"+e.Key] = true
	}
	return out, nil
}
