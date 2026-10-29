package store

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/georgysavva/scany/v2/sqlscan"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/metadata"
)

// The catalog is mirrored under MangaBaka's own names and fields, so a
// change on their side is a change in one place here.
//
//   - A Collection is one product line of a series: a language, a
//     publisher, an edition and a medium.
//   - A Work is one book of a collection, with its own ISBN and date.
//
// What is collected sits above them. An Edition is MangaBaka's edition for
// one series, language and type; it groups the collections that differ
// only in medium or publisher. A Book is the works of those collections
// that carry the same number: the digital and the paperback volume 3 are
// two works and one book.

// Collection is a catalog collection filed under the edition it belongs to.
type Collection struct {
	ID                  int64  `json:"id"`
	EditionID           int64  `json:"editionId"`
	ProviderID          string `json:"providerId"`
	Publisher           string `json:"publisher"`
	Medium              string `json:"medium"`
	Format              string `json:"format"`
	Status              string `json:"status"`
	RelatedCollectionID string `json:"relatedCollectionId,omitempty"`
	CountMain           int    `json:"countMain"`
	CountExtra          int    `json:"countExtra"`
	CountOther          int    `json:"countOther"`
	StartDate           string `json:"startDate,omitempty"`
	EndDate             string `json:"endDate,omitempty"`
	SyncState           string `json:"syncState"`
}

const collectionColumns = `c.id,c.edition_id,c.provider_id,c.publisher,c.medium,c.format,c.status,c.related_collection_id,
	COALESCE(c.count_main,0) AS count_main,COALESCE(c.count_extra,0) AS count_extra,COALESCE(c.count_other,0) AS count_other,
	COALESCE(c.start_date,'') AS start_date,COALESCE(c.end_date,'') AS end_date,c.sync_state`

// Work is a catalog work filed under the book it is a copy of.
type Work struct {
	ID           int64  `json:"id"`
	CollectionID int64  `json:"collectionId"`
	BookID       int64  `json:"bookId"`
	ProviderID   string `json:"providerId"`
	SequenceNum  int64  `json:"sequenceNum"`
	SequenceStr  string `json:"sequenceStr" db:"sequence_string"`
	NumberKnown  bool   `json:"numberKnown"`
	CountType    string `json:"countType"`
	ReleaseDate  string `json:"releaseDate,omitempty"`
	ISBN         string `json:"isbn,omitempty"`
	Pages        int    `json:"pages,omitempty"`
	SubTitle     string `json:"subTitle,omitempty"`
	Revision     string `json:"revision,omitempty"`
	SyncState    string `json:"syncState"`
	Medium       string `json:"medium"`
	Publisher    string `json:"publisher"`
}

const workColumns = `w.id,w.collection_id,w.book_id,w.provider_id,w.sequence_num,w.sequence_string,w.number_known,w.count_type,
	w.release_date,w.isbn,w.pages,w.sub_title,w.revision,w.sync_state,c.medium,c.publisher`

// FieldList is the set of fields edited by hand, stored as the keys of a
// JSON object of the edited values.
type FieldList []string

func (f *FieldList) Scan(src any) error {
	var raw []byte
	switch v := src.(type) {
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	}
	fields := map[string]json.RawMessage{}
	_ = json.Unmarshal(raw, &fields)
	*f = FieldList{}
	for k := range fields {
		*f = append(*f, k)
	}
	slices.Sort(*f)
	return nil
}

// mediumOrder is which copy of a book speaks for it when several exist:
// its ISBN, page count and the edition's publisher. Release quality is the
// profile's business, not this order's.
var mediumOrder = []string{"digital", "paperback", "hardcover"}

func mediumRank(medium string) int {
	if i := slices.Index(mediumOrder, medium); i >= 0 {
		return i
	}
	return len(mediumOrder)
}

// preferred orders catalog rows: present in the catalog first, then by medium.
func preferred(aState, aMedium string, aID int64, bState, bMedium string, bID int64) int {
	return cmp.Or(
		cmp.Compare(b2i(aState != "linked"), b2i(bState != "linked")),
		cmp.Compare(mediumRank(aMedium), mediumRank(bMedium)),
		cmp.Compare(aID, bID),
	)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ListCollections returns the catalog collections of a series, each
// edition's best one first.
func (s *Store) ListCollections(ctx context.Context, seriesID int64) ([]Collection, error) {
	cols, err := list[Collection](ctx, s, `SELECT `+collectionColumns+` FROM collections c JOIN editions e ON e.id=c.edition_id WHERE e.series_id=? ORDER BY c.id`, seriesID)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(cols, func(a, b Collection) int {
		return cmp.Or(cmp.Compare(a.EditionID, b.EditionID), preferred(a.SyncState, a.Medium, a.ID, b.SyncState, b.Medium, b.ID))
	})
	return cols, nil
}

// ListWorks returns the catalog works behind a book, the preferred first.
func (s *Store) ListWorks(ctx context.Context, bookID int64) ([]Work, error) {
	works, err := list[Work](ctx, s, `SELECT `+workColumns+` FROM works w JOIN collections c ON c.id=w.collection_id WHERE w.book_id=? ORDER BY w.id`, bookID)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(works, func(a, b Work) int { return preferred(a.SyncState, a.Medium, a.ID, b.SyncState, b.Medium, b.ID) })
	return works, nil
}

// decorate attaches to each edition its collections and the names of the
// series' other catalog editions.
func (s *Store) decorate(ctx context.Context, seriesID int64, eds []Edition) error {
	cols, err := s.ListCollections(ctx, seriesID)
	if err != nil {
		return err
	}
	labels, err := list[struct {
		ID                  int64
		ProviderEditionName string
		Type                string
		Language            string
	}](ctx, s, `SELECT id,CASE WHEN provider_edition_name<>'' THEN provider_edition_name ELSE edition_name END AS provider_edition_name,type,language FROM editions WHERE series_id=?`, seriesID)
	if err != nil {
		return err
	}
	last, err := list[struct {
		EditionID int64
		Last      int64
	}](ctx, s, `SELECT edition_id,MAX(sequence_num) AS last FROM books WHERE count_type='main' AND number_known=TRUE AND edition_id IN(SELECT id FROM editions WHERE series_id=?) GROUP BY edition_id`, seriesID)
	if err != nil {
		return err
	}
	for i := range eds {
		base := func(lang string) string { return strings.ToLower(strings.SplitN(lang, "-", 2)[0]) }
		for _, l := range last {
			if l.EditionID == eds[i].ID {
				eds[i].LastNumber = l.Last
			}
			for _, o := range labels {
				if o.ID == l.EditionID && o.ID != eds[i].ID && o.Type == eds[i].Type && base(o.Language) == base(eds[i].Language) {
					eds[i].OtherLast = max(eds[i].OtherLast, l.Last)
				}
			}
		}
		eds[i].Collections = []Collection{}
		for _, c := range cols {
			if c.EditionID == eds[i].ID {
				eds[i].Collections = append(eds[i].Collections, c)
			}
		}
		for _, l := range labels {
			if l.ID != eds[i].ID && l.Type == eds[i].Type && !strings.EqualFold(l.ProviderEditionName, eds[i].ProviderEditionName) && !strings.EqualFold(l.ProviderEditionName, eds[i].EditionName) {
				eds[i].OtherLabels = append(eds[i].OtherLabels, l.ProviderEditionName)
			}
		}
	}
	return nil
}

// syncCatalog files a complete catalog snapshot of a series: each
// collection under its edition, each work under its book. Collections and
// works the snapshot no longer lists become stale; nothing is deleted.
func (s *Store) syncCatalog(ctx context.Context, tx *sql.Tx, seriesID int64, collections []metadata.Collection, works map[string][]metadata.Work) error {
	inSeries := `(SELECT id FROM editions WHERE series_id=?)`
	if _, err := tx.ExecContext(ctx, s.q(`UPDATE works SET sync_state='stale' WHERE collection_id IN(SELECT id FROM collections WHERE edition_id IN`+inSeries+`)`), seriesID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.q(`UPDATE collections SET sync_state='stale' WHERE edition_id IN`+inSeries), seriesID); err != nil {
		return err
	}
	// The collection that speaks for each edition's name and language.
	speaker := map[int64]metadata.Collection{}
	for _, c := range collections {
		var colID, edID int64
		err := tx.QueryRowContext(ctx, s.q(`SELECT c.id,c.edition_id FROM collections c JOIN editions e ON e.id=c.edition_id WHERE e.series_id=? AND c.provider_id=? ORDER BY c.id LIMIT 1`), seriesID, c.ProviderID).Scan(&colID, &edID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if edID, err = s.editionFor(ctx, tx, seriesID, c); err != nil {
				return err
			}
			if err = tx.QueryRowContext(ctx, s.q(`INSERT INTO collections(edition_id,provider_id) VALUES(?,?) RETURNING id`), edID, c.ProviderID).Scan(&colID); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if edID, err = s.rehome(ctx, tx, colID, edID, c); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE collections SET publisher=?,medium=?,format=?,status=?,related_collection_id=?,count_main=?,count_extra=?,count_other=?,start_date=?,end_date=?,raw_json=?,updated_at=?,sync_state='linked' WHERE id=?`),
			c.Publisher, c.Medium, c.Format, c.Status, c.RelatedID, c.CountMain, c.CountExtra, c.CountOther, c.StartDate, c.EndDate, string(c.RawJSON), utcNow(), colID); err != nil {
			return err
		}
		for _, w := range works[c.ProviderID] {
			if err = s.syncWork(ctx, tx, colID, edID, w); err != nil {
				return err
			}
		}
		if old, ok := speaker[edID]; !ok || mediumRank(c.Medium) < mediumRank(old.Medium) {
			speaker[edID] = c
		}
	}
	for edID, c := range speaker {
		if err := s.nameEdition(ctx, tx, edID, c); err != nil {
			return err
		}
	}
	if err := s.groupEditions(ctx, tx, seriesID); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM editions WHERE series_id=? AND custom=FALSE ORDER BY id`), seriesID)
	if err != nil {
		return err
	}
	ids, err := scanIDs(rows)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = s.deriveEdition(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

func scanIDs(rows *sql.Rows) ([]int64, error) {
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// editionFor finds the edition a new collection belongs to: the series'
// catalog edition of the same language, type and MangaBaka edition, or a
// new one. An edition someone separated is never joined again.
func (s *Store) editionFor(ctx context.Context, tx *sql.Tx, seriesID int64, c metadata.Collection) (int64, error) {
	var id int64
	query := `SELECT id FROM editions WHERE series_id=? AND custom=FALSE AND split=FALSE AND language=? AND type=? AND provider_edition_id=? ORDER BY id LIMIT 1`
	key := c.EditionID
	if key == "" {
		// A catalog without edition ids: the edition's name is all there is.
		query = strings.Replace(query, "provider_edition_id=?", "edition_name=?", 1)
		key = c.EditionName
	}
	err := tx.QueryRowContext(ctx, s.q(query), seriesID, c.Language, c.Type, key).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, s.q(`INSERT INTO editions(series_id,provider_edition_id,provider_edition_name,edition_name,language,type,sync_state,numbering_source,updated_at) VALUES(?,?,?,?,?,?,'linked','catalog',?) RETURNING id`),
			seriesID, c.EditionID, c.EditionName, c.EditionName, c.Language, c.Type, utcNow()).Scan(&id)
	}
	return id, err
}

// rehome keeps a known collection where it is unless the catalog moved it
// to another edition, language or type. Alone in its edition, the edition
// changes with it; among siblings it leaves them for an edition of its own.
func (s *Store) rehome(ctx context.Context, tx *sql.Tx, colID, edID int64, c metadata.Collection) (int64, error) {
	var ref, language, ctype string
	var siblings int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COALESCE(provider_edition_id,''),language,type,(SELECT COUNT(*) FROM collections x WHERE x.edition_id=editions.id) FROM editions WHERE id=?`), edID).Scan(&ref, &language, &ctype, &siblings); err != nil {
		return 0, err
	}
	same := (ref == "" || c.EditionID == "" || ref == c.EditionID) && ctype == c.Type
	if siblings > 1 {
		same = same && language == c.Language
	}
	if same {
		return edID, nil
	}
	if siblings <= 1 {
		_, err := tx.ExecContext(ctx, s.q(`UPDATE editions SET type=? WHERE id=?`), c.Type, edID)
		return edID, err
	}
	return s.detach(ctx, tx, colID, edID, c.Type, false)
}

// nameEdition applies what the catalog says an edition is, leaving fields
// edited by hand alone. A change of language sends its files to review.
func (s *Store) nameEdition(ctx context.Context, tx *sql.Tx, edID int64, c metadata.Collection) error {
	var raw, language string
	var custom bool
	if err := tx.QueryRowContext(ctx, s.q(`SELECT overrides_json,language,custom FROM editions WHERE id=?`), edID).Scan(&raw, &language, &custom); err != nil {
		return err
	}
	if custom {
		return nil
	}
	if _, err := tx.ExecContext(ctx, s.q(`UPDATE editions SET provider_edition_id=?,provider_edition_name=? WHERE id=?`), c.EditionID, c.EditionName, edID); err != nil {
		return err
	}
	if err := s.setUnlessOverridden(ctx, tx, "editions", edID, raw, map[string]any{"edition_name": c.EditionName, "language": c.Language}); err != nil {
		return err
	}
	if overridden(raw)["language"] || language == c.Language {
		return nil
	}
	return s.invalidateEditionCoverage(ctx, tx, edID)
}

func overridden(raw string) map[string]bool {
	fields := map[string]json.RawMessage{}
	_ = json.Unmarshal([]byte(raw), &fields)
	out := map[string]bool{}
	for k := range fields {
		out[k] = true
	}
	return out
}

// setUnlessOverridden writes catalog values to the fields nobody edited.
// table is "editions" or "books", never caller input.
func (s *Store) setUnlessOverridden(ctx context.Context, tx *sql.Tx, table string, id int64, overrides string, fields map[string]any) error {
	local := overridden(overrides)
	for k, v := range fields {
		if local[k] {
			continue
		}
		if _, err := tx.ExecContext(ctx, s.q(`UPDATE `+table+` SET `+k+`=? WHERE id=?`), v, id); err != nil {
			return err
		}
	}
	return nil
}

// numberKnown says whether a catalog work has a number at all: a numeric
// sequence, or a label that reads as one (volume 0 is a number).
func numberKnown(w metadata.Work) bool {
	if w.NumberKnown || w.SequenceNum != 0 {
		return true
	}
	_, err := strconv.ParseFloat(w.SequenceStr, 64)
	return err == nil
}

// syncWork records a catalog work under the book of its edition that
// carries the same number, kind and revision, creating the book when the
// edition has none. Two works of one collection are never the same book.
func (s *Store) syncWork(ctx context.Context, tx *sql.Tx, colID, edID int64, w metadata.Work) error {
	known := numberKnown(w)
	var id int64
	err := tx.QueryRowContext(ctx, s.q(`SELECT id FROM works WHERE collection_id=? AND provider_id=?`), colID, w.ProviderID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		var bookID int64
		err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM books WHERE edition_id=? AND count_type=? AND revision=? AND number_known=?
			AND ((number_known=TRUE AND sequence_num=?) OR (number_known=FALSE AND COALESCE(sequence_string,'')=?))
			AND NOT EXISTS(SELECT 1 FROM works x WHERE x.book_id=books.id AND x.collection_id=?) ORDER BY id LIMIT 1`),
			edID, w.CountType, w.Revision, known, int64(w.SequenceNum), w.SequenceStr, colID).Scan(&bookID)
		if errors.Is(err, sql.ErrNoRows) {
			err = tx.QueryRowContext(ctx, s.q(`INSERT INTO books(edition_id,sequence_num,sequence_string,count_type,number_known,revision,sync_state) VALUES(?,?,?,?,?,?,'linked') RETURNING id`),
				edID, int64(w.SequenceNum), w.SequenceStr, w.CountType, known, w.Revision).Scan(&bookID)
		}
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, s.q(`INSERT INTO works(collection_id,book_id,provider_id) VALUES(?,?,?) RETURNING id`), colID, bookID, w.ProviderID).Scan(&id)
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, s.q(`UPDATE works SET sequence_num=?,sequence_string=?,number_known=?,count_type=?,release_date=?,isbn=?,pages=?,sub_title=?,revision=?,raw_json=?,sync_state='linked' WHERE id=?`),
		int64(w.SequenceNum), w.SequenceStr, known, w.CountType, w.ReleaseDate, w.ISBN, w.Pages, w.SubTitle, w.Revision, string(w.RawJSON), id)
	return err
}

type workRow struct {
	Work
	RawJSON sql.NullString
}

// deriveEdition recomputes what an edition and its books show from their
// collections and works. The preferred copy supplies a book's ISBN and
// pages; its date is the earliest any copy came out, since a book that
// exists in print exists. Fields edited by hand are left alone.
func (s *Store) deriveEdition(ctx context.Context, tx *sql.Tx, edID int64) error {
	var cols []Collection
	if err := sqlscan.Select(ctx, tx, &cols, s.q(`SELECT `+collectionColumns+` FROM collections c WHERE c.edition_id=? ORDER BY c.id`), edID); err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	slices.SortStableFunc(cols, func(a, b Collection) int { return preferred(a.SyncState, a.Medium, a.ID, b.SyncState, b.Medium, b.ID) })
	var raw string
	if err := tx.QueryRowContext(ctx, s.q(`SELECT overrides_json FROM editions WHERE id=?`), edID).Scan(&raw); err != nil {
		return err
	}
	best := cols[0]
	state, start, end := "stale", "", ""
	for _, c := range cols {
		if c.SyncState != "linked" {
			continue
		}
		state = "linked"
		if c.StartDate != "" && (start == "" || c.StartDate < start) {
			start = c.StartDate
		}
		end = max(end, c.EndDate)
	}
	if err := s.setUnlessOverridden(ctx, tx, "editions", edID, raw, map[string]any{"publisher": best.Publisher, "medium": best.Medium}); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.q(`UPDATE editions SET sync_state=?,start_date=?,end_date=?,updated_at=?,
		count_main=(SELECT COUNT(*) FROM books b WHERE b.edition_id=editions.id AND b.count_type='main' AND b.sync_state<>'retired'),
		count_extra=(SELECT COUNT(*) FROM books b WHERE b.edition_id=editions.id AND b.count_type='extra' AND b.sync_state<>'retired'),
		count_other=(SELECT COUNT(*) FROM books b WHERE b.edition_id=editions.id AND b.count_type='other' AND b.sync_state<>'retired')
		WHERE id=?`), state, start, end, utcNow(), edID); err != nil {
		return err
	}

	var works []workRow
	if err := sqlscan.Select(ctx, tx, &works, s.q(`SELECT `+workColumns+`,w.raw_json FROM works w JOIN collections c ON c.id=w.collection_id WHERE c.edition_id=? ORDER BY w.id`), edID); err != nil {
		return err
	}
	slices.SortStableFunc(works, func(a, b workRow) int {
		return cmp.Or(cmp.Compare(a.BookID, b.BookID), preferred(a.SyncState, a.Medium, a.ID, b.SyncState, b.Medium, b.ID))
	})
	for i := 0; i < len(works); {
		j := i
		first, date, linked := works[i], "", false
		for ; j < len(works) && works[j].BookID == first.BookID; j++ {
			w := works[j]
			if w.SyncState != first.SyncState {
				continue // stale copies say nothing while a listed one exists
			}
			linked = linked || w.SyncState == "linked"
			if w.ReleaseDate != "" && (date == "" || w.ReleaseDate < date) {
				date = w.ReleaseDate
			}
		}
		i = j
		var overrides string
		if err := tx.QueryRowContext(ctx, s.q(`SELECT overrides_json FROM books WHERE id=?`), first.BookID).Scan(&overrides); err != nil {
			return err
		}
		if err := s.setUnlessOverridden(ctx, tx, "books", first.BookID, overrides, map[string]any{
			"sequence_num": first.SequenceNum, "sequence_string": first.SequenceStr, "number_known": first.NumberKnown,
			"count_type": first.CountType, "revision": first.Revision, "release_date": date, "isbn": first.ISBN, "pages": first.Pages,
		}); err != nil {
			return err
		}
		bookState := "stale"
		if linked {
			bookState = "linked"
		}
		if _, err := tx.ExecContext(ctx, s.q(`UPDATE books SET provider_id=?,raw_json=?,sync_state=CASE WHEN sync_state='retired' THEN 'retired' ELSE ? END WHERE id=?`), first.ProviderID, first.RawJSON, bookState, first.BookID); err != nil {
			return err
		}
	}
	return nil
}

// groupEditions folds catalog editions of one series that are the same
// MangaBaka edition in the same language into one, as long as nothing of
// the person's would be merged by it: an edition with files, pending
// downloads or more than its default version stays as it is, to be merged
// by hand. The edition with files, else a collected one, is kept.
func (s *Store) groupEditions(ctx context.Context, tx *sql.Tx, seriesID int64) error {
	type row struct {
		ID                int64
		Language          string
		Type              string
		ProviderEditionID string
		Name              string
		Busy              bool
		Monitored         bool
	}
	var rows []row
	if err := sqlscan.Select(ctx, tx, &rows, s.q(`SELECT e.id,e.language,e.type,e.provider_edition_id,e.edition_name AS name,
		(EXISTS(SELECT 1 FROM asset_contents a JOIN books b ON b.id=a.book_id WHERE b.edition_id=e.id)
		 OR EXISTS(SELECT 1 FROM asset_coverage a JOIN books b ON b.id=a.book_id WHERE b.edition_id=e.id)
		 OR EXISTS(SELECT 1 FROM book_reservations r JOIN books b ON b.id=r.book_id WHERE b.edition_id=e.id)
		 OR (SELECT COUNT(*) FROM versions v WHERE v.edition_id=e.id)>1) AS busy,
		EXISTS(SELECT 1 FROM versions v WHERE v.edition_id=e.id AND v.monitored=TRUE) AS monitored
		FROM editions e WHERE e.series_id=? AND e.custom=FALSE AND e.split=FALSE AND COALESCE(e.provider_edition_id,'')<>'' ORDER BY e.id`), seriesID); err != nil {
		return err
	}
	groups := map[[3]string][]row{}
	var order [][3]string
	for _, r := range rows {
		key := [3]string{r.Language, r.Type, r.ProviderEditionID}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], r)
	}
	pending, err := s.pendingEditions(ctx, tx, seriesID)
	if err != nil {
		return err
	}
	for _, key := range order {
		group := groups[key]
		if len(group) < 2 {
			continue
		}
		slices.SortStableFunc(group, func(a, b row) int {
			return cmp.Or(cmp.Compare(b2i(!a.Busy), b2i(!b.Busy)), cmp.Compare(b2i(!a.Monitored), b2i(!b.Monitored)), cmp.Compare(a.ID, b.ID))
		})
		keep := group[0]
		for _, other := range group[1:] {
			if other.Busy || pending[other.ID] || pending[keep.ID] {
				continue
			}
			if other.Monitored && !keep.Monitored {
				if _, err = tx.ExecContext(ctx, s.q(`UPDATE versions SET monitored=TRUE WHERE edition_id=? AND is_default=TRUE`), keep.ID); err != nil {
					return err
				}
			}
			if err = s.foldEdition(ctx, tx, other.ID, keep.ID, nil); err != nil {
				return err
			}
			// Nobody asked for this, so it is said: History shows it.
			note, _ := json.Marshal(DecisionBody{Event: "editions_grouped", Title: keep.Name + " · " + key[0] + ": MangaBaka lists its media as one edition, so they are collected as one", Provider: "mangabaka"})
			if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO decisions(series_id,body_json) VALUES(?,?)`), seriesID, string(note)); err != nil {
				return err
			}
		}
	}
	return nil
}

// pendingEditions lists the editions of a series with downloads under way.
func (s *Store) pendingEditions(ctx context.Context, tx *sql.Tx, seriesID int64) (map[int64]bool, error) {
	rows, err := tx.QueryContext(ctx, s.q(`SELECT books_json FROM queue_items WHERE series_id=? AND status IN('queued','downloading','importing','needs_review','retry_wait','dispatch_unknown')`), seriesID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[int64]bool{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var books []BookRef
		if err = json.Unmarshal([]byte(raw), &books); err != nil {
			return nil, err
		}
		for _, b := range books {
			out[b.EditionID] = true
		}
	}
	return out, rows.Err()
}

// foldEdition empties one edition into another and deletes it. mapping
// says which book of from is which book of into; a nil mapping pairs them
// by number. A paired book hands its catalog works, files and per-book
// choices to its counterpart; an unpaired one moves over whole. The
// versions of from are the caller's to move first, or they go with it.
func (s *Store) foldEdition(ctx context.Context, tx *sql.Tx, from, into int64, mapping map[int64]int64) error {
	var fromBooks, intoBooks []Book
	if err := sqlscan.Select(ctx, tx, &fromBooks, s.q(`SELECT `+bookColumns+` FROM books WHERE edition_id=? ORDER BY sequence_num,id`), from); err != nil {
		return err
	}
	if mapping == nil {
		if err := sqlscan.Select(ctx, tx, &intoBooks, s.q(`SELECT `+bookColumns+` FROM books WHERE edition_id=? ORDER BY sequence_num,id`), into); err != nil {
			return err
		}
		mapping = pairBooks(fromBooks, intoBooks)
	}
	for _, b := range fromBooks {
		target, ok := mapping[b.ID]
		if !ok {
			if _, err := tx.ExecContext(ctx, s.q(`UPDATE books SET edition_id=? WHERE id=?`), into, b.ID); err != nil {
				return err
			}
			continue
		}
		for _, stmt := range []string{
			`INSERT INTO asset_coverage(file_id,version_id,book_id,verified,pinned,evidence,selected) SELECT file_id,version_id,?,verified,pinned,evidence,selected FROM asset_coverage WHERE book_id=? ON CONFLICT DO NOTHING`,
			`INSERT INTO asset_contents(file_id,book_id,verified,evidence) SELECT file_id,?,verified,evidence FROM asset_contents WHERE book_id=? ON CONFLICT DO NOTHING`,
			`INSERT INTO book_overrides(version_id,book_id,monitored) SELECT version_id,?,monitored FROM book_overrides WHERE book_id=? ON CONFLICT DO NOTHING`,
			`UPDATE works SET book_id=? WHERE book_id=?`,
		} {
			if _, err := tx.ExecContext(ctx, s.q(stmt), target, b.ID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, s.q(`DELETE FROM books WHERE id=?`), b.ID); err != nil {
			return err
		}
	}
	for _, stmt := range []string{
		`UPDATE collections SET edition_id=? WHERE edition_id=?`,
		`UPDATE source_bindings SET edition_id=? WHERE edition_id=?`,
	} {
		if _, err := tx.ExecContext(ctx, s.q(stmt), into, from); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, s.q(`DELETE FROM editions WHERE id=?`), from); err != nil {
		return err
	}
	return s.deriveEdition(ctx, tx, into)
}

// pairBooks matches each book of one edition with the first unclaimed
// book of another that carries the same number.
func pairBooks(from, into []Book) map[int64]int64 {
	out := map[int64]int64{}
	used := map[int64]bool{}
	for _, a := range from {
		for _, b := range into {
			if !used[b.ID] && sameNumber(a, b) {
				out[a.ID], used[b.ID] = b.ID, true
				break
			}
		}
	}
	return out
}

// detach gives one collection an edition of its own, copied from the one
// it leaves. Each of its works takes a new book there. A book left behind
// with no catalog copy goes when nothing of the person's hangs on it, and
// otherwise stays as a local book. split marks a person's decision, which
// later refreshes respect; without it the new edition may be grouped again.
func (s *Store) detach(ctx context.Context, tx *sql.Tx, colID, edID int64, ctype string, split bool) (int64, error) {
	var newID int64
	if err := tx.QueryRowContext(ctx, s.q(`INSERT INTO editions(series_id,provider_edition_id,provider_edition_name,edition_name,language,type,publisher,medium,profile_id,sync_state,numbering_source,split,updated_at)
		SELECT e.series_id,e.provider_edition_id,e.provider_edition_name,e.edition_name,e.language,?,c.publisher,c.medium,e.profile_id,'linked','catalog',?,? FROM editions e JOIN collections c ON c.id=? WHERE e.id=? RETURNING id`),
		ctype, split, utcNow(), colID, edID).Scan(&newID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO versions(edition_id,name,is_default,monitored) VALUES(?,'Default',TRUE,FALSE)`), newID); err != nil {
		return 0, err
	}
	var works []Work
	if err := sqlscan.Select(ctx, tx, &works, s.q(`SELECT `+workColumns+` FROM works w JOIN collections c ON c.id=w.collection_id WHERE w.collection_id=? ORDER BY w.id`), colID); err != nil {
		return 0, err
	}
	for _, w := range works {
		var bookID int64
		if err := tx.QueryRowContext(ctx, s.q(`INSERT INTO books(edition_id,sequence_num,sequence_string,count_type,number_known,revision,sync_state) VALUES(?,?,?,?,?,?,'linked') RETURNING id`),
			newID, w.SequenceNum, w.SequenceStr, w.CountType, w.NumberKnown, w.Revision).Scan(&bookID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, s.q(`UPDATE works SET book_id=? WHERE id=?`), bookID, w.ID); err != nil {
			return 0, err
		}
	}
	if _, err := tx.ExecContext(ctx, s.q(`UPDATE collections SET edition_id=? WHERE id=?`), newID, colID); err != nil {
		return 0, err
	}
	orphan := `edition_id=? AND COALESCE(provider_id,'')<>'' AND NOT EXISTS(SELECT 1 FROM works w WHERE w.book_id=books.id)`
	if _, err := tx.ExecContext(ctx, s.q(`DELETE FROM books WHERE `+orphan+`
		AND NOT EXISTS(SELECT 1 FROM asset_contents a WHERE a.book_id=books.id)
		AND NOT EXISTS(SELECT 1 FROM asset_coverage a WHERE a.book_id=books.id)
		AND NOT EXISTS(SELECT 1 FROM book_reservations r WHERE r.book_id=books.id)`), edID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, s.q(`UPDATE books SET provider_id=NULL,sync_state='local' WHERE `+orphan), edID); err != nil {
		return 0, err
	}
	for _, id := range []int64{edID, newID} {
		if err := s.deriveEdition(ctx, tx, id); err != nil {
			return 0, err
		}
	}
	return newID, nil
}

// SeparateCollection takes one collection out of its edition, for the case
// the grouping gets wrong: two publishers' "Standard Edition" that are
// different translations. The new edition is not collected until asked.
func (s *Store) SeparateCollection(ctx context.Context, seriesID, collectionID int64) (int64, error) {
	col, err := get[Collection](ctx, s, `SELECT `+collectionColumns+` FROM collections c JOIN editions e ON e.id=c.edition_id WHERE c.id=? AND e.series_id=?`, collectionID, seriesID)
	if err != nil {
		return 0, err
	}
	ed, err := s.GetEdition(ctx, col.EditionID)
	if err != nil {
		return 0, err
	}
	if len(ed.Collections) < 2 {
		return 0, apperr.Invalidf("this is the edition's only collection")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = s.lockEditions(ctx, tx, false, ed.ID); err != nil {
		return 0, err
	}
	id, err := s.detach(ctx, tx, collectionID, ed.ID, ed.Type, true)
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}
