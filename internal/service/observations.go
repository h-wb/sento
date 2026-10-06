package service

import (
	"context"
	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

func (s *SearchService) observeCandidates(ctx context.Context, run *searchRun, cands []provider.Candidate) {
	if run.ledger != LedgerChapter {
		return
	}
	count := 0
	for _, cand := range cands {
		if cand.Source == nil {
			continue
		}
		cid := run.sourceEditions[cand.Source.BindingID]
		col := collectionByID(run.collections, cid)
		if col == nil || !col.DiscoverChapters {
			continue
		}
		rc := ReleaseCandidate{Parse: parse.ParseAnchored(cand.Title, parse.ChannelSource, run.titles), cand: cand}
		if rc.Parse.Language == "" {
			rc.Parse.Language = cand.Source.Lang
		}
		if rc.Parse.ContentType != parse.ContentManga || !editionCompatibleWithEvidence(rc.Parse, *col, cid) {
			continue
		}
		ranges := s.coverageRanges(run.ledger, &rc)
		for _, r := range ranges {
			if r.From != r.To || r.From < 0 {
				continue
			}
			exists := false
			for _, w := range run.units[cid] {
				if w.NumberKnown && w.SequenceNum == int64(r.From) {
					exists = true
				}
			}
			if exists {
				continue
			}
			if count >= 100 {
				return
			}
			count++
			_ = s.store.ObserveUnit(ctx, run.seriesID, store.UnitObservation{CollectionID: cid, Source: cand.Source.SourceID, ExternalKey: cand.Source.ChapterKey, Label: mediafile.RenderPlainNum(int64(r.From)), Number: int64(r.From)})
		}
	}
}
