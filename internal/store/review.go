package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/h-wb/sento/internal/apperr"
)

// ReviewItem is one question for a person: a file to match ("file"), a
// download that needs attention ("download"), a new chapter from a source
// ("chapter"), a catalog edition that looks like a local one ("edition"),
// a source that found nothing for the series ("source", ItemID the
// indexer entry).
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
	// Hint is what a file's own name says it holds when no edition claims
	// it yet, so review can prefill the ledger and numbers.
	Hint *FileHint `json:"hint,omitempty" db:"-"`
	// ReleaseKey groups a pack's file questions: the download its files
	// arrived in. Empty for a file that came in on its own.
	ReleaseKey string `json:"releaseKey,omitempty" db:"-"`
	// Release names the group in words (the download's title).
	Release string `json:"release,omitempty" db:"-"`
}

// FileHint is a file name's parsed ledger and numbers ("volume", ["1"]),
// offered to the reviewer when no edition fit decided them.
type FileHint struct {
	Ledger  string   `json:"ledger,omitempty"`
	Numbers []string `json:"numbers,omitempty"`
}

// ReviewProposal is a file's proposed mapping and why: the books, the
// edition they belong to, and the evidence in words.
type ReviewProposal struct {
	Books    []BookRef `json:"books"`
	Edition  string    `json:"edition"`
	Label    string    `json:"label"`
	Evidence string    `json:"evidence"`
}

// ReviewSkip is one file a bulk review action left alone, and why.
type ReviewSkip struct {
	FileID int64  `json:"fileId"`
	Reason string `json:"reason"`
}

// ReviewResolution is what a bulk decline did: the files it marked reviewed
// and unmapped, and those it left as they were.
type ReviewResolution struct {
	Resolved []int64      `json:"resolved"`
	Skipped  []ReviewSkip `json:"skipped"`
}

// DeclineFiles resolves file reviews in bulk: the exact counterpart of
// saving each file's mapping with no books (MapFile with empty slots), one
// decision for a whole pack. Every resolved file stays in the library,
// reviewed, claiming no book — and so shows up in cleanup as an unmapped
// file. The batch is one transaction; a file that no longer waits for a
// mapping decision is skipped with the reason, never overwritten.
func (s *Store) DeclineFiles(ctx context.Context, seriesID int64, fileIDs []int64) (*ReviewResolution, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	res := &ReviewResolution{Resolved: []int64{}, Skipped: []ReviewSkip{}}
	for _, fid := range slices.Compact(slices.Sorted(slices.Values(fileIDs))) {
		var fileSeries int64
		var waiting bool
		err := tx.QueryRowContext(ctx, s.q(`SELECT series_id, needs_review OR EXISTS(SELECT 1 FROM asset_contents c WHERE c.file_id=files.id AND c.verified=FALSE) FROM files WHERE id=?`), fid).Scan(&fileSeries, &waiting)
		if errors.Is(err, sql.ErrNoRows) {
			res.Skipped = append(res.Skipped, ReviewSkip{fid, "File is no longer available"})
			continue
		}
		if err != nil {
			return nil, err
		}
		if fileSeries != seriesID {
			res.Skipped = append(res.Skipped, ReviewSkip{fid, "File does not belong to this series"})
			continue
		}
		if err = s.rejectTrashEdit(ctx, tx, fid); err != nil {
			if errors.Is(err, apperr.ErrConflict) {
				res.Skipped = append(res.Skipped, ReviewSkip{fid, err.Error()})
				continue
			}
			return nil, err
		}
		if !waiting {
			res.Skipped = append(res.Skipped, ReviewSkip{fid, "File is no longer waiting for review"})
			continue
		}
		if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM asset_coverage WHERE file_id=?`), fid); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM asset_contents WHERE file_id=?`), fid); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE files SET needs_review=FALSE,unrequested=TRUE WHERE id=?`), fid); err != nil {
			return nil, err
		}
		res.Resolved = append(res.Resolved, fid)
	}
	return res, tx.Commit()
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
 FROM files f JOIN series s ON s.id=f.series_id WHERE (?=0 OR s.id=?) AND NOT EXISTS(SELECT 1 FROM file_trash WHERE file_id=f.id) AND (f.needs_review=TRUE OR f.presence IN('unavailable','unknown') OR EXISTS(SELECT 1 FROM asset_contents c WHERE c.file_id=f.id AND c.verified=FALSE))
 UNION ALL
 SELECT 'queue-'||CAST(q.id AS TEXT),'download',q.series_id,s.title,q.id,q.title,COALESCE(q.error,''),q.status
 FROM queue_items q JOIN series s ON s.id=q.series_id WHERE (?=0 OR s.id=?) AND q.status IN('needs_review','dispatch_unknown','failed','retry_wait')
 UNION ALL
 SELECT 'chapter-'||CAST(o.id AS TEXT),'chapter',c.series_id,s.title,o.id,o.label,'New chapter from '||o.source,o.state
 FROM book_observations o JOIN editions c ON c.id=o.edition_id JOIN series s ON s.id=c.series_id WHERE (?=0 OR s.id=?) AND o.state='pending'
 UNION ALL
 SELECT 'source-'||CAST(b.series_id AS TEXT)||'-'||b.source_id,'source',b.series_id,s.title,i.id,i.name,'Found nothing for this series. Pick the right entry, or turn the source off for it.','unmatched'
 FROM indexers i JOIN source_bindings b ON b.source_id=CAST(i.id AS TEXT) JOIN series s ON s.id=b.series_id
 LEFT JOIN series_site_settings o ON o.series_id=b.series_id AND o.provider='indexer' AND o.site_id=CAST(i.id AS TEXT)
 WHERE (?=0 OR s.id=?) AND i.protocol='source' AND COALESCE(o.enabled,i.enabled)=TRUE AND b.state='unmatched'
 AND NOT EXISTS(SELECT 1 FROM source_bindings a WHERE a.series_id=b.series_id AND a.source_id=b.source_id AND a.state='active')
 GROUP BY b.series_id,b.source_id,s.title,i.id,i.name
 ORDER BY series_title,kind,item_id`, sid, sid, sid, sid, sid, sid, sid, sid)
}
