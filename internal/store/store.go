// Package store is the SQLite persistence layer: hand-written SQL, scany
// for row mapping (columns match snake-cased field names or `db` tags).
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/h-wb/sento/internal/metadata"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store { return &Store{db: db} }

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
	AddedAt       string `json:"addedAt"`
	RefreshedAt   string `json:"refreshedAt,omitempty"`
}

type Collection struct {
	ID          int64  `json:"id"`
	SeriesID    int64  `json:"seriesId"`
	ProviderID  string `json:"-" db:"mb_collection_id"`
	EditionName string `json:"editionName"`
	Language    string `json:"language"`
	Publisher   string `json:"publisher"`
	Medium      string `json:"medium"`
	Type        string `json:"type" db:"ctype"`
	Monitored   bool   `json:"monitored"`
	CountMain   int    `json:"countMain"`
	CountExtra  int    `json:"countExtra"`
	CountOther  int    `json:"countOther"`
	StartDate   string `json:"startDate,omitempty"`
	EndDate     string `json:"endDate,omitempty"`
	Custom      bool   `json:"custom"`
}

type Work struct {
	ID           int64  `json:"id"`
	CollectionID int64  `json:"collectionId"`
	SequenceNum  int64  `json:"sequenceNum"` // hundredths
	SequenceStr  string `json:"sequenceStr" db:"sequence_string"`
	CountType    string `json:"countType"`
	ReleaseDate  string `json:"releaseDate,omitempty"`
	ISBN         string `json:"isbn,omitempty"`
	Pages        int    `json:"pages,omitempty"`
}

type ChapterLedger struct {
	SeriesID  int64  `json:"seriesId"`
	Monitored bool   `json:"monitored"`
	Language  string `json:"language"`
	CutoffNum int64  `json:"cutoffNum"`
}

type File struct {
	ID          int64  `json:"id"`
	SeriesID    int64  `json:"seriesId"`
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	Provenance  string `json:"provenance"`
	Unrequested bool   `json:"unrequested"`
	AddedAt     string `json:"addedAt"`
}

type SlotLink struct {
	FileID       int64
	Ledger       string
	CollectionID int64 // 0 = chapter ledger
	Number       int64
}

type Binding struct {
	ID            int64  `json:"id"`
	SeriesID      int64  `json:"seriesId"`
	SourceID      string `json:"sourceId"`
	SourceMangaID int64  `json:"sourceMangaId"`
	SourceName    string `json:"sourceName"`
	MangaTitle    string `json:"mangaTitle"`
	State         string `json:"state"`
	Priority      int    `json:"priority"`
}

// --- series ---

const seriesColumns = `
	id, mangabaka_id, title, sort_title, type, status,
	COALESCE(content_rating,'') AS content_rating, COALESCE(year,0) AS year,
	COALESCE(total_chapters,0) AS total_chapters,
	COALESCE(final_volume,0) AS final_volume,
	COALESCE(description,'') AS description, COALESCE(cover_url,'') AS cover_url,
	monitored, COALESCE(path,'') AS path, added_at,
	COALESCE(refreshed_at,'') AS refreshed_at`

func (s *Store) ListSeries(ctx context.Context) ([]Series, error) {
	return list[Series](ctx, s, `SELECT `+seriesColumns+` FROM series ORDER BY sort_title`)
}

func (s *Store) GetSeries(ctx context.Context, id int64) (*Series, error) {
	return get[Series](ctx, s, `SELECT `+seriesColumns+` FROM series WHERE id = ?`, id)
}

func (s *Store) DeleteSeries(ctx context.Context, id int64) error {
	return s.touch(ctx, `DELETE FROM series WHERE id = ?`, id)
}

// SetSeriesMonitored gates all automatic searches for the series.
func (s *Store) SetSeriesMonitored(ctx context.Context, id int64, monitored bool) error {
	return s.touch(ctx, `UPDATE series SET monitored = ? WHERE id = ?`, monitored, id)
}

// AddSeriesParams carries everything needed to create a library entry from
// fetched metadata in one transaction.
type AddSeriesParams struct {
	Series        *metadata.Series
	Collections   []metadata.Collection
	Works         map[string][]metadata.Work // keyed by collection provider id
	MonitorAll    bool                       // monitor every volume-type collection
	MonitorIDs    []string                   // or: provider ids to monitor
	ChapterLang   string
	ChapterCutoff int64 // hundredths; default: total_chapters*100 (latest at add)
}

func (s *Store) AddSeries(ctx context.Context, p AddSeriesParams) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	md := p.Series
	res, err := tx.ExecContext(ctx, `
		INSERT INTO series (mangabaka_id, title, sort_title, type, status, content_rating,
			year, total_chapters, final_volume, description, cover_url, raw_json,
			raw_schema_version, refreshed_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,datetime('now'))`,
		md.MangaBakaID, md.Title, sortTitle(md.Title), md.Type, md.Status, md.ContentRating,
		md.Year, md.TotalChapters, md.FinalVolume, md.Description, md.CoverURL,
		string(md.RawJSON), 1)
	if err != nil {
		return 0, fmt.Errorf("inserting series: %w", err)
	}
	seriesID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	monitor := map[string]bool{}
	for _, id := range p.MonitorIDs {
		monitor[id] = true
	}

	for _, col := range p.Collections {
		monitored := p.MonitorAll && col.Type == "volume" || monitor[col.ProviderID]
		colRes, err := tx.ExecContext(ctx, `
			INSERT INTO collections (series_id, mb_collection_id, edition_name, language,
				publisher, medium, ctype, monitored, count_main, count_extra, count_other,
				start_date, end_date, raw_json, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,datetime('now'))`,
			seriesID, col.ProviderID, col.EditionName, col.Language, col.Publisher,
			col.Medium, col.Type, monitored, col.CountMain, col.CountExtra, col.CountOther,
			col.StartDate, col.EndDate, string(col.RawJSON))
		if err != nil {
			return 0, fmt.Errorf("inserting collection %q: %w", col.EditionName, err)
		}
		colID, err := colRes.LastInsertId()
		if err != nil {
			return 0, err
		}
		for _, w := range p.Works[col.ProviderID] {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO works (collection_id, mb_work_id, sequence_num, sequence_string,
					count_type, release_date, isbn, pages, raw_json)
				VALUES (?,?,?,?,?,?,?,?,?)`,
				colID, w.ProviderID, int64(w.SequenceNum), w.SequenceStr, w.CountType,
				w.ReleaseDate, w.ISBN, w.Pages, string(w.RawJSON)); err != nil {
				return 0, fmt.Errorf("inserting work: %w", err)
			}
		}
	}

	cutoff := p.ChapterCutoff
	if cutoff == 0 {
		cutoff = int64(md.TotalChapters) * 100
	}
	lang := p.ChapterLang
	if lang == "" {
		lang = "en"
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO chapter_ledgers (series_id, monitored, language, cutoff_num)
		VALUES (?,?,?,?)`, seriesID, true, lang, cutoff); err != nil {
		return 0, fmt.Errorf("inserting chapter ledger: %w", err)
	}

	return seriesID, tx.Commit()
}

// --- collections / works ---

const collectionColumns = `
	id, series_id, COALESCE(mb_collection_id,'') AS mb_collection_id,
	edition_name, language, publisher, medium, ctype, monitored,
	COALESCE(count_main,0) AS count_main, COALESCE(count_extra,0) AS count_extra,
	COALESCE(count_other,0) AS count_other,
	COALESCE(start_date,'') AS start_date, COALESCE(end_date,'') AS end_date, custom`

func (s *Store) ListCollections(ctx context.Context, seriesID int64) ([]Collection, error) {
	return list[Collection](ctx, s, `SELECT `+collectionColumns+`
		FROM collections WHERE series_id = ? ORDER BY edition_name, language, medium`, seriesID)
}

func (s *Store) GetCollection(ctx context.Context, id int64) (*Collection, error) {
	return get[Collection](ctx, s, `SELECT `+collectionColumns+` FROM collections WHERE id = ?`, id)
}

func (s *Store) SetCollectionMonitored(ctx context.Context, seriesID, collectionID int64, monitored bool) error {
	return s.touch(ctx, `UPDATE collections SET monitored = ? WHERE id = ? AND series_id = ?`,
		monitored, collectionID, seriesID)
}

// AddCustomCollection creates a user-defined edition. Its work list is
// cloned from basedOn (numbering only — release dates of a variant are
// unknown, the calendar must not lie) or generated 1..volumes. Metadata
// refresh adopts the row if MangaBaka later lists the same identity.
func (s *Store) AddCustomCollection(ctx context.Context, c Collection, basedOn int64, volumes int) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO collections (series_id, edition_name, language, publisher, medium,
			ctype, monitored, custom, updated_at)
		VALUES (?,?,?,?,?,'volume',?,1,datetime('now'))`,
		c.SeriesID, c.EditionName, c.Language, c.Publisher, c.Medium, c.Monitored)
	if err != nil {
		return 0, fmt.Errorf("inserting custom collection: %w", err)
	}
	colID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if basedOn > 0 {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO works (collection_id, sequence_num, sequence_string, count_type)
			SELECT ?, sequence_num, sequence_string, count_type
			FROM works WHERE collection_id = ?
			  AND collection_id IN (SELECT id FROM collections WHERE series_id = ?)`,
			colID, basedOn, c.SeriesID); err != nil {
			return 0, fmt.Errorf("cloning works: %w", err)
		}
	} else {
		for i := 1; i <= volumes; i++ {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO works (collection_id, sequence_num, sequence_string, count_type)
				VALUES (?,?,?,'main')`, colID, int64(i)*100, fmt.Sprintf("%d", i)); err != nil {
				return 0, fmt.Errorf("generating work %d: %w", i, err)
			}
		}
	}
	return colID, tx.Commit()
}

// MergeCollection folds a custom edition into another collection of the
// same series: file/override links re-point by volume number, monitoring
// is OR-ed, the custom row is deleted (works cascade). Returns the slot
// numbers that have no work in the target (links move anyway — files stay
// owned — but those slots won't show on the target's grid).
func (s *Store) MergeCollection(ctx context.Context, seriesID, fromID, intoID int64) ([]int64, error) {
	from, err := s.GetCollection(ctx, fromID)
	if err != nil {
		return nil, err
	}
	into, err := s.GetCollection(ctx, intoID)
	if err != nil {
		return nil, err
	}
	if from.SeriesID != seriesID || into.SeriesID != seriesID {
		return nil, fmt.Errorf("collections belong to another series")
	}
	if !from.Custom {
		return nil, fmt.Errorf("only custom editions can be merged away")
	}

	unmatched, err := list[int64](ctx, s, `
		SELECT DISTINCT number FROM file_slot_links
		WHERE ledger = 'volume' AND collection_id = ?
		  AND number NOT IN (SELECT sequence_num FROM works WHERE collection_id = ?)
		ORDER BY number`, fromID, intoID)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	for _, table := range []string{"file_slot_links", "slot_overrides"} {
		// UPDATE OR IGNORE skips rows that would collide with an existing
		// link in the target; the DELETE clears those leftovers.
		if _, err := tx.ExecContext(ctx, `UPDATE OR IGNORE `+table+`
			SET collection_id = ? WHERE ledger = 'volume' AND collection_id = ?`,
			intoID, fromID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+`
			WHERE ledger = 'volume' AND collection_id = ?`, fromID); err != nil {
			return nil, err
		}
	}
	if from.Monitored && !into.Monitored {
		if _, err := tx.ExecContext(ctx, `UPDATE collections SET monitored = 1 WHERE id = ?`, intoID); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM collections WHERE id = ?`, fromID); err != nil {
		return nil, err
	}
	return unmatched, tx.Commit()
}

// DeleteCustomCollection removes a custom edition and its volume links
// (files themselves stay in the library).
func (s *Store) DeleteCustomCollection(ctx context.Context, seriesID, collectionID int64) error {
	col, err := s.GetCollection(ctx, collectionID)
	if err != nil {
		return err
	}
	if col.SeriesID != seriesID {
		return ErrNotFound
	}
	if !col.Custom {
		return fmt.Errorf("only custom editions can be deleted")
	}
	for _, table := range []string{"file_slot_links", "slot_overrides"} {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM `+table+`
			WHERE ledger = 'volume' AND collection_id = ?`, collectionID); err != nil {
			return err
		}
	}
	return s.touch(ctx, `DELETE FROM collections WHERE id = ?`, collectionID)
}

func (s *Store) ListWorks(ctx context.Context, collectionID int64) ([]Work, error) {
	return list[Work](ctx, s, `
		SELECT id, collection_id, sequence_num, COALESCE(sequence_string,'') AS sequence_string,
		       count_type, COALESCE(release_date,'') AS release_date,
		       COALESCE(isbn,'') AS isbn, COALESCE(pages,0) AS pages
		FROM works WHERE collection_id = ? ORDER BY sequence_num`, collectionID)
}

// --- chapter ledger ---

func (s *Store) GetChapterLedger(ctx context.Context, seriesID int64) (*ChapterLedger, error) {
	return get[ChapterLedger](ctx, s, `
		SELECT series_id, monitored, language, cutoff_num
		FROM chapter_ledgers WHERE series_id = ?`, seriesID)
}

func (s *Store) UpdateChapterLedger(ctx context.Context, l ChapterLedger) error {
	return s.touch(ctx, `
		UPDATE chapter_ledgers SET monitored = ?, language = ?, cutoff_num = ?
		WHERE series_id = ?`, l.Monitored, l.Language, l.CutoffNum, l.SeriesID)
}

// --- files & links ---

func (s *Store) AddFile(ctx context.Context, f File, parse any, links []SlotLink) (int64, error) {
	parseJSON, _ := json.Marshal(parse)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO files (series_id, path, size, parse_json, provenance, unrequested)
		VALUES (?,?,?,?,?,?)`,
		f.SeriesID, f.Path, f.Size, string(parseJSON), f.Provenance, f.Unrequested)
	if err != nil {
		return 0, err
	}
	fileID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for _, l := range links {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO file_slot_links (file_id, ledger, collection_id, number)
			VALUES (?,?,?,?)`, fileID, l.Ledger, l.CollectionID, l.Number); err != nil {
			return 0, err
		}
	}
	return fileID, tx.Commit()
}

func (s *Store) ListFiles(ctx context.Context, seriesID int64) ([]File, error) {
	return list[File](ctx, s, `
		SELECT id, series_id, path, size, provenance, unrequested, added_at
		FROM files WHERE series_id = ? ORDER BY path`, seriesID)
}

// DeleteFile removes a file row (links cascade) and returns its path so
// the caller can remove it from disk.
func (s *Store) DeleteFile(ctx context.Context, id int64) (string, error) {
	path, err := get[string](ctx, s, `SELECT path FROM files WHERE id = ?`, id)
	if err != nil {
		return "", err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM files WHERE id = ?`, id); err != nil {
		return "", err
	}
	return *path, nil
}

// SlotLinks returns all links for a series keyed by "ledger/collection/number".
func (s *Store) SlotLinks(ctx context.Context, seriesID int64) (map[string][]int64, error) {
	links, err := list[SlotLink](ctx, s, `
		SELECT l.ledger, l.collection_id, l.number, l.file_id
		FROM file_slot_links l JOIN files f ON f.id = l.file_id
		WHERE f.series_id = ?`, seriesID)
	if err != nil {
		return nil, err
	}
	out := map[string][]int64{}
	for _, l := range links {
		key := SlotKey(l.Ledger, l.CollectionID, l.Number)
		out[key] = append(out[key], l.FileID)
	}
	return out, nil
}

func SlotKey(ledger string, collectionID, number int64) string {
	return fmt.Sprintf("%s/%d/%d", ledger, collectionID, number)
}

// --- bindings ---

func (s *Store) ListBindings(ctx context.Context, seriesID int64) ([]Binding, error) {
	return list[Binding](ctx, s, `
		SELECT id, series_id, source_id, source_manga_id,
		       COALESCE(source_name,'') AS source_name,
		       COALESCE(manga_title,'') AS manga_title, state, priority
		FROM source_bindings WHERE series_id = ? ORDER BY priority, id`, seriesID)
}

// AddBinding upserts one series↔manga binding (a site can hold several).
// An active binding obsoletes any "unmatched" sentinel of the same site.
func (s *Store) AddBinding(ctx context.Context, b Binding) (int64, error) {
	state := orDefault(b.State, "active")
	var id int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO source_bindings (series_id, source_id, source_manga_id, source_name, manga_title, state, priority)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(series_id, source_id, source_manga_id) DO UPDATE SET
			source_name = excluded.source_name,
			manga_title = excluded.manga_title,
			state = excluded.state
		RETURNING id`,
		b.SeriesID, b.SourceID, b.SourceMangaID, b.SourceName, b.MangaTitle, state, b.Priority).Scan(&id)
	if err != nil {
		return 0, err
	}
	if state == "active" {
		_, _ = s.db.ExecContext(ctx, `
			DELETE FROM source_bindings
			WHERE series_id = ? AND source_id = ? AND state = 'unmatched'`,
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
