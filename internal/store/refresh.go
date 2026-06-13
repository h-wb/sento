package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/h-wb/sento/internal/metadata"
)

// ReconcileMetadata applies a fresh provider snapshot to an existing series.
// Collection identity is the descriptor tuple (D13): provider UUIDs are
// silently re-bound; vanished descriptors are kept (never destructive);
// new works are inserted, existing ones updated in place.
func (s *Store) ReconcileMetadata(
	ctx context.Context,
	seriesID int64,
	md *metadata.Series,
	collections []metadata.Collection,
	works map[string][]metadata.Work,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, s.q(`
		UPDATE series SET title = ?, sort_title = ?, type = ?, status = ?,
			content_rating = ?, year = ?, total_chapters = ?, final_volume = ?,
			description = ?, cover_url = ?, raw_json = ?, raw_schema_version = ?,
			refreshed_at = ?
		WHERE id = ?`),
		md.Title, sortTitle(md.Title), md.Type, md.Status, md.ContentRating, md.Year,
		md.TotalChapters, md.FinalVolume, md.Description, md.CoverURL,
		string(md.RawJSON), 1, utcNow(), seriesID); err != nil {
		return err
	}

	for _, col := range collections {
		var colID int64
		err := tx.QueryRowContext(ctx, s.q(`
			SELECT id FROM collections
			WHERE series_id = ? AND edition_name = ? AND language = ?
			  AND publisher = ? AND medium = ? AND ctype = ?`),
			seriesID, col.EditionName, col.Language, col.Publisher, col.Medium, col.Type,
		).Scan(&colID)

		switch {
		case errors.Is(err, sql.ErrNoRows):
			if err := tx.QueryRowContext(ctx, s.q(`
				INSERT INTO collections (series_id, mb_collection_id, edition_name,
					language, publisher, medium, ctype, monitored, count_main,
					count_extra, count_other, start_date, end_date, raw_json, updated_at)
				VALUES (?,?,?,?,?,?,?,FALSE,?,?,?,?,?,?,?) RETURNING id`),
				seriesID, col.ProviderID, col.EditionName, col.Language, col.Publisher,
				col.Medium, col.Type, col.CountMain, col.CountExtra, col.CountOther,
				col.StartDate, col.EndDate, string(col.RawJSON), utcNow()).Scan(&colID); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			// Existing descriptor: re-bind provider id and refresh counts.
			// A custom edition with the same identity is adopted here —
			// MangaBaka caught up with what the user hand-created.
			if _, err := tx.ExecContext(ctx, s.q(`
				UPDATE collections SET mb_collection_id = ?, count_main = ?,
					count_extra = ?, count_other = ?, start_date = ?, end_date = ?,
					raw_json = ?, custom = FALSE, updated_at = ?
				WHERE id = ?`),
				col.ProviderID, col.CountMain, col.CountExtra, col.CountOther,
				col.StartDate, col.EndDate, string(col.RawJSON), utcNow(), colID); err != nil {
				return err
			}
		}

		for _, w := range works[col.ProviderID] {
			if _, err := tx.ExecContext(ctx, s.q(`
				INSERT INTO works (collection_id, mb_work_id, sequence_num,
					sequence_string, count_type, release_date, isbn, pages, raw_json)
				VALUES (?,?,?,?,?,?,?,?,?)
				ON CONFLICT(collection_id, sequence_num, count_type) DO UPDATE SET
					mb_work_id = excluded.mb_work_id,
					release_date = excluded.release_date,
					isbn = excluded.isbn,
					pages = excluded.pages,
					raw_json = excluded.raw_json`),
				colID, w.ProviderID, int64(w.SequenceNum), w.SequenceStr, w.CountType,
				w.ReleaseDate, w.ISBN, w.Pages, string(w.RawJSON)); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}
