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
	bindings, err := s.store.ListBindings(ctx, run.seriesID)
	if err != nil {
		return
	}
	trusted := map[int64]bool{}
	for _, b := range bindings {
		trusted[b.ID] = b.TrustChapters && b.State == "active"
	}
	count := 0
	for _, cand := range cands {
		if cand.Source == nil {
			continue
		}
		cid := run.sourceEditions[cand.Source.BindingID]
		col := editionByID(run.editions, cid)
		if col == nil || !col.DiscoverChapters {
			continue
		}
		rc := ReleaseCandidate{Parse: parse.ParseAnchored(cand.Title, parse.ChannelSource, run.titles), cand: cand}
		if rc.Parse.Language == "" {
			rc.Parse.Language = cand.Source.Lang
		}
		if v, _ := run.shelf.judge(rc.Parse, *col, cid); rc.Parse.ContentType != parse.ContentManga || v != fitYes {
			continue
		}
		ranges := s.coverageRanges(run.ledger, &rc)
		for _, r := range ranges {
			if r.From != r.To || r.From < 0 {
				continue
			}
			exists := false
			for _, w := range run.books[cid] {
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
			o := store.BookObservation{EditionID: cid, Source: cand.Source.SourceID, ExternalKey: cand.Source.ChapterKey, Label: mediafile.RenderPlainNum(int64(r.From)), Number: int64(r.From)}
			if err := s.store.ObserveBook(ctx, run.seriesID, o); err == nil && trusted[cand.Source.BindingID] {
				_ = s.store.AcceptTrustedObservation(ctx, run.seriesID, o)
			}
		}
	}
}
