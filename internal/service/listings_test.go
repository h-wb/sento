package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/h-wb/sento/internal/extensions"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

type listingHost struct {
	fakeExtHost
	mu       sync.Mutex
	children map[string][]extensions.Manga
	fail     map[string]bool
	listed   []string
	searched []string
}

func (h *listingHost) Sources(context.Context) ([]extensions.Source, error) {
	return []extensions.Source{{ID: "src1", Name: "Directory Source", Lang: "en"}}, nil
}

func (h *listingHost) Search(context.Context, string, string) ([]extensions.Manga, error) {
	return []extensions.Manga{{Key: "root", Title: "One Piece"}}, nil
}

func (h *listingHost) Listings(_ context.Context, _, key string) ([]extensions.Manga, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.listed = append(h.listed, key)
	if h.fail[key] {
		return nil, errors.New("folder unavailable")
	}
	return h.children[key], nil
}

func (h *listingHost) Chapters(_ context.Context, _, key string) ([]extensions.Chapter, error) {
	h.mu.Lock()
	h.searched = append(h.searched, key)
	h.mu.Unlock()
	name := "Vol. 1"
	if key != "root" {
		name += " (Digital)"
	}
	return []extensions.Chapter{{Key: key + " release", Name: name}}, nil
}

// An existing root binding must gain nested releases in the same search,
// with ordinary profile scoring and independently disabled matches.
func TestNestedListingSearch(t *testing.T) {
	lib, st := setup(t)
	ctx := context.Background()
	if err := NewProfiles(st).EnsureDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	sid, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377, Monitor: "all"})
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := json.Marshal(SourceSettings{SourceID: "src1"})
	iid, err := st.AddIndexer(ctx, store.Indexer{
		Name: "Directory Source", Protocol: store.IndexerSource, Enabled: true,
		UseVolumes: true, UseChapters: true, Settings: settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"root", "disabled"} {
		state := "active"
		if key == "disabled" {
			state = "disabled"
		}
		if _, err := st.AddBinding(ctx, store.Binding{
			SeriesID: sid, SourceID: fmt.Sprint(iid), SourceName: "Directory Source",
			SourceMangaKey: key, MangaTitle: "One Piece", State: state,
		}); err != nil {
			t.Fatal(err)
		}
	}
	h := &listingHost{children: map[string][]extensions.Manga{
		"root":    {{Key: "digital", Title: "One Piece"}, {Key: "disabled", Title: "One Piece"}},
		"digital": {{Key: "fixed", Title: "One Piece"}, {Key: "root", Title: "One Piece"}}, // cycle
	}}
	p := provider.NewSource(h, t.TempDir(), provider.DownloadConfig{}, IndexerEntries(st, store.IndexerSource))
	svc := NewSearchService(st, provider.NewRegistry(p))
	result, err := svc.FindCandidates(ctx, sid, LedgerVolume, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.BindingNotes) != 0 {
		t.Fatal(result.BindingNotes)
	}
	if len(h.listed) != 3 || slices.Contains(h.listed, "disabled") || slices.Contains(h.searched, "disabled") {
		t.Fatalf("disabled entry traversed or cycle repeated: listed %v, searched %v", h.listed, h.searched)
	}
	var root, digital *ReleaseCandidate
	for i := range result.Candidates {
		c := &result.Candidates[i]
		switch c.cand.Source.ChapterKey {
		case "root release":
			root = c
		case "digital release":
			digital = c
		}
	}
	if root == nil || digital == nil || !digital.Parse.Quality.Digital || digital.Score <= root.Score || len(digital.Rejections) != 0 {
		t.Fatalf("nested Digital release should outrank root: root %+v, digital %+v", root, digital)
	}
	bindings, err := st.ListBindings(ctx, sid)
	if err != nil || len(bindings) != 4 {
		t.Fatalf("discovered matches not persisted: %v %v", bindings, err)
	}
	// Rediscovery must not undo a person's mapping or disabled state,
	// including when a concurrent discovery did not know about the row.
	editions, _ := st.ListEditions(ctx, sid)
	edition := chapterCollection(t, editions)
	for _, b := range bindings {
		if b.SourceMangaKey == "digital" {
			if err := st.SetBindingEdition(ctx, sid, b.ID, edition.ID); err != nil {
				t.Fatal(err)
			}
			if err := st.SetBindingState(ctx, sid, b.ID, "disabled"); err != nil {
				t.Fatal(err)
			}
			restored, err := st.DiscoverBinding(ctx, b)
			if err != nil || restored.State != "disabled" || restored.EditionID != edition.ID || !restored.TrustChapters {
				t.Fatalf("discovery overwrote saved match: %+v %v", restored, err)
			}
		}
	}
	h.listed, h.searched = nil, nil
	if _, err := svc.FindCandidates(ctx, sid, LedgerVolume, nil); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(h.listed, "digital") || slices.Contains(h.searched, "digital") {
		t.Fatalf("disabled digital entry reactivated: listed %v, searched %v", h.listed, h.searched)
	}
}

func TestListingDiscoveryLimitsAndPartialFailure(t *testing.T) {
	lib, st := setup(t)
	ctx := context.Background()
	sid, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}
	root := store.Binding{SeriesID: sid, SourceID: "1", SourceMangaKey: "root", State: "active"}
	if root.ID, err = st.AddBinding(ctx, root); err != nil {
		t.Fatal(err)
	}
	svc := NewSearchService(st, provider.NewRegistry())
	h := &listingHost{fail: map[string]bool{"root": true}}
	p := provider.NewSource(h, t.TempDir(), provider.DownloadConfig{}, func(context.Context) ([]provider.IndexerEntry, error) {
		return []provider.IndexerEntry{{ID: 1, SourceID: "src1"}}, nil
	}).(provider.MangaListings)
	got, notes := svc.expandListings(ctx, p, []store.Binding{root})
	if len(got) != 1 || got[0].ID != root.ID || len(notes) != 1 {
		t.Fatalf("failed discovery lost its root or error: %v %v", got, notes)
	}
	h.fail = nil
	h.children = map[string][]extensions.Manga{}
	for i := range 100 {
		h.children["root"] = append(h.children["root"], extensions.Manga{Key: fmt.Sprint(i), Title: "One Piece"})
	}
	got, notes = svc.expandListings(ctx, p, []store.Binding{root})
	if len(got) != 32 || len(notes) != 1 {
		t.Fatalf("discovery not bounded or limit unreported: %d entries, %v", len(got), notes)
	}
}
