package store

import "context"

// CalendarEntry is one dated volume release of a monitored collection.
type CalendarEntry struct {
	WorkID       int64  `json:"workId"`
	TargetID     int64  `json:"targetId"`
	VersionName  string `json:"versionName"`
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
		SELECT w.id AS work_id,t.id AS target_id,t.name AS version_name,s.id AS series_id, s.title AS series_title,
		       c.id AS collection_id, c.edition_name,
		       w.sequence_num, COALESCE(w.sequence_string,'') AS sequence_string,
		       w.release_date,
		       EXISTS(SELECT 1 FROM asset_coverage a JOIN files f ON f.id=a.file_id
                   WHERE a.work_id=w.id AND a.target_id=t.id AND a.verified=TRUE AND f.needs_review=FALSE) AS owned
		FROM works w
		JOIN collections c ON c.id = w.collection_id
		JOIN download_targets t ON t.collection_id=c.id
		JOIN series s ON s.id = c.series_id
		WHERE t.monitored = TRUE AND c.ctype = 'volume' AND w.sync_state NOT IN('stale','retired') AND c.sync_state NOT IN('stale','superseded')
          AND NOT EXISTS(SELECT 1 FROM unit_overrides u WHERE u.target_id=t.id AND u.work_id=w.id AND u.monitored=FALSE)
		  AND w.release_date >= ? AND w.release_date <= ?
		ORDER BY w.release_date, s.sort_title, w.sequence_num`, from, to)
}
