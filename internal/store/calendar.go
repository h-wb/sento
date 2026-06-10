package store

import "context"

// CalendarEntry is one dated volume release of a monitored collection.
type CalendarEntry struct {
	SeriesID     int64  `json:"seriesId"`
	SeriesTitle  string `json:"seriesTitle"`
	CollectionID int64  `json:"collectionId"`
	EditionName  string `json:"editionName"`
	SequenceNum  int64  `json:"sequenceNum"` // hundredths
	SequenceStr  string `json:"sequenceStr" db:"sequence_string"`
	ReleaseDate  string `json:"releaseDate"`
	Owned        bool   `json:"owned"`
}

// Calendar lists dated volume releases of monitored collections between
// from and to (inclusive, YYYY-MM-DD).
func (s *Store) Calendar(ctx context.Context, from, to string) ([]CalendarEntry, error) {
	return list[CalendarEntry](ctx, s, `
		SELECT s.id AS series_id, s.title AS series_title,
		       c.id AS collection_id, c.edition_name,
		       w.sequence_num, COALESCE(w.sequence_string,'') AS sequence_string,
		       w.release_date,
		       EXISTS(
		           SELECT 1 FROM file_slot_links l
		           WHERE l.ledger = 'volume' AND l.collection_id = c.id
		             AND l.number = w.sequence_num
		       ) AS owned
		FROM works w
		JOIN collections c ON c.id = w.collection_id
		JOIN series s ON s.id = c.series_id
		WHERE c.monitored = 1 AND c.ctype = 'volume'
		  AND w.release_date >= ? AND w.release_date <= ?
		ORDER BY w.release_date, s.sort_title, w.sequence_num`, from, to)
}
