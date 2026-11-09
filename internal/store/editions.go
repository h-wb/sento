package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/h-wb/sento/internal/apperr"
)

// Edition is what is collected: MangaBaka's edition for one series,
// language and type, or a local one the catalog does not list. Publisher
// and Medium are those of its preferred collection, or as entered.
type Edition struct {
	NumberingSource string `json:"numberingSource"`
	SyncState       string `json:"syncState"`
	ID              int64  `json:"id"`
	SeriesID        int64  `json:"seriesId"`
	// ProviderEditionID is MangaBaka's edition id, shared by every series
	// that has that edition ("Standard Edition", "3-in-1 Edition").
	ProviderEditionID string `json:"providerEditionId,omitempty"`
	// ProviderEditionName is MangaBaka's name for it, whatever EditionName
	// was changed to.
	ProviderEditionName string `json:"providerEditionName,omitempty"`
	EditionName         string `json:"editionName"`
	Language            string `json:"language"`
	Publisher           string `json:"publisher"`
	Medium              string `json:"medium"`
	Type                string `json:"type"`
	Monitored           bool   `json:"monitored"`
	CountMain           int    `json:"countMain"`
	CountExtra          int    `json:"countExtra"`
	CountOther          int    `json:"countOther"`
	StartDate           string `json:"startDate,omitempty"`
	EndDate             string `json:"endDate,omitempty"`
	Custom              bool   `json:"custom"`
	// Split: someone separated this edition from the one it was grouped with.
	Split     bool  `json:"split"`
	ProfileID int64 `json:"profileId,omitempty" db:"profile_id"` // 0 = inherit from series
	CutoffNum int64 `json:"cutoffNum" db:"cutoff_num"`           // chapter editions: skip through this number (exclusive cutoff)
	// ReaderFields are the edition's own templates for the ComicInfo
	// fields readers file it by, keyed by field ("series"); a field it
	// does not name follows the media settings.
	ReaderFields ReaderFields `json:"readerFields" db:"reader_fields_json"`
	// Overrides names the fields edited by hand, which refreshes leave alone.
	Overrides FieldList `json:"overrides" db:"overrides_json" tstype:"string[]"`
	// Collections are the catalog collections grouped under this edition,
	// the preferred first; none for a local edition.
	Collections []Collection `json:"collections" db:"-"`
	// LastNumber is the highest numbered main book (hundredths), 0 when
	// none is numbered.
	LastNumber int64 `json:"-" db:"last_number"`
}

// HasMedium reports whether the edition exists in a medium.
func (e Edition) HasMedium(medium string) bool {
	for _, c := range e.Collections {
		if c.Medium == medium {
			return true
		}
	}
	return e.Medium == medium
}

type Book struct {
	ProviderID  string `json:"providerId,omitempty"`
	NumberKnown bool   `json:"numberKnown"`
	Revision    string `json:"revision,omitempty"`
	SyncState   string `json:"syncState"`
	ID          int64  `json:"id"`
	EditionID   int64  `json:"editionId"`
	SequenceNum int64  `json:"sequenceNum"` // hundredths
	SequenceStr string `json:"sequenceStr" db:"sequence_string"`
	CountType   string `json:"countType"`
	ReleaseDate string `json:"releaseDate,omitempty"`
	ISBN        string `json:"isbn,omitempty"`
	Pages       int    `json:"pages,omitempty"`
	// Overrides names the fields edited by hand, which refreshes leave alone.
	Overrides FieldList `json:"overrides" db:"overrides_json" tstype:"string[]"`
	// Copies counts the catalog works behind the book: one per medium it
	// is listed in, none for a book added by hand.
	Copies int `json:"copies"`
}

const editionColumns = `
	id, series_id, COALESCE(provider_edition_id,'') AS provider_edition_id, provider_edition_name,
	edition_name, language, publisher, medium, type, EXISTS(SELECT 1 FROM versions t WHERE t.edition_id=editions.id AND t.monitored=TRUE) AS monitored,
	COALESCE(count_main,0) AS count_main, COALESCE(count_extra,0) AS count_extra,
	COALESCE(count_other,0) AS count_other,
	COALESCE(start_date,'') AS start_date, COALESCE(end_date,'') AS end_date, custom, split, reader_fields_json,
	COALESCE(profile_id,0) AS profile_id, COALESCE((SELECT t.cutoff_num FROM versions t WHERE t.edition_id=editions.id AND t.is_default=TRUE),0) AS cutoff_num,sync_state,numbering_source,overrides_json,
	COALESCE((SELECT MAX(w.sequence_num) FROM books w WHERE w.edition_id=editions.id AND w.count_type='main' AND w.number_known=TRUE),0) AS last_number`

func (s *Store) ListEditions(ctx context.Context, seriesID int64) ([]Edition, error) {
	eds, err := list[Edition](ctx, s, `SELECT `+editionColumns+`
		FROM editions WHERE series_id = ? ORDER BY edition_name, language, medium, id`, seriesID)
	if err != nil {
		return nil, err
	}
	return eds, s.decorate(ctx, seriesID, eds)
}

func (s *Store) GetEdition(ctx context.Context, id int64) (*Edition, error) {
	e, err := get[Edition](ctx, s, `SELECT `+editionColumns+` FROM editions WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	one := []Edition{*e}
	if err = s.decorate(ctx, e.SeriesID, one); err != nil {
		return nil, err
	}
	return &one[0], nil
}

func (s *Store) SetEditionMonitored(ctx context.Context, seriesID, editionID int64, monitored bool) error {
	c, err := s.GetEdition(ctx, editionID)
	if err != nil {
		return err
	}
	if c.SeriesID != seriesID {
		return ErrNotFound
	}
	if _, err := s.db.ExecContext(ctx, s.q(`UPDATE versions SET monitored=? WHERE is_default=TRUE AND edition_id IN(SELECT id FROM editions WHERE id=? AND series_id=?)`), monitored, editionID, seriesID); err != nil {
		return err
	}
	return nil
}

// SetEditionReaderFields replaces the edition's own reader field
// templates; none left means it follows the media settings throughout.
func (s *Store) SetEditionReaderFields(ctx context.Context, seriesID, editionID int64, fields ReaderFields) error {
	c, err := s.GetEdition(ctx, editionID)
	if err != nil {
		return err
	}
	if c.SeriesID != seriesID {
		return ErrNotFound
	}
	if fields == nil {
		fields = ReaderFields{}
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	return s.touch(ctx, `UPDATE editions SET reader_fields_json=? WHERE id=?`, string(raw), editionID)
}

// ReaderFields maps a presented ComicInfo field to its template.
type ReaderFields map[string]string

func (f *ReaderFields) Scan(src any) error {
	*f = ReaderFields{}
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), f)
	case []byte:
		return json.Unmarshal(v, f)
	case nil:
		return nil
	default:
		return fmt.Errorf("cannot scan reader fields from %T", src)
	}
}

// AddCustomEdition creates an independent local edition. Volume numbering may
// be cloned or generated explicitly; chapter editions start with explicit books.
// Catalog refresh never adopts local editions without an explicit link.
func (s *Store) AddCustomEdition(ctx context.Context, c Edition, basedOn int64, volumes int) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var duplicate int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM editions WHERE series_id=? AND custom=TRUE AND edition_name=? AND language=? AND publisher=? AND medium=? AND type=?`), c.SeriesID, c.EditionName, c.Language, c.Publisher, c.Medium, orDefault(c.Type, "volume")).Scan(&duplicate); err != nil {
		return 0, err
	}
	if duplicate > 0 {
		return 0, apperr.Conflictf("a local edition with these details already exists; collect it again with another profile instead")
	}
	ctype := orDefault(c.Type, "volume")
	var colID int64
	err = tx.QueryRowContext(ctx, s.q(`
		INSERT INTO editions (series_id, provider_edition_id, provider_edition_name, edition_name, language, publisher, medium,
			type, custom, updated_at)
		VALUES (?,?,?,?,?,?,?,?,TRUE,?) RETURNING id`),
		c.SeriesID, c.ProviderEditionID, c.ProviderEditionName, c.EditionName, c.Language, c.Publisher, c.Medium, ctype,
		utcNow()).Scan(&colID)
	if err != nil {
		if c := conflict(err, "an edition with this name/language/publisher/medium already exists"); c != err {
			return 0, c
		}
		return 0, fmt.Errorf("inserting local edition: %w", err)
	}

	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO versions(edition_id,name,is_default,monitored,cutoff_num) VALUES(?,'Default',TRUE,?,?)`), colID, c.Monitored, c.CutoffNum); err != nil {
		return 0, err
	}

	if basedOn > 0 {
		var sourceType string
		if err := tx.QueryRowContext(ctx, s.q(`SELECT type FROM editions WHERE id=? AND series_id=?`), basedOn, c.SeriesID).Scan(&sourceType); err != nil {
			return 0, ErrNotFound
		}
		if sourceType != ctype {
			return 0, apperr.Invalidf("clone must have the same book type")
		}
		if _, err := tx.ExecContext(ctx, s.q(`
			INSERT INTO books (edition_id, sequence_num, sequence_string, count_type, number_known, revision)
			SELECT ?, sequence_num, sequence_string, count_type, number_known, revision
			FROM books WHERE edition_id = ?
			  AND edition_id IN (SELECT id FROM editions WHERE series_id = ?)`),
			colID, basedOn, c.SeriesID); err != nil {
			return 0, fmt.Errorf("cloning works: %w", err)
		}
	} else if ctype == "volume" {
		for i := 1; i <= volumes; i++ {
			if _, err := tx.ExecContext(ctx, s.q(`
				INSERT INTO books (edition_id, sequence_num, sequence_string, count_type)
				VALUES (?,?,?,'main')`), colID, int64(i)*100, fmt.Sprintf("%d", i)); err != nil {
				return 0, fmt.Errorf("generating work %d: %w", i, err)
			}
		}
	}
	return colID, tx.Commit()
}

// MergeEditions folds one edition into another of the same series: a
// local duplicate into its twin, or a catalog edition into the one it was
// kept apart from. Every book needs an unambiguous counterpart or none.
// Collected versions and their policies survive independently, and files
// stay in place.
func (s *Store) MergeEditions(ctx context.Context, seriesID, fromID, intoID int64) ([]int64, error) {
	p, err := s.PreviewMerge(ctx, seriesID, fromID, intoID)
	if err != nil {
		return nil, err
	}
	if p.Pending {
		return nil, apperr.Conflictf("edition has pending downloads")
	}
	if len(p.Ambiguous) > 0 {
		return nil, apperr.Conflictf("every book needs an unambiguous match before merging editions")
	}
	if err := s.EnsureModels(ctx, seriesID); err != nil {
		return nil, err
	}
	versions, err := s.ListVersions(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = s.lockEditions(ctx, tx, false, fromID, intoID); err != nil {
		return nil, err
	}

	// Preserve acquisition policies as independent versions rather than
	// combining different policies into one default.
	for _, t := range versions {
		if t.EditionID != fromID {
			continue
		}
		name := t.Name + " (merged " + fmt.Sprint(t.ID) + ")"
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE versions SET edition_id=?,name=?,is_default=FALSE WHERE id=?`), intoID, name, t.ID); err != nil {
			return nil, err
		}
	}
	if err = s.foldEdition(ctx, tx, fromID, intoID, p.Mapping); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE editions SET split=FALSE WHERE id=?`), intoID); err != nil {
		return nil, err
	}
	return []int64{}, tx.Commit()
}

// DeleteCustomEdition removes a custom collection and its book links
// (files themselves stay in the library).
func (s *Store) DeleteCustomEdition(ctx context.Context, seriesID, editionID int64) error {
	col, err := s.GetEdition(ctx, editionID)
	if err != nil {
		return err
	}
	if col.SeriesID != seriesID {
		return ErrNotFound
	}
	if !col.Custom {
		return apperr.Invalidf("only local editions can be deleted")
	}
	pending, e := s.EditionPending(ctx, editionID)
	if e != nil {
		return e
	}
	if pending {
		return apperr.Conflictf("edition has pending downloads")
	}
	n, err := get[int](ctx, s, `SELECT COUNT(*) FROM asset_contents a JOIN books w ON w.id=a.book_id WHERE w.edition_id=?`, editionID)
	if err != nil {
		return err
	}
	if *n > 0 {
		return apperr.Conflictf("unlink owned files before deleting this edition")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = s.lockEditions(ctx, tx, false, editionID); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, s.q(`UPDATE source_bindings SET edition_id=0 WHERE edition_id=?`), editionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.q(`DELETE FROM editions WHERE id = ?`), editionID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListBooks(ctx context.Context, editionID int64) ([]Book, error) {
	return list[Book](ctx, s, `SELECT `+bookColumns+` FROM books WHERE edition_id=? ORDER BY sequence_num, id`, editionID)
}

// SetEditionCutoff changes the default version’s exclusive chapter cutoff.
func (s *Store) SetEditionCutoff(ctx context.Context, seriesID, editionID, cutoff int64) error {
	c, err := s.GetEdition(ctx, editionID)
	if err != nil {
		return err
	}
	if c.SeriesID != seriesID {
		return ErrNotFound
	}
	if err := s.touch(ctx, `UPDATE versions SET cutoff_num=? WHERE edition_id=? AND is_default=TRUE`, cutoff, editionID); err != nil && err != ErrNotFound {
		return err
	}
	return nil
}
