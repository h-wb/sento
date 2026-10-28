package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/h-wb/sento/internal/db/dbtest"
	"github.com/h-wb/sento/internal/metadata"
	"github.com/h-wb/sento/internal/store"
)

// withPaperback adds the paperback collection of the Standard Edition to
// the test catalog: the same MangaBaka edition as the digital one, its
// own works, and a fourth volume the digital collection does not have.
func withPaperback(p *fakeProvider) *fakeProvider {
	for i := range p.collections {
		p.collections[i].EditionID = "ed-" + p.collections[i].EditionName
	}
	p.collections = append(p.collections, metadata.Collection{
		ProviderID: "uuid-paper", EditionID: "ed-Standard Edition", EditionName: "Standard Edition",
		Language: "en", Publisher: "VIZ Media", Medium: "paperback", Type: "volume", CountMain: 4,
	})
	p.works["uuid-paper"] = []metadata.Work{
		{ProviderID: "p1", SequenceNum: 100, SequenceStr: "1", CountType: "main", ReleaseDate: "2001-01-01", ISBN: "paper-1"},
		{ProviderID: "p2", SequenceNum: 200, SequenceStr: "2", CountType: "main", ReleaseDate: "2004-01-01", ISBN: "paper-2"},
		{ProviderID: "p3", SequenceNum: 300, SequenceStr: "3", CountType: "main", ReleaseDate: "2005-01-01", ISBN: "paper-3"},
		{ProviderID: "p4", SequenceNum: 400, SequenceStr: "4", CountType: "main", ReleaseDate: "2006-01-01", ISBN: "paper-4"},
	}
	return p
}

func standardEditions(t *testing.T, st *store.Store, id int64) []store.Edition {
	t.Helper()
	all, err := st.ListEditions(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	var out []store.Edition
	for _, e := range all {
		if e.EditionName == "Standard Edition" && e.Type == "volume" {
			out = append(out, e)
		}
	}
	return out
}

// Collections that are the same MangaBaka edition in the same language
// are one edition: a book is wanted once, whatever media it exists in.
func TestCollectionsOfOneEditionAreGrouped(t *testing.T) {
	conn, dialect := dbtest.Open(t)
	st := store.New(conn, dialect)
	lib := NewLibrary(st, withPaperback(testProvider()))
	ctx := context.Background()
	id, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}
	eds := standardEditions(t, st, id)
	if len(eds) != 1 {
		t.Fatalf("digital and paperback should be one edition: %+v", eds)
	}
	e := eds[0]
	if len(e.Collections) != 2 || e.Collections[0].Medium != "digital" || e.Collections[1].Medium != "paperback" {
		t.Fatalf("collections = %+v", e.Collections)
	}
	// The preferred collection speaks for the edition.
	if e.Medium != "digital" || e.Publisher != "Shonen Jump" || !e.Monitored || e.CountMain != 4 {
		t.Fatalf("edition = %+v", e)
	}
	if !e.HasMedium("paperback") || e.HasMedium("hardcover") {
		t.Fatal("the edition should exist in both its media")
	}
	books, err := st.ListBooks(ctx, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 4 {
		t.Fatalf("books are the union by number: %+v", books)
	}
	// Volume 1: the digital copy's ISBN, the earliest date of any copy.
	if books[0].ISBN != "9781569319017" || books[0].ReleaseDate != "2001-01-01" || books[0].ProviderID != "w1" {
		t.Fatalf("volume 1 = %+v", books[0])
	}
	// Volume 3 is announced digitally for 2099 but has been in print since 2005.
	if books[2].ReleaseDate != "2005-01-01" {
		t.Fatalf("volume 3 = %+v", books[2])
	}
	// Volume 4 exists only in paperback and is a book all the same.
	if books[3].ISBN != "paper-4" || books[3].ProviderID != "p4" {
		t.Fatalf("volume 4 = %+v", books[3])
	}
	works, err := st.ListWorks(ctx, books[0].ID)
	if err != nil || len(works) != 2 || works[0].Medium != "digital" || works[1].ISBN != "paper-1" {
		t.Fatalf("works = %+v %v", works, err)
	}

	// A refresh changes nothing, and a collection the catalog drops goes
	// stale without taking the edition or its books with it.
	p := withPaperback(testProvider())
	if err = st.ReconcileMetadata(ctx, id, p.series, p.collections, p.works); err != nil {
		t.Fatal(err)
	}
	if again, _ := st.ListBooks(ctx, e.ID); len(again) != 4 || again[0].ID != books[0].ID {
		t.Fatalf("refresh changed the books: %+v", again)
	}
	p.collections = p.collections[1:] // the digital collection disappears
	if err = st.ReconcileMetadata(ctx, id, p.series, p.collections, p.works); err != nil {
		t.Fatal(err)
	}
	e = standardEditions(t, st, id)[0]
	if e.SyncState != "linked" || e.Medium != "paperback" || e.Publisher != "VIZ Media" {
		t.Fatalf("the paperback should now speak for the edition: %+v", e)
	}
	after, _ := st.ListBooks(ctx, e.ID)
	if len(after) != 4 || after[0].ISBN != "paper-1" || after[0].SyncState != "linked" {
		t.Fatalf("books after the digital collection left: %+v", after)
	}
}

// Editions that were separate before grouping existed are folded together
// by a refresh, unless both hold something of the person's: those are
// merged by hand.
func TestRefreshGroupsExistingEditions(t *testing.T) {
	for _, owned := range []bool{false, true} {
		conn, dialect := dbtest.Open(t)
		st := store.New(conn, dialect)
		lib := NewLibrary(st, withPaperback(testProvider()))
		ctx := context.Background()
		id, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
		if err != nil {
			t.Fatal(err)
		}
		e := standardEditions(t, st, id)[0]
		apart, err := st.SeparateCollection(ctx, id, e.Collections[1].ID)
		if err != nil {
			t.Fatal(err)
		}
		eds := standardEditions(t, st, id)
		if len(eds) != 2 {
			t.Fatalf("separating should leave two editions: %+v", eds)
		}
		paper, err := st.GetEdition(ctx, apart)
		if err != nil || !paper.Split || paper.Monitored || paper.Medium != "paperback" || len(paper.Collections) != 1 {
			t.Fatalf("separated edition = %+v %v", paper, err)
		}
		kept, _ := st.ListBooks(ctx, e.ID)
		moved, _ := st.ListBooks(ctx, apart)
		if len(kept) != 3 || len(moved) != 4 || kept[0].ISBN != "9781569319017" || moved[0].ISBN != "paper-1" {
			t.Fatalf("books after separating: %+v / %+v", kept, moved)
		}
		// A person's separation is respected by refreshes.
		p := withPaperback(testProvider())
		if err = st.ReconcileMetadata(ctx, id, p.series, p.collections, p.works); err != nil {
			t.Fatal(err)
		}
		if len(standardEditions(t, st, id)) != 2 {
			t.Fatal("refresh regrouped a separated edition")
		}
		// Without that mark it is the state an older library is in.
		if _, err = conn.ExecContext(ctx, fmt.Sprintf(`UPDATE editions SET split=FALSE WHERE id=%d`, apart)); err != nil {
			t.Fatal(err)
		}
		if owned {
			versions, _ := st.ListVersions(ctx, id)
			for _, v := range versions {
				if v.EditionID == apart {
					ownedModelFile(t, st, id, *paper, moved[0], v)
				}
				if v.EditionID == e.ID {
					ownedModelFile(t, st, id, e, kept[0], v)
				}
			}
		}
		if err = st.ReconcileMetadata(ctx, id, p.series, p.collections, p.works); err != nil {
			t.Fatal(err)
		}
		eds = standardEditions(t, st, id)
		switch {
		case owned && len(eds) != 2:
			t.Fatal("an edition with files was merged without anyone asking")
		case !owned && (len(eds) != 1 || len(eds[0].Collections) != 2):
			t.Fatalf("refresh should group the editions: %+v", eds)
		}
		if !owned {
			if books, _ := st.ListBooks(ctx, eds[0].ID); len(books) != 4 {
				t.Fatalf("grouped books: %+v", books)
			}
			continue
		}
		// With files on both sides it is merged by hand, and the files follow.
		if _, err = st.MergeEditions(ctx, id, apart, e.ID); err != nil {
			t.Fatal(err)
		}
		eds = standardEditions(t, st, id)
		if len(eds) != 1 || len(eds[0].Collections) != 2 {
			t.Fatalf("merge should leave one edition: %+v", eds)
		}
		coverage, err := st.Coverage(ctx, id)
		files := map[int64]bool{}
		for _, c := range coverage {
			files[c.FileID] = true
			if c.BookID != kept[0].ID {
				t.Fatalf("a file was left on a book that is gone: %+v", coverage)
			}
		}
		if err != nil || len(files) != 2 {
			t.Fatalf("both files should now be copies of the one book: %+v %v", coverage, err)
		}
	}
}
