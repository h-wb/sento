package service

import (
	"context"
	"os"
	"path/filepath"
	"slices"
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
func (f *fakeProvider) GetEditions(context.Context) ([]metadata.Edition, error) {
	return []metadata.Edition{{ID: "ed-Standard Edition", Name: "Standard Edition"}}, nil
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
	cols, err := st.ListEditions(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cols {
		// The chapter edition is always monitored at add (cutoff = latest,
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
	cols, err := st.ListEditions(ctx, id)
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

	cols, err := st.ListEditions(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 3 { // 2 volume editions + the default chapter edition
		t.Fatalf("want 3 collections, got %d", len(cols))
	}
	for _, c := range cols {
		if !c.Monitored {
			t.Errorf("collection %s should be monitored (monitor: all)", c.EditionName)
		}
	}

	// Chapter edition: cutoff defaults to latest at add (D6).
	chapters := chapterCollection(t, cols)
	if chapters.CutoffNum != 1184*100 {
		t.Fatalf("cutoff: want %d, got %d", 1184*100, chapters.CutoffNum)
	}

	// Volume books: vol 1+2 wanted (released, no files); vol 3 future → not wanted.
	var standard store.Edition
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
	cols, err := st.ListEditions(ctx, id)
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
	p.works["uuid-standard-REGENERATED"] = append(slices.Clone(p.works["uuid-standard"]),
		metadata.Work{ProviderID: "w5", SequenceNum: 400, SequenceStr: "4", CountType: "main", ReleaseDate: "2024-01-01"})
	p.series.FinalVolume = 115

	if err := lib.Refresh(ctx, id); err != nil {
		t.Fatal(err)
	}

	cols, err := st.ListEditions(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 3 { // 2 volume editions + the chapter edition
		t.Fatalf("descriptor matching must not duplicate collections: got %d", len(cols))
	}
	var standard store.Edition
	for _, c := range cols {
		if c.EditionName == "Standard Edition" {
			standard = c
		}
	}
	if !standard.Monitored {
		t.Error("monitored flag must survive refresh")
	}
	works, err := st.ListBooks(ctx, standard.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 4 {
		t.Fatalf("new work should be inserted: want 4, got %d", len(works))
	}
}

// Collected volumes hold the chapters out so far, so only new chapters are
// wanted; a series without volumes (a one-shot) wants every chapter.
func TestAddSeriesChapterCutoffFollowsVolumes(t *testing.T) {
	ctx := context.Background()
	lib, st := setup(t)
	id, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}
	cutoff := func(id int64) int64 {
		t.Helper()
		cols, err := st.ListEditions(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cols {
			if c.Type == "chapter" {
				return c.CutoffNum
			}
		}
		t.Fatal("no chapter edition")
		return 0
	}
	if got := cutoff(id); got != 1184*100 {
		t.Errorf("with volumes: skip through %d, want the latest chapter", got)
	}
	oneShot := testProvider()
	oneShot.series.MangaBakaID, oneShot.series.Title, oneShot.series.TotalChapters, oneShot.series.FinalVolume = 900, "The Enigma of Amigara Fault", 1, 0
	oneShot.collections = nil
	id, err = NewLibrary(st, oneShot).AddSeries(ctx, AddSeriesRequest{MangaBakaID: 900})
	if err != nil {
		t.Fatal(err)
	}
	if got := cutoff(id); got != 0 {
		t.Errorf("without volumes: skip through %d, want every chapter", got)
	}
}

// Importing a folder whose series is already in the library links the
// folder's files to it, in place, instead of refusing.
func TestImportFolderLinksIntoAnExistingSeries(t *testing.T) {
	lib, st := setup(t)
	ctx := context.Background()
	id, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := st.GetSeries(ctx, id)
	dir := filepath.Join(t.TempDir(), "One Piece")
	if err = os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "ONE PIECE v01.cbz"), []byte("pages"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, result, existing, err := lib.ImportFolder(ctx, NewScanner(st, t.TempDir()), dir, 377)
	if err != nil || got != id || !existing {
		t.Fatalf("import into the existing series: id %d (want %d) existing %v err %v", got, id, existing, err)
	}
	if result.Linked != 1 {
		t.Errorf("files linked: %+v", result)
	}
	if after, _ := st.GetSeries(ctx, id); after.Path != before.Path {
		t.Errorf("the series' own folder changed: %q → %q", before.Path, after.Path)
	}
}

// Indexers are searched by the names releases in the edition's language
// use: the title, its titles in that language, the romanized one; other
// languages' aliases only find other languages' releases.
func TestQueryTitlesKeepToTheEditionsLanguage(t *testing.T) {
	p := testProvider()
	p.series.RawJSON = []byte(`{"id":377,"native_title":"ワンピース","romanized_title":"Wan Pīsu",
		"secondary_titles":{"en":[{"title":"One Piece: Grand Line"}],"ko":[{"title":"원피스"}],"ms":[{"title":"Budak Getah"}]}}`)
	conn, dialect := dbtest.Open(t)
	st := store.New(conn, dialect)
	ctx := context.Background()
	id, err := NewLibrary(st, p).AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.QueryTitles(ctx, id, "en")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"ONE PIECE", "One Piece: Grand Line", "Wan Pīsu"}) {
		t.Fatalf("query titles = %q", got)
	}
	if all, _ := st.SeriesTitles(ctx, id, "en"); !slices.Contains(all, "Budak Getah") {
		t.Fatalf("matching lost an alias: %q", all)
	}
}
