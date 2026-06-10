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
	"github.com/h-wb/sento/internal/match"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// Ledger names the two slot ledgers a search can target.
const (
	LedgerVolume  = "volume"
	LedgerChapter = "chapter"
)

// SearchService is the unified release search: same pipeline for volumes
// and chapters (providers → parse → funnel → score → grab); only coverage
// matching, routing and the protocol preference differ per ledger.
type SearchService struct {
	store    *store.Store
	registry *provider.Registry
}

func NewSearchService(st *store.Store, registry *provider.Registry) *SearchService {
	return &SearchService{store: st, registry: registry}
}

func (s *SearchService) HasProviders() bool      { return !s.registry.Empty() }
func (s *SearchService) HasSourceProvider() bool { return s.registry.Get("suwayomi") != nil }

// TargetSlot restricts an interactive search to one slot (collectionID 0 =
// chapter ledger).
type TargetSlot struct {
	CollectionID int64 `json:"collectionId"`
	Number       int64 `json:"number"` // hundredths
}

// ReleaseCandidate is one candidate run through the funnel, for either
// ledger.
type ReleaseCandidate struct {
	GUID         string       `json:"guid"`
	Provider     string       `json:"provider"`
	Protocol     string       `json:"protocol"`
	Site         string       `json:"site"`
	SiteID       string       `json:"siteId"`
	Title        string       `json:"title"`
	Size         int64        `json:"size,omitempty"`
	Seeders      int          `json:"seeders,omitempty"`
	ChapterID    int64        `json:"chapterId,omitempty"`
	Scanlator    string       `json:"scanlator,omitempty"`
	CollectionID int64        `json:"collectionId,omitempty"`
	Covered      []int64      `json:"covered,omitempty"`
	Score        int          `json:"score"`
	Rejections   []string     `json:"rejections,omitempty"`
	Parse        parse.Result `json:"parse"`
	cand         provider.Candidate
}

// SearchResult carries candidates plus diagnostics so empty results can
// explain themselves.
type SearchResult struct {
	Ledger         string             `json:"ledger"`
	Candidates     []ReleaseCandidate `json:"candidates"`
	Searched       []string           `json:"searched"`
	Dropped        int                `json:"dropped"` // entries for the other ledger
	ProviderErrors []string           `json:"providerErrors,omitempty"`
	BindingNotes   []string           `json:"bindingNotes,omitempty"`
}

type siteState struct {
	providerID string
	site       provider.Site
	setting    store.SiteSetting
	usable     bool
}

// effectiveSites resolves the per-series site configuration for a ledger.
func (s *SearchService) effectiveSites(ctx context.Context, seriesID int64, searchLedger string) ([]siteState, error) {
	states, err := (&SiteService{store: s.store, registry: s.registry}).liveSites(ctx)
	if err != nil {
		return nil, err
	}
	overrides, err := s.store.ListSiteOverrides(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	for i := range states {
		st := &states[i]
		var ov *store.SiteOverride
		if o, ok := overrides[[2]string{st.providerID, st.site.ID}]; ok {
			ov = &o
		}
		st.setting = store.EffectiveSiteSetting(st.setting, ov)
		use := st.setting.Enabled && st.site.Enabled
		if use {
			switch searchLedger {
			case LedgerVolume:
				use = st.setting.UseVolumes
			case LedgerChapter:
				use = st.setting.UseChapters
			}
		}
		st.usable = use
	}
	return states, nil
}

// ensureBindings auto-resolves missing series↔manga bindings for usable
// extension sites (spec.md §6: auto-bind at ≥0.9 title similarity).
// Resolution runs in parallel with a per-site time budget; definitive
// misses are persisted as state "unmatched" so slow sources aren't
// re-probed on every search (rebind in the UI retries).
func (s *SearchService) ensureBindings(ctx context.Context, seriesID int64, titles []string, sites []siteState, notes *[]string) {
	resolver, ok := s.registry.Get("suwayomi").(provider.MangaResolver)
	if !ok {
		return
	}
	bindings, err := s.store.ListBindings(ctx, seriesID)
	if err != nil {
		return
	}
	bound := map[string]bool{}
	for _, b := range bindings {
		bound[b.SourceID] = true // any state: unmatched rows stop re-probing
	}

	var pendingSites []siteState
	for _, st := range sites {
		if st.usable && st.providerID == "suwayomi" && !bound[st.site.ID] {
			pendingSites = append(pendingSites, st)
		}
	}
	if len(pendingSites) == 0 {
		return
	}

	type outcome struct {
		st    siteState
		note  string
		bind  *store.Binding
		fatal bool // transient error: do not persist
	}
	ch := make(chan outcome, len(pendingSites))
	for _, st := range pendingSites {
		go func(st siteState) {
			resolveCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			refs, err := resolver.ResolveManga(resolveCtx, st.site.ID, titles[0])
			if err != nil {
				ch <- outcome{st: st, note: fmt.Sprintf("%s: search failed (%v)", st.site.Name, err), fatal: true}
				return
			}
			bestScore, bestRef := 0.0, provider.MangaRef{}
			for _, ref := range refs {
				for _, t := range titles {
					if sc := match.Similarity(match.Normalize(t), match.Normalize(ref.Title)); sc > bestScore {
						bestScore, bestRef = sc, ref
					}
				}
			}
			switch {
			case bestScore >= match.AutoThreshold:
				ch <- outcome{st: st, note: fmt.Sprintf("auto-bound %s → %q", st.site.Name, bestRef.Title),
					bind: &store.Binding{
						SeriesID: seriesID, SourceID: st.site.ID,
						SourceMangaID: bestRef.ID, SourceName: st.site.Name,
						State: "active", Priority: st.setting.Priority,
					}}
			case bestScore > 0:
				ch <- outcome{st: st, note: fmt.Sprintf("%s: no confident match (best %q %.2f)", st.site.Name, bestRef.Title, bestScore),
					bind: &store.Binding{
						SeriesID: seriesID, SourceID: st.site.ID,
						SourceMangaID: bestRef.ID, SourceName: st.site.Name,
						State: "unmatched",
					}}
			default:
				ch <- outcome{st: st, note: fmt.Sprintf("%s: no results", st.site.Name),
					bind: &store.Binding{
						SeriesID: seriesID, SourceID: st.site.ID,
						SourceMangaID: 0, SourceName: st.site.Name,
						State: "unmatched",
					}}
			}
		}(st)
	}
	for range pendingSites {
		o := <-ch
		*notes = append(*notes, o.note)
		if o.bind != nil {
			if _, err := s.store.AddBinding(ctx, *o.bind); err != nil {
				slog.Warn("persisting binding failed", "site", o.st.site.Name, "error", err)
			}
		}
	}
}

// FindCandidates runs the unified search for one ledger. With a target,
// only that slot counts as wanted (interactive search, any slot state).
func (s *SearchService) FindCandidates(ctx context.Context, seriesID int64, searchLedger string, target *TargetSlot) (*SearchResult, error) {
	if s.registry.Empty() {
		return nil, fmt.Errorf("no search providers configured")
	}
	titles, err := s.store.SeriesTitles(ctx, seriesID)
	if err != nil {
		return nil, err
	}

	result := &SearchResult{Ledger: searchLedger, Candidates: []ReleaseCandidate{}}

	sites, err := s.effectiveSites(ctx, seriesID, searchLedger)
	if err != nil {
		return nil, err
	}
	allowed := map[string]map[string]bool{}
	priorities := map[[2]string]int{}
	for _, st := range sites {
		if allowed[st.providerID] == nil {
			allowed[st.providerID] = map[string]bool{}
		}
		if st.usable {
			allowed[st.providerID][st.site.ID] = true
			result.Searched = append(result.Searched, st.providerID+"/"+st.site.Name)
		}
		priorities[[2]string{st.providerID, st.site.ID}] = st.setting.Priority
	}

	s.ensureBindings(ctx, seriesID, titles, sites, &result.BindingNotes)

	bindings, err := s.store.ListBindings(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	req := provider.SearchRequest{
		SeriesID: seriesID, Query: titles[0], Titles: titles, Allowed: allowed,
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

	wanted, collections, chapterLang, err := s.wantedSlots(ctx, seriesID, searchLedger, target)
	if err != nil {
		return nil, err
	}

	candidates, errs := s.registry.SearchAll(ctx, req)
	for _, e := range errs {
		slog.Warn("search provider degraded", "error", e)
		result.ProviderErrors = append(result.ProviderErrors, e.Error())
	}

	for _, cand := range candidates {
		rc := ReleaseCandidate{
			GUID: cand.GUID(), Provider: cand.Provider, Protocol: cand.Protocol,
			Site: cand.Site, SiteID: cand.SiteID, Title: cand.Title, cand: cand,
		}
		channel := parse.ChannelTorrent
		if cand.Protocol == provider.ProtocolSource {
			channel = parse.ChannelSource
		}
		if cand.Torrent != nil {
			rc.Size = cand.Torrent.Size
			rc.Seeders = cand.Torrent.Seeders
		}
		if cand.Source != nil {
			rc.ChapterID = cand.Source.ChapterID
			rc.Scanlator = cand.Source.Scanlator
		}
		rc.Parse = parse.ParseAnchored(cand.Title, channel, titles)

		ranges := s.coverageRanges(searchLedger, &rc)
		// Keep number-less candidates only as potential whole-series packs
		// (volume ledger).
		isPack := searchLedger == LedgerVolume && rc.Parse.Complete
		if len(ranges) == 0 && !isPack {
			result.Dropped++
			continue
		}
		s.funnel(searchLedger, &rc, ranges, collections, wanted, chapterLang, target)
		rc.Score -= priorities[[2]string{rc.Provider, rc.SiteID}]
		result.Candidates = append(result.Candidates, rc)
	}

	sort.Slice(result.Candidates, func(i, j int) bool {
		return result.Candidates[i].Score > result.Candidates[j].Score
	})
	return result, nil
}

// coverageRanges extracts the ledger-relevant number ranges of a candidate.
func (s *SearchService) coverageRanges(searchLedger string, rc *ReleaseCandidate) []parse.Range {
	if searchLedger == LedgerVolume {
		return rc.Parse.Volumes
	}
	ranges := rc.Parse.Chapters
	// Source corroboration for terse names ("1184"): the extension's own
	// recognition.
	if len(ranges) == 0 && rc.cand.Source != nil && rc.cand.Source.ChapterNumber > 0 {
		n := parse.Num(rc.cand.Source.ChapterNumber*100 + 0.5)
		ranges = []parse.Range{{From: n, To: n}}
	}
	return ranges
}

// wantedSlots computes the wanted slot sets for a ledger:
// collectionID → slot set (chapter ledger uses collectionID 0).
func (s *SearchService) wantedSlots(ctx context.Context, seriesID int64, searchLedger string, target *TargetSlot) (map[int64]map[int64]bool, []store.Collection, string, error) {
	collections, err := s.store.ListCollections(ctx, seriesID)
	if err != nil {
		return nil, nil, "", err
	}
	chapterLang := ""
	if cl, err := s.store.GetChapterLedger(ctx, seriesID); err == nil {
		chapterLang = cl.Language
	}
	if target != nil {
		return map[int64]map[int64]bool{target.CollectionID: {target.Number: true}}, collections, chapterLang, nil
	}

	links, err := s.store.SlotLinks(ctx, seriesID)
	if err != nil {
		return nil, nil, "", err
	}
	pending, err := s.store.PendingSlotKeys(ctx, seriesID)
	if err != nil {
		return nil, nil, "", err
	}
	wanted := map[int64]map[int64]bool{}

	if searchLedger == LedgerVolume {
		for _, col := range collections {
			if !col.Monitored || col.Type != "volume" {
				continue
			}
			works, err := s.store.ListWorks(ctx, col.ID)
			if err != nil {
				return nil, nil, "", err
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
		return wanted, collections, chapterLang, nil
	}

	series, err := s.store.GetSeries(ctx, seriesID)
	if err != nil {
		return nil, nil, "", err
	}
	cl, err := s.store.GetChapterLedger(ctx, seriesID)
	if err != nil {
		return nil, nil, "", err
	}
	for _, slot := range ledger.BuildChapterSlots(*cl, series.TotalChapters, links, nil) {
		if !slot.Wanted || pending[store.SlotKey("chapter", 0, slot.Number)] {
			continue
		}
		if wanted[0] == nil {
			wanted[0] = map[int64]bool{}
		}
		wanted[0][slot.Number] = true
	}
	return wanted, collections, chapterLang, nil
}

// funnel applies hard rejections then scoring (decision-engine.md §3–4),
// parameterized by ledger.
func (s *SearchService) funnel(searchLedger string, c *ReleaseCandidate, ranges []parse.Range,
	collections []store.Collection, wanted map[int64]map[int64]bool, chapterLang string, target *TargetSlot,
) {
	reject := func(code string) { c.Rejections = append(c.Rejections, code) }

	// R1 content-type gate.
	if c.Parse.ContentType != parse.ContentManga {
		reject("rejected.content_type")
		return
	}
	// Series match (source candidates are binding-scoped; anchor waived).
	if c.Protocol != provider.ProtocolSource {
		if !c.Parse.Anchored || c.Parse.TitleLeftover != "" {
			reject("rejected.series_match")
			return
		}
	}

	// Routing: volumes route to a collection (R6); chapters use ledger 0.
	colID := int64(0)
	wantLang := chapterLang
	if searchLedger == LedgerVolume {
		if target != nil {
			colID = target.CollectionID
		} else {
			var ok bool
			colID, ok = routeEdition(c.Parse, collections, wanted)
			if !ok {
				reject("rejected.edition_routing")
				return
			}
		}
		for _, col := range collections {
			if col.ID == colID {
				wantLang = col.Language
			}
		}
	}
	c.CollectionID = colID

	// R2 language gate (untagged passes; soft, force-grabbable).
	if c.Parse.Language != "" && c.Parse.Language != "multi" && wantLang != "" &&
		baseLang(c.Parse.Language) != baseLang(wantLang) {
		reject("rejected.language")
		return
	}

	// R5 slot coverage. A whole-series pack covers everything wanted
	// (volume ledger only).
	if len(ranges) == 0 && searchLedger == LedgerVolume && c.Parse.Complete {
		for n := range wanted[colID] {
			c.Covered = append(c.Covered, n)
		}
	} else {
		totalCovered := 0
		for _, r := range ranges {
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

	// Torrent-only health checks.
	if c.Protocol == provider.ProtocolTorrent {
		if c.Size > 0 {
			perUnit := c.Size / int64(len(c.Covered))
			minSize, maxSize := int64(20<<20), int64(1500<<20)
			if searchLedger == LedgerChapter {
				minSize, maxSize = 2<<20, 200<<20
			}
			if perUnit < minSize || perUnit > maxSize {
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

	// Scoring (decision-engine.md §4): protocol preference flips per ledger.
	score := len(c.Covered) * 1000
	switch {
	case searchLedger == LedgerVolume && c.Protocol == provider.ProtocolTorrent:
		score += 200
	case searchLedger == LedgerChapter && c.Protocol == provider.ProtocolSource:
		score += 200
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

func baseLang(lang string) string {
	base, _, _ := strings.Cut(lang, "-")
	return strings.ToLower(base)
}

// routeEdition maps edition hints to a monitored collection (R6).
func routeEdition(p parse.Result, collections []store.Collection,
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
	matchName := func(substr string) (int64, bool) {
		for _, col := range monitored {
			if strings.Contains(strings.ToLower(col.EditionName), substr) {
				return col.ID, true
			}
		}
		return 0, false
	}
	switch {
	case p.Edition.Omnibus:
		return matchName("omnibus")
	case p.Edition.BoxSet:
		return matchName("box")
	case p.Edition.Colored:
		if id, ok := matchName("color"); ok {
			return id, true
		}
	case p.Edition.Deluxe:
		if id, ok := matchName("deluxe"); ok {
			return id, true
		}
	}
	if id, ok := matchName("standard"); ok {
		return id, true
	}
	return monitored[0].ID, true
}

// Rejections that survive a force-grab (decision-engine.md §7).
var hardRejections = map[string]bool{
	"rejected.content_type":    true,
	"rejected.series_match":    true,
	"rejected.edition_routing": true,
	"rejected.no_coverage":     true,
}

// GrabBest greedily grabs eligible candidates by slot coverage.
func (s *SearchService) GrabBest(ctx context.Context, seriesID int64, searchLedger string, candidates []ReleaseCandidate) []int64 {
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
		qid, err := s.Grab(ctx, seriesID, searchLedger, c, false)
		if err != nil {
			slog.Warn("grab failed", "series", seriesID, "release", c.Title, "error", err)
			continue
		}
		queued = append(queued, qid)
		for _, n := range c.Covered {
			taken[n] = true
		}
	}
	return queued
}

// Grab routes a candidate to its owning provider and records the queue
// item. force bypasses soft rejections; hard gates always hold.
func (s *SearchService) Grab(ctx context.Context, seriesID int64, searchLedger string, c ReleaseCandidate, force bool) (int64, error) {
	for _, r := range c.Rejections {
		if hardRejections[r] {
			return 0, fmt.Errorf("candidate is rejected: %s", r)
		}
		if !force {
			return 0, fmt.Errorf("candidate is rejected: %s (use force to override)", r)
		}
	}
	prov := s.registry.Get(c.Provider)
	if prov == nil {
		return 0, fmt.Errorf("provider %q is not configured", c.Provider)
	}

	slots := make([]store.ExpectedSlot, 0, len(c.Covered))
	for _, n := range c.Covered {
		slots = append(slots, store.ExpectedSlot{
			Ledger: searchLedger, CollectionID: c.CollectionID, Number: n,
		})
	}
	slotsJSON, _ := json.Marshal(slots)

	externalID, err := prov.Grab(ctx, c.cand)
	if err != nil {
		return 0, err
	}
	return s.store.AddQueueItem(ctx, store.QueueItem{
		SeriesID:      seriesID,
		Protocol:      c.Protocol,
		ExternalID:    externalID,
		Title:         c.Title,
		ExpectedSlots: slotsJSON,
	})
}
