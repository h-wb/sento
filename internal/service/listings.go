package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// Root matches and their nested listings are ordinary bindings. Walk
// active entries on every search so existing matches acquire new folders
// too; disabled entries stay disabled and are never traversed. A listing
// is named by its path ("One Piece - Digital Colored Comics - Definitive
// Edition (VLT)"), so its releases inherit what the path says, and one
// named for another work is switched off (listingBelongs).
func (s *SearchService) expandListings(ctx context.Context, source provider.MangaListings, bindings []store.Binding, titles []string) ([]store.Binding, []string) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	const maxListings = 32
	bindings = append([]store.Binding(nil), bindings...)
	byKey := map[string]int{}
	for i, b := range bindings {
		byKey[b.SourceMangaKey] = i
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
			if ref.Key == "" {
				continue
			}
			name, belongs := listingName(b.MangaTitle, ref.Title), listingBelongs(ref.Title, titles)
			if at, known := byKey[ref.Key]; known {
				// Listings found before they carried their own names are
				// renamed, and judged by the name, once.
				if old := bindings[at]; old.MangaTitle != name && old.ID > 0 {
					if err := s.store.RenameListing(ctx, old.SeriesID, old.ID, name, belongs); err == nil {
						bindings[at].MangaTitle = name
						if !belongs {
							bindings[at].State = "disabled"
						}
					}
				}
				continue
			}
			if len(byKey) >= maxListings {
				notes = append(notes, fmt.Sprintf("%s: nested listing discovery limited to %d entries", b.SourceName, maxListings))
				return bindings, notes
			}
			state := "active"
			if !belongs {
				state = "disabled"
			}
			child, err := s.store.DiscoverBinding(ctx, store.Binding{
				SeriesID: b.SeriesID, SourceID: b.SourceID, SourceName: b.SourceName,
				SourceMangaKey: ref.Key, MangaTitle: name, State: state, Priority: b.Priority,
			})
			if err != nil {
				notes = append(notes, fmt.Sprintf("%s: saving nested listing failed (%v)", b.SourceName, err))
				continue
			}
			byKey[ref.Key] = len(bindings)
			bindings = append(bindings, *child)
		}
	}
	return bindings, notes
}

// listingName is a nested listing's path: its parent's name and its own,
// the source's sorting mark ("!") and a repeat of the series' title
// ("One Piece - Digital Colored Comics" under "One Piece") dropped.
func listingName(parent, own string) string {
	own = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(own), "!"))
	series, _, _ := strings.Cut(parent, " - ")
	if series != "" && len(own) >= len(series) && strings.EqualFold(own[:len(series)], series) {
		own = strings.TrimLeft(own[len(series):], " -–:")
	}
	switch {
	case own == "":
		return parent
	case parent == "":
		return own
	default:
		return parent + " - " + own
	}
}

// listingBelongs reports whether a nested listing holds the series' own
// releases, by its own name. A folder named for another work that carries
// the series' title ("One Piece - Shokugeki no Sanji", "One Piece - Art
// Books and Databooks") does not, nor one holding no manga; one named by
// release words alone ("!Digital", "!upscaled B&W") stays with its parent.
func listingBelongs(own string, titles []string) bool {
	p := parse.ParseAnchored(strings.TrimLeft(strings.TrimSpace(own), "!"), parse.ChannelSource, titles)
	return p.ContentType == parse.ContentManga && (!p.Anchored || p.TitleLeftover == "")
}
