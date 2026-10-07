package service

import (
	"context"
	"testing"

	"github.com/h-wb/sento/internal/db/dbtest"
	"github.com/h-wb/sento/internal/metadata"
	"github.com/h-wb/sento/internal/store"
)

type fakeProvider struct {
	series      *metadata.Series
	collections []metadata.Collection
	works       map[string][]metadata.Work
}

func (f *fakeProvider) Search(context.Context, string) ([]metadata.SeriesRef, error) {
	return nil, nil
}
func (f *fakeProvider) GetSeries(context.Context, int64) (*metadata.Series, error) {
	return f.series, nil
}
func (f *fakeProvider) GetCollections(context.Context, int64) ([]metadata.Collection, error) {
	return f.collections, nil
}
func (f *fakeProvider) GetWorks(context.Context, int64) (map[string][]metadata.Work, error) {
	return f.works, nil
}

func testProvider() *fakeProvider {
	return &fakeProvider{
		series: &metadata.Series{
			MangaBakaID:   377,
			Title:         "ONE PIECE",
			Type:          "manga",
			Status:        "releasing",
			TotalChapters: 1184,
			FinalVolume:   114,
			RawJSON:       []byte(`{"id":377}`),
		},
		collections: []metadata.Collection{
			{
				ProviderID:  "uuid-standard",
				EditionName: "Standard Edition",
				Language:    "en",
				Publisher:   "Shonen Jump",
				Medium:      "digital",
				Type:        "volume",
				CountMain:   3,
			},
			{
				ProviderID:  "uuid-omnibus",
				EditionName: "Omnibus Edition",
				Language:    "en",
				Publisher:   "Shonen Jump",
				Medium:      "paperback",
				Type:        "volume",
				CountMain:   1,
			},
		},
		works: map[string][]metadata.Work{
			"uuid-standard": {
				{ProviderID: "w1", SequenceNum: 100, SequenceStr: "1", CountType: "main", ReleaseDate: "2003-09-02", ISBN: "9781569319017"},
				{ProviderID: "w2", SequenceNum: 200, SequenceStr: "2", CountType: "main", ReleaseDate: "2003-11-04"},
				{ProviderID: "w3", SequenceNum: 300, SequenceStr: "3", CountType: "main", ReleaseDate: "2099-01-01"}, // future
			},
			"uuid-omnibus": {
				{ProviderID: "w4", SequenceNum: 100, SequenceStr: "1", CountType: "main"},
			},
		},
	}
}

func setup(t *testing.T) (*Library, *store.Store) {
	t.Helper()
	conn, dialect := dbtest.Open(t)
	st := store.New(conn, dialect)
	return NewLibrary(st, testProvider()), st
}

func TestAddSeriesDefaultMonitor(t *testing.T) {
	lib, st := setup(t)
	ctx := context.Background()

	// No mode given: the Standard Edition is monitored, the rest is not.
	id, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}
	cols, err := st.ListCollections(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cols {
		// The chapter collection is always monitored at add (cutoff = latest,
		// so nothing is wanted until new chapters release).
		want := c.Type == "chapter" || (c.EditionName == "Standard Edition" && c.Type == "volume")
		if c.Monitored != want {
			t.Errorf("%s %s: monitored=%v, want %v", c.EditionName, c.Type, c.Monitored, want)
		}
	}
}

func TestAddSeriesMonitorNone(t *testing.T) {
	lib, st := setup(t)
	ctx := context.Background()

	id, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377, Monitor: "none"})
	if err != nil {
		t.Fatal(err)
	}
	cols, err := st.ListCollections(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cols {
		if c.Monitored && c.Type != "chapter" {
			t.Errorf("%s: monitored, want none", c.EditionName)
		}
	}
}

func TestDefaultEditionID(t *testing.T) {
	cols := []metadata.Collection{
		{ProviderID: "chap", EditionName: "Standard Edition", Language: "fr", Type: "chapter"},
		{ProviderID: "omni-en", EditionName: "Omnibus Edition", Language: "en", Type: "volume"},
		{ProviderID: "std-fr", EditionName: "Standard Edition", Language: "fr", Type: "volume"},
		{ProviderID: "std-en", EditionName: "Standard Edition", Language: "en", Type: "volume"},
	}
	if got := defaultEditionID(cols, ""); got != "std-en" {
		t.Errorf("default lang: got %s, want std-en", got)
	}
	if got := defaultEditionID(cols, "fr"); got != "std-fr" {
		t.Errorf("fr: got %s, want std-fr", got)
	}
	if got := defaultEditionID(nil, ""); got != "" {
		t.Errorf("empty: got %s, want empty", got)
	}
}

func TestAddSeriesAndSlots(t *testing.T) {
	lib, st := setup(t)
	ctx := context.Background()

	id, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377, Monitor: "all"})
	if err != nil {
		t.Fatal(err)
	}

	series, err := st.GetSeries(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if series.Title != "ONE PIECE" || series.TotalChapters != 1184 {
		t.Fatalf("unexpected series: %+v", series)
	}

	cols, err := st.ListCollections(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 3 { // 2 volume editions + the default chapter collection
		t.Fatalf("want 3 collections, got %d", len(cols))
	}
	for _, c := range cols {
		if !c.Monitored {
			t.Errorf("collection %s should be monitored (monitor: all)", c.EditionName)
		}
	}

	// Chapter collection: cutoff defaults to latest at add (D6).
	chapters := chapterCollection(t, cols)
	if chapters.CutoffNum != 1184*100 {
		t.Fatalf("cutoff: want %d, got %d", 1184*100, chapters.CutoffNum)
	}

	// Volume slots: vol 1+2 wanted (released, no files); vol 3 future → not wanted.
	var standard store.Collection
	for _, c := range cols {
		if c.EditionName == "Standard Edition" {
			standard = c
		}
	}
	slots := booksOf(t, lib, st, id, standard.ID)
	if len(slots) != 3 {
		t.Fatalf("want 3 slots, got %d", len(slots))
	}
	if !slots[0].Wanted || !slots[1].Wanted {
		t.Errorf("vols 1-2 should be wanted: %+v", slots[:2])
	}
	if slots[2].Wanted || slots[2].Availability == "released" {
		t.Errorf("vol 3 is unreleased, should not be wanted: %+v", slots[2])
	}

	// Link a file to vol 1 → no longer wanted.
	_, err = st.AddFile(ctx, store.File{
		SeriesID: id, Path: "/library/One Piece/One Piece v01.cbz", Size: 1,
		Provenance: "torrent",
	}, nil, fixtureLinks(t, st, id, standard.ID, 100))
	if err != nil {
		t.Fatal(err)
	}
	slots = booksOf(t, lib, st, id, standard.ID)
	// The file's path was never checked on disk, which holds the series'
	// acquisition until it is: the book is claimed and no longer wanted.
	if slots[0].Wanted || slots[0].State == "missing" {
		t.Errorf("vol 1 should no longer be wanted: %+v", slots[0])
	}
}

func TestChapterSlots(t *testing.T) {
	lib, st := setup(t)
	ctx := context.Background()

	id, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377, ChapterCutoff: 118200})
	if err != nil {
		t.Fatal(err)
	}
	cols, err := st.ListCollections(ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	slots := booksOf(t, lib, st, id, chapterCollection(t, cols).ID)
	if len(slots) != 1184 {
		t.Fatalf("want 1184 slots, got %d", len(slots))
	}
	// cutoff 1182 → chapters 1183, 1184 monitored/wanted
	wanted := 0
	for _, s := range slots {
		if s.Wanted {
			wanted++
		}
	}
	if wanted != 2 {
		t.Fatalf("want 2 wanted chapter slots, got %d", wanted)
	}
}

func TestRefreshReconciliation(t *testing.T) {
	lib, st := setup(t)
	ctx := context.Background()

	id, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377, Monitor: "all"})
	if err != nil {
		t.Fatal(err)
	}

	// Simulate MangaBaka regenerating collection UUIDs and adding a volume.
	p := lib.provider.(*fakeProvider)
	p.collections[0].ProviderID = "uuid-standard-REGENERATED"
	p.works["uuid-standard-REGENERATED"] = append(p.works["uuid-standard"],
		metadata.Work{ProviderID: "w5", SequenceNum: 400, SequenceStr: "4", CountType: "main", ReleaseDate: "2024-01-01"})
	p.series.FinalVolume = 115

	if err := lib.Refresh(ctx, id); err != nil {
		t.Fatal(err)
	}

	cols, err := st.ListCollections(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 3 { // 2 volume editions + the chapter collection
		t.Fatalf("descriptor matching must not duplicate collections: got %d", len(cols))
	}
	var standard store.Collection
	for _, c := range cols {
		if c.EditionName == "Standard Edition" {
			standard = c
		}
	}
	if !standard.Monitored {
		t.Error("monitored flag must survive refresh")
	}
	works, err := st.ListWorks(ctx, standard.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 4 {
		t.Fatalf("new work should be inserted: want 4, got %d", len(works))
	}
}
