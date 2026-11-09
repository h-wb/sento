package service

import (
	"context"
	"testing"

	"github.com/h-wb/sento/internal/store"
)

func chapterCollection(t *testing.T, cols []store.Edition) store.Edition {
	t.Helper()
	for _, c := range cols {
		if c.Type == "chapter" {
			return c
		}
	}
	t.Fatalf("no chapter collection in %+v", cols)
	return store.Edition{}
}

func collectionByName(t *testing.T, cols []store.Edition, name, lang string) store.Edition {
	t.Helper()
	for _, c := range cols {
		if c.EditionName == name && c.Language == lang {
			return c
		}
	}
	t.Fatalf("collection %s/%s not found in %+v", name, lang, cols)
	return store.Edition{}
}

func TestCustomEditions(t *testing.T) {
	t.Parallel()
	lib, st := setup(t)
	ctx := context.Background()

	id, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377, Monitor: "all"})
	if err != nil {
		t.Fatal(err)
	}
	cols, _ := st.ListEditions(ctx, id)
	std := collectionByName(t, cols, "Standard Edition", "en")

	// Language variant: clones numbering, drops release dates.
	frID, err := st.AddCustomEdition(ctx, store.Edition{
		SeriesID: id, EditionName: "Standard Edition", Language: "fr", Monitored: true,
	}, std.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	frBooks, _ := st.ListBooks(ctx, frID)
	if len(frBooks) != 3 {
		t.Fatalf("variant must clone 3 works, got %d", len(frBooks))
	}
	for _, w := range frBooks {
		if w.ReleaseDate != "" {
			t.Fatalf("variant works must not inherit release dates: %+v", w)
		}
	}

	// From scratch: generated numbering.
	scratchID, err := st.AddCustomEdition(ctx, store.Edition{
		SeriesID: id, EditionName: "Fan Colored", Language: "en",
	}, 0, 5)
	if err != nil {
		t.Fatal(err)
	}
	if works, _ := st.ListBooks(ctx, scratchID); len(works) != 5 || works[4].SequenceNum != 500 {
		t.Fatalf("scratch edition works wrong: %+v", works)
	}

	// Duplicate identity rejected by the schema.
	if _, err := st.AddCustomEdition(ctx, store.Edition{
		SeriesID: id, EditionName: "Fan Colored", Language: "en",
	}, 0, 5); err == nil {
		t.Fatal("want UNIQUE violation for duplicate identity")
	}

	// Refresh adopts a custom row whose identity MangaBaka now lists
	// (Standard/fr matches nothing → stays custom; the en rows stay bound).
	if err := lib.Refresh(ctx, id); err != nil {
		t.Fatal(err)
	}
	cols, _ = st.ListEditions(ctx, id)
	if c := collectionByName(t, cols, "Standard Edition", "fr"); !c.Custom {
		t.Fatal("fr variant must stay custom after refresh (no MB identity match)")
	}
	if c := collectionByName(t, cols, "Standard Edition", "en"); c.Custom {
		t.Fatal("en edition must not become custom")
	}

	// Different editions cannot be merged by number. Ownership is retained.
	fileID, err := st.AddFile(ctx, store.File{SeriesID: id, Path: "/tmp/v9.cbz", Size: 1}, nil, fixtureLinks(t, st, id, scratchID, 200, 400))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.MergeEditions(ctx, id, scratchID, std.ID); err == nil {
		t.Fatal("different editions must not be merged by number")
	}
	coverage, err := st.Coverage(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(coverage) != 2 || coverage[0].FileID != fileID {
		t.Fatalf("merge refusal changed ownership: %+v", coverage)
	}
	if _, err = st.GetEdition(ctx, scratchID); err != nil {
		t.Fatal(err)
	}

	// Non-custom editions refuse to merge away or be deleted.
	if _, err := st.MergeEditions(ctx, id, std.ID, frID); err == nil {
		t.Fatal("want refusal to merge a MangaBaka edition away")
	}
	if err := st.DeleteCustomEdition(ctx, id, std.ID); err == nil {
		t.Fatal("want refusal to delete a MangaBaka edition")
	}
	if err := st.DeleteCustomEdition(ctx, id, frID); err != nil {
		t.Fatal(err)
	}
}

// A patch that only marks the edition as its own series still applies.
func TestUpdateEditionOwnSeries(t *testing.T) {
	t.Parallel()
	lib, st := setup(t)
	ctx := context.Background()
	id, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}
	cols, err := st.ListEditions(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	std := collectionByName(t, cols, "Standard Edition", "en")
	own := true
	if err := lib.UpdateEdition(ctx, id, std.ID, EditionUpdate{OwnSeries: &own}); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetEdition(ctx, std.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.OwnSeries {
		t.Fatal("an own-series-only patch did not apply")
	}
	own = false
	if err := lib.UpdateEdition(ctx, id, std.ID, EditionUpdate{OwnSeries: &own}); err != nil {
		t.Fatal(err)
	}
	if got, _ = st.GetEdition(ctx, std.ID); got.OwnSeries {
		t.Fatal("the own-series patch did not clear")
	}
}
