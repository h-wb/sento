// Package store is the SQLite persistence layer. Hand-written queries for
// now; sqlc can replace the internals later without changing callers.
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
	MangaBakaID   int64  `json:"mangabakaId"`
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
	ProviderID  string `json:"-"`
	EditionName string `json:"editionName"`
	Language    string `json:"language"`
	Publisher   string `json:"publisher"`
	Medium      string `json:"medium"`
	Type        string `json:"type"`
	Monitored   bool   `json:"monitored"`
	CountMain   int    `json:"countMain"`
	CountExtra  int    `json:"countExtra"`
	CountOther  int    `json:"countOther"`
	StartDate   string `json:"startDate,omitempty"`
	EndDate     string `json:"endDate,omitempty"`
}

type Work struct {
	ID           int64  `json:"id"`
	CollectionID int64  `json:"collectionId"`
	SequenceNum  int64  `json:"sequenceNum"` // hundredths
	SequenceStr  string `json:"sequenceStr"`
	CountType    string `json:"countType"`
	ReleaseDate  string `json:"releaseDate,omitempty"`
	ISBN         string `json:"isbn,omitempty"`
	Pages        int    `json:"pages,omitempty"`
}

type ChapterLedger struct {
	SeriesID      int64  `json:"seriesId"`
	Monitored     bool   `json:"monitored"`
	Language      string `json:"language"`
	CutoffNum     int64  `json:"cutoffNum"`
	SourceSeenMax int64  `json:"sourceSeenMax"`
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
	State         string `json:"state"`
	Priority      int    `json:"priority"`
}

// --- series ---

func (s *Store) ListSeries(ctx context.Context) ([]Series, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, mangabaka_id, title, sort_title, type, status,
		       COALESCE(content_rating,''), COALESCE(year,0), COALESCE(total_chapters,0),
		       COALESCE(final_volume,0), COALESCE(description,''), COALESCE(cover_url,''),
		       monitored, COALESCE(path,''), added_at, COALESCE(refreshed_at,'')
		FROM series ORDER BY sort_title`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Series
	for rows.Next() {
		var x Series
		if err := rows.Scan(&x.ID, &x.MangaBakaID, &x.Title, &x.SortTitle, &x.Type, &x.Status,
			&x.ContentRating, &x.Year, &x.TotalChapters, &x.FinalVolume, &x.Description,
			&x.CoverURL, &x.Monitored, &x.Path, &x.AddedAt, &x.RefreshedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) GetSeries(ctx context.Context, id int64) (*Series, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, mangabaka_id, title, sort_title, type, status,
		       COALESCE(content_rating,''), COALESCE(year,0), COALESCE(total_chapters,0),
		       COALESCE(final_volume,0), COALESCE(description,''), COALESCE(cover_url,''),
		       monitored, COALESCE(path,''), added_at, COALESCE(refreshed_at,'')
		FROM series WHERE id = ?`, id)
	var x Series
	err := row.Scan(&x.ID, &x.MangaBakaID, &x.Title, &x.SortTitle, &x.Type, &x.Status,
		&x.ContentRating, &x.Year, &x.TotalChapters, &x.FinalVolume, &x.Description,
		&x.CoverURL, &x.Monitored, &x.Path, &x.AddedAt, &x.RefreshedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &x, nil
}

func (s *Store) DeleteSeries(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM series WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
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
				INSERT OR IGNORE INTO works (collection_id, mb_work_id, sequence_num,
					sequence_string, count_type, release_date, isbn, pages, raw_json)
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
		INSERT INTO chapter_ledgers (series_id, monitored, language, cutoff_num, source_seen_max)
		VALUES (?,?,?,?,0)`, seriesID, true, lang, cutoff); err != nil {
		return 0, fmt.Errorf("inserting chapter ledger: %w", err)
	}

	return seriesID, tx.Commit()
}

// --- collections / works ---

func (s *Store) ListCollections(ctx context.Context, seriesID int64) ([]Collection, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, series_id, COALESCE(mb_collection_id,''), edition_name, language,
		       publisher, medium, ctype, monitored, COALESCE(count_main,0),
		       COALESCE(count_extra,0), COALESCE(count_other,0),
		       COALESCE(start_date,''), COALESCE(end_date,'')
		FROM collections WHERE series_id = ? ORDER BY edition_name, language, medium`, seriesID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Collection
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.SeriesID, &c.ProviderID, &c.EditionName, &c.Language,
			&c.Publisher, &c.Medium, &c.Type, &c.Monitored, &c.CountMain, &c.CountExtra,
			&c.CountOther, &c.StartDate, &c.EndDate); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetCollection(ctx context.Context, id int64) (*Collection, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, series_id, COALESCE(mb_collection_id,''), edition_name, language,
		       publisher, medium, ctype, monitored, COALESCE(count_main,0),
		       COALESCE(count_extra,0), COALESCE(count_other,0),
		       COALESCE(start_date,''), COALESCE(end_date,'')
		FROM collections WHERE id = ?`, id)
	var c Collection
	err := row.Scan(&c.ID, &c.SeriesID, &c.ProviderID, &c.EditionName, &c.Language,
		&c.Publisher, &c.Medium, &c.Type, &c.Monitored, &c.CountMain, &c.CountExtra,
		&c.CountOther, &c.StartDate, &c.EndDate)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) SetCollectionMonitored(ctx context.Context, seriesID, collectionID int64, monitored bool) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE collections SET monitored = ? WHERE id = ? AND series_id = ?`,
		monitored, collectionID, seriesID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListWorks(ctx context.Context, collectionID int64) ([]Work, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, collection_id, sequence_num, COALESCE(sequence_string,''), count_type,
		       COALESCE(release_date,''), COALESCE(isbn,''), COALESCE(pages,0)
		FROM works WHERE collection_id = ? ORDER BY sequence_num`, collectionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Work
	for rows.Next() {
		var w Work
		if err := rows.Scan(&w.ID, &w.CollectionID, &w.SequenceNum, &w.SequenceStr,
			&w.CountType, &w.ReleaseDate, &w.ISBN, &w.Pages); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// --- chapter ledger ---

func (s *Store) GetChapterLedger(ctx context.Context, seriesID int64) (*ChapterLedger, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT series_id, monitored, language, cutoff_num, source_seen_max
		FROM chapter_ledgers WHERE series_id = ?`, seriesID)
	var l ChapterLedger
	err := row.Scan(&l.SeriesID, &l.Monitored, &l.Language, &l.CutoffNum, &l.SourceSeenMax)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *Store) UpdateChapterLedger(ctx context.Context, l ChapterLedger) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE chapter_ledgers SET monitored = ?, language = ?, cutoff_num = ?
		WHERE series_id = ?`, l.Monitored, l.Language, l.CutoffNum, l.SeriesID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) BumpSourceSeenMax(ctx context.Context, seriesID, seen int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE chapter_ledgers SET source_seen_max = MAX(source_seen_max, ?)
		WHERE series_id = ?`, seen, seriesID)
	return err
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, series_id, path, size, provenance, unrequested, added_at
		FROM files WHERE series_id = ? ORDER BY path`, seriesID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []File
	for rows.Next() {
		var f File
		if err := rows.Scan(&f.ID, &f.SeriesID, &f.Path, &f.Size, &f.Provenance,
			&f.Unrequested, &f.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// DeleteFile removes a file row (links cascade) and returns its path so
// the caller can remove it from disk.
func (s *Store) DeleteFile(ctx context.Context, id int64) (string, error) {
	var path string
	err := s.db.QueryRowContext(ctx, `SELECT path FROM files WHERE id = ?`, id).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM files WHERE id = ?`, id); err != nil {
		return "", err
	}
	return path, nil
}

// SlotLinks returns all links for a series keyed by "ledger/collection/number".
func (s *Store) SlotLinks(ctx context.Context, seriesID int64) (map[string][]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.ledger, l.collection_id, l.number, l.file_id
		FROM file_slot_links l JOIN files f ON f.id = l.file_id
		WHERE f.series_id = ?`, seriesID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string][]int64{}
	for rows.Next() {
		var ledger string
		var colID, number, fileID int64
		if err := rows.Scan(&ledger, &colID, &number, &fileID); err != nil {
			return nil, err
		}
		key := SlotKey(ledger, colID, number)
		out[key] = append(out[key], fileID)
	}
	return out, rows.Err()
}

func SlotKey(ledger string, collectionID, number int64) string {
	return fmt.Sprintf("%s/%d/%d", ledger, collectionID, number)
}

// --- bindings ---

func (s *Store) ListBindings(ctx context.Context, seriesID int64) ([]Binding, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, series_id, source_id, source_manga_id, COALESCE(source_name,''), state, priority
		FROM source_bindings WHERE series_id = ? ORDER BY priority`, seriesID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Binding
	for rows.Next() {
		var b Binding
		if err := rows.Scan(&b.ID, &b.SeriesID, &b.SourceID, &b.SourceMangaID,
			&b.SourceName, &b.State, &b.Priority); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) AddBinding(ctx context.Context, b Binding) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO source_bindings (series_id, source_id, source_manga_id, source_name, state, priority)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(series_id, source_id) DO UPDATE SET
			source_manga_id = excluded.source_manga_id,
			source_name = excluded.source_name,
			state = excluded.state`,
		b.SeriesID, b.SourceID, b.SourceMangaID, b.SourceName, orDefault(b.State, "active"), b.Priority)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) DeleteBinding(ctx context.Context, seriesID, bindingID int64) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM source_bindings WHERE id = ? AND series_id = ?`, bindingID, seriesID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
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
