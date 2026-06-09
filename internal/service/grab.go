package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/h-wb/sento/internal/library"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/store"
	"github.com/h-wb/sento/internal/suwayomi"
)

// Grabber runs the chapter path: source chapter lists → parse → match
// wanted slots → enqueue on Suwayomi → poll → import CBZ → link slots
// (decision-engine.md, source protocol only; torrents are M4).
type Grabber struct {
	store       *store.Store
	suwa        *suwayomi.Client
	naming      library.NamingConfig
	libraryRoot string
}

func NewGrabber(st *store.Store, suwa *suwayomi.Client, naming library.NamingConfig, libraryRoot string) *Grabber {
	return &Grabber{store: st, suwa: suwa, naming: naming, libraryRoot: libraryRoot}
}

// ChapterCandidate is one source chapter matched to a wanted slot.
type ChapterCandidate struct {
	SlotNumber  int64  `json:"slotNumber"` // hundredths
	ChapterID   int64  `json:"chapterId"`  // suwayomi chapter id
	ChapterName string `json:"chapterName"`
	Scanlator   string `json:"scanlator,omitempty"`
	SourceID    string `json:"sourceId"`
	BindingID   int64  `json:"bindingId"`
}

// FindChapterCandidates fetches chapter lists from all active bindings and
// matches them against the series' wanted chapter slots.
func (g *Grabber) FindChapterCandidates(ctx context.Context, seriesID int64, wanted map[int64]bool) ([]ChapterCandidate, error) {
	titles, err := g.store.SeriesTitles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	bindings, err := g.store.ListBindings(ctx, seriesID)
	if err != nil {
		return nil, err
	}

	chosen := map[int64]ChapterCandidate{} // slot → candidate (first binding by priority wins)
	for _, b := range bindings {
		if b.State != "active" {
			continue
		}
		chapters, err := g.suwa.GetChapters(ctx, b.SourceMangaID)
		if err != nil {
			slog.Warn("binding chapter fetch failed", "binding", b.ID, "error", err)
			continue
		}

		var maxSeen int64
		for _, ch := range chapters {
			res := parse.ParseAnchored(ch.Name, parse.ChannelSource, titles)
			// Corroborate with Suwayomi's own recognition when our parse
			// found nothing (terse names like "1184").
			ranges := res.Chapters
			if len(ranges) == 0 && ch.ChapterNumber > 0 {
				n := parse.Num(ch.ChapterNumber*100 + 0.5)
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
					if _, taken := chosen[n]; taken {
						continue
					}
					chosen[n] = ChapterCandidate{
						SlotNumber:  n,
						ChapterID:   ch.ID,
						ChapterName: ch.Name,
						Scanlator:   ch.Scanlator,
						SourceID:    b.SourceID,
						BindingID:   b.ID,
					}
				}
			}
		}
		if maxSeen > 0 {
			if err := g.store.BumpSourceSeenMax(ctx, seriesID, maxSeen); err != nil {
				slog.Warn("bumping source_seen_max failed", "error", err)
			}
		}
	}

	out := make([]ChapterCandidate, 0, len(chosen))
	for _, c := range chosen {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SlotNumber < out[j].SlotNumber })
	return out, nil
}

// GrabChapters enqueues downloads for candidates and records queue items.
func (g *Grabber) GrabChapters(ctx context.Context, seriesID int64, candidates []ChapterCandidate) ([]int64, error) {
	var queued []int64
	for _, c := range candidates {
		slots, _ := json.Marshal([]store.ExpectedSlot{{Ledger: "chapter", Number: c.SlotNumber}})
		qid, err := g.store.AddQueueItem(ctx, store.QueueItem{
			SeriesID:      seriesID,
			Protocol:      "source",
			ExternalID:    fmt.Sprintf("%d", c.ChapterID),
			Title:         c.ChapterName,
			ExpectedSlots: slots,
		})
		if err != nil {
			return queued, err
		}
		if err := g.suwa.EnqueueDownload(ctx, c.ChapterID); err != nil {
			_ = g.store.UpdateQueueItem(ctx, qid, store.QueueFailed, err.Error())
			continue
		}
		queued = append(queued, qid)
	}
	return queued, nil
}

// ProcessQueue advances active source-protocol queue items: poll download
// state, then fetch + import + link + clean up. Returns imported count.
func (g *Grabber) ProcessQueue(ctx context.Context) (int, error) {
	items, err := g.store.ListQueueItems(ctx, true)
	if err != nil {
		return 0, err
	}
	imported := 0
	for _, item := range items {
		if item.Protocol != "source" {
			continue
		}
		if err := g.processItem(ctx, item); err != nil {
			slog.Warn("queue item failed", "id", item.ID, "title", item.Title, "error", err)
			_ = g.store.UpdateQueueItem(ctx, item.ID, store.QueueFailed, err.Error())
			continue
		}
		// Re-read status via imported counter only; processItem updates rows.
		imported++
	}
	return imported, nil
}

func (g *Grabber) processItem(ctx context.Context, item store.QueueItem) error {
	var chapterID int64
	if _, err := fmt.Sscanf(item.ExternalID, "%d", &chapterID); err != nil {
		return fmt.Errorf("bad external id %q", item.ExternalID)
	}

	st, err := g.suwa.DownloadStateOf(ctx, chapterID)
	if err != nil {
		return err
	}
	switch {
	case st.Downloaded:
		// fall through to import
	case st.Queued:
		if item.Status != store.QueueDownloading {
			return g.store.UpdateQueueItem(ctx, item.ID, store.QueueDownloading, "")
		}
		return nil
	default:
		// Neither downloaded nor queued: stuck (adapter §1.4); kick once.
		age := time.Since(parseTime(item.CreatedAt))
		if age > 2*time.Minute {
			_ = g.suwa.Recover(ctx)
			if age > 10*time.Minute {
				return fmt.Errorf("download neither queued nor completed after %s", age.Round(time.Second))
			}
		}
		return nil
	}

	if err := g.store.UpdateQueueItem(ctx, item.ID, store.QueueImporting, ""); err != nil {
		return err
	}
	if err := g.importChapter(ctx, item, chapterID); err != nil {
		return err
	}
	return g.store.UpdateQueueItem(ctx, item.ID, store.QueueImported, "")
}

func (g *Grabber) importChapter(ctx context.Context, item store.QueueItem, chapterID int64) error {
	series, err := g.store.GetSeries(ctx, item.SeriesID)
	if err != nil {
		return err
	}
	var slots []store.ExpectedSlot
	if err := json.Unmarshal(item.ExpectedSlots, &slots); err != nil || len(slots) == 0 {
		return fmt.Errorf("queue item has no expected slots")
	}
	chapterNum := slots[0].Number

	ledgerInfo, err := g.store.GetChapterLedger(ctx, item.SeriesID)
	if err != nil {
		return err
	}

	seriesDir := series.Path
	if seriesDir == "" {
		seriesDir = g.libraryRoot + "/" + library.Render(g.naming.SeriesFolder, library.NameTokens{
			SeriesTitle: series.Title, SeriesID: series.MangaBakaID, Volume: -1, Chapter: -1,
		})
		if err := g.store.SetSeriesPath(ctx, item.SeriesID, seriesDir); err != nil {
			return err
		}
	}

	fileName := library.Render(g.naming.ChapterFile, library.NameTokens{
		SeriesTitle: series.Title,
		SeriesID:    series.MangaBakaID,
		Volume:      -1,
		Chapter:     chapterNum,
		Lang:        ledgerInfo.Language,
	})
	destPath := seriesDir
	if g.naming.ChapterDir != "" {
		destPath += "/" + g.naming.ChapterDir
	}
	destPath += "/" + fileName

	body, _, err := g.suwa.FetchCBZ(ctx, chapterID)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()

	info := library.ComicInfo{
		Series:      series.Title,
		Number:      library.RenderPlainNum(chapterNum),
		Summary:     series.Description,
		Web:         fmt.Sprintf("https://mangabaka.org/%d", series.MangaBakaID),
		LanguageISO: ledgerInfo.Language,
		Notes:       fmt.Sprintf("mangabaka:%d sento:v1", series.MangaBakaID),
	}
	size, err := library.ImportCBZ(body, destPath, info)
	if err != nil {
		return err
	}

	links := make([]store.SlotLink, 0, len(slots))
	for _, sl := range slots {
		links = append(links, store.SlotLink{Ledger: sl.Ledger, CollectionID: sl.CollectionID, Number: sl.Number})
	}
	if _, err := g.store.AddFile(ctx, store.File{
		SeriesID:   item.SeriesID,
		Path:       destPath,
		Size:       size,
		Provenance: "source",
	}, nil, links); err != nil {
		return err
	}

	// Bounded Suwayomi disk usage: delete after successful import.
	if err := g.suwa.DeleteDownload(ctx, chapterID); err != nil {
		slog.Warn("cleanup of suwayomi download failed", "chapter", chapterID, "error", err)
	}
	return nil
}

func parseTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Now()
}
