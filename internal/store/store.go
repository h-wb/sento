// Package store is the SQLite persistence layer: hand-written SQL, scany
// for row mapping (columns match snake-cased field names or `db` tags).
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/metadata"
)

// ErrNotFound is the apperr sentinel: services and handlers match it with
// errors.Is and the HTTP layer maps it to 404.
var ErrNotFound = apperr.ErrNotFound

// conflict maps a uniqueness violation to apperr.ErrConflict with a
// caller-supplied message; other errors pass through.
func conflict(err error, msg string) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
		return apperr.Conflictf("%s", msg)
	}
	if strings.Contains(err.Error(), "UNIQUE constraint") { // modernc/sqlite
		return apperr.Conflictf("%s", msg)
	}
	return err
}

// Store speaks the portable subset of SQL shared by SQLite and PostgreSQL;
// queries are written with ? placeholders and rebound per dialect (q).
type Store struct {
	db *sql.DB
	pg bool
}

// New wraps a connection opened by internal/db; dialect is one of its
// Dialect constants.
func New(db *sql.DB, dialect string) *Store {
	return &Store{db: db, pg: dialect == "postgres"}
}

// q rebinds ? placeholders to PostgreSQL's $N form. Queries never contain
// a literal '?' (no SQL string literal here uses one), so the rewrite is a
// plain scan.
func (s *Store) q(query string) string {
	if !s.pg || !strings.ContainsRune(query, '?') {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// utcNow renders the canonical stored timestamp: UTC, second precision,
// the same format both dialects' column defaults produce.
func utcNow() string { return time.Now().UTC().Format("2006-01-02 15:04:05") }

// insert runs an INSERT ... RETURNING id and scans the new row id (the
// portable replacement for LastInsertId, which pgx does not implement).
func (s *Store) insert(ctx context.Context, query string, args ...any) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, s.q(query), args...).Scan(&id)
	return id, err
}

// --- rows ---

type Series struct {
	ID            int64  `json:"id"`
	MangaBakaID   int64  `json:"mangabakaId" db:"mangabaka_id"`
	Title         string `json:"title"`
	SortTitle     string `json:"sortTitle"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	ContentRating string `json:"contentRating,omitempty"`
	Year          int    `json:"year,omitempty"`
	TotalChapters int    `json:"totalChapters"`
	FinalVolume   int    `json:"finalVolume"`
	Description   string `json:"description,omitempty"`
	CoverURL      string `json:"coverUrl,omitempty"`
	Monitored     bool   `json:"monitored"`
	Path          string `json:"path,omitempty"`
	// ChapterPath is where the series' chapters live when they are kept
	// apart from its volumes; empty = under Path.
	ChapterPath string `json:"chapterPath,omitempty"`
	ProfileID   int64  `json:"profileId,omitempty" db:"profile_id"` // 0 = instance default
	AddedAt     string `json:"addedAt"`
	RefreshedAt string `json:"refreshedAt,omitempty"`
}

// Edition is what is collected: MangaBaka's edition for one series,
// language and type, or a local one the catalog does not list. Publisher
// and Medium are those of its preferred collection, or as entered.
type Edition struct {
	NumberingSource  string `json:"numberingSource"`
	DiscoverChapters bool   `json:"discoverChapters"`
	SyncState        string `json:"syncState"`
	ID               int64  `json:"id"`
	SeriesID         int64  `json:"seriesId"`
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
	// Overrides names the fields edited by hand, which refreshes leave alone.
	Overrides FieldList `json:"overrides" db:"overrides_json" tstype:"string[]"`
	// Collections are the catalog collections grouped under this edition,
	// the preferred first; none for a local edition.
	Collections []Collection `json:"collections" db:"-"`
	// OtherLabels names the series' other catalog editions of this type.
	OtherLabels []string `json:"-" db:"-"`
	// LastNumber is the highest numbered main book (hundredths), 0 when
	// none is numbered: a release past it is not this numbering.
	LastNumber int64 `json:"-" db:"-"`
	// OtherLast is the highest LastNumber among the series' other editions
	// of this type and language: numbers past LastNumber only prove a
	// release is another numbering when another edition reaches them.
	OtherLast int64 `json:"-" db:"-"`
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

type File struct {
	ContentBooks  []BookRef  `json:"-" db:"-"` // intrinsic contents, without version selections
	Claims        AssetFacts `json:"claims" db:"claims_json"`
	Presence      string     `json:"presence"`
	PresenceError string     `json:"presenceError"`
	CheckedAt     string     `json:"checkedAt"`
	Facts         AssetFacts `json:"facts" db:"facts_json"`
	NeedsReview   bool       `json:"needsReview"`
	Hash          string     `json:"hash,omitempty"`
	ID            int64      `json:"id"`
	SeriesID      int64      `json:"seriesId"`
	Path          string     `json:"path"`
	Size          int64      `json:"size"`
	Provenance    string     `json:"provenance"`
	Unrequested   bool       `json:"unrequested"`
	AddedAt       string     `json:"addedAt"`
}

type BookLink struct {
	VersionID int64
	BookID    int64
	FileID    int64
	Ledger    string
	EditionID int64
	Number    int64
}

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

// --- series ---

const seriesColumns = `
	id, mangabaka_id, title, sort_title, type, status,
	COALESCE(content_rating,'') AS content_rating, COALESCE(year,0) AS year,
	COALESCE(total_chapters,0) AS total_chapters,
	COALESCE(final_volume,0) AS final_volume,
	COALESCE(description,'') AS description, COALESCE(cover_url,'') AS cover_url,
	monitored, COALESCE(path,'') AS path, COALESCE(chapter_path,'') AS chapter_path,
	COALESCE(profile_id,0) AS profile_id, added_at,
	COALESCE(refreshed_at,'') AS refreshed_at`

func (s *Store) ListSeries(ctx context.Context) ([]Series, error) {
	return list[Series](ctx, s, `SELECT `+seriesColumns+` FROM series ORDER BY sort_title`)
}

func (s *Store) GetSeries(ctx context.Context, id int64) (*Series, error) {
	return get[Series](ctx, s, `SELECT `+seriesColumns+` FROM series WHERE id = ?`, id)
}

func (s *Store) DeleteSeries(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, s.q(`DELETE FROM series WHERE id=?`), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// SetSeriesMonitored gates all automatic searches for the series.
func (s *Store) SetSeriesMonitored(ctx context.Context, id int64, monitored bool) error {
	return s.touch(ctx, `UPDATE series SET monitored = ? WHERE id = ?`, monitored, id)
}

// AddSeriesParams carries everything needed to create a library entry from
// fetched metadata in one transaction.
type AddSeriesParams struct {
	MonitorNone   bool
	Series        *metadata.Series
	Collections   []metadata.Collection
	Works         map[string][]metadata.Work // keyed by collection provider id
	MonitorAll    bool                       // monitor every volume-type collection
	MonitorIDs    []string                   // or: provider ids to monitor
	ChapterLang   string
	ChapterCutoff int64 // hundredths; default: total_chapters*100 (latest at add) when volumes are collected, else 0
	ProfileID     int64 // 0 = instance default (stored NULL)
}

func (s *Store) AddSeries(ctx context.Context, p AddSeriesParams) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	md := p.Series
	var profileID any // NULL = instance default
	if p.ProfileID > 0 {
		profileID = p.ProfileID
	}
	var seriesID int64
	err = tx.QueryRowContext(ctx, s.q(`
		INSERT INTO series (mangabaka_id, title, sort_title, type, status, content_rating,
			year, total_chapters, final_volume, description, cover_url, profile_id, raw_json,
			raw_schema_version, refreshed_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) RETURNING id`),
		md.MangaBakaID, md.Title, sortTitle(md.Title), md.Type, md.Status, md.ContentRating,
		md.Year, md.TotalChapters, md.FinalVolume, md.Description, md.CoverURL, profileID,
		string(md.RawJSON), 1, utcNow()).Scan(&seriesID)
	if err != nil {
		if c := conflict(err, "series is already in the library"); c != err {
			return 0, c
		}
		return 0, fmt.Errorf("inserting series: %w", err)
	}

	monitor := map[string]bool{}
	for _, id := range p.MonitorIDs {
		monitor[id] = true
	}

	lang := p.ChapterLang
	if lang == "" {
		lang = "en"
	}

	if err = s.syncCatalog(ctx, tx, seriesID, p.Collections, p.Works); err != nil {
		return 0, fmt.Errorf("filing the catalog: %w", err)
	}
	// An edition is collected when any of its collections was chosen.
	var chapterColID int64 // first catalog chapter edition of the wanted language
	volumes := false       // some volume edition is collected
	for _, col := range p.Collections {
		var edID int64
		if err = tx.QueryRowContext(ctx, s.q(`SELECT edition_id FROM collections WHERE provider_id=? AND edition_id IN(SELECT id FROM editions WHERE series_id=?)`), col.ProviderID, seriesID).Scan(&edID); err != nil {
			return 0, err
		}
		monitored := p.MonitorAll && col.Type == "volume" || monitor[col.ProviderID]
		volumes = volumes || monitored && col.Type == "volume"
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO versions(edition_id,name,is_default,monitored) SELECT ?,'Default',TRUE,FALSE WHERE NOT EXISTS(SELECT 1 FROM versions WHERE edition_id=?)`), edID, edID); err != nil {
			return 0, err
		}
		if monitored {
			if _, err = tx.ExecContext(ctx, s.q(`UPDATE versions SET monitored=TRUE WHERE edition_id=? AND is_default=TRUE`), edID); err != nil {
				return 0, err
			}
		}
		if chapterColID == 0 && col.Type == "chapter" && strings.EqualFold(col.Language, lang) {
			chapterColID = edID
		}
	}

	// The chapter ledger is a chapter-type collection: adopt the MangaBaka
	// one of the wanted language when present, else create the default.
	// Collected volumes hold the chapters out so far, so only new chapters
	// are wanted; without volumes the chapters are all there is (a
	// one-shot, a chapter-only series), so every one is.
	cutoff := p.ChapterCutoff
	if cutoff == 0 && volumes {
		cutoff = int64(md.TotalChapters) * 100
	}
	if chapterColID != 0 {
		if _, err := tx.ExecContext(ctx, s.q(`
			UPDATE versions SET monitored=?,cutoff_num=? WHERE edition_id=? AND is_default=TRUE`),
			!p.MonitorNone, cutoff, chapterColID); err != nil {
			return 0, fmt.Errorf("adopting chapter edition: %w", err)
		}
	} else {
		if err = tx.QueryRowContext(ctx, s.q(`INSERT INTO editions(series_id,edition_name,language,type,custom,updated_at,numbering_source) VALUES(?,'Chapters',?,'chapter',TRUE,?,'series_count') RETURNING id`), seriesID, lang, utcNow()).Scan(&chapterColID); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO versions(edition_id,name,is_default,monitored,cutoff_num) VALUES(?,'Default',TRUE,?,?)`), chapterColID, !p.MonitorNone, cutoff); err != nil {
			return 0, err
		}
	}

	return seriesID, tx.Commit()
}

// --- collections / works ---

const editionColumns = `
	id, series_id, COALESCE(provider_edition_id,'') AS provider_edition_id, provider_edition_name,
	edition_name, language, publisher, medium, type, EXISTS(SELECT 1 FROM versions t WHERE t.edition_id=editions.id AND t.monitored=TRUE) AS monitored,
	COALESCE(count_main,0) AS count_main, COALESCE(count_extra,0) AS count_extra,
	COALESCE(count_other,0) AS count_other,
	COALESCE(start_date,'') AS start_date, COALESCE(end_date,'') AS end_date, custom, split,
	COALESCE(profile_id,0) AS profile_id, COALESCE((SELECT t.cutoff_num FROM versions t WHERE t.edition_id=editions.id AND t.is_default=TRUE),0) AS cutoff_num,sync_state,discover_chapters,numbering_source,overrides_json`

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
		return 0, apperr.Conflictf("a local edition with these details already exists; add a version instead")
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

// --- files & links ---

// AddFile records a library file and its book links. Re-adding a known
// path updates the row in place (paths are unique) and adds any new links,
// so an import retried after a partial failure converges instead of
// tripping over what the first attempt already recorded.
func (s *Store) AddFile(ctx context.Context, f File, parse any, links []BookLink) (int64, error) {
	if err := s.EnsureModels(ctx, f.SeriesID); err != nil {
		return 0, err
	}
	parseJSON, _ := json.Marshal(parse)
	factsJSON, _ := json.Marshal(f.Facts)
	claimsJSON, _ := json.Marshal(f.Claims)
	presence := "present"
	if info, e := os.Stat(f.Path); e != nil || info.IsDir() {
		presence = "unknown"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	cids := []int64{}
	for _, l := range links {
		cids = append(cids, l.EditionID)
	}
	for _, b := range f.ContentBooks {
		cids = append(cids, b.EditionID)
	}
	if err = s.lockEditions(ctx, tx, true, cids...); err != nil {
		return 0, err
	}
	var oldID int64
	var oldHash, oldFacts string
	oldErr := tx.QueryRowContext(ctx, s.q(`SELECT id,COALESCE(hash,''),facts_json FROM files WHERE path=? AND series_id=?`), f.Path, f.SeriesID).Scan(&oldID, &oldHash, &oldFacts)
	if oldErr != nil && !errors.Is(oldErr, sql.ErrNoRows) {
		return 0, oldErr
	}
	if oldErr == nil && f.Hash != "" && oldHash == f.Hash {
		var manual int
		if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM asset_contents WHERE file_id=? AND evidence='manual'`), oldID).Scan(&manual); err != nil {
			return 0, err
		}
		if manual > 0 {
			// A retry or scan must not undo a person's verified contents or corrections.
			_, err = tx.ExecContext(ctx, s.q(`UPDATE files SET presence=?,presence_error='',checked_at=? WHERE id=?`), presence, utcNow(), oldID)
			if err != nil {
				return 0, err
			}
			return oldID, tx.Commit()
		}
		var previous AssetFacts
		if err = json.Unmarshal([]byte(oldFacts), &previous); err != nil {
			return 0, err
		}
		if f.Facts.Sources == nil {
			f.Facts.Sources = map[string]string{}
		}
		if previous.Sources["language"] == "manual" {
			f.Facts.Language = previous.Language
			f.Facts.Sources["language"] = "manual"
		}
		if previous.Sources["group"] == "manual" {
			f.Facts.Group = previous.Group
			f.Facts.Sources["group"] = "manual"
		}
		factsJSON, _ = json.Marshal(f.Facts)
	}

	var fileID int64
	if err := tx.QueryRowContext(ctx, s.q(`
		INSERT INTO files (series_id, path, size, parse_json, provenance, unrequested,needs_review,facts_json,claims_json,hash,presence,checked_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(path) DO UPDATE SET
			series_id = excluded.series_id,
			size = excluded.size,
			parse_json = excluded.parse_json,
			provenance = excluded.provenance,
			unrequested = excluded.unrequested,
			needs_review = excluded.needs_review,
 facts_json=excluded.facts_json,claims_json=excluded.claims_json,hash=excluded.hash,presence=excluded.presence,checked_at=excluded.checked_at
		WHERE files.series_id = excluded.series_id
		RETURNING id`),
		f.SeriesID, f.Path, f.Size, string(parseJSON), f.Provenance, f.Unrequested, f.NeedsReview, string(factsJSON), string(claimsJSON), f.Hash, presence, utcNow()).Scan(&fileID); err != nil {
		return 0, err
	}
	for _, l := range links {
		var valid int
		if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM books w JOIN versions t ON t.edition_id=w.edition_id JOIN editions c ON c.id=w.edition_id WHERE w.id=? AND t.id=? AND c.id=? AND c.series_id=?`), l.BookID, l.VersionID, l.EditionID, f.SeriesID).Scan(&valid); err != nil {
			return 0, err
		}
		if valid != 1 {
			return 0, apperr.Invalidf("file mapping does not belong to this edition/version")
		}
		if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO asset_coverage(file_id,version_id,book_id,evidence) VALUES(?,?,?,?) ON CONFLICT DO NOTHING`), fileID, l.VersionID, l.BookID, orDefault(f.Provenance, "observed")); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO asset_contents(file_id,book_id,evidence) VALUES(?,?,?) ON CONFLICT DO NOTHING`), fileID, l.BookID, orDefault(f.Provenance, "observed")); err != nil {
			return 0, err
		}

	}

	for _, b := range f.ContentBooks {
		var valid int
		if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM books w JOIN editions c ON c.id=w.edition_id WHERE w.id=? AND c.id=? AND c.series_id=?`), b.BookID, b.EditionID, f.SeriesID).Scan(&valid); err != nil {
			return 0, err
		}
		if valid != 1 {
			return 0, apperr.Invalidf("file contents do not belong to this series and edition")
		}
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO asset_contents(file_id,book_id,evidence) VALUES(?,?,?) ON CONFLICT DO NOTHING`), fileID, b.BookID, orDefault(f.Provenance, "observed")); err != nil {
			return 0, err
		}
	}
	return fileID, tx.Commit()
}

func (s *Store) ListFiles(ctx context.Context, seriesID int64) ([]File, error) {
	return list[File](ctx, s, `
		SELECT id, series_id, path, size, provenance, unrequested, added_at,needs_review,COALESCE(hash,'') AS hash,facts_json,claims_json,presence,presence_error,checked_at
		FROM files WHERE series_id = ? ORDER BY path`, seriesID)
}

// ForgetFile drops what is recorded about a file whose bytes were replaced
// on disk: its contents, selections and pins described the old bytes. The
// file itself is untouched; the caller records it afresh.
func (s *Store) ForgetFile(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, s.q(`DELETE FROM files WHERE id = ?`), id)
	return err
}

// SetFileSize records a file's size after its bytes changed without its
// pages changing (retagged metadata).
func (s *Store) SetFileSize(ctx context.Context, id, size int64) error {
	_, err := s.db.ExecContext(ctx, s.q(`UPDATE files SET size = ? WHERE id = ?`), size, id)
	return err
}

// DeleteFile removes a file row (links cascade) and returns its path so
// the caller can remove it from disk.
func (s *Store) DeleteFile(ctx context.Context, id int64) (string, error) {
	pinned, err := get[int](ctx, s, `SELECT COUNT(*) FROM asset_coverage WHERE file_id=? AND pinned=TRUE`, id)
	if err != nil {
		return "", err
	}
	if *pinned > 0 {
		return "", apperr.Conflictf("unpin retained copies before deleting this file")
	}
	path, err := get[string](ctx, s, `SELECT path FROM files WHERE id = ?`, id)
	if err != nil {
		return "", err
	}
	if _, err := s.db.ExecContext(ctx, s.q(`DELETE FROM files WHERE id = ?`), id); err != nil {
		return "", err
	}
	return *path, nil
}

// LowestOwnedChapters returns, per chapter edition of a series, the lowest
// numbered chapter a verified file holds.
func (s *Store) LowestOwnedChapters(ctx context.Context, seriesID int64) (map[int64]int64, error) {
	rows, err := list[struct {
		EditionID int64
		Lowest    int64
	}](ctx, s, `
		SELECT w.edition_id, MIN(w.sequence_num) AS lowest
		FROM asset_contents a JOIN books w ON w.id=a.book_id JOIN editions c ON c.id=w.edition_id JOIN files f ON f.id=a.file_id
		WHERE f.series_id=? AND c.type='chapter' AND a.verified=TRUE AND f.needs_review=FALSE AND w.sequence_num>0
		GROUP BY w.edition_id`, seriesID)
	if err != nil {
		return nil, err
	}
	out := map[int64]int64{}
	for _, r := range rows {
		out[r.EditionID] = r.Lowest
	}
	return out, nil
}

// BookKey identifies one book of one collected version: the book of
// wanting, reserving and owning.
func BookKey(versionID, bookID int64) string {
	return fmt.Sprintf("%d/%d", versionID, bookID)
}

// --- bindings ---

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
			trust_chapters=excluded.trust_chapters,
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

// SetBindingState toggles one binding ("active" / "disabled").
func (s *Store) SetBindingState(ctx context.Context, seriesID, bindingID int64, state string) error {
	return s.touch(ctx, `UPDATE source_bindings SET state = ? WHERE id = ? AND series_id = ?`,
		state, bindingID, seriesID)
}

func (s *Store) DeleteBinding(ctx context.Context, seriesID, bindingID int64) error {
	return s.touch(ctx, `DELETE FROM source_bindings WHERE id = ? AND series_id = ?`, bindingID, seriesID)
}

// --- helpers ---

func sortTitle(title string) string {
	t := strings.ToLower(strings.TrimSpace(title))
	for _, art := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(t, art) {
			return strings.TrimSpace(t[len(art):])
		}
	}
	return t
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
