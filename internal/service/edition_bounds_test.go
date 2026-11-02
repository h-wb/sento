package service

import (
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/store"
)

func TestCompletedEditionDoesNotClipAnotherNumberingToItsBooks(t *testing.T) {
	edition := store.Edition{
		ID: 1, Type: LedgerVolume, Language: "en", EditionName: "Full Color Collection Edition",
		NumberingSource: "catalog", LastNumber: 800, CountMain: 8,
		Collections: []store.Collection{
			{SyncState: "linked", Status: "completed", CountMain: 8, Medium: "digital"},
			{SyncState: "linked", Status: "completed", CountMain: 8, Medium: "paperback"},
		},
	}
	for _, evidence := range []int64{0, edition.ID} {
		p := parse.ParseAnchored("Example - Full Color v01-10 (Digital)", parse.ChannelTorrent, []string{"Example"})
		verdict, reason := newShelf([]store.Edition{edition}).judge(p, edition, evidence)
		if verdict != fitUnsure || !strings.Contains(reason, "ends at 8") {
			t.Fatalf("clipped ten-volume release to eight-book edition: %v, %s", verdict, reason)
		}
	}
	p := parse.ParseAnchored("Example - Full Color v01-08 (Digital)", parse.ChannelTorrent, []string{"Example"})
	if verdict, reason := newShelf([]store.Edition{edition}).judge(p, edition, 0); verdict != fitYes {
		t.Fatalf("refused matching numbering: %v, %s", verdict, reason)
	}
	p = parse.ParseAnchored("Example - Full Color v01-07 COMPLETE (Digital)", parse.ChannelTorrent, []string{"Example"})
	if verdict, reason := newShelf([]store.Edition{edition}).judge(p, edition, 0); verdict != fitUnsure {
		t.Fatalf("accepted conflicting completion claim: %v, %s", verdict, reason)
	}
	p = parse.ParseAnchored("Example - Full Color v01-07 (Digital)", parse.ChannelTorrent, []string{"Example"})
	if verdict, reason := newShelf([]store.Edition{edition}).judge(p, edition, 0); verdict != fitYes {
		t.Fatalf("refused a partial pack in the same numbering: %v, %s", verdict, reason)
	}
	for _, status := range []string{"unknown", "releasing"} {
		e := edition
		e.Collections = []store.Collection{{SyncState: "linked", Status: status, CountMain: 8}}
		p := parse.ParseAnchored("Example - Full Color v01-10 (Digital)", parse.ChannelTorrent, []string{"Example"})
		if verdict, reason := newShelf([]store.Edition{e}).judge(p, e, 0); verdict != fitYes {
			t.Fatalf("treated %s catalog line as a closed numbering: %v, %s", status, verdict, reason)
		}
	}
	for _, collections := range [][]store.Collection{
		{{SyncState: "linked", Status: "completed", CountMain: 8}, {SyncState: "linked", Status: "unknown", CountMain: 8}},
		{{SyncState: "linked", Status: "completed", CountMain: 7}},
		{{SyncState: "stale", Status: "completed", CountMain: 8}},
	} {
		e := edition
		e.Collections = collections
		if completedNumbering(e) {
			t.Fatalf("accepted incomplete or conflicting completion evidence: %+v", collections)
		}
	}
}
