package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/h-wb/sento/internal/ledger"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// VolumeSearcher runs the volume path: provider candidates (every site,
// any protocol — decision-engine.md §2) → parse → reject/score funnel →
// grab via the owning provider.
type VolumeSearcher struct {
	store    *store.Store
	registry *provider.Registry
}

func NewVolumeSearcher(st *store.Store, registry *provider.Registry) *VolumeSearcher {
	return &VolumeSearcher{store: st, registry: registry}
}

// VolumeCandidate is one candidate run through the funnel, with the
// decision trace (interactive search shows everything).
type VolumeCandidate struct {
	GUID         string       `json:"guid"`
	Provider     string       `json:"provider"`
	Protocol     string       `json:"protocol"`
	Site         string       `json:"site"`
	Title        string       `json:"title"`
	Size         int64        `json:"size,omitempty"`
	Seeders      int          `json:"seeders,omitempty"`
	ChapterID    int64        `json:"chapterId,omitempty"`
	CollectionID int64        `json:"collectionId,omitempty"`
	Covered      []int64      `json:"covered,omitempty"`
	Score        int          `json:"score"`
	Rejections   []string     `json:"rejections,omitempty"`
	Parse        parse.Result `json:"parse"`
	cand         provider.Candidate
}

// HasProviders reports whether any search provider is configured.
func (v *VolumeSearcher) HasProviders() bool { return !v.registry.Empty() }

// TargetSlot restricts an interactive search to one volume slot.
type TargetSlot struct {
	CollectionID int64 `json:"collectionId"`
	Number       int64 `json:"number"` // hundredths
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

// FindVolumeCandidates runs search + funnel. With a target, only that slot
// counts as wanted and edition routing is pinned to its collection.
func (v *VolumeSearcher) FindVolumeCandidates(ctx context.Context, seriesID int64, target *TargetSlot) ([]VolumeCandidate, error) {
	if v.registry.Empty() {
		return nil, fmt.Errorf("no search providers configured")
	}
	grabber := &Grabber{store: v.store, registry: v.registry}
	req, err := grabber.searchRequest(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	wanted, collections, err := v.wantedVolumes(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	if target != nil {
		wanted = map[int64]map[int64]bool{target.CollectionID: {target.Number: true}}
	}

	candidates, errs := v.registry.SearchAll(ctx, req)
	for _, e := range errs {
		slog.Warn("volume search provider degraded", "error", e)
	}

	out := make([]VolumeCandidate, 0, len(candidates))
	for _, cand := range candidates {
		vc := VolumeCandidate{
			GUID: cand.GUID(), Provider: cand.Provider,
			Protocol: cand.Protocol, Site: cand.Site, Title: cand.Title,
			cand: cand,
		}
		channel := parse.ChannelTorrent
		if cand.Protocol == provider.ProtocolSource {
			channel = parse.ChannelSource
		}
		if cand.Torrent != nil {
			vc.Size = cand.Torrent.Size
			vc.Seeders = cand.Torrent.Seeders
		}
		if cand.Source != nil {
			vc.ChapterID = cand.Source.ChapterID
		}
		vc.Parse = parse.ParseAnchored(cand.Title, channel, req.Titles)
		// Source entries that don't parse as volumes belong to the chapter
		// ledger; drop them from the volume funnel entirely.
		if cand.Protocol == provider.ProtocolSource && len(vc.Parse.Volumes) == 0 {
			continue
		}
		v.funnel(&vc, collections, wanted, target)
		out = append(out, vc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
}

// funnel applies hard rejections then scoring (decision-engine.md §3–4).
func (v *VolumeSearcher) funnel(c *VolumeCandidate,
	collections []store.Collection, wanted map[int64]map[int64]bool, target *TargetSlot,
) {
	reject := func(code string) { c.Rejections = append(c.Rejections, code) }

	// R1 content-type gate.
	if c.Parse.ContentType != parse.ContentManga {
		reject("rejected.content_type")
		return
	}
	// Series match: the anchored parse must have located one of the
	// series' titles — except source candidates, whose binding already
	// scopes them to the series.
	if !c.Parse.Anchored && c.Protocol != provider.ProtocolSource {
		reject("rejected.series_match")
		return
	}

	// R6 edition routing (pinned to the target's collection when interactive).
	colID := int64(0)
	if target != nil {
		colID = target.CollectionID
	} else {
		var ok bool
		colID, ok = v.routeEdition(c.Parse, collections, wanted)
		if !ok {
			reject("rejected.edition_routing")
			return
		}
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

	// Torrent-only health checks: size sanity (20–1500 MB per covered
	// volume [default]) and seeders. Source candidates have neither.
	if c.Protocol == provider.ProtocolTorrent {
		if c.Size > 0 {
			perVol := c.Size / int64(len(c.Covered))
			if perVol < 20<<20 || perVol > 1500<<20 {
				reject("rejected.size")
			}
		}
		if c.Seeders < 1 {
			reject("rejected.seeders")
		}
	}
	if len(c.Rejections) > 0 {
		return
	}

	// Scoring (decision-engine.md §4 defaults).
	score := len(c.Covered) * 1000
	if c.Protocol == provider.ProtocolTorrent {
		score += 200 // protocol preference: torrent for volume ledger
	}
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

// routeEdition maps edition hints to a monitored collection (R6).
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

// Rejections that survive a force-grab (decision-engine.md §7).
var hardRejections = map[string]bool{
	"rejected.content_type":    true,
	"rejected.series_match":    true,
	"rejected.edition_routing": true,
	"rejected.no_coverage":     true, // nothing to link a grab to
}

// GrabBest greedily grabs eligible candidates by slot coverage
// (decision-engine.md §4 selection).
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
		qid, err := v.GrabVolume(ctx, seriesID, c, false)
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

// GrabVolume routes a candidate to its owning provider and records the
// queue item. force bypasses soft rejections; hard gates always hold.
func (v *VolumeSearcher) GrabVolume(ctx context.Context, seriesID int64, c VolumeCandidate, force bool) (int64, error) {
	for _, r := range c.Rejections {
		if hardRejections[r] {
			return 0, fmt.Errorf("candidate is rejected: %s", r)
		}
		if !force {
			return 0, fmt.Errorf("candidate is rejected: %s (use force to override)", r)
		}
	}
	prov := v.registry.Get(c.Provider)
	if prov == nil {
		return 0, fmt.Errorf("provider %q is not configured", c.Provider)
	}

	slots := make([]store.ExpectedSlot, 0, len(c.Covered))
	for _, n := range c.Covered {
		slots = append(slots, store.ExpectedSlot{Ledger: "volume", CollectionID: c.CollectionID, Number: n})
	}
	slotsJSON, _ := json.Marshal(slots)

	externalID, err := prov.Grab(ctx, c.cand)
	if err != nil {
		return 0, err
	}
	return v.store.AddQueueItem(ctx, store.QueueItem{
		SeriesID:      seriesID,
		Protocol:      c.Protocol,
		ExternalID:    externalID,
		Title:         c.Title,
		ExpectedSlots: slotsJSON,
	})
}
