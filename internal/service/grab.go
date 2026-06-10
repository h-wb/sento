package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"

	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// Grabber runs the chapter path: provider candidates → parse → match
// wanted slots → grab via the owning provider. Import happens in Importer.
type Grabber struct {
	store    *store.Store
	registry *provider.Registry
}

func NewGrabber(st *store.Store, registry *provider.Registry) *Grabber {
	return &Grabber{store: st, registry: registry}
}

// ChapterCandidate is one source candidate matched to a wanted slot
// (the UI/grab round-trip DTO).
type ChapterCandidate struct {
	SlotNumber  int64  `json:"slotNumber"` // hundredths
	ChapterID   int64  `json:"chapterId"`
	ChapterName string `json:"chapterName"`
	Scanlator   string `json:"scanlator,omitempty"`
	SourceID    string `json:"sourceId"`
	BindingID   int64  `json:"bindingId,omitempty"`
	Provider    string `json:"provider"`
	Site        string `json:"site,omitempty"`
}

func (g *Grabber) searchRequest(ctx context.Context, seriesID int64) (provider.SearchRequest, error) {
	titles, err := g.store.SeriesTitles(ctx, seriesID)
	if err != nil {
		return provider.SearchRequest{}, err
	}
	bindings, err := g.store.ListBindings(ctx, seriesID)
	if err != nil {
		return provider.SearchRequest{}, err
	}
	req := provider.SearchRequest{SeriesID: seriesID, Titles: titles}
	if len(titles) > 0 {
		req.Query = titles[0]
	}
	for _, b := range bindings {
		if b.State != "active" {
			continue
		}
		req.Bindings = append(req.Bindings, provider.Binding{
			BindingID: b.ID, SourceID: b.SourceID,
			SourceMangaID: b.SourceMangaID, SourceName: b.SourceName,
		})
	}
	return req, nil
}

// FindChapterCandidates matches provider candidates against the given
// chapter slots. Only source-protocol candidates are considered for the
// chapter ledger in v1 (torrent chapter import is roadmap). With
// includeAll, every match is returned (interactive search); otherwise the
// first candidate per slot wins (automatic mode, binding priority order).
func (g *Grabber) FindChapterCandidates(ctx context.Context, seriesID int64, wanted map[int64]bool, includeAll bool) ([]ChapterCandidate, error) {
	req, err := g.searchRequest(ctx, seriesID)
	if err != nil {
		return nil, err
	}

	suwa := g.registry.Get("suwayomi")
	if suwa == nil {
		return nil, fmt.Errorf("no source provider configured")
	}
	candidates, err := suwa.Search(ctx, req)
	if err != nil {
		slog.Warn("chapter search degraded", "error", err)
	}

	chosen := map[int64]ChapterCandidate{}
	var all []ChapterCandidate
	var maxSeen int64

	for _, cand := range candidates {
		if cand.Source == nil {
			continue
		}
		res := parse.ParseAnchored(cand.Title, parse.ChannelSource, req.Titles)
		ranges := res.Chapters
		// Corroborate with the source's own recognition when our parse
		// found nothing (terse names like "1184").
		if len(ranges) == 0 && cand.Source.ChapterNumber > 0 {
			n := parse.Num(cand.Source.ChapterNumber*100 + 0.5)
			ranges = []parse.Range{{From: n, To: n}}
		}
		for _, r := range ranges {
			if int64(r.To) > maxSeen {
				maxSeen = int64(r.To)
			}
			// Integer slots only (D8): walk whole numbers in range.
			for n := (int64(r.From) + 99) / 100 * 100; n <= int64(r.To); n += 100 {
				if !wanted[n] {
					continue
				}
				cc := ChapterCandidate{
					SlotNumber:  n,
					ChapterID:   cand.Source.ChapterID,
					ChapterName: cand.Title,
					Scanlator:   cand.Source.Scanlator,
					SourceID:    cand.Source.SourceID,
					BindingID:   cand.Source.BindingID,
					Provider:    cand.Provider,
					Site:        cand.Site,
				}
				if includeAll {
					all = append(all, cc)
					continue
				}
				if _, taken := chosen[n]; taken {
					continue
				}
				chosen[n] = cc
			}
		}
	}

	if maxSeen > 0 {
		if err := g.store.BumpSourceSeenMax(ctx, seriesID, maxSeen); err != nil {
			slog.Warn("bumping source_seen_max failed", "error", err)
		}
	}

	if includeAll {
		sort.Slice(all, func(i, j int) bool { return all[i].SlotNumber < all[j].SlotNumber })
		return all, nil
	}
	out := make([]ChapterCandidate, 0, len(chosen))
	for _, c := range chosen {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SlotNumber < out[j].SlotNumber })
	return out, nil
}

// GrabChapters grabs candidates via their owning provider and records
// queue items.
func (g *Grabber) GrabChapters(ctx context.Context, seriesID int64, candidates []ChapterCandidate) ([]int64, error) {
	var queued []int64
	for _, c := range candidates {
		prov := g.registry.Get(orDefaultStr(c.Provider, "suwayomi"))
		if prov == nil {
			return queued, fmt.Errorf("provider %q is not configured", c.Provider)
		}
		slots, _ := json.Marshal([]store.ExpectedSlot{{Ledger: "chapter", Number: c.SlotNumber}})
		qid, err := g.store.AddQueueItem(ctx, store.QueueItem{
			SeriesID:      seriesID,
			Protocol:      provider.ProtocolSource,
			ExternalID:    fmt.Sprintf("%d", c.ChapterID),
			Title:         c.ChapterName,
			ExpectedSlots: slots,
		})
		if err != nil {
			return queued, err
		}
		if _, err := prov.Grab(ctx, provider.Candidate{
			Provider: prov.ID(), Protocol: provider.ProtocolSource, Title: c.ChapterName,
			Source: &provider.SourceRef{ChapterID: c.ChapterID, SourceID: c.SourceID},
		}); err != nil {
			_ = g.store.UpdateQueueItem(ctx, qid, store.QueueFailed, err.Error())
			continue
		}
		queued = append(queued, qid)
	}
	return queued, nil
}

// HasSourceProvider reports whether chapter search/grab is possible.
func (g *Grabber) HasSourceProvider() bool { return g.registry.Get("suwayomi") != nil }

func orDefaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
