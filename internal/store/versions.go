package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/h-wb/sento/internal/apperr"
)

type CollectedVersion struct {
	IncludeExtras bool   `json:"includeExtras"`
	ID            int64  `json:"id"`
	EditionID     int64  `json:"editionId"`
	Name          string `json:"name"`
	IsDefault     bool   `json:"isDefault"`
	Monitored     bool   `json:"monitored"`
	ProfileID     int64  `json:"profileId"`
	// ProfileName names the card's own profile; empty when it inherits.
	ProfileName string `json:"profileName"`
	CutoffNum   int64  `json:"cutoffNum"`
	// RequiredGroup and AllowUnknown are the old copy restrictions, read
	// only by MigrateCopyRules, which turns them into profiles. Saving
	// never writes them.
	RequiredGroup  string `json:"-"`
	AllowUnknown   bool   `json:"-"`
	AcquireUnknown bool   `json:"acquireUnknown"`
}

const versionColumns = `id,edition_id,name,is_default,monitored,COALESCE(profile_id,0) AS profile_id,COALESCE((SELECT name FROM profiles WHERE profiles.id=versions.profile_id),'') AS profile_name,cutoff_num,required_group,allow_unknown,acquire_unknown,include_extras`

// insertDefaultVersion adds an edition's default version row.
func (s *Store) insertDefaultVersion(ctx context.Context, tx *sql.Tx, editionID int64, monitored bool, cutoff int64) error {
	_, err := tx.ExecContext(ctx, s.q(`INSERT INTO versions(edition_id,name,is_default,monitored,cutoff_num) VALUES(?,'Default',TRUE,?,?)`), editionID, monitored, cutoff)
	return err
}

// bookInVersion reports whether the book is in the version in the edition of
// the series.
func (s *Store) bookInVersion(ctx context.Context, q querier, seriesID, bookID, versionID, editionID int64) (bool, error) {
	n, err := getQ[int](ctx, s, q, `SELECT COUNT(*) FROM books w JOIN versions t ON t.edition_id=w.edition_id JOIN editions c ON c.id=w.edition_id WHERE w.id=? AND t.id=? AND c.id=? AND c.series_id=?`, bookID, versionID, editionID, seriesID)
	if err != nil {
		return false, err
	}
	return *n == 1, nil
}

// filePins loads the book keys of a file's pinned coverage rows.
func (s *Store) filePins(ctx context.Context, q querier, fileID int64) (map[string]bool, error) {
	rows, err := listQ[struct {
		VersionID int64 `db:"version_id"`
		BookID    int64 `db:"book_id"`
	}](ctx, s, q, `SELECT version_id,book_id FROM asset_coverage WHERE file_id=? AND pinned=TRUE`, fileID)
	if err != nil {
		return nil, err
	}
	pins := map[string]bool{}
	for _, r := range rows {
		pins[BookKey(r.VersionID, r.BookID)] = true
	}
	return pins, nil
}

func (s *Store) ListVersions(ctx context.Context, seriesID int64) ([]CollectedVersion, error) {
	return list[CollectedVersion](ctx, s, `SELECT `+versionColumns+` FROM versions WHERE edition_id IN(SELECT id FROM editions WHERE series_id=?) ORDER BY edition_id,is_default DESC,id`, seriesID)
}

func (s *Store) GetVersion(ctx context.Context, id int64) (*CollectedVersion, error) {
	return get[CollectedVersion](ctx, s, `SELECT `+versionColumns+` FROM versions WHERE id=?`, id)
}

func (s *Store) SaveVersion(ctx context.Context, seriesID int64, t CollectedVersion) (int64, error) {
	c, err := s.editionOf(ctx, seriesID, t.EditionID)
	if err != nil {
		return 0, err
	}
	t.Name = strings.TrimSpace(t.Name)
	if t.CutoffNum < 0 {
		return 0, apperr.Invalidf("chapter cutoff cannot be negative")
	}
	if t.Name == "" {
		return 0, apperr.Invalidf("a name is required")
	}
	var prof any
	if t.ProfileID > 0 {
		if _, err = s.GetProfile(ctx, t.ProfileID); err != nil {
			return 0, err
		}
		prof = t.ProfileID
	}

	if t.ID == 0 {
		var id int64
		e := s.inTx(ctx, func(tx *sql.Tx) error {
			if err := s.lockEditions(ctx, tx, true, c.ID); err != nil {
				return err
			}
			err := tx.QueryRowContext(ctx, s.q(`INSERT INTO versions(edition_id,name,monitored,profile_id,cutoff_num,acquire_unknown,include_extras) VALUES(?,?,?,?,?,?,?) RETURNING id`), t.EditionID, t.Name, t.Monitored, prof, t.CutoffNum, t.AcquireUnknown, t.IncludeExtras).Scan(&id)
			if err != nil {
				return conflict(err, "this edition is already collected under that name")
			}
			return nil
		})
		if e != nil {
			return 0, e
		}
		return id, nil
	}

	old, err := s.GetVersion(ctx, t.ID)
	if err != nil {
		return 0, err
	}
	if old.EditionID != c.ID {
		return 0, ErrNotFound
	}
	pending, err := s.EditionPending(ctx, c.ID)
	if err != nil {
		return 0, err
	}
	if pending && (t.Name != old.Name || t.ProfileID != old.ProfileID || t.CutoffNum != old.CutoffNum || t.AcquireUnknown != old.AcquireUnknown || t.IncludeExtras != old.IncludeExtras) {
		return 0, apperr.Conflictf("finish pending downloads before changing how this edition is collected")
	}
	monitorOnly := t.Name == old.Name && t.ProfileID == old.ProfileID && t.CutoffNum == old.CutoffNum && t.AcquireUnknown == old.AcquireUnknown && t.IncludeExtras == old.IncludeExtras
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		if err := s.lockEditions(ctx, tx, monitorOnly, c.ID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, s.q(`UPDATE versions SET name=?,monitored=?,profile_id=?,cutoff_num=?,acquire_unknown=?,include_extras=? WHERE id=?`), t.Name, t.Monitored, prof, t.CutoffNum, t.AcquireUnknown, t.IncludeExtras, t.ID)
		return err
	})
	return t.ID, err
}

// ProfileTaken reports whether another way of collecting the edition
// (not versionID) already uses the release profile (0: the series').
func (s *Store) ProfileTaken(ctx context.Context, editionID, versionID, profileID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM versions WHERE edition_id=? AND id<>? AND COALESCE(profile_id,0)=?`), editionID, versionID, profileID).Scan(&n)
	return n > 0, err
}

func (s *Store) DeleteVersion(ctx context.Context, seriesID, id int64) error {
	t, err := s.GetVersion(ctx, id)
	if err != nil {
		return err
	}
	c, err := s.editionOf(ctx, seriesID, t.EditionID)
	if err != nil {
		return err
	}
	// Any way an edition is collected can be removed but the last: that
	// one is switched off instead, and the edition stays in the library.
	var count int
	if err = s.db.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM versions WHERE edition_id=?`), c.ID).Scan(&count); err != nil {
		return err
	}
	if count <= 1 {
		return apperr.Invalidf("an edition is always collected one way; stop collecting it instead")
	}
	pending, err := s.EditionPending(ctx, c.ID)
	if err != nil {
		return err
	}
	if pending {
		return apperr.Conflictf("edition has pending downloads")
	}
	// Version deletion drops references, never assets or files.
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if err := s.lockEditions(ctx, tx, false, c.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.q(`DELETE FROM versions WHERE id=?`), id); err != nil {
			return err
		}
		// The edition's remaining oldest way of collecting it becomes its
		// default (it carries the edition's catalog-link and scan defaults).
		if t.IsDefault {
			if _, err := tx.ExecContext(ctx, s.q(`UPDATE versions SET is_default=TRUE WHERE id=(SELECT MIN(id) FROM versions WHERE edition_id=?)`), c.ID); err != nil {
				return err
			}
		}
		return nil
	})
}
