//go:build contract

// Contract tests against the live MangaBaka API. They exist to catch
// upstream drift — a renamed field, a dropped endpoint, a changed
// envelope — which book tests against fixtures cannot see. Network-bound
// and therefore never part of `go test ./...`:
//
//	mise run test:contract
package mangabaka

import (
	"context"
	"testing"
	"time"
)

// probeSeries is a long-lived, widely-licensed series that exercises every
// endpoint including collections/works (many series legitimately have
// none — Berserk, for one — so a thin series would give a false green).
const probeSeries = "one piece"

func newContractClient(t *testing.T) (*Client, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	return New(""), ctx
}

func TestContractSearch(t *testing.T) {
	c, ctx := newContractClient(t)

	refs, err := c.Search(ctx, probeSeries)
	if err != nil {
		t.Fatalf("Search(%q): %v", probeSeries, err)
	}
	if len(refs) == 0 {
		t.Fatalf("Search(%q) returned no results — upstream search or the envelope shape changed", probeSeries)
	}
	first := refs[0]
	if first.MangaBakaID == 0 {
		t.Error("SeriesRef.MangaBakaID is 0 — the `id` field moved or changed type")
	}
	if first.Title == "" {
		t.Error("SeriesRef.Title is empty — the `title` field moved")
	}
	if first.CoverURL == "" {
		t.Error("SeriesRef.CoverURL is empty — the `cover.x350.x1` / `cover.raw.url` shape changed")
	}
}

func TestContractGetSeries(t *testing.T) {
	c, ctx := newContractClient(t)

	refs, err := c.Search(ctx, probeSeries)
	if err != nil || len(refs) == 0 {
		t.Fatalf("Search(%q) for probe id: %v (%d results)", probeSeries, err, len(refs))
	}
	id := refs[0].MangaBakaID

	s, err := c.GetSeries(ctx, id)
	if err != nil {
		t.Fatalf("GetSeries(%d): %v", id, err)
	}
	if s.Title == "" {
		t.Error("Series.Title is empty")
	}
	if s.Year == 0 {
		t.Error("Series.Year is 0 — the `year` field moved or changed type")
	}
	if s.Status == "" {
		t.Error("Series.Status is empty — the `status` field moved")
	}
	if len(s.Authors) == 0 {
		t.Error("Series.Authors is empty — `authors` is no longer a string array")
	}
	// `total_chapters` and `final_volume` are stringly typed upstream;
	// atoiLoose swallows a type change silently, so assert the value.
	if s.TotalChapters == 0 {
		t.Error("Series.TotalChapters is 0 — `total_chapters` is no longer a numeric string")
	}
	if len(s.Titles) == 0 {
		t.Error("Series.Titles is empty — the `titles` array moved (upstream also ships `secondary_titles`)")
	}
	if len(s.RawJSON) == 0 {
		t.Error("Series.RawJSON is empty — the envelope no longer carries `data`")
	}
}

func TestContractCollectionsAndWorks(t *testing.T) {
	c, ctx := newContractClient(t)

	refs, err := c.Search(ctx, probeSeries)
	if err != nil || len(refs) == 0 {
		t.Fatalf("Search(%q) for probe id: %v (%d results)", probeSeries, err, len(refs))
	}
	id := refs[0].MangaBakaID

	cols, err := c.GetCollections(ctx, id)
	if err != nil {
		t.Fatalf("GetCollections(%d): %v", id, err)
	}
	if len(cols) == 0 {
		t.Fatalf("GetCollections(%d) is empty — add-series would monitor nothing", id)
	}
	var withLang int
	for _, col := range cols {
		if col.ProviderID == "" {
			t.Error("Collection.ProviderID is empty — the `id` field moved")
		}
		if col.Language != "" {
			withLang++
		}
	}
	if withLang == 0 {
		t.Error("no collection has a Language — the `language.iso` shape changed")
	}

	// GetWorks keys by collection id and follows pagination; both are
	// contract surface, so assert the join actually lands.
	works, err := c.GetWorks(ctx, id)
	if err != nil {
		t.Fatalf("GetWorks(%d): %v", id, err)
	}
	if len(works) == 0 {
		t.Fatalf("GetWorks(%d) is empty — the `collections` back-reference on works changed", id)
	}
	var total, sequenced int
	for _, col := range cols {
		for _, w := range works[col.ProviderID] {
			total++
			if w.SequenceNum != 0 {
				sequenced++
			}
		}
	}
	if total == 0 {
		t.Fatal("no work joined to any collection — works[].collections[].id no longer matches collection ids")
	}
	if sequenced == 0 {
		t.Error("no work has a SequenceNum — `sequence_numeric`/`sequence_string` changed")
	}
	// One Piece runs well past a single 25-row page; a collapse to one
	// page means pagination.next stopped being followed.
	if total < 30 {
		t.Errorf("only %d works joined; expected pagination to fetch several pages", total)
	}
}
