package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/h-wb/sento/internal/metadata"
)

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
