package service

import (
	"context"
	"testing"

	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/store"
)

func chapterCollection(t *testing.T, cols []store.Collection) store.Collection {
	t.Helper()
	for _, c := range cols {
		if c.Type == "chapter" {
			return c
		}
	}
	t.Fatalf("no chapter collection in %+v", cols)
	return store.Collection{}
}

func collectionByName(t *testing.T, cols []store.Collection, name, lang string) store.Collection {
	t.Helper()
	for _, c := range cols {
		if c.EditionName == name && c.Language == lang {
			return c
		}
	}
	t.Fatalf("collection %s/%s not found in %+v", name, lang, cols)
	return store.Collection{}
}

func TestCustomEditions(t *testing.T) {
	lib, st := setup(t)
	ctx := context.Background()

	id, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377, Monitor: "all"})
	if err != nil {
		t.Fatal(err)
	}
	cols, _ := st.ListCollections(ctx, id)
	std := collectionByName(t, cols, "Standard Edition", "en")

	// Language variant: clones numbering, drops release dates.
	frID, err := st.AddCustomCollection(ctx, store.Collection{
		SeriesID: id, EditionName: "Standard Edition", Language: "fr", Monitored: true,
	}, std.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	frWorks, _ := st.ListWorks(ctx, frID)
	if len(frWorks) != 3 {
		t.Fatalf("variant must clone 3 works, got %d", len(frWorks))
	}
	for _, w := range frWorks {
		if w.ReleaseDate != "" {
			t.Fatalf("variant works must not inherit release dates: %+v", w)
		}
	}

	// From scratch: generated numbering.
	scratchID, err := st.AddCustomCollection(ctx, store.Collection{
		SeriesID: id, EditionName: "Fan Colored", Language: "en",
	}, 0, 5)
	if err != nil {
		t.Fatal(err)
	}
	if works, _ := st.ListWorks(ctx, scratchID); len(works) != 5 || works[4].SequenceNum != 500 {
		t.Fatalf("scratch edition works wrong: %+v", works)
	}

	// Duplicate identity rejected by the schema.
	if _, err := st.AddCustomCollection(ctx, store.Collection{
		SeriesID: id, EditionName: "Fan Colored", Language: "en",
	}, 0, 5); err == nil {
		t.Fatal("want UNIQUE violation for duplicate identity")
	}

	// Refresh adopts a custom row whose identity MangaBaka now lists
	// (Standard/fr matches nothing → stays custom; the en rows stay bound).
	if err := lib.Refresh(ctx, id); err != nil {
		t.Fatal(err)
	}
	cols, _ = st.ListCollections(ctx, id)
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
	if _, err = st.MergeCollection(ctx, id, scratchID, std.ID); err == nil {
		t.Fatal("different editions must not be merged by number")
	}
	coverage, err := st.Coverage(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(coverage) != 2 || coverage[0].FileID != fileID {
		t.Fatalf("merge refusal changed ownership: %+v", coverage)
	}
	if _, err = st.GetCollection(ctx, scratchID); err != nil {
		t.Fatal(err)
	}

	// Non-custom collections refuse to merge away or be deleted.
	if _, err := st.MergeCollection(ctx, id, std.ID, frID); err == nil {
		t.Fatal("want refusal to merge a MangaBaka edition away")
	}
	if err := st.DeleteCustomCollection(ctx, id, std.ID); err == nil {
		t.Fatal("want refusal to delete a MangaBaka edition")
	}
	if err := st.DeleteCustomCollection(ctx, id, frID); err != nil {
		t.Fatal(err)
	}
}

func TestRouteEditionLanguage(t *testing.T) {
	en := store.Collection{ID: 1, EditionName: "Standard Edition", Language: "en"}
	fr := store.Collection{ID: 2, EditionName: "Standard Edition", Language: "fr"}
	cols := []store.Collection{en, fr}
	wanted := map[int64]map[int64]bool{1: {100: true}, 2: {100: true}}

	route := func(lang string) int64 {
		id, ok := routeCollection(parse.Result{Language: lang}, cols, wanted)
		if !ok {
			t.Fatalf("routing failed for lang %q", lang)
		}
		return id
	}
	if got := route("fr"); got != fr.ID {
		t.Fatalf("FR release must route to the fr edition, got %d", got)
	}
	if got := route("en"); got != en.ID {
		t.Fatalf("EN release must route to the en edition, got %d", got)
	}
	if got := route(""); got != en.ID {
		t.Fatalf("untagged release keeps the first name match, got %d", got)
	}
	// No matching language → fall back to all monitored.
	if got := route("ja"); got != en.ID {
		t.Fatalf("unmatched language falls back, got %d", got)
	}
}
