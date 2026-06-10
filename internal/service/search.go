package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
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

// resolveBinding auto-resolves one site's series↔manga binding (spec.md
// §6: auto-bind at ≥0.9 title similarity) under a time budget. Definitive
// misses are persisted as state "unmatched" so slow sources aren't
// re-probed on every search (rebind in the UI retries); transient errors
// are not persisted. Returns the persisted binding, if any.
func (s *SearchService) resolveBinding(ctx context.Context, resolver provider.MangaResolver, seriesID int64, titles []string, st siteState) (*store.Binding, string) {
	resolveCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	refs, err := resolver.ResolveManga(resolveCtx, st.site.ID, titles[0])
	if err != nil {
		return nil, fmt.Sprintf("%s: search failed (%v)", st.site.Name, err)
	}
	bestScore, bestRef := 0.0, provider.MangaRef{}
	for _, ref := range refs {
		for _, t := range titles {
			if sc := match.Similarity(match.Normalize(t), match.Normalize(ref.Title)); sc > bestScore {
				bestScore, bestRef = sc, ref
			}
		}
	}
	bind := store.Binding{
		SeriesID: seriesID, SourceID: st.site.ID,
		SourceMangaID: bestRef.ID, SourceName: st.site.Name,
		State: "unmatched",
	}
	var note string
	switch {
	case bestScore >= match.AutoThreshold:
		bind.State, bind.Priority = "active", st.setting.Priority
		note = fmt.Sprintf("auto-bound %s → %q", st.site.Name, bestRef.Title)
	case bestScore > 0:
		note = fmt.Sprintf("%s: no confident match (best %q %.2f)", st.site.Name, bestRef.Title, bestScore)
	default:
		note = fmt.Sprintf("%s: no results", st.site.Name)
	}
	if id, err := s.store.AddBinding(ctx, bind); err != nil {
		slog.Warn("persisting binding failed", "site", st.site.Name, "error", err)
	} else {
		bind.ID = id
	}
	return &bind, note
}

// SearchEvent is one frame of a streaming search: "sites" announces what
// will be searched, "batch" delivers processed candidates as soon as one
// search unit finishes (a single extension source, or one provider's
// indexer sweep), "done" closes the stream.
type SearchEvent struct {
	Type         string             `json:"type" tstype:"'sites' | 'batch' | 'done' | 'error'"`
	Ledger       string             `json:"ledger,omitempty"`
	Searched     []string           `json:"searched,omitempty"`
	Provider     string             `json:"provider,omitempty"`
	Sites        []string           `json:"sites,omitempty"` // searched entries this batch covers
	Candidates   []ReleaseCandidate `json:"candidates,omitempty"`
	Dropped      int                `json:"dropped,omitempty"`
	Error        string             `json:"error,omitempty"`
	BindingNotes []string           `json:"bindingNotes,omitempty"`
}

// searchRun is the resolved context of one search, shared by all provider
// goroutines (read-only after prepare).
type searchRun struct {
	seriesID    int64
	ledger      string
	target      *TargetSlot
	titles      []string
	sites       []siteState
	allowed     map[string]map[string]bool
	priorities  map[[2]string]int
	searched    []string
	wanted      map[int64]map[int64]bool
	collections []store.Collection
	chapterLang string
}

func (s *SearchService) prepare(ctx context.Context, seriesID int64, searchLedger string, target *TargetSlot) (*searchRun, error) {
	if s.registry.Empty() {
		return nil, fmt.Errorf("no search providers configured")
	}
	titles, err := s.store.SeriesTitles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	sites, err := s.effectiveSites(ctx, seriesID, searchLedger)
	if err != nil {
		return nil, err
	}
	run := &searchRun{
		seriesID: seriesID, ledger: searchLedger, target: target,
		titles: titles, sites: sites,
		allowed: map[string]map[string]bool{}, priorities: map[[2]string]int{},
	}
	for _, st := range sites {
		if run.allowed[st.providerID] == nil {
			run.allowed[st.providerID] = map[string]bool{}
		}
		if st.usable {
			run.allowed[st.providerID][st.site.ID] = true
			run.searched = append(run.searched, st.providerID+"/"+st.site.Name)
		}
		run.priorities[[2]string{st.providerID, st.site.ID}] = st.setting.Priority
	}
	run.wanted, run.collections, run.chapterLang, err = s.wantedSlots(ctx, seriesID, searchLedger, target)
	if err != nil {
		return nil, err
	}
	return run, nil
}

// process parses, funnels and scores one provider's raw candidates.
func (s *SearchService) process(run *searchRun, cands []provider.Candidate) (out []ReleaseCandidate, dropped int) {
	for _, cand := range cands {
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
		rc.Parse = parse.ParseAnchored(cand.Title, channel, run.titles)

		ranges := s.coverageRanges(run.ledger, &rc)
		// Keep number-less candidates only as potential whole-series packs
		// (volume ledger).
		isPack := run.ledger == LedgerVolume && rc.Parse.Complete
		if len(ranges) == 0 && !isPack {
			dropped++
			continue
		}
		s.funnel(run.ledger, &rc, ranges, run.collections, run.wanted, run.chapterLang, run.target)
		rc.Score -= run.priorities[[2]string{rc.Provider, rc.SiteID}]
		out = append(out, rc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, dropped
}

// searchOnce runs one search unit (one provider call under an Allowed
// filter) and funnels its results.
func (s *SearchService) searchOnce(ctx context.Context, p provider.Provider, run *searchRun,
	allowed map[string]map[string]bool, siteNames []string, bindings []provider.Binding,
) SearchEvent {
	ev := SearchEvent{Type: "batch", Provider: p.ID(), Sites: siteNames}
	cands, err := p.Search(ctx, provider.SearchRequest{
		SeriesID: run.seriesID, Query: run.titles[0], Titles: run.titles,
		Allowed: allowed, Bindings: bindings,
	})
	if err != nil {
		slog.Warn("search degraded", "provider", p.ID(), "sites", siteNames, "error", err)
		ev.Error = fmt.Sprintf("%s: %v", strings.Join(siteNames, ", "), err) // partial results still count
	}
	ev.Candidates, ev.Dropped = s.process(run, cands)
	return ev
}

// streamProvider fans one provider out into search units. Extension
// providers get one unit per site, each resolving its own missing binding
// before searching — a sluggish source (or a slow first-time resolution)
// doesn't hold back its siblings. Other providers are one unit: their
// fan-out is server-side (Prowlarr).
func (s *SearchService) streamProvider(ctx context.Context, p provider.Provider, run *searchRun, ch chan<- SearchEvent, wg *sync.WaitGroup) {
	var mySites []string
	for _, st := range run.sites {
		if st.usable && st.providerID == p.ID() {
			mySites = append(mySites, st.providerID+"/"+st.site.Name)
		}
	}

	resolver, isResolver := p.(provider.MangaResolver)
	if !isResolver {
		ch <- s.searchOnce(ctx, p, run, run.allowed, mySites, nil)
		return
	}

	bindings, err := s.store.ListBindings(ctx, run.seriesID)
	if err != nil {
		ch <- SearchEvent{Type: "batch", Provider: p.ID(), Sites: mySites,
			Error: fmt.Sprintf("%s: %v", p.ID(), err)}
		return
	}
	bySite := map[string][]store.Binding{}
	for _, b := range bindings {
		bySite[b.SourceID] = append(bySite[b.SourceID], b)
	}

	for _, st := range run.sites {
		if !st.usable || st.providerID != p.ID() {
			continue
		}
		wg.Add(1)
		go func(st siteState) {
			defer wg.Done()
			siteBindings := bySite[st.site.ID]
			var notes []string
			if len(siteBindings) == 0 { // never probed: resolve now
				bind, note := s.resolveBinding(ctx, resolver, run.seriesID, run.titles, st)
				notes = append(notes, note)
				if bind != nil {
					siteBindings = []store.Binding{*bind}
				}
			}
			var active []provider.Binding
			for _, b := range siteBindings {
				if b.State != "active" {
					continue
				}
				active = append(active, provider.Binding{
					BindingID: b.ID, SourceID: b.SourceID,
					SourceMangaID: b.SourceMangaID, SourceName: b.SourceName,
				})
			}
			ev := s.searchOnce(ctx, p, run,
				map[string]map[string]bool{p.ID(): {st.site.ID: true}},
				[]string{st.providerID + "/" + st.site.Name}, active)
			ev.BindingNotes = notes
			ch <- ev
		}(st)
	}
}

// FindCandidatesStream runs the unified search for one ledger, emitting a
// batch per search unit as it completes. emit is always called from this
// goroutine. With a target, only that slot counts as wanted (interactive
// search, any slot state).
func (s *SearchService) FindCandidatesStream(ctx context.Context, seriesID int64, searchLedger string, target *TargetSlot, emit func(SearchEvent)) error {
	run, err := s.prepare(ctx, seriesID, searchLedger, target)
	if err != nil {
		return err
	}
	emit(SearchEvent{Type: "sites", Ledger: searchLedger, Searched: run.searched})

	ch := make(chan SearchEvent)
	var wg sync.WaitGroup
	for _, p := range s.registry.All() {
		wg.Add(1)
		go func(p provider.Provider) {
			defer wg.Done()
			s.streamProvider(ctx, p, run, ch, &wg)
		}(p)
	}
	go func() { wg.Wait(); close(ch) }()
	for ev := range ch {
		emit(ev)
	}
	emit(SearchEvent{Type: "done", Ledger: searchLedger})
	return nil
}

// FindCandidates runs the unified search and collects the whole stream
// into one result (scheduler, auto-grab and grab-by-GUID paths).
func (s *SearchService) FindCandidates(ctx context.Context, seriesID int64, searchLedger string, target *TargetSlot) (*SearchResult, error) {
	result := &SearchResult{Ledger: searchLedger, Candidates: []ReleaseCandidate{}}
	err := s.FindCandidatesStream(ctx, seriesID, searchLedger, target, func(ev SearchEvent) {
		switch ev.Type {
		case "sites":
			result.Searched = ev.Searched
		case "batch":
			result.Candidates = append(result.Candidates, ev.Candidates...)
			result.Dropped += ev.Dropped
			result.BindingNotes = append(result.BindingNotes, ev.BindingNotes...)
			if ev.Error != "" {
				result.ProviderErrors = append(result.ProviderErrors, ev.Error)
			}
		}
	})
	if err != nil {
		return nil, err
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

	// R2 language gate (untagged passes). Soft: record and keep going so
	// Covered is still computed — a force-grab needs the slot list.
	if c.Parse.Language != "" && c.Parse.Language != "multi" && wantLang != "" &&
		baseLang(c.Parse.Language) != baseLang(wantLang) {
		reject("rejected.language")
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
		ExpectedSlots: store.JSONText(slotsJSON),
	})
}
