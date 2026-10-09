package store

import "context"

// CalendarEntry is one dated volume release of a monitored collection.
type CalendarEntry struct {
	BookID      int64  `json:"bookId"`
	VersionID   int64  `json:"versionId"`
	VersionName string `json:"versionName"`
	SeriesID    int64  `json:"seriesId"`
	SeriesTitle string `json:"seriesTitle"`
	EditionID   int64  `json:"editionId"`
	EditionName string `json:"editionName"`
	SequenceNum int64  `json:"sequenceNum"` // hundredths
	SequenceStr string `json:"sequenceStr" db:"sequence_string"`
	ReleaseDate string `json:"releaseDate"`
	Owned       bool   `json:"owned"`
}

// Calendar lists dated volume releases of monitored collections between
// from and to (inclusive, YYYY-MM-DD).
func (s *Store) Calendar(ctx context.Context, from, to string) ([]CalendarEntry, error) {
	return list[CalendarEntry](ctx, s, `
		SELECT w.id AS book_id,t.id AS version_id,t.name AS version_name,s.id AS series_id, s.title AS series_title,
		       c.id AS edition_id, c.edition_name,
		       w.sequence_num, COALESCE(w.sequence_string,'') AS sequence_string,
		       w.release_date,
		       FALSE AS owned
		FROM books w
		JOIN editions c ON c.id = w.edition_id
		JOIN versions t ON t.edition_id=c.id
		JOIN series s ON s.id = c.series_id
		WHERE t.monitored = TRUE AND c.type = 'volume' AND w.sync_state NOT IN('stale','retired') AND c.sync_state<>'stale'
          AND NOT EXISTS(SELECT 1 FROM book_overrides u WHERE u.version_id=t.id AND u.book_id=w.id AND u.monitored=FALSE)
		  AND w.release_date >= ? AND w.release_date <= ?
		ORDER BY w.release_date, s.sort_title, w.sequence_num`, from, to)
}
