package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/h-wb/sento/internal/apperr"
)

type EditionMetadataPatch struct {
	MakeLocal        bool     `json:"makeLocal,omitempty"`
	NumberingSource  *string  `json:"numberingSource"`
	DiscoverChapters *bool    `json:"discoverChapters"`
	EditionName      *string  `json:"editionName"`
	Language         *string  `json:"language"`
	Publisher        *string  `json:"publisher"`
	Medium           *string  `json:"medium"`
	Reset            []string `json:"reset"`
}

func (s *Store) EditEdition(ctx context.Context, seriesID, cid int64, p EditionMetadataPatch) error {
	c, err := s.GetEdition(ctx, cid)
	if err != nil {
		return err
	}
	if c.SeriesID != seriesID {
		return ErrNotFound
	}
	pending, err := s.EditionPending(ctx, cid)
	if err != nil {
		return err
	}
	if pending {
		return apperr.Conflictf("finish pending downloads before editing edition identity")
	}
	raw, err := get[string](ctx, s, `SELECT overrides_json FROM editions WHERE id=?`, cid)
	if err != nil {
		return err
	}
	overrides := map[string]any{}
	if err = json.Unmarshal([]byte(*raw), &overrides); err != nil {
		return err
	}
	fields := map[string]*string{"edition_name": p.EditionName, "language": p.Language, "publisher": p.Publisher, "medium": p.Medium}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = s.lockEditions(ctx, tx, false, cid); err != nil {
		return err
	}

	if p.MakeLocal {
		// The collections go and their works with them; the books stay.
		if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM collections WHERE edition_id=?`), cid); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE editions SET custom=TRUE,sync_state='local',numbering_source='manual' WHERE id=?`), cid); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE books SET provider_id=NULL,sync_state=CASE WHEN sync_state='retired' THEN 'retired' ELSE 'local' END WHERE edition_id=?`), cid); err != nil {
			return err
		}
	}
	if p.NumberingSource != nil {
		v := *p.NumberingSource
		if v != "manual" && v != "series_count" && v != "catalog" {
			return apperr.Invalidf("unknown numbering source")
		}
		if v == "series_count" && (c.Type != "chapter" || !c.Custom) {
			return apperr.Invalidf("series counts apply only to local chapter editions")
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE editions SET numbering_source=? WHERE id=?`), v, cid); err != nil {
			return err
		}
	}
	if p.DiscoverChapters != nil {
		if c.Type != "chapter" && *p.DiscoverChapters {
			return apperr.Invalidf("source discovery applies to chapter editions")
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE editions SET discover_chapters=? WHERE id=?`), *p.DiscoverChapters, cid); err != nil {
			return err
		}
	}
	// A change of language sends the edition's files back to review.
	identityChanged := false
	for k, v := range fields {
		if v == nil {
			continue
		}
		text := strings.TrimSpace(*v)
		if (k == "language" || k == "edition_name") && text == "" {
			return apperr.Invalidf("name and language are required")
		}
		before := map[string]string{"edition_name": c.EditionName, "language": c.Language, "publisher": c.Publisher, "medium": c.Medium}
		if before[k] != text {
			if k == "language" {
				identityChanged = true
			}
			overrides[k] = text
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE editions SET `+k+`=? WHERE id=?`), text, cid); err != nil {
			return err
		}
	}
	if identityChanged {
		if err = s.invalidateEditionCoverage(ctx, tx, cid); err != nil {
			return err
		}
	}

	for _, k := range p.Reset {
		if _, ok := fields[k]; !ok {
			return apperr.Invalidf("unknown override field")
		}
		delete(overrides, k)
	}
	body, _ := json.Marshal(overrides)
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE editions SET overrides_json=? WHERE id=?`), string(body), cid); err != nil {
		return err
	}
	return tx.Commit()
}

type CatalogLinkPreview struct {
	LocalVersions   []CollectedVersion `json:"localVersions"`
	CatalogVersions []CollectedVersion `json:"catalogVersions"`
	FileCount       int                `json:"fileCount"`
	Pending         bool               `json:"pending"`
	LocalBooks      []Book             `json:"localBooks"`
	CatalogBooks    []Book             `json:"catalogBooks"`
	Local           *Edition           `json:"local"`
	Catalog         *Edition           `json:"catalog"`
	Mapping         map[int64]int64    `json:"mapping"`
	Unmatched       []int64            `json:"unmatched"`
	// Ambiguous lists the unmatched books that had several candidates.
	Ambiguous []int64 `json:"ambiguous"`
}

// PreviewCatalogLink pairs the books of a local edition with those of the
// catalog edition it would become.
func (s *Store) PreviewCatalogLink(ctx context.Context, seriesID, localID, catalogID int64) (*CatalogLinkPreview, error) {
	p, err := s.previewBooks(ctx, seriesID, localID, catalogID)
	if err != nil {
		return nil, err
	}
	if !p.Local.Custom || p.Catalog.Custom {
		return nil, apperr.Invalidf("choose a local edition and a catalog edition")
	}
	if !sameLabel(p.Local, p.Catalog) {
		return nil, apperr.Invalidf("catalog link must match publication type, language and edition")
	}
	return p, nil
}

// PreviewMerge pairs the books of an edition with those of the one it
// would be merged into: a local duplicate, or a catalog edition of the
// same MangaBaka edition that was kept apart.
func (s *Store) PreviewMerge(ctx context.Context, seriesID, fromID, intoID int64) (*CatalogLinkPreview, error) {
	p, err := s.previewBooks(ctx, seriesID, fromID, intoID)
	if err != nil {
		return nil, err
	}
	from, into := p.Local, p.Catalog
	duplicate := from.Custom && from.EditionName == into.EditionName && from.Publisher == into.Publisher && from.Medium == into.Medium
	siblings := !from.Custom && !into.Custom && sameLabel(from, into) && (from.ProviderEditionID != "" || from.EditionName == into.EditionName)
	if !duplicate && !siblings {
		return nil, apperr.Invalidf("only duplicate editions can be merged")
	}
	return p, nil
}

// sameLabel says whether two editions of a series can be the same
// publication: one type, one language, and one MangaBaka edition when
// both name one.
func sameLabel(a, b *Edition) bool {
	if a.Type != b.Type || a.Language != b.Language {
		return false
	}
	return a.ProviderEditionID == "" || b.ProviderEditionID == "" || a.ProviderEditionID == b.ProviderEditionID
}

func (s *Store) previewBooks(ctx context.Context, seriesID, localID, catalogID int64) (*CatalogLinkPreview, error) {
	local, err := s.GetEdition(ctx, localID)
	if err != nil {
		return nil, err
	}
	catalog, err := s.GetEdition(ctx, catalogID)
	if err != nil {
		return nil, err
	}
	if local.SeriesID != seriesID || catalog.SeriesID != seriesID {
		return nil, ErrNotFound
	}
	if local.ID == catalog.ID {
		return nil, apperr.Invalidf("choose two different editions")
	}
	lw, err := s.ListBooks(ctx, localID)
	if err != nil {
		return nil, err
	}
	cw, err := s.ListBooks(ctx, catalogID)
	if err != nil {
		return nil, err
	}
	out := &CatalogLinkPreview{Local: local, Catalog: catalog, LocalBooks: lw, CatalogBooks: cw, Mapping: map[int64]int64{}, Unmatched: []int64{}, Ambiguous: []int64{}}
	// Two catalog editions hold different copies of the same books, each
	// with its own ISBN; only the number can pair those.
	same := sameBook
	if !local.Custom && !catalog.Custom {
		same = sameNumber
	}
	used := map[int64]bool{}
	for _, a := range lw {
		var matches []int64
		for _, b := range cw {
			if same(a, b) {
				matches = append(matches, b.ID)
			}
		}
		switch {
		case len(matches) == 1 && !used[matches[0]]:
			out.Mapping[a.ID], used[matches[0]] = matches[0], true
		case len(matches) == 0:
			out.Unmatched = append(out.Unmatched, a.ID)
		default:
			out.Unmatched = append(out.Unmatched, a.ID)
			out.Ambiguous = append(out.Ambiguous, a.ID)
		}
	}
	versions, err := s.ListVersions(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	out.LocalVersions = []CollectedVersion{}
	out.CatalogVersions = []CollectedVersion{}
	for _, t := range versions {
		if t.EditionID == localID {
			out.LocalVersions = append(out.LocalVersions, t)
		}
		if t.EditionID == catalogID {
			out.CatalogVersions = append(out.CatalogVersions, t)
		}
	}
	n, err := get[int](ctx, s, `SELECT COUNT(DISTINCT file_id) FROM asset_contents WHERE book_id IN(SELECT id FROM books WHERE edition_id=?)`, localID)
	if err != nil {
		return nil, err
	}
	out.FileCount = *n
	for _, cid := range []int64{localID, catalogID} {
		pending, e := s.EditionPending(ctx, cid)
		if e != nil {
			return nil, e
		}
		out.Pending = out.Pending || pending
	}
	return out, nil
}

// sameBook says whether two books of different editions are the same book:
// the same number, and no ISBN that says otherwise.
func sameBook(a, b Book) bool {
	return sameNumber(a, b) && (a.ISBN == "" || b.ISBN == "" || a.ISBN == b.ISBN)
}

// sameNumber compares what a book is called: the same number when both
// have one (labels vary: "1", "01"), otherwise the same label; and never
// across kinds or revisions.
func sameNumber(a, b Book) bool {
	if a.CountType != b.CountType || a.Revision != b.Revision || a.NumberKnown != b.NumberKnown {
		return false
	}
	if a.NumberKnown {
		return a.SequenceNum == b.SequenceNum
	}
	return a.SequenceStr == b.SequenceStr
}

// LinkCatalog turns a local edition into the catalog edition it stood in
// for. The local row survives with its id, files, versions and edits and
// takes over the catalog edition's collections, works and remaining
// books; the catalog row, now empty, is deleted. From then on it is a
// catalog edition like any other.
func (s *Store) LinkCatalog(ctx context.Context, seriesID, localID, catalogID int64) error {
	p, err := s.PreviewCatalogLink(ctx, seriesID, localID, catalogID)
	if err != nil {
		return err
	}
	if p.Pending {
		return apperr.Conflictf("edition has pending downloads")
	}
	n, err := get[int](ctx, s, `SELECT COUNT(*) FROM asset_contents a JOIN books w ON w.id=a.book_id WHERE w.edition_id=?`, catalogID)
	if err != nil {
		return err
	}
	if *n > 0 {
		return apperr.Conflictf("catalog edition already owns files; merge mappings need review")
	}
	for _, v := range p.CatalogVersions {
		if !v.IsDefault {
			return apperr.Conflictf("this edition is collected with several profiles; remove the extra cards before linking")
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = s.lockEditions(ctx, tx, false, localID, catalogID); err != nil {
		return err
	}

	// The link is the person's confirmation of identity, so the catalog's
	// name is adopted here, without sending files back to review. A name
	// edited by hand keeps its edit.
	var raw string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT overrides_json FROM editions WHERE id=?`), localID).Scan(&raw); err != nil {
		return err
	}
	if err = s.setUnlessOverridden(ctx, tx, "editions", localID, raw, map[string]any{"edition_name": p.Catalog.EditionName}); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE editions SET custom=FALSE,provider_edition_id=?,provider_edition_name=?,sync_state='linked',numbering_source='catalog',split=? WHERE id=?`), p.Catalog.ProviderEditionID, p.Catalog.ProviderEditionName, p.Catalog.Split, localID); err != nil {
		return err
	}
	// Matched catalog books hand their works to the local ones; the rest
	// join the edition now instead of waiting for the next refresh.
	pairs := map[int64]int64{}
	for local, catalog := range p.Mapping {
		pairs[catalog] = local
	}
	if err = s.foldEdition(ctx, tx, catalogID, localID, pairs); err != nil {
		return err
	}
	return tx.Commit()
}

// LinkSuggestion is a catalog edition that looks like the one a local
// edition has been standing in for.
type LinkSuggestion struct {
	SeriesID     int64 `json:"seriesId"`
	LocalID      int64 `json:"localId"`
	CatalogID    int64 `json:"catalogId"`
	Matched      int   `json:"matched"`
	LocalBooks   int   `json:"localBooks"`
	CatalogBooks int   `json:"catalogBooks"`
	local        *Edition
	catalog      *Edition
}

// LinkSuggestions finds, for each local edition (of one series, or all with
// seriesID 0), the catalog edition it could be linked to: same type and
// language, the same MangaBaka edition when the local one names one,
// linkable as it stands, not declined before, and agreeing on at least
// half of the local books. Only the best candidate per local edition is
// offered. Nothing is linked without a person.
func (s *Store) LinkSuggestions(ctx context.Context, seriesID int64) ([]LinkSuggestion, error) {
	pairs, err := list[LinkSuggestion](ctx, s, `
		SELECT l.series_id, l.id AS local_id, c.id AS catalog_id
		FROM editions l JOIN editions c ON c.series_id=l.series_id AND c.type=l.type AND c.language=l.language
		WHERE l.custom=TRUE AND c.custom=FALSE AND c.sync_state='linked' AND (?=0 OR l.series_id=?)
		  AND NOT EXISTS(SELECT 1 FROM edition_link_declines d WHERE d.edition_id=l.id AND d.declined_id=c.id)
		  AND NOT EXISTS(SELECT 1 FROM asset_contents a JOIN books w ON w.id=a.book_id WHERE w.edition_id=c.id)
		  AND NOT EXISTS(SELECT 1 FROM versions t WHERE t.edition_id=c.id AND t.is_default=FALSE)
		ORDER BY l.id, c.id`, seriesID, seriesID)
	if err != nil {
		return nil, err
	}
	best := map[int64]LinkSuggestion{}
	var order []int64
	for _, pair := range pairs {
		p, err := s.PreviewCatalogLink(ctx, pair.SeriesID, pair.LocalID, pair.CatalogID)
		if err != nil {
			if errors.Is(err, apperr.ErrInvalid) {
				continue // not the same publication
			}
			return nil, err
		}
		pair.Matched, pair.LocalBooks, pair.CatalogBooks = len(p.Mapping), len(p.LocalBooks), len(p.CatalogBooks)
		pair.local, pair.catalog = p.Local, p.Catalog
		if pair.Matched*2 < pair.LocalBooks {
			continue
		}
		old, seen := best[pair.LocalID]
		if !seen {
			order = append(order, pair.LocalID)
		}
		if !seen || pair.Matched > old.Matched {
			best[pair.LocalID] = pair
		}
	}
	out := make([]LinkSuggestion, 0, len(order))
	for _, id := range order {
		out = append(out, best[id])
	}
	return out, nil
}

// DeclineCatalogLink records that a catalog edition is not the local one.
func (s *Store) DeclineCatalogLink(ctx context.Context, seriesID, localID, catalogID int64) error {
	local, err := s.GetEdition(ctx, localID)
	if err != nil {
		return err
	}
	catalog, err := s.GetEdition(ctx, catalogID)
	if err != nil {
		return err
	}
	if local.SeriesID != seriesID || catalog.SeriesID != seriesID {
		return ErrNotFound
	}
	if catalog.Custom {
		return apperr.Invalidf("choose a catalog edition")
	}
	_, err = s.db.ExecContext(ctx, s.q(`INSERT INTO edition_link_declines(edition_id,declined_id) VALUES(?,?) ON CONFLICT DO NOTHING`), localID, catalogID)
	return err
}

func (s *Store) invalidateEditionCoverage(ctx context.Context, tx *sql.Tx, cid int64) error {
	if _, err := tx.ExecContext(ctx, s.q(`UPDATE asset_contents SET verified=FALSE WHERE book_id IN(SELECT id FROM books WHERE edition_id=?)`), cid); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, s.q(`UPDATE files SET needs_review=TRUE WHERE id IN(SELECT file_id FROM asset_contents WHERE book_id IN(SELECT id FROM books WHERE edition_id=?)) AND NOT EXISTS(SELECT 1 FROM asset_contents a WHERE a.file_id=files.id AND a.verified=TRUE)`), cid)
	return err
}
