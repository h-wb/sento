package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/h-wb/sento/internal/extensions"
	"github.com/h-wb/sento/internal/match"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// maxAutoBindings caps how many entries one site auto-binds: enough to
// cover duplicate listings, small enough that a generic title can't fan a
// search out over a whole result page.
const maxAutoBindings = 5

// resolveBindings auto-resolves one site's series↔manga bindings (spec.md
// §6: auto-bind at ≥0.9 title similarity) under a time budget. EVERY entry
// clearing the threshold is bound — sites like Madokami list the same
// series several times and the funnel picks per book at search time, with
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

func hasLatin(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}
