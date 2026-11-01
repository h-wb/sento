package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
)

// The upgrade path: mapping a file to a book another verified file holds
// displaces the owner when the newcomer outscores it by the profile's
// margin; pinned holds and disabled upgrades are left alone, and an
// explicit force replaces regardless.
func TestUpgradePathDisplacesOnlyBetterFiles(t *testing.T) {
	lib, st, sid, standard, books, version := modelFixture(t)
	ctx := context.Background()
	fidDigital, err := st.AddFormat(ctx, profile.Format{Name: "Digital", Conditions: []profile.Condition{{Type: profile.CondFlag, Value: "digital"}}})
	if err != nil {
		t.Fatal(err)
	}
	prof := profile.Profile{
		Name: "Standard", UpgradesEnabled: true, MinUpgradeScore: 1,
		MinFormatScore: profile.MinScoreOff, FormatScores: map[int64]int{fidDigital: 100},
	}
	prof.ID, err = st.AddProfile(ctx, prof)
	if err != nil {
		t.Fatal(err)
	}
	version.ProfileID = prof.ID
	if _, err = lib.SaveVersion(ctx, sid, version); err != nil {
		t.Fatal(err)
	}

	addFile := func(name string, facts store.AssetFacts, links []store.BookLink) int64 {
		t.Helper()
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte("archive bytes"), 0o600); err != nil {
			t.Fatal(err)
		}
		facts.Title = name
		facts.FactsKnown = true
		fid, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: path, Size: 13, Facts: facts}, nil, links)
		if err != nil {
			t.Fatal(err)
		}
		return fid
	}
	slot := store.BookRef{Ledger: standard.Type, EditionID: standard.ID, Number: books[0].SequenceNum, VersionID: version.ID, BookID: books[0].ID}
	link := store.BookLink{Ledger: standard.Type, EditionID: standard.ID, Number: books[0].SequenceNum, VersionID: version.ID, BookID: books[0].ID}

	// The current owner: plain facts, score 0 under the profile's formats.
	oldID := addFile("ONE PIECE v01.cbz", store.AssetFacts{}, []store.BookLink{link})

	owners := func() []store.BookOwner {
		t.Helper()
		os, err := st.BookOwners(ctx, books[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		return os
	}

	// A better file (Digital flag, +100): mapping it displaces the old one.
	better := addFile("ONE PIECE v01 (Digital).cbz", store.AssetFacts{Flags: map[string]bool{"digital": true}}, nil)
	res, err := ApplyUpgrades(ctx, st, sid, better, []store.BookRef{slot}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Replaced) != 1 || res.Replaced[0].FileID != oldID || res.Replaced[0].FromScore != 0 || res.Replaced[0].ToScore != 100 {
		t.Fatalf("expected old file displaced 0→100, got %+v", res.Replaced)
	}
	for _, o := range owners() {
		if o.FileID == oldID {
			t.Fatalf("old file still holds the book: %+v", o)
		}
	}

	// A worse file (score 0 against the digital owner's 100) does not
	// displace; it is kept as an extra.
	worse := addFile("ONE PIECE v01 [repack].cbz", store.AssetFacts{}, nil)
	res, err = ApplyUpgrades(ctx, st, sid, worse, []store.BookRef{slot}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Replaced) != 0 {
		t.Fatalf("worse file displaced the owner: %+v", res.Replaced)
	}
	still := false
	for _, o := range owners() {
		if o.FileID == better {
			still = true
		}
	}
	if !still {
		t.Fatal("better file lost the book to a worse one")
	}

	// Force replaces even a better current owner.
	res, err = ApplyUpgrades(ctx, st, sid, worse, []store.BookRef{slot}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Replaced) != 1 || res.Replaced[0].FileID != better {
		t.Fatalf("force did not displace the better file: %+v", res.Replaced)
	}

	// With upgrades disabled, nothing is displaced automatically.
	prof.UpgradesEnabled = false
	if err := st.UpdateProfile(ctx, prof); err != nil {
		t.Fatal(err)
	}
	best := addFile("ONE PIECE v01 (Digital) (Premium).cbz", store.AssetFacts{Flags: map[string]bool{"digital": true, "premium": true}}, nil)
	res, err = ApplyUpgrades(ctx, st, sid, best, []store.BookRef{slot}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Replaced) != 0 {
		t.Fatalf("displaced with upgrades disabled: %+v", res.Replaced)
	}
	_ = lib
}
