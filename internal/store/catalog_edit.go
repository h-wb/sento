package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/h-wb/sento/internal/apperr"
	"strings"
)

type CollectionMetadataPatch struct {
	NumberingSource  *string  `json:"numberingSource"`
	EditionLayout    *string  `json:"editionLayout"`
	ContentVariant   *string  `json:"contentVariant"`
	DiscoverChapters *bool    `json:"discoverChapters"`
	EditionName      *string  `json:"editionName"`
	Language         *string  `json:"language"`
	Publisher        *string  `json:"publisher"`
	Medium           *string  `json:"medium"`
	Reset            []string `json:"reset"`
}

func (s *Store) EditCollection(ctx context.Context, seriesID, cid int64, p CollectionMetadataPatch) error {
	c, err := s.GetCollection(ctx, cid)
	if err != nil {
		return err
	}
	if c.SeriesID != seriesID {
		return ErrNotFound
	}
	pending, err := s.CollectionPending(ctx, cid)
	if err != nil {
		return err
	}
	if pending {
		return apperr.Conflictf("finish pending downloads before editing edition identity")
	}
	raw, err := get[string](ctx, s, `SELECT overrides_json FROM collections WHERE id=?`, cid)
	if err != nil {
		return err
	}
	overrides := map[string]any{}
	if err = json.Unmarshal([]byte(*raw), &overrides); err != nil {
		return err
	}
	fields := map[string]*string{"edition_name": p.EditionName, "language": p.Language, "publisher": p.Publisher, "medium": p.Medium, "edition_layout": p.EditionLayout, "content_variant": p.ContentVariant}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if p.NumberingSource != nil {
		v := *p.NumberingSource
		if v != "manual" && v != "series_count" && v != "catalog" {
			return apperr.Invalidf("unknown numbering source")
		}
		if v == "series_count" && (c.Type != "chapter" || !c.Custom) {
			return apperr.Invalidf("series counts apply only to local chapter editions")
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE collections SET numbering_source=? WHERE id=?`), v, cid); err != nil {
			return err
		}
	}
	if p.DiscoverChapters != nil {
		if c.Type != "chapter" && *p.DiscoverChapters {
			return apperr.Invalidf("source discovery applies to chapter editions")
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE collections SET discover_chapters=? WHERE id=?`), *p.DiscoverChapters, cid); err != nil {
			return err
		}
	}
	identityChanged := false
	for k, v := range fields {
		if v == nil {
			continue
		}
		text := strings.TrimSpace(*v)
		if k == "edition_layout" && text != "standard" && text != "omnibus" && text != "boxset" && text != "deluxe" && text != "custom" {
			return apperr.Invalidf("unknown edition layout")
		}
		if k == "content_variant" && text != "original" && text != "colored" {
			return apperr.Invalidf("unknown content variant")
		}
		if (k == "language" || k == "edition_name") && text == "" {
			return apperr.Invalidf("name and language are required")
		}
		before := map[string]string{"edition_name": c.EditionName, "language": c.Language, "publisher": c.Publisher, "medium": c.Medium, "edition_layout": c.EditionLayout, "content_variant": c.ContentVariant}
		if before[k] != text {
			if k == "language" || k == "medium" || k == "edition_layout" || k == "content_variant" {
				identityChanged = true
			}
			overrides[k] = text
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE collections SET `+k+`=? WHERE id=?`), text, cid); err != nil {
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
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE collections SET overrides_json=? WHERE id=?`), string(body), cid); err != nil {
		return err
	}
	return tx.Commit()
}

type CatalogLinkPreview struct {
	LocalTargets   []DownloadTarget `json:"localTargets"`
	CatalogTargets []DownloadTarget `json:"catalogTargets"`
	FileCount      int              `json:"fileCount"`
	Pending        bool             `json:"pending"`
	LocalUnits     []Work           `json:"localUnits"`
	CatalogUnits   []Work           `json:"catalogUnits"`
	Local          *Collection      `json:"local"`
	Catalog        *Collection      `json:"catalog"`
	Mapping        map[int64]int64  `json:"mapping"`
	Unmatched      []int64          `json:"unmatched"`
}

func (s *Store) PreviewCatalogLink(ctx context.Context, seriesID, localID, catalogID int64) (*CatalogLinkPreview, error) {
	return s.previewUnitLink(ctx, seriesID, localID, catalogID, true)
}

func (s *Store) PreviewMerge(ctx context.Context, seriesID, fromID, intoID int64) (*CatalogLinkPreview, error) {
	p, err := s.previewUnitLink(ctx, seriesID, fromID, intoID, false)
	if err != nil {
		return nil, err
	}
	if p.Local.EditionName != p.Catalog.EditionName || p.Local.Publisher != p.Catalog.Publisher || p.Local.Medium != p.Catalog.Medium {
		return nil, apperr.Invalidf("only duplicate editions can be merged")
	}
	return p, nil
}

func (s *Store) previewUnitLink(ctx context.Context, seriesID, localID, catalogID int64, catalogOnly bool) (*CatalogLinkPreview, error) {
	if err := s.EnsureModels(ctx, seriesID); err != nil {
		return nil, err
	}
	local, err := s.GetCollection(ctx, localID)
	if err != nil {
		return nil, err
	}
	catalog, err := s.GetCollection(ctx, catalogID)
	if err != nil {
		return nil, err
	}
	if local.SeriesID != seriesID || catalog.SeriesID != seriesID {
		return nil, ErrNotFound
	}
	if !local.Custom || (catalogOnly && catalog.ProviderID == "") || local.ID == catalog.ID || catalog.SyncState == "superseded" {
		return nil, apperr.Invalidf("choose a local edition and a catalog edition")
	}
	if local.Type != catalog.Type || local.Language != catalog.Language || local.EditionLayout != catalog.EditionLayout || local.ContentVariant != catalog.ContentVariant {
		return nil, apperr.Invalidf("catalog link must match publication type, language, layout and content version")
	}
	lw, err := s.ListWorks(ctx, localID)
	if err != nil {
		return nil, err
	}
	cw, err := s.ListWorks(ctx, catalogID)
	if err != nil {
		return nil, err
	}
	out := &CatalogLinkPreview{Local: local, Catalog: catalog, LocalUnits: lw, CatalogUnits: cw, Mapping: map[int64]int64{}, Unmatched: []int64{}}
	for _, a := range lw {
		var match int64
		for _, b := range cw {
			if a.SequenceStr == b.SequenceStr && a.CountType == b.CountType && a.Revision == b.Revision && (a.ISBN == "" || b.ISBN == "" || a.ISBN == b.ISBN) {
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
	targets, err := s.ListTargets(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	out.LocalTargets = []DownloadTarget{}
	out.CatalogTargets = []DownloadTarget{}
	for _, t := range targets {
		if t.CollectionID == localID {
			out.LocalTargets = append(out.LocalTargets, t)
		}
		if t.CollectionID == catalogID {
			out.CatalogTargets = append(out.CatalogTargets, t)
		}
	}
	n, err := get[int](ctx, s, `SELECT COUNT(DISTINCT file_id) FROM asset_coverage WHERE work_id IN(SELECT id FROM works WHERE collection_id=?)`, localID)
	if err != nil {
		return nil, err
	}
	out.FileCount = *n
	for _, cid := range []int64{localID, catalogID} {
		pending, e := s.CollectionPending(ctx, cid)
		if e != nil {
			return nil, e
		}
		out.Pending = out.Pending || pending
	}
	return out, nil
}
func (s *Store) LinkCatalog(ctx context.Context, seriesID, localID, catalogID int64) error {
	p, err := s.PreviewCatalogLink(ctx, seriesID, localID, catalogID)
	if err != nil {
		return err
	}
	for _, id := range []int64{localID, catalogID} {
		pending, e := s.CollectionPending(ctx, id)
		if e != nil {
			return e
		}
		if pending {
			return apperr.Conflictf("edition has pending downloads")
		}
	}
	n, err := get[int](ctx, s, `SELECT COUNT(*) FROM asset_coverage a JOIN works w ON w.id=a.work_id WHERE w.collection_id=?`, catalogID)
	if err != nil {
		return err
	}
	if *n > 0 {
		return apperr.Conflictf("catalog edition already owns files; merge mappings need review")
	}
	versions, err := s.ListTargets(ctx, seriesID)
	if err != nil {
		return err
	}
	for _, v := range versions {
		if v.CollectionID == catalogID && !v.IsDefault {
			return apperr.Conflictf("catalog edition has additional collected versions; review those before linking")
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for local, catalog := range p.Mapping {
		var ref sql.NullString
		if err = tx.QueryRowContext(ctx, s.q(`SELECT mb_work_id FROM works WHERE id=?`), catalog).Scan(&ref); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE works SET mb_work_id=?,sync_state='linked' WHERE id=?`), ref, local); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE collections SET mb_collection_id=?,sync_state='linked',numbering_source='catalog',raw_json=(SELECT raw_json FROM collections WHERE id=?) WHERE id=?`), p.Catalog.ProviderID, catalogID, localID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE collections SET sync_state='superseded',monitored=FALSE WHERE id=?`), catalogID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE source_bindings SET collection_id=? WHERE collection_id=?`), localID, catalogID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE download_targets SET monitored=FALSE WHERE collection_id=?`), catalogID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO external_refs(provider,kind,external_id,local_id,last_seen) VALUES('mangabaka','collection',?,?,?) ON CONFLICT DO NOTHING`), p.Catalog.ProviderID, localID, utcNow()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) invalidateEditionCoverage(ctx context.Context, tx *sql.Tx, cid int64) error {
	if _, err := tx.ExecContext(ctx, s.q(`UPDATE asset_coverage SET verified=FALSE WHERE work_id IN(SELECT id FROM works WHERE collection_id=?)`), cid); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, s.q(`UPDATE files SET needs_review=TRUE WHERE id IN(SELECT file_id FROM asset_coverage WHERE work_id IN(SELECT id FROM works WHERE collection_id=?)) AND NOT EXISTS(SELECT 1 FROM asset_coverage a WHERE a.file_id=files.id AND a.verified=TRUE)`), cid)
	return err
}
