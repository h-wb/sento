package store

import (
	"context"
)

type Binding struct {
	TrustChapters  bool   `json:"trustChapters"`
	EditionID      int64  `json:"editionId"`
	ID             int64  `json:"id"`
	SeriesID       int64  `json:"seriesId"`
	SourceID       string `json:"sourceId"`
	SourceMangaKey string `json:"sourceMangaKey"` // opaque, minted by the extensions sidecar
	SourceName     string `json:"sourceName"`
	MangaTitle     string `json:"mangaTitle"`
	State          string `json:"state"`
	Priority       int    `json:"priority"`
}

func (s *Store) ListBindings(ctx context.Context, seriesID int64) ([]Binding, error) {
	return list[Binding](ctx, s, `
		SELECT id, series_id, source_id, source_manga_key,
		       COALESCE(source_name,'') AS source_name,
		       COALESCE(manga_title,'') AS manga_title, state, priority,edition_id,trust_chapters
		FROM source_bindings WHERE series_id = ? ORDER BY priority, id`, seriesID)
}

// AddBinding upserts one series↔manga binding (a site can hold several).
// An active binding obsoletes any "unmatched" sentinel of the same site.
func (s *Store) AddBinding(ctx context.Context, b Binding) (int64, error) {
	if b.EditionID > 0 {
		c, e := s.GetEdition(ctx, b.EditionID)
		if e != nil {
			return 0, e
		}
		if c.SeriesID != b.SeriesID {
			return 0, ErrNotFound
		}
	}
	state := orDefault(b.State, "active")
	var id int64
	err := s.db.QueryRowContext(ctx, s.q(`
		INSERT INTO source_bindings (series_id, source_id, source_manga_key, source_name, manga_title, state, priority,edition_id,trust_chapters)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON CONFLICT(series_id, source_id, source_manga_key) DO UPDATE SET
			source_name = excluded.source_name,
			manga_title = excluded.manga_title,
			state = excluded.state,
			trust_chapters=CASE WHEN excluded.edition_id>0 THEN excluded.trust_chapters ELSE source_bindings.trust_chapters END,
			edition_id=CASE WHEN excluded.edition_id>0 THEN excluded.edition_id ELSE source_bindings.edition_id END
		RETURNING id`),
		b.SeriesID, b.SourceID, b.SourceMangaKey, b.SourceName, b.MangaTitle, state, b.Priority, b.EditionID, b.TrustChapters).Scan(&id)
	if err != nil {
		return 0, err
	}
	if state == "active" {
		_, _ = s.db.ExecContext(ctx, s.q(`
			DELETE FROM source_bindings
			WHERE series_id = ? AND source_id = ? AND state = 'unmatched'`),
			b.SeriesID, b.SourceID)
	}
	return id, nil
}

// DiscoverBinding adds a newly found listing without overwriting an
// existing match's state, edition, trust, or priority, even if another
// search or a person's edit saved it after the discovery began.
func (s *Store) DiscoverBinding(ctx context.Context, b Binding) (*Binding, error) {
	return get[Binding](ctx, s, `
		INSERT INTO source_bindings (series_id, source_id, source_manga_key, source_name, manga_title, state, priority)
		VALUES (?,?,?,?,?,'active',?)
		ON CONFLICT(series_id, source_id, source_manga_key) DO UPDATE SET
			source_manga_key = excluded.source_manga_key
		RETURNING id, series_id, source_id, source_manga_key,
		          source_name, manga_title, state, priority, edition_id, trust_chapters`,
		b.SeriesID, b.SourceID, b.SourceMangaKey, b.SourceName, b.MangaTitle, b.Priority)
}

// SetBindingState toggles one binding ("active" / "disabled").
func (s *Store) SetBindingState(ctx context.Context, seriesID, bindingID int64, state string) error {
	return s.touch(ctx, `UPDATE source_bindings SET state = ? WHERE id = ? AND series_id = ?`,
		state, bindingID, seriesID)
}

func (s *Store) DeleteBinding(ctx context.Context, seriesID, bindingID int64) error {
	return s.touch(ctx, `DELETE FROM source_bindings WHERE id = ? AND series_id = ?`, bindingID, seriesID)
}
