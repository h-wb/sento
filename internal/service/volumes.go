package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/h-wb/sento/internal/ledger"
	"github.com/h-wb/sento/internal/library"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/prowlarr"
	"github.com/h-wb/sento/internal/qbittorrent"
	"github.com/h-wb/sento/internal/store"
)

// VolumeSearcher implements the torrent volume path: Prowlarr search →
// parse → reject/score funnel (decision-engine.md) → grab via qBittorrent
// → import completed torrents by hardlink.
type VolumeSearcher struct {
	store       *store.Store
	prowlarr    *prowlarr.Client
	qbit        *qbittorrent.Client
	naming      library.NamingConfig
	libraryRoot string
	category    string
}

func NewVolumeSearcher(st *store.Store, pr *prowlarr.Client, qb *qbittorrent.Client,
	naming library.NamingConfig, libraryRoot, category string,
) *VolumeSearcher {
	return &VolumeSearcher{
		store: st, prowlarr: pr, qbit: qb,
		naming: naming, libraryRoot: libraryRoot, category: category,
	}
}

// VolumeCandidate is one Prowlarr release run through the funnel. Rejected
// candidates carry their reason codes (interactive search shows the full
// funnel, decision-engine.md §7).
type VolumeCandidate struct {
	GUID         string       `json:"guid"`
	Title        string       `json:"title"`
	Indexer      string       `json:"indexer"`
	Size         int64        `json:"size"`
	Seeders      int          `json:"seeders"`
	InfoHash     string       `json:"infoHash,omitempty"`
	CollectionID int64        `json:"collectionId,omitempty"`
	Covered      []int64      `json:"covered,omitempty"` // wanted slot numbers (hundredths)
	Score        int          `json:"score"`
	Rejections   []string     `json:"rejections,omitempty"`
	Parse        parse.Result `json:"parse"`
	downloadURL  string
	magnetURL    string
}

// wantedVolumes maps collectionID → set of wanted slot numbers.
func (v *VolumeSearcher) wantedVolumes(ctx context.Context, seriesID int64) (map[int64]map[int64]bool, []store.Collection, error) {
	collections, err := v.store.ListCollections(ctx, seriesID)
	if err != nil {
		return nil, nil, err
	}
	links, err := v.store.SlotLinks(ctx, seriesID)
	if err != nil {
		return nil, nil, err
	}
	pending, err := v.store.PendingSlotKeys(ctx, seriesID)
	if err != nil {
		return nil, nil, err
	}
	wanted := map[int64]map[int64]bool{}
	for _, col := range collections {
		if !col.Monitored || col.Type != "volume" {
			continue
		}
		works, err := v.store.ListWorks(ctx, col.ID)
		if err != nil {
			return nil, nil, err
		}
		for _, slot := range ledger.BuildVolumeSlots(col, works, links, nil, time.Now()) {
			if !slot.Wanted || pending[store.SlotKey("volume", col.ID, slot.Number)] {
				continue
			}
			if wanted[col.ID] == nil {
				wanted[col.ID] = map[int64]bool{}
			}
			wanted[col.ID][slot.Number] = true
		}
	}
	return wanted, collections, nil
}

// FindVolumeCandidates runs the search + funnel for a series.
func (v *VolumeSearcher) FindVolumeCandidates(ctx context.Context, seriesID int64) ([]VolumeCandidate, error) {
	series, err := v.store.GetSeries(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	titles, err := v.store.SeriesTitles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	wanted, collections, err := v.wantedVolumes(ctx, seriesID)
	if err != nil {
		return nil, err
	}

	releases, err := v.prowlarr.Search(ctx, series.Title, nil)
	if err != nil {
		return nil, err
	}

	out := make([]VolumeCandidate, 0, len(releases))
	for _, rel := range releases {
		c := VolumeCandidate{
			GUID: rel.GUID, Title: rel.Title, Indexer: rel.Indexer,
			Size: rel.Size, Seeders: rel.Seeders, InfoHash: rel.InfoHash,
			downloadURL: rel.DownloadURL, magnetURL: rel.MagnetURL,
		}
		c.Parse = parse.ParseAnchored(rel.Title, parse.ChannelTorrent, titles)
		v.funnel(&c, series, collections, wanted)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
}

// funnel applies hard rejections then scoring (decision-engine.md §3–4).
func (v *VolumeSearcher) funnel(c *VolumeCandidate, series *store.Series,
	collections []store.Collection, wanted map[int64]map[int64]bool,
) {
	reject := func(code string) { c.Rejections = append(c.Rejections, code) }

	// R1 content-type gate.
	if c.Parse.ContentType != parse.ContentManga {
		reject("rejected.content_type")
		return
	}
	// Series match: the anchored parse must have actually located one of
	// the series' titles in the release name.
	if !c.Parse.Anchored {
		reject("rejected.series_match")
		return
	}

	// R6 edition routing.
	colID, ok := v.routeEdition(c.Parse, collections, wanted)
	if !ok {
		reject("rejected.edition_routing")
		return
	}
	c.CollectionID = colID

	// R5 slot coverage. A whole-series pack covers everything wanted.
	volumes := c.Parse.Volumes
	if len(volumes) == 0 && c.Parse.Complete {
		for n := range wanted[colID] {
			c.Covered = append(c.Covered, n)
		}
	} else {
		totalCovered := 0
		for _, r := range volumes {
			for n := (int64(r.From) + 99) / 100 * 100; n <= int64(r.To); n += 100 {
				totalCovered++
				if wanted[colID][n] {
					c.Covered = append(c.Covered, n)
				}
			}
		}
		// D12 pack rule: ≥50% of covered slots must be wanted to auto-grab.
		if totalCovered > 0 && len(c.Covered)*2 < totalCovered && len(c.Covered) > 0 {
			reject("rejected.pack_threshold")
		}
	}
	if len(c.Covered) == 0 {
		reject("rejected.no_coverage")
		return
	}
	sort.Slice(c.Covered, func(i, j int) bool { return c.Covered[i] < c.Covered[j] })

	// Size sanity: 20–1500 MB per covered volume [default].
	if c.Size > 0 {
		perVol := c.Size / int64(len(c.Covered))
		if perVol < 20<<20 || perVol > 1500<<20 {
			reject("rejected.size")
		}
	}
	if c.Seeders < 1 {
		reject("rejected.seeders")
	}
	if len(c.Rejections) > 0 {
		return
	}

	// Scoring (decision-engine.md §4 defaults).
	score := len(c.Covered) * 1000
	score += 200 // protocol preference: torrent for volume ledger
	if c.Parse.Quality.Digital {
		score += 100
	}
	switch {
	case c.Seeders > 20:
		score += 50
	case c.Seeders > 5:
		score += 25
	}
	if c.Parse.Confidence < 0.8 {
		score -= 100
	}
	c.Score = score
}

// routeEdition maps edition hints to a monitored collection (R6): hints
// match descriptors; plain releases route to the sole monitored collection.
func (v *VolumeSearcher) routeEdition(p parse.Result, collections []store.Collection,
	wanted map[int64]map[int64]bool,
) (int64, bool) {
	monitored := make([]store.Collection, 0, len(collections))
	for _, col := range collections {
		if wanted[col.ID] != nil {
			monitored = append(monitored, col)
		}
	}
	if len(monitored) == 0 {
		return 0, false
	}
	match := func(substr string) (int64, bool) {
		for _, col := range monitored {
			if strings.Contains(strings.ToLower(col.EditionName), substr) {
				return col.ID, true
			}
		}
		return 0, false
	}
	switch {
	case p.Edition.Omnibus:
		return match("omnibus")
	case p.Edition.BoxSet:
		return match("box")
	case p.Edition.Colored:
		if id, ok := match("color"); ok {
			return id, true
		}
	case p.Edition.Deluxe:
		if id, ok := match("deluxe"); ok {
			return id, true
		}
	}
	// Plain release: prefer a "standard" edition, else the first monitored.
	if id, ok := match("standard"); ok {
		return id, true
	}
	return monitored[0].ID, true
}

// GrabBest greedily grabs eligible candidates by slot coverage: the
// highest-scoring candidate wins its slots, later candidates only count if
// they cover something still missing (decision-engine.md §4 selection).
func (v *VolumeSearcher) GrabBest(ctx context.Context, seriesID int64, candidates []VolumeCandidate) []int64 {
	var queued []int64
	taken := map[int64]bool{}
	for _, c := range candidates {
		if len(c.Rejections) > 0 {
			continue
		}
		news := false
		for _, n := range c.Covered {
			if !taken[n] {
				news = true
				break
			}
		}
		if !news {
			continue
		}
		qid, err := v.GrabVolume(ctx, seriesID, c)
		if err != nil {
			slog.Warn("volume grab failed", "series", seriesID, "release", c.Title, "error", err)
			continue
		}
		queued = append(queued, qid)
		for _, n := range c.Covered {
			taken[n] = true
		}
	}
	return queued
}

// GrabVolume sends a candidate to qBittorrent and records the queue item.
func (v *VolumeSearcher) GrabVolume(ctx context.Context, seriesID int64, c VolumeCandidate) (int64, error) {
	if len(c.Rejections) > 0 {
		return 0, fmt.Errorf("candidate is rejected: %v", c.Rejections)
	}
	link := c.magnetURL
	if link == "" {
		link = c.downloadURL
	}
	if link == "" {
		return 0, fmt.Errorf("candidate has no magnet or download url")
	}

	slots := make([]store.ExpectedSlot, 0, len(c.Covered))
	for _, n := range c.Covered {
		slots = append(slots, store.ExpectedSlot{Ledger: "volume", CollectionID: c.CollectionID, Number: n})
	}
	slotsJSON, _ := json.Marshal(slots)

	externalID := c.InfoHash
	if externalID == "" {
		externalID = c.GUID
	}
	qid, err := v.store.AddQueueItem(ctx, store.QueueItem{
		SeriesID:      seriesID,
		Protocol:      "torrent",
		ExternalID:    strings.ToLower(externalID),
		Title:         c.Title,
		ExpectedSlots: slotsJSON,
	})
	if err != nil {
		return 0, err
	}
	if err := v.qbit.AddTorrent(ctx, link, v.category); err != nil {
		_ = v.store.UpdateQueueItem(ctx, qid, store.QueueFailed, err.Error())
		return 0, err
	}
	return qid, nil
}

// ProcessTorrentQueue imports completed torrents: every archive inside is
// parsed (anchored) and hardlinked into the library for each expected slot
// it covers. Torrent data is never moved (seeding, spec.md §7).
func (v *VolumeSearcher) ProcessTorrentQueue(ctx context.Context) (int, error) {
	items, err := v.store.ListQueueItems(ctx, true)
	if err != nil {
		return 0, err
	}
	var torrentItems []store.QueueItem
	for _, item := range items {
		if item.Protocol == "torrent" {
			torrentItems = append(torrentItems, item)
		}
	}
	if len(torrentItems) == 0 {
		return 0, nil
	}

	torrents, err := v.qbit.List(ctx, v.category)
	if err != nil {
		return 0, err
	}
	byHash := map[string]qbittorrent.Torrent{}
	for _, t := range torrents {
		byHash[strings.ToLower(t.Hash)] = t
	}

	imported := 0
	for _, item := range torrentItems {
		t, found := byHash[item.ExternalID]
		switch {
		case !found:
			continue // not yet visible in qbit
		case !t.Completed():
			if item.Status != store.QueueDownloading {
				_ = v.store.UpdateQueueItem(ctx, item.ID, store.QueueDownloading, "")
			}
			continue
		}
		if err := v.importTorrent(ctx, item, t); err != nil {
			slog.Warn("torrent import failed", "id", item.ID, "error", err)
			_ = v.store.UpdateQueueItem(ctx, item.ID, store.QueueFailed, err.Error())
			continue
		}
		_ = v.store.UpdateQueueItem(ctx, item.ID, store.QueueImported, "")
		imported++
	}
	return imported, nil
}

func (v *VolumeSearcher) importTorrent(ctx context.Context, item store.QueueItem, t qbittorrent.Torrent) error {
	series, err := v.store.GetSeries(ctx, item.SeriesID)
	if err != nil {
		return err
	}
	titles, err := v.store.SeriesTitles(ctx, item.SeriesID)
	if err != nil {
		return err
	}
	var expected []store.ExpectedSlot
	if err := json.Unmarshal(item.ExpectedSlots, &expected); err != nil {
		return err
	}
	expectedSet := map[int64]store.ExpectedSlot{}
	var collectionID int64
	for _, sl := range expected {
		expectedSet[sl.Number] = sl
		collectionID = sl.CollectionID
	}

	editionName, lang := v.collectionDescriptor(ctx, collectionID)

	files, err := v.qbit.Files(ctx, t.Hash)
	if err != nil {
		return err
	}

	seriesDir := series.Path
	if seriesDir == "" {
		seriesDir = filepath.Join(v.libraryRoot, library.Render(v.naming.SeriesFolder, library.NameTokens{
			SeriesTitle: series.Title, SeriesID: series.MangaBakaID, Volume: -1, Chapter: -1,
		}))
		if err := v.store.SetSeriesPath(ctx, item.SeriesID, seriesDir); err != nil {
			return err
		}
	}

	linked := 0
	for _, rel := range files {
		ext := strings.ToLower(filepath.Ext(rel))
		if ext != ".cbz" && ext != ".cbr" {
			continue
		}
		res := parse.ParseAnchored(filepath.Base(rel), parse.ChannelFile, titles)
		for _, r := range res.Volumes {
			for n := (int64(r.From) + 99) / 100 * 100; n <= int64(r.To); n += 100 {
				sl, ok := expectedSet[n]
				if !ok {
					continue
				}
				dest := filepath.Join(seriesDir, library.Render(v.naming.VolumeFile, library.NameTokens{
					SeriesTitle: series.Title, SeriesID: series.MangaBakaID,
					Volume: n, Chapter: -1, Edition: editionName, Lang: lang,
				}))
				src := filepath.Join(t.SavePath, rel)
				if err := library.HardlinkOrCopy(src, dest); err != nil {
					return fmt.Errorf("linking %s: %w", rel, err)
				}
				size := int64(0)
				if fi, err := os.Stat(dest); err == nil {
					size = fi.Size()
				}
				if _, err := v.store.AddFile(ctx, store.File{
					SeriesID:   item.SeriesID,
					Path:       dest,
					Size:       size,
					Provenance: "torrent",
				}, res, []store.SlotLink{{Ledger: sl.Ledger, CollectionID: sl.CollectionID, Number: sl.Number}}); err != nil {
					return err
				}
				linked++
			}
		}
	}
	if linked == 0 {
		return fmt.Errorf("no archive in torrent matched the expected slots")
	}
	return nil
}

func (v *VolumeSearcher) collectionDescriptor(ctx context.Context, collectionID int64) (string, string) {
	col, err := v.store.GetCollection(ctx, collectionID)
	if err != nil {
		return "", ""
	}
	return col.EditionName, col.Language
}
