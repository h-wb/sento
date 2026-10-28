package store

import (
	"context"

	"github.com/h-wb/sento/internal/metadata"
)

// ReconcileMetadata applies a complete catalog snapshot of a series.
// Collections and works are matched by their catalog ids first. Local
// editions are never silently adopted; LinkSuggestions offers the link
// instead. Entries the snapshot no longer lists become stale without
// losing their books, versions, coverage or edits.
func (s *Store) ReconcileMetadata(ctx context.Context, seriesID int64, md *metadata.Series, collections []metadata.Collection, works map[string][]metadata.Work) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, s.q(`UPDATE series SET title=?,sort_title=?,type=?,status=?,content_rating=?,year=?,total_chapters=?,final_volume=?,description=?,cover_url=?,raw_json=?,raw_schema_version=1,refreshed_at=? WHERE id=?`), md.Title, sortTitle(md.Title), md.Type, md.Status, md.ContentRating, md.Year, md.TotalChapters, md.FinalVolume, md.Description, md.CoverURL, string(md.RawJSON), utcNow(), seriesID)
	if err != nil {
		return err
	}
	if err = s.syncCatalog(ctx, tx, seriesID, collections, works); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return s.EnsureModels(ctx, seriesID)
}
