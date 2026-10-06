package store

import "context"

// CatalogPayloads returns the raw MangaBaka documents behind a book: its
// series and, when the book is linked to the catalog, the book itself.
// They carry what the extracted columns do not (credits, genres, tags,
// per-book descriptions). Either may be empty.
func (s *Store) CatalogPayloads(ctx context.Context, seriesID, workID int64) (series, work string, err error) {
	row, err := get[struct {
		RawJSON string `db:"raw_json"`
	}](ctx, s, `SELECT COALESCE(raw_json,'') AS raw_json FROM series WHERE id = ?`, seriesID)
	if err != nil {
		return "", "", err
	}
	series = row.RawJSON
	if workID == 0 {
		return series, "", nil
	}
	row, err = get[struct {
		RawJSON string `db:"raw_json"`
	}](ctx, s, `SELECT COALESCE(raw_json,'') AS raw_json FROM works WHERE id = ?`, workID)
	if err != nil {
		return "", "", err
	}
	return series, row.RawJSON, nil
}

// SetFileLocation records where a library file now lives after it was
// renamed or rewritten, with its new size and content fingerprint.
func (s *Store) SetFileLocation(ctx context.Context, fileID int64, path string, size int64, hash string) error {
	return s.touch(ctx, `UPDATE files SET path=?, size=?, hash=?, modified_at=? WHERE id=?`, path, size, hash, utcNow(), fileID)
}
