package service

import (
	"context"
	"fmt"
	"time"

	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// Root matches and their nested listings are ordinary bindings. Walk
// active entries on every search so existing matches acquire new folders
// too; disabled entries stay disabled and are never traversed.
func (s *SearchService) expandListings(ctx context.Context, source provider.MangaListings, bindings []store.Binding) ([]store.Binding, []string) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	const maxListings = 32
	bindings = append([]store.Binding(nil), bindings...)
	seen := map[string]bool{}
	for _, b := range bindings {
		seen[b.SourceMangaKey] = true
	}
	var notes []string
	probes := 0
	for i := 0; i < len(bindings); i++ {
		b := bindings[i]
		if b.State != "active" || b.SourceMangaKey == "" {
			continue
		}
		if probes >= maxListings {
			notes = append(notes, fmt.Sprintf("%s: nested listing discovery limited to %d entries", b.SourceName, maxListings))
			break
		}
		probes++
		refs, err := source.Listings(ctx, b.SourceID, b.SourceMangaKey)
		if err != nil {
			notes = append(notes, fmt.Sprintf("%s: nested listing discovery failed (%v)", b.SourceName, err))
			if ctx.Err() != nil {
				break
			}
			continue // the parent and other matches remain searchable
		}
		for _, ref := range refs {
			if ref.Key == "" || seen[ref.Key] {
				continue
			}
			if len(seen) >= maxListings {
				notes = append(notes, fmt.Sprintf("%s: nested listing discovery limited to %d entries", b.SourceName, maxListings))
				return bindings, notes
			}
			child, err := s.store.DiscoverBinding(ctx, store.Binding{
				SeriesID: b.SeriesID, SourceID: b.SourceID, SourceName: b.SourceName,
				SourceMangaKey: ref.Key, MangaTitle: ref.Title, Priority: b.Priority,
			})
			if err != nil {
				notes = append(notes, fmt.Sprintf("%s: saving nested listing failed (%v)", b.SourceName, err))
				continue
			}
			seen[ref.Key] = true
			bindings = append(bindings, *child)
		}
	}
	return bindings, notes
}
