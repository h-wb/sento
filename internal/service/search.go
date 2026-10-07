package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/extensions"
	"github.com/h-wb/sento/internal/match"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/profile"
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
	profiles *ProfileService
}

func NewSearchService(st *store.Store, registry *provider.Registry) *SearchService {
	return &SearchService{store: st, registry: registry, profiles: NewProfiles(st)}
}

func (s *SearchService) HasProviders() bool { return !s.registry.Empty() }
func (s *SearchService) HasSourceProvider() bool {
	return s.registry.ForProtocol(provider.ProtocolSource) != nil
}

// TargetSlot scopes an interactive search to one collected version: one
// of its books (WorkID), or every wanted book of it (All).
type TargetSlot struct {
	TargetID int64 `json:"targetId"`
	WorkID   int64 `json:"workId,omitempty"`
	All      bool  `json:"all,omitempty"`
	// AllBooks widens an edition-wide search from the wanted books to
	// every book of the version, owned and paused ones included: looking
	// for a better copy, or at what exists before collecting.
	AllBooks bool `json:"allBooks,omitempty"`
}

// ReleaseCandidate is one candidate run through the funnel, for either
// ledger.
type ReleaseCandidate struct {
	// languageSource names where an inferred Parse.Language came from
	// when the release name did not state it.
	languageSource string

	Decision         ReleaseDecision       `json:"decision"`
	ScoreParts       []ScorePart           `json:"scoreParts,omitempty"`
	ParseTrace       []parse.Step          `json:"parseTrace,omitempty"`
	TargetSnapshot   *store.DownloadTarget `json:"targetSnapshot,omitempty"`
	FormatSnapshot   []profile.Format      `json:"formatSnapshot,omitempty"`
	PolicySnapshot   *profile.Profile      `json:"policySnapshot,omitempty"`
	EditionSnapshot  *store.Collection     `json:"editionSnapshot,omitempty"`
	RepresentedUnits int                   `json:"representedUnits"`
	TargetID         int64                 `json:"targetId,omitempty"`
	Coverage         []store.ExpectedSlot  `json:"coverage,omitempty"`
	ProfileID        int64                 `json:"profileId,omitempty"`
	GUID             string                `json:"guid"`
	Provider         string                `json:"provider"`
	Protocol         string                `json:"protocol"`
	Site             string                `json:"site"`
	SiteID           string                `json:"siteId"`
	Title            string                `json:"title"`
	URL              string                `json:"url,omitempty"` // release page on the indexer/source site
	Size             int64                 `json:"size,omitempty"`
	Seeders          int                   `json:"seeders,omitempty"`
	Scanlator        string                `json:"scanlator,omitempty"`
	CollectionID     int64                 `json:"collectionId,omitempty"`
	Score            int                   `json:"score"`
	Formats          []FormatHit           `json:"formats,omitempty"`     // matched custom formats
	FormatScore      int                   `json:"formatScore,omitempty"` // their summed score
	Rejections       []string              `json:"rejections,omitempty"`
	Parse            parse.Result          `json:"parse"`
	cand             provider.Candidate
}

// FormatHit is one custom format the candidate matched, with the score
// the active profile gives it.
type FormatHit struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
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

// effectiveSites resolves the per-series indexer configuration for a
// ledger: entries with overrides applied and ledger usability.
func (s *SearchService) effectiveSites(ctx context.Context, seriesID int64, searchLedger string) ([]siteState, error) {
	states, err := liveSiteStates(ctx, s.store, s.registry)
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
		if o, ok := overrides[st.eff.ID]; ok {
			ov = &o
		}
		st.eff = store.EffectiveIndexer(st.eff, ov)
		use := st.eff.Enabled && st.reachable
		if use {
			switch searchLedger {
			case LedgerVolume:
				use = st.eff.UseVolumes
			case LedgerChapter:
				use = st.eff.UseChapters
			}
		}
		st.usable = use
	}
	return states, nil
}

// maxAutoBindings caps how many entries one site auto-binds: enough to
// cover duplicate listings, small enough that a generic title can't fan a
// search out over a whole result page.
const maxAutoBindings = 5

// resolveBindings auto-resolves one site's series↔manga bindings (spec.md
// §6: auto-bind at ≥0.9 title similarity) under a time budget. EVERY entry
// clearing the threshold is bound — sites like Madokami list the same
// series several times and the funnel picks per slot at search time, with
// better information than a bind-time argmax. Definitive misses persist as
// one "unmatched" sentinel so slow sources aren't re-probed on every
// search (rebind in the UI retries); transient errors are not persisted.
func (s *SearchService) resolveBindings(ctx context.Context, resolver provider.MangaResolver, seriesID int64, titles []string, st siteState) ([]store.Binding, string) {
	resolveCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	type scored struct {
		ref   extensions.Manga
		score float64
	}
	var matches []scored
	bestScore, bestRef := 0.0, extensions.Manga{}
	for i, query := range resolveQueries(titles) {
		refs, err := resolver.ResolveManga(resolveCtx, st.entryID, query)
		if err != nil {
			if i == 0 {
				return nil, fmt.Sprintf("%s: search failed (%v)", st.name, err)
			}
			break // keep what earlier queries found
		}
		seen := map[string]bool{}
		for _, m := range matches {
			seen[m.ref.Key] = true
		}
		for _, ref := range refs {
			score := 0.0
			for _, t := range titles {
				if sc := match.Similarity(match.Normalize(t), match.Normalize(ref.Title)); sc > score {
					score = sc
				}
			}
			if score > bestScore {
				bestScore, bestRef = score, ref
			}
			if score >= match.AutoThreshold && !seen[ref.Key] {
				matches = append(matches, scored{ref, score})
			}
		}
		if len(matches) > 0 {
			break
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].score > matches[j].score })
	if len(matches) > maxAutoBindings {
		matches = matches[:maxAutoBindings]
	}

	if len(matches) == 0 {
		bind := store.Binding{
			SeriesID: seriesID, SourceID: st.entryID,
			SourceMangaKey: bestRef.Key, SourceName: st.name,
			MangaTitle: bestRef.Title, State: "unmatched",
		}
		note := fmt.Sprintf("%s: no results", st.name)
		if bestScore > 0 {
			note = fmt.Sprintf("%s: no confident match (best %q %.2f)", st.name, bestRef.Title, bestScore)
		}
		if _, err := s.store.AddBinding(ctx, bind); err != nil {
			slog.Warn("persisting binding failed", "site", st.name, "error", err)
		}
		return nil, note
	}

	var bound []store.Binding
	var names []string
	for _, m := range matches {
		bind := store.Binding{
			SeriesID: seriesID, SourceID: st.entryID,
			SourceMangaKey: m.ref.Key, SourceName: st.name,
			MangaTitle: m.ref.Title, State: "active", Priority: st.eff.Priority,
		}
		if id, err := s.store.AddBinding(ctx, bind); err != nil {
			slog.Warn("persisting binding failed", "site", st.name, "error", err)
		} else {
			bind.ID = id
		}
		bound = append(bound, bind)
		names = append(names, fmt.Sprintf("%q", m.ref.Title))
	}
	return bound, fmt.Sprintf("auto-bound %s → %s", st.name, strings.Join(names, ", "))
}

// resolveQueries lists the queries to try against a source, in order:
// the primary title, its normalized form, then Latin-script aliases.
// Source search engines differ in fuzziness — Japscan finds "Hunter x
// Hunter" but not "HUNTER×HUNTER" — so a miss retries with a variant.
func resolveQueries(titles []string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(q string) {
		key := strings.ToLower(strings.TrimSpace(q))
		if key == "" || seen[key] || len(out) >= 3 {
			return
		}
		seen[key] = true
		out = append(out, q)
	}
	add(titles[0])
	add(match.Normalize(titles[0]))
	for _, t := range titles[1:] {
		if hasLatin(t) {
			add(t)
		}
	}
	return out
}

// textQuery is the title as trackers index it: release names spell
// "HUNTER×HUNTER" as "Hunter x Hunter", and symbols match nothing.
func textQuery(title string) string {
	if q := match.Normalize(title); q != "" {
		return q
	}
	return title
}

func hasLatin(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
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
	targets        []store.DownloadTarget
	units          map[int64][]store.Work
	targetWanted   map[int64]map[int64]bool
	targetProfiles map[int64]*profile.Profile
	sourceEditions map[int64]int64
	ownedScores    map[int64]map[int64]int
	seriesID       int64
	ledger         string
	target         *TargetSlot
	titles         []string
	sites          []siteState
	allowed        map[string]map[string]bool
	priorities     map[[2]string]int
	searched       []string
	collections    []store.Collection
	blocked        map[string]bool // "protocol/externalKey" bans
	// profiles is the effective release profile per collection id, with
	// the series-level profile under key 0 (nil map = built-ins).
	profiles map[int64]*profile.Profile
	formats  []profile.Format
}

// profileFor resolves the effective profile of a routed candidate.
func (run *searchRun) profileFor(collectionID int64) *profile.Profile {
	if p, ok := run.profiles[collectionID]; ok {
		return p
	}
	return run.profiles[0]
}

func (s *SearchService) prepare(ctx context.Context, seriesID int64, searchLedger string, target *TargetSlot) (*searchRun, error) {
	if s.registry.Empty() {
		return nil, apperr.Unavailablef("no search providers configured")
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
			run.allowed[st.providerID][st.entryID] = true
			run.searched = append(run.searched, st.providerID+"/"+st.name)
		}
		run.priorities[[2]string{st.providerID, st.entryID}] = st.eff.Priority
	}
	if run.blocked, err = s.store.BlockedKeys(ctx); err != nil {
		return nil, err
	}
	if run.profiles, run.formats, err = s.profiles.EffectiveProfiles(ctx, seriesID); err != nil {
		return nil, err
	}
	if err = s.prepareTargets(ctx, run); err != nil {
		return nil, err
	}
	return run, nil
}

// searchOnce runs one search unit (one provider call under an Allowed
// filter) and funnels its results.
func (s *SearchService) searchOnce(ctx context.Context, p provider.Provider, run *searchRun,
	allowed map[string]map[string]bool, siteNames []string, bindings []provider.Binding,
) SearchEvent {
	ev := SearchEvent{Type: "batch", Provider: p.ID(), Sites: siteNames}
	cands, err := p.Search(ctx, provider.SearchRequest{
		SeriesID: run.seriesID, Query: textQuery(run.titles[0]), Titles: run.titles,
		Allowed: allowed, Bindings: bindings,
	})
	if err != nil {
		slog.Warn("search degraded", "provider", p.ID(), "sites", siteNames, "error", err)
		ev.Error = fmt.Sprintf("%s: %v", strings.Join(siteNames, ", "), err) // partial results still count
	}
	s.observeCandidates(ctx, run, cands)
	ev.Candidates, ev.Dropped = s.process(run, cands)
	return ev
}

// streamProvider fans one provider out into one search unit per usable
// indexer entry — a slow indexer (or a slow first-time binding
// resolution) doesn't hold back its siblings. Source entries resolve
// their missing bindings before searching.
func (s *SearchService) streamProvider(ctx context.Context, p provider.Provider, run *searchRun, ch chan<- SearchEvent, wg *sync.WaitGroup) {
	resolver, isResolver := p.(provider.MangaResolver)

	var bySite map[string][]store.Binding
	if isResolver {
		bindings, err := s.store.ListBindings(ctx, run.seriesID)
		if err != nil {
			ch <- SearchEvent{Type: "batch", Provider: p.ID(),
				Error: fmt.Sprintf("%s: %v", p.ID(), err)}
			return
		}
		bySite = map[string][]store.Binding{}
		for _, b := range bindings {
			bySite[b.SourceID] = append(bySite[b.SourceID], b)
		}
	}

	for _, st := range run.sites {
		if !st.usable || st.providerID != p.ID() {
			continue
		}
		wg.Add(1)
		go func(st siteState) {
			defer wg.Done()
			var notes []string
			var active []provider.Binding
			if isResolver {
				siteBindings := bySite[st.entryID]
				if len(siteBindings) == 0 { // never probed: resolve now
					bound, note := s.resolveBindings(ctx, resolver, run.seriesID, run.titles, st)
					notes = append(notes, note)
					siteBindings = bound
				}
				for _, b := range siteBindings {
					if b.State != "active" {
						continue
					}
					active = append(active, provider.Binding{
						BindingID: b.ID, SourceID: b.SourceID,
						SourceMangaKey: b.SourceMangaKey, SourceName: b.SourceName,
					})
				}
			}
			ev := s.searchOnce(ctx, p, run,
				map[string]map[string]bool{p.ID(): {st.entryID: true}},
				[]string{st.providerID + "/" + st.name}, active)
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
	if len(run.targets) > 0 && len(run.targetWanted) == 0 {
		emit(SearchEvent{Type: "sites", Ledger: searchLedger, BindingNotes: []string{"No books can be acquired in this selection. Check the edition’s book statuses, or open a specific book to search for another copy."}})
		emit(SearchEvent{Type: "done", Ledger: searchLedger})
		return nil
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
			result.BindingNotes = append(result.BindingNotes, ev.BindingNotes...)
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
