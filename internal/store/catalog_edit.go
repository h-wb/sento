package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/metadata"
	"slices"
	"strings"
)

type EditionMetadataPatch struct {
	MakeLocal        bool      `json:"makeLocal,omitempty"`
	NumberingSource  *string   `json:"numberingSource"`
	EditionTraits    *[]string `json:"editionTraits"`
	ContentVariant   *string   `json:"contentVariant"`
	DiscoverChapters *bool     `json:"discoverChapters"`
	EditionName      *string   `json:"editionName"`
	Language         *string   `json:"language"`
	Publisher        *string   `json:"publisher"`
	Medium           *string   `json:"medium"`
	Reset            []string  `json:"reset"`
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
	var traits *string
	if p.EditionTraits != nil {
		values := slices.Clone(*p.EditionTraits)
		slices.Sort(values)
		values = slices.Compact(values)
		for _, v := range values {
			if v != "standard" && v != "omnibus" && v != "deluxe" && v != "boxset" {
				return apperr.Invalidf("unknown edition trait")
			}
		}
		if slices.Contains(values, "standard") && slices.Contains(values, "omnibus") {
			return apperr.Invalidf("choose standard numbering or omnibus numbering")
		}
		raw := metadata.TraitsJSON(values)
		traits = &raw
	}
	fields := map[string]*string{"edition_name": p.EditionName, "language": p.Language, "publisher": p.Publisher, "medium": p.Medium, "edition_traits": traits, "content_variant": p.ContentVariant}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = s.lockEditions(ctx, tx, false, cid); err != nil {
		return err
	}

	if p.MakeLocal {
		if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM external_refs WHERE (kind='edition' AND local_id=?) OR (kind='book' AND local_id IN(SELECT id FROM books WHERE edition_id=?))`), cid, cid); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE editions SET custom=TRUE,provider_id=NULL,sync_state='local',numbering_source='manual' WHERE id=?`), cid); err != nil {
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
	identityChanged := false
	for k, v := range fields {
		if v == nil {
			continue
		}
		text := strings.TrimSpace(*v)
		if k == "content_variant" && text != "original" && text != "colored" && text != "unknown" {
			return apperr.Invalidf("unknown content variant")
		}
		if (k == "language" || k == "edition_name") && text == "" {
			return apperr.Invalidf("name and language are required")
		}
		before := map[string]string{"edition_name": c.EditionName, "language": c.Language, "publisher": c.Publisher, "medium": c.Medium, "edition_traits": metadata.TraitsJSON(c.EditionTraits), "content_variant": c.ContentVariant}
		if before[k] != text {
			if k == "language" || k == "medium" || k == "edition_traits" || k == "content_variant" {
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
}

func (s *Store) PreviewCatalogLink(ctx context.Context, seriesID, localID, catalogID int64) (*CatalogLinkPreview, error) {
	return s.previewBookLink(ctx, seriesID, localID, catalogID, true)
}

func (s *Store) PreviewMerge(ctx context.Context, seriesID, fromID, intoID int64) (*CatalogLinkPreview, error) {
	p, err := s.previewBookLink(ctx, seriesID, fromID, intoID, false)
	if err != nil {
		return nil, err
	}
	if p.Local.EditionName != p.Catalog.EditionName || p.Local.Publisher != p.Catalog.Publisher || p.Local.Medium != p.Catalog.Medium {
		return nil, apperr.Invalidf("only duplicate editions can be merged")
	}
	return p, nil
}

func (s *Store) previewBookLink(ctx context.Context, seriesID, localID, catalogID int64, catalogOnly bool) (*CatalogLinkPreview, error) {
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
	if !local.Custom || (catalogOnly && catalog.ProviderID == "") || local.ID == catalog.ID {
		return nil, apperr.Invalidf("choose a local edition and a catalog edition")
	}
	traits := slices.Equal(local.EditionTraits, catalog.EditionTraits)
	variant := local.ContentVariant == catalog.ContentVariant
	if catalogOnly {
		// A local edition that never said what it is takes the catalog's word.
		traits = traits || len(local.EditionTraits) == 0
		variant = variant || local.ContentVariant == "unknown"
	}
	if local.Type != catalog.Type || local.Language != catalog.Language || !traits || !variant {
		return nil, apperr.Invalidf("catalog link must match publication type, language, layout and content version")
	}
	lw, err := s.ListBooks(ctx, localID)
	if err != nil {
		return nil, err
	}
	cw, err := s.ListBooks(ctx, catalogID)
	if err != nil {
		return nil, err
	}
	out := &CatalogLinkPreview{Local: local, Catalog: catalog, LocalBooks: lw, CatalogBooks: cw, Mapping: map[int64]int64{}, Unmatched: []int64{}}
	for _, a := range lw {
		var match int64
		for _, b := range cw {
			if sameBook(a, b) {
				if match > 0 {
					match = -1
					break
				}
				match = b.ID
			}
		}
		if match > 0 {
			out.Mapping[a.ID] = match
		} else {
			out.Unmatched = append(out.Unmatched, a.ID)
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
// the same number when both have one (labels vary: "1", "01"), otherwise the
// same label; and never across kinds, revisions or differing ISBNs.
func sameBook(a, b Book) bool {
	if a.CountType != b.CountType || a.Revision != b.Revision || a.NumberKnown != b.NumberKnown {
		return false
	}
	if a.ISBN != "" && b.ISBN != "" && a.ISBN != b.ISBN {
		return false
	}
	if a.NumberKnown {
		return a.SequenceNum == b.SequenceNum
	}
	return a.SequenceStr == b.SequenceStr
}

// LinkCatalog turns a local edition into the catalog edition it stood in
// for. The local row survives with its id, files, versions and overrides
// and takes over the catalog row's references, descriptors and remaining
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
			return apperr.Conflictf("catalog edition has additional collected versions; review those before linking")
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

	matched := map[int64]bool{}
	for local, catalog := range p.Mapping {
		matched[catalog] = true
		var ref sql.NullString
		if err = tx.QueryRowContext(ctx, s.q(`SELECT provider_id FROM books WHERE id=?`), catalog).Scan(&ref); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE books SET provider_id=?,sync_state='linked' WHERE id=?`), ref, local); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO external_refs(provider,kind,external_id,local_id,last_seen) SELECT provider,kind,external_id,?,last_seen FROM external_refs WHERE kind='book' AND local_id=? ON CONFLICT DO NOTHING`), local, catalog); err != nil {
			return err
		}
	}
	// Catalog books the local edition did not have join it now instead of
	// waiting for the next refresh; their references follow by id.
	for _, b := range p.CatalogBooks {
		if matched[b.ID] {
			continue
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE books SET edition_id=? WHERE id=?`), localID, b.ID); err != nil {
			return err
		}
	}
	// The link is the person's confirmation of identity, so the catalog's
	// descriptors are adopted here, without sending files back to review.
	// Fields edited by hand keep their override.
	var raw string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT overrides_json FROM editions WHERE id=?`), localID).Scan(&raw); err != nil {
		return err
	}
	overrides := map[string]json.RawMessage{}
	_ = json.Unmarshal([]byte(raw), &overrides)
	adopt := map[string]any{"edition_name": p.Catalog.EditionName, "publisher": p.Catalog.Publisher, "medium": p.Catalog.Medium}
	if len(p.Local.EditionTraits) == 0 {
		adopt["edition_traits"] = metadata.TraitsJSON(p.Catalog.EditionTraits)
	}
	if p.Local.ContentVariant == "unknown" {
		adopt["content_variant"] = p.Catalog.ContentVariant
	}
	for k, v := range adopt {
		if _, ok := overrides[k]; ok {
			continue
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE editions SET `+k+`=? WHERE id=?`), v, localID); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE editions SET custom=FALSE,provider_id=?,sync_state='linked',numbering_source='catalog',count_main=?,count_extra=?,count_other=?,start_date=?,end_date=?,raw_json=(SELECT raw_json FROM editions WHERE id=?),updated_at=? WHERE id=?`),
		p.Catalog.ProviderID, p.Catalog.CountMain, p.Catalog.CountExtra, p.Catalog.CountOther, p.Catalog.StartDate, p.Catalog.EndDate, catalogID, utcNow(), localID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE source_bindings SET edition_id=? WHERE edition_id=?`), localID, catalogID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO external_refs(provider,kind,external_id,local_id,last_seen) VALUES('mangabaka','edition',?,?,?) ON CONFLICT DO NOTHING`), p.Catalog.ProviderID, localID, utcNow()); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM external_refs WHERE (kind='edition' AND local_id=?) OR (kind='book' AND local_id IN(SELECT id FROM books WHERE edition_id=?))`), catalogID, catalogID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM editions WHERE id=?`), catalogID); err != nil {
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
// language, compatible traits, linkable as it stands, not declined before,
// and agreeing on at least half of the local books. Only the best candidate
// per local edition is offered. Nothing is linked without a person.
func (s *Store) LinkSuggestions(ctx context.Context, seriesID int64) ([]LinkSuggestion, error) {
	pairs, err := list[LinkSuggestion](ctx, s, `
		SELECT l.series_id, l.id AS local_id, c.id AS catalog_id
		FROM editions l JOIN editions c ON c.series_id=l.series_id AND c.type=l.type AND c.language=l.language
		WHERE l.custom=TRUE AND c.custom=FALSE AND c.sync_state='linked' AND COALESCE(c.provider_id,'')<>'' AND (?=0 OR l.series_id=?)
		  AND NOT EXISTS(SELECT 1 FROM edition_link_declines d WHERE d.edition_id=l.id AND d.external_id=c.provider_id)
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
	if catalog.ProviderID == "" {
		return apperr.Invalidf("choose a catalog edition")
	}
	_, err = s.db.ExecContext(ctx, s.q(`INSERT INTO edition_link_declines(edition_id,external_id) VALUES(?,?) ON CONFLICT DO NOTHING`), localID, catalog.ProviderID)
	return err
}

func (s *Store) invalidateEditionCoverage(ctx context.Context, tx *sql.Tx, cid int64) error {
	if _, err := tx.ExecContext(ctx, s.q(`UPDATE asset_contents SET verified=FALSE WHERE book_id IN(SELECT id FROM books WHERE edition_id=?)`), cid); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, s.q(`UPDATE files SET needs_review=TRUE WHERE id IN(SELECT file_id FROM asset_contents WHERE book_id IN(SELECT id FROM books WHERE edition_id=?)) AND NOT EXISTS(SELECT 1 FROM asset_contents a WHERE a.file_id=files.id AND a.verified=TRUE)`), cid)
	return err
}

// EditionTraitList stores known traits; [] explicitly means unspecified.
type EditionTraitList []string

func (t *EditionTraitList) Scan(src any) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), t)
	case []byte:
		return json.Unmarshal(v, t)
	default:
		*t = EditionTraitList{}
		return nil
	}
}
