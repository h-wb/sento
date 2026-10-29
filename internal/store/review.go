package store

import (
	"context"
	"fmt"
	"strings"
)

type ReviewItem struct {
	Status      string `json:"status"`
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	SeriesID    int64  `json:"seriesId"`
	SeriesTitle string `json:"seriesTitle"`
	ItemID      int64  `json:"itemId"`
	Title       string `json:"title"`
	Detail      string `json:"detail"`
	// RelatedID is the catalog edition of an "edition" item (ItemID is the
	// local one).
	RelatedID int64 `json:"relatedId,omitempty" db:"-"`
	// Proposal is what the evidence says a file holds; nothing is recorded
	// until it is accepted.
	Proposal *ReviewProposal `json:"proposal,omitempty" db:"-"`
	// Sighting is the evidence of a "missing-edition" item.
	Sighting *EditionSighting `json:"sighting,omitempty" db:"-"`
}

// ReviewProposal is a file's proposed mapping and why: the books, the
// edition they belong to, and the evidence in words.
type ReviewProposal struct {
	Books    []BookRef `json:"books"`
	Edition  string    `json:"edition"`
	Label    string    `json:"label"`
	Evidence string    `json:"evidence"`
}

func (s *Store) ReviewItems(ctx context.Context, sid int64) ([]ReviewItem, error) {
	items, err := s.reviewRows(ctx, sid)
	if err != nil {
		return nil, err
	}
	links, err := s.LinkSuggestions(ctx, sid)
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		series, err := s.GetSeries(ctx, l.SeriesID)
		if err != nil {
			return nil, err
		}
		detail := fmt.Sprintf("MangaBaka lists “%s”, which looks like this edition.", editionText(l.catalog))
		if l.LocalBooks > 0 {
			detail += fmt.Sprintf(" %d of your %d books match its %d.", l.Matched, l.LocalBooks, l.CatalogBooks)
		}
		items = append(items, ReviewItem{
			ID: fmt.Sprintf("edition-%d-%d", l.LocalID, l.CatalogID), Kind: "edition", Status: "link",
			SeriesID: l.SeriesID, SeriesTitle: series.Title, ItemID: l.LocalID, RelatedID: l.CatalogID,
			Title: editionText(l.local), Detail: detail,
		})
	}
	return items, nil
}

func editionText(e *Edition) string {
	parts := []string{e.EditionName, e.Language}
	for _, p := range []string{e.Publisher, e.Medium} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

func (s *Store) reviewRows(ctx context.Context, sid int64) ([]ReviewItem, error) {
	return list[ReviewItem](ctx, s, `
 SELECT 'file-'||CAST(f.id AS TEXT) AS id,'file' AS kind,f.series_id,s.title AS series_title,f.id AS item_id,f.path AS title,
 CASE WHEN f.presence='unavailable' THEN 'Storage unavailable: '||f.presence_error WHEN f.presence='unknown' THEN 'File presence has not been checked' ELSE 'Confirm the edition and books in this file' END AS detail, f.presence AS status
 FROM files f JOIN series s ON s.id=f.series_id WHERE (?=0 OR s.id=?) AND (f.needs_review=TRUE OR f.unrequested=TRUE OR f.presence IN('unavailable','unknown') OR EXISTS(SELECT 1 FROM asset_contents c WHERE c.file_id=f.id AND c.verified=FALSE))
 UNION ALL
 SELECT 'queue-'||CAST(q.id AS TEXT),'download',q.series_id,s.title,q.id,q.title,COALESCE(q.error,''),q.status
 FROM queue_items q JOIN series s ON s.id=q.series_id WHERE (?=0 OR s.id=?) AND q.status IN('needs_review','dispatch_unknown','failed','retry_wait')
 UNION ALL
 SELECT 'chapter-'||CAST(o.id AS TEXT),'chapter',c.series_id,s.title,o.id,o.label,'New chapter from '||o.source,o.state
 FROM book_observations o JOIN editions c ON c.id=o.edition_id JOIN series s ON s.id=c.series_id WHERE (?=0 OR s.id=?) AND o.state='pending'
 ORDER BY series_title,kind,item_id LIMIT 500`, sid, sid, sid, sid, sid, sid)
}

// EditionSighting is an edition releases state that the series lacks:
// "deluxe" in English, seen in Releases distinct releases.
type EditionSighting struct {
	SeriesID int64  `json:"seriesId"`
	Facet    string `json:"facet"`
	Language string `json:"language"`
	Releases int    `json:"releases"`
	Example  string `json:"example"`
	SeenAt   string `json:"seenAt"`
}

// RecordEditionSighting keeps the latest evidence for a missing edition.
// A dismissal survives: the same releases will be seen again.
func (s *Store) RecordEditionSighting(ctx context.Context, e EditionSighting) error {
	_, err := s.db.ExecContext(ctx, s.q(`INSERT INTO edition_sightings(series_id,facet,language,releases,example,seen_at) VALUES(?,?,?,?,?,?)
 ON CONFLICT(series_id,facet,language) DO UPDATE SET releases=excluded.releases,example=excluded.example,seen_at=excluded.seen_at`),
		e.SeriesID, e.Facet, e.Language, e.Releases, e.Example, utcNow())
	return err
}

// EditionSightings lists the sightings not dismissed (sid 0: every series).
func (s *Store) EditionSightings(ctx context.Context, sid int64) ([]EditionSighting, error) {
	return list[EditionSighting](ctx, s, `SELECT series_id,facet,language,releases,example,seen_at FROM edition_sightings
 WHERE (?=0 OR series_id=?) AND dismissed=FALSE ORDER BY series_id,facet,language`, sid, sid)
}

// DismissEditionSighting records "not an edition I want".
func (s *Store) DismissEditionSighting(ctx context.Context, seriesID int64, facet, language string) error {
	return s.touch(ctx, `UPDATE edition_sightings SET dismissed=TRUE WHERE series_id=? AND facet=? AND language=?`, seriesID, facet, language)
}
