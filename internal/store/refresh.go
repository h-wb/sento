package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/h-wb/sento/internal/metadata"
)

// ReconcileMetadata matches known external identity first. Local editions are
// never silently adopted. A complete snapshot marks disappeared entries stale
// without deleting their books, targets, coverage or user overrides.
func (s *Store) ReconcileMetadata(ctx context.Context, seriesID int64, md *metadata.Series, collections []metadata.Collection, works map[string][]metadata.Work) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, s.q(`UPDATE series SET title=?,sort_title=?,type=?,status=?,content_rating=?,year=?,total_chapters=?,final_volume=?,description=?,cover_url=?,raw_json=?,raw_schema_version=1,refreshed_at=? WHERE id=?`), md.Title, sortTitle(md.Title), md.Type, md.Status, md.ContentRating, md.Year, md.TotalChapters, md.FinalVolume, md.Description, md.CoverURL, string(md.RawJSON), utcNow(), seriesID)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE collections SET sync_state='stale' WHERE series_id=? AND custom=FALSE AND sync_state<>'superseded'`), seriesID); err != nil {
		return err
	}
	for _, c := range collections {
		var cid int64
		err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM collections WHERE series_id=? AND (mb_collection_id=? OR id IN(SELECT local_id FROM external_refs WHERE provider='mangabaka' AND kind='collection' AND external_id=?)) AND sync_state<>'superseded' ORDER BY custom DESC,id LIMIT 1`), seriesID, c.ProviderID, c.ProviderID).Scan(&cid)
		if errors.Is(err, sql.ErrNoRows) {
			// Exact descriptors are fallback evidence only among already linked rows.
			err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM collections WHERE series_id=? AND custom=FALSE AND edition_name=? AND language=? AND publisher=? AND medium=? AND ctype=? AND (SELECT COUNT(*) FROM collections x WHERE x.series_id=collections.series_id AND x.custom=FALSE AND x.edition_name=collections.edition_name AND x.language=collections.language AND x.publisher=collections.publisher AND x.medium=collections.medium AND x.ctype=collections.ctype AND x.sync_state<>'superseded')=1 AND sync_state<>'superseded' ORDER BY id LIMIT 1`), seriesID, c.EditionName, c.Language, c.Publisher, c.Medium, c.Type).Scan(&cid)
		}
		if errors.Is(err, sql.ErrNoRows) {
			err = tx.QueryRowContext(ctx, s.q(`INSERT INTO collections(series_id,mb_collection_id,edition_name,language,publisher,medium,ctype,sync_state,raw_json,numbering_source,edition_traits,content_variant) VALUES(?,?,?,?,?,?,?,'linked',?,'catalog',?,?) RETURNING id`), seriesID, c.ProviderID, c.EditionName, c.Language, c.Publisher, c.Medium, c.Type, string(c.RawJSON), metadata.TraitsJSON(metadata.EditionTraits(c.EditionName)), metadata.ContentVariant(c.EditionName)).Scan(&cid)
		} else if err == nil {
			var raw, oldLanguage, oldMedium, oldLayout, oldVariant string
			if err = tx.QueryRowContext(ctx, s.q(`SELECT overrides_json,language,medium,edition_traits,content_variant FROM collections WHERE id=?`), cid).Scan(&raw, &oldLanguage, &oldMedium, &oldLayout, &oldVariant); err != nil {
				return err
			}
			overrides := map[string]json.RawMessage{}
			_ = json.Unmarshal([]byte(raw), &overrides)
			layout, variant := metadata.TraitsJSON(metadata.EditionTraits(c.EditionName)), metadata.ContentVariant(c.EditionName)
			if layout == "[]" {
				layout = oldLayout
			}
			if variant == "unknown" {
				variant = oldVariant
			}
			identityChanged := false
			for k, v := range map[string]any{"edition_name": c.EditionName, "language": c.Language, "publisher": c.Publisher, "medium": c.Medium, "ctype": c.Type, "edition_traits": layout, "content_variant": variant} {
				if _, ok := overrides[k]; !ok {
					old := map[string]string{"language": oldLanguage, "medium": oldMedium, "edition_traits": oldLayout, "content_variant": oldVariant}
					if before, strong := old[k]; strong && before != v {
						identityChanged = true
					}
					if _, err = tx.ExecContext(ctx, s.q(`UPDATE collections SET `+k+`=? WHERE id=?`), v, cid); err != nil {
						return err
					}
				}
			}
			if identityChanged {
				if err = s.invalidateEditionCoverage(ctx, tx, cid); err != nil {
					return err
				}
			}
		}
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE collections SET mb_collection_id=?,count_main=?,count_extra=?,count_other=?,start_date=?,end_date=?,raw_json=?,sync_state='linked',updated_at=? WHERE id=?`), c.ProviderID, c.CountMain, c.CountExtra, c.CountOther, c.StartDate, c.EndDate, string(c.RawJSON), utcNow(), cid); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO external_refs(provider,kind,external_id,local_id,last_seen) VALUES('mangabaka','collection',?,?,?) ON CONFLICT(provider,kind,external_id,local_id) DO UPDATE SET last_seen=excluded.last_seen`), c.ProviderID, cid, utcNow()); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE works SET sync_state='stale' WHERE collection_id=? AND COALESCE(mb_work_id,'')<>'' AND sync_state<>'retired'`), cid); err != nil {
			return err
		}
		for _, w := range works[c.ProviderID] {
			if err = s.upsertWork(ctx, tx, cid, w); err != nil {
				return err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return s.EnsureModels(ctx, seriesID)
}
