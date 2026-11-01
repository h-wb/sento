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
	placed := s.placeChapterListings(ctx, run, bindings, cands)
	for id := range placed {
		trusted[id] = true
	}
	count := 0
	for _, cand := range cands {
		if cand.Source == nil {
			continue
		}
		cid := run.sourceEditions[cand.Source.BindingID]
		if cid == 0 {
			cid = placed[cand.Source.BindingID]
		}
		col := editionByID(run.editions, cid)
		if col == nil || col.Type != LedgerChapter {
			continue
		}
		rc := ReleaseCandidate{Parse: parse.ParseAnchored(cand.Title, parse.ChannelSource, run.titles), cand: cand}
		if rc.Parse.Language == "" {
			rc.Parse.Language = cand.Source.Lang
		}
		if v, _ := run.shelf.judge(rc.Parse, *col, cid); rc.Parse.ContentType != parse.ContentManga || v != fitYes {
			continue
		}
		ranges := coverageRanges(run.ledger, &rc)
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

// placeChapterListings places, without asking, a source's match whose
// edition no one has set, when there is no choice to make: it is the
// source's only match for the series, the series collects exactly one
// chapter edition, and what the match lists are chapters (a listing of
// volumes is evidence for a volume edition, never this one). Placing
// verifies the match, so its new chapters are added as they appear
// (store.SetBindingEdition). It returns the matches placed, by id.
func (s *SearchService) placeChapterListings(ctx context.Context, run *searchRun, bindings []store.Binding, cands []provider.Candidate) map[int64]int64 {
	home := run.chapterHome()
	if home == 0 {
		return nil
	}
	perSource := map[string]int{}
	for _, b := range bindings {
		if b.State == "active" {
			perSource[b.SourceID]++
		}
	}
	type listing struct{ chapters, volumes int }
	listings := map[int64]*listing{}
	for _, cand := range cands {
		if cand.Source == nil || run.sourceEditions[cand.Source.BindingID] != 0 {
			continue
		}
		l := listings[cand.Source.BindingID]
		if l == nil {
			l = &listing{}
			listings[cand.Source.BindingID] = l
		}
		p := parse.ParseAnchored(cand.Title, parse.ChannelSource, run.titles)
		switch {
		case len(bookRanges(p, LedgerVolume)) > 0:
			l.volumes++
		case len(bookRanges(p, LedgerChapter)) > 0:
			l.chapters++
		}
	}
	placed := map[int64]int64{}
	for _, b := range bindings {
		l := listings[b.ID]
		if l == nil || b.State != "active" || b.EditionID != 0 || perSource[b.SourceID] != 1 || l.chapters == 0 || l.volumes > 0 {
			continue
		}
		if err := s.store.SetBindingEdition(ctx, run.seriesID, b.ID, home); err == nil {
			placed[b.ID] = home
		}
	}
	return placed
}

// chapterHome is the one chapter edition the run collects, or 0 when it
// collects none or several.
func (run *searchRun) chapterHome() int64 {
	if run.ledger != LedgerChapter {
		return 0
	}
	home := int64(0)
	for _, v := range run.versions {
		col := editionByID(run.editions, v.EditionID)
		if col == nil || col.Type != LedgerChapter || !v.Monitored {
			continue
		}
		if home != 0 && home != col.ID {
			return 0
		}
		home = col.ID
	}
	return home
}
