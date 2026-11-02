package service

import (
	"context"
	"testing"

	"github.com/h-wb/sento/internal/store"
)

// Multi-binding semantics: a site holds several manga entries, upserts key
// on the entry, and an active binding clears the "unmatched" sentinel.
func TestMultiBindings(t *testing.T) {
	t.Parallel()
	lib, st := setup(t)
	ctx := context.Background()

	id, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}

	// A probe that found nothing confident persists one sentinel.
	if _, err := st.AddBinding(ctx, store.Binding{
		SeriesID: id, SourceID: "madokami", SourceMangaKey: "7",
		MangaTitle: "One Piece Party", State: "unmatched",
	}); err != nil {
		t.Fatal(err)
	}

	// Two confident entries on the same site both bind; the sentinel goes.
	for mangaKey, title := range map[string]string{"1": "One Piece", "2": "One Piece (2024)"} {
		if _, err := st.AddBinding(ctx, store.Binding{
			SeriesID: id, SourceID: "madokami", SourceMangaKey: mangaKey,
			MangaTitle: title,
		}); err != nil {
			t.Fatal(err)
		}
	}
	bindings, err := st.ListBindings(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 2 {
		t.Fatalf("want 2 bindings (sentinel cleared), got %+v", bindings)
	}
	for _, b := range bindings {
		if b.State != "active" {
			t.Fatalf("want active bindings, got %+v", b)
		}
	}

	// Re-binding the same entry upserts in place, no duplicate row.
	bid, err := st.AddBinding(ctx, store.Binding{
		SeriesID: id, SourceID: "madokami", SourceMangaKey: "1", MangaTitle: "One Piece (retitled)",
	})
	if err != nil {
		t.Fatal(err)
	}
	bindings, _ = st.ListBindings(ctx, id)
	if len(bindings) != 2 {
		t.Fatalf("upsert duplicated a binding: %+v", bindings)
	}

	// Per-entry toggle.
	if err := st.SetBindingState(ctx, id, bid, "disabled"); err != nil {
		t.Fatal(err)
	}
	bindings, _ = st.ListBindings(ctx, id)
	for _, b := range bindings {
		if b.ID == bid {
			if b.State != "disabled" || b.MangaTitle != "One Piece (retitled)" {
				t.Fatalf("toggle/upsert lost: %+v", b)
			}
		}
	}
	if err := st.SetBindingState(ctx, id, 9999, "active"); err == nil {
		t.Fatal("want ErrNotFound for unknown binding id")
	}
}

func TestResolveQueries(t *testing.T) {
	t.Parallel()
	got := resolveQueries([]string{"HUNTER×HUNTER", "ハンター×ハンター", "Hunter x Hunter", "Cazador X"})
	// Primary, its normalized form, then the first distinct Latin alias —
	// the native-script alias is skipped, "Hunter x Hunter" dedupes against
	// the normalized primary.
	want := []string{"HUNTER×HUNTER", "hunter x hunter", "Cazador X"}
	if len(got) != len(want) {
		t.Fatalf("resolveQueries = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("resolveQueries = %v, want %v", got, want)
		}
	}
}
