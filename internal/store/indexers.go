package store

import (
	"context"
)

// Indexer protocols.
const (
	IndexerTorznab = "torznab"
	IndexerSource  = "source"
)

// Indexer is one explicit searchable entry (Sonarr's model): a Torznab
// endpoint or an extension source on the host. The user-given name is
// the display name everywhere (candidates, history, queue). Settings is
// the protocol-specific document (TorznabSettings / SourceSettings in
// the service layer).
type Indexer struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Protocol    string   `json:"protocol" tstype:"'torznab' | 'source'"`
	Enabled     bool     `json:"enabled"`
	UseVolumes  bool     `json:"useVolumes"`
	UseChapters bool     `json:"useChapters"`
	Priority    int      `json:"priority"`
	Settings    JSONText `json:"settings" tstype:"any"`
}

const indexerColumns = `id, name, protocol, enabled, use_volumes, use_chapters, priority,
	settings_json AS settings`

func (s *Store) ListIndexers(ctx context.Context) ([]Indexer, error) {
	return list[Indexer](ctx, s, `SELECT `+indexerColumns+` FROM indexers ORDER BY name, id`)
}

func (s *Store) GetIndexer(ctx context.Context, id int64) (*Indexer, error) {
	return get[Indexer](ctx, s, `SELECT `+indexerColumns+` FROM indexers WHERE id = ?`, id)
}

func (s *Store) AddIndexer(ctx context.Context, ix Indexer) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO indexers (name, protocol, enabled, use_volumes, use_chapters, priority, settings_json)
		VALUES (?,?,?,?,?,?,?)`,
		ix.Name, ix.Protocol, ix.Enabled, ix.UseVolumes, ix.UseChapters, ix.Priority, string(ix.Settings))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateIndexer(ctx context.Context, ix Indexer) error {
	return s.touch(ctx, `
		UPDATE indexers SET name = ?, enabled = ?, use_volumes = ?, use_chapters = ?,
			priority = ?, settings_json = ?
		WHERE id = ?`,
		ix.Name, ix.Enabled, ix.UseVolumes, ix.UseChapters, ix.Priority, string(ix.Settings), ix.ID)
}

func (s *Store) DeleteIndexer(ctx context.Context, id int64) error {
	// Per-series overrides of the entry go with it.
	_, _ = s.db.ExecContext(ctx,
		`DELETE FROM series_site_settings WHERE provider = 'indexer' AND site_id = ?`, id)
	return s.touch(ctx, `DELETE FROM indexers WHERE id = ?`, id)
}
