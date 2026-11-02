package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

func TestUpgradeKeepsBlackAndWhiteAndColoredCopies(t *testing.T) {
	lib, st, sid, edition, books, bw := modelFixture(t)
	ctx := context.Background()
	im := NewImporter(st, provider.NewRegistry(), t.TempDir(), nil)
	colored, err := st.AddFormat(ctx, profile.Format{Name: "Colored", Conditions: []profile.Condition{{Type: profile.CondFlag, Value: "colored"}}})
	if err != nil {
		t.Fatal(err)
	}
	digital, err := st.AddFormat(ctx, profile.Format{Name: "Digital", Conditions: []profile.Condition{{Type: profile.CondFlag, Value: "digital"}}})
	if err != nil {
		t.Fatal(err)
	}
	premium, err := st.AddFormat(ctx, profile.Format{Name: "Premium", Conditions: []profile.Condition{{Type: profile.CondFlag, Value: "premium"}}})
	if err != nil {
		t.Fatal(err)
	}
	addProfile := func(p profile.Profile) int64 {
		t.Helper()
		p.MinFormatScore, p.UpgradesEnabled, p.MinUpgradeScore = profile.MinScoreOff, true, 1
		id, err := st.AddProfile(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	bw.ProfileID = addProfile(profile.Profile{Name: "Black and white", Refused: []int64{colored}, FormatScores: map[int64]int{digital: 100, premium: 200}})
	bw.Monitored = true
	if _, err := lib.SaveVersion(ctx, sid, bw); err != nil {
		t.Fatal(err)
	}
	color := store.CollectedVersion{EditionID: edition.ID, Monitored: true, ProfileID: addProfile(profile.Profile{Name: "Colored", Required: []int64{colored}, FormatScores: map[int64]int{digital: 100}})}
	color.ID, err = lib.SaveVersion(ctx, sid, color)
	if err != nil {
		t.Fatal(err)
	}
	add := func(name string, flags map[string]bool) int64 {
		t.Helper()
		path := filepath.Join(im.libraryRoot, name)
		writeCBZ(t, path, name)
		id, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: path, Facts: store.AssetFacts{Title: name, FactsKnown: true, Flags: flags}}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	colorSlot, bwSlot := modelRef(edition, books[0], color), modelRef(edition, books[0], bw)
	colorFile := add("ONE PIECE v01 (Colored).cbz", map[string]bool{"digital": true, "colored": true})
	if _, err := im.ApplyUpgrades(ctx, sid, colorFile, []store.BookRef{colorSlot}, false); err != nil {
		t.Fatal(err)
	}
	if err := st.PinCoverage(ctx, sid, store.AssetCoverage{FileID: colorFile, VersionID: color.ID, BookID: books[0].ID, Pinned: true}); err != nil {
		t.Fatal(err)
	}
	if err := im.OrganizeReviewedFile(ctx, sid, colorFile); err != nil {
		t.Fatal(err)
	}

	// Equal scores and the other copy's pin must not block this copy.
	bwFile := add("ONE PIECE v01 (Digital).cbz", map[string]bool{"digital": true})
	result, err := im.ApplyUpgrades(ctx, sid, bwFile, []store.BookRef{bwSlot}, false)
	if err != nil || len(result.Replaced) != 0 || len(result.Kept) != 0 {
		t.Fatalf("another copy interfered: %+v, %v", result, err)
	}
	if err := im.OrganizeReviewedFile(ctx, sid, bwFile); err != nil {
		t.Fatal(err)
	}
	assertOwners := func(bwID int64) {
		t.Helper()
		states, err := lib.BooksByVersion(ctx, sid)
		if err != nil {
			t.Fatal(err)
		}
		for vid, fid := range map[int64]int64{bw.ID: bwID, color.ID: colorFile} {
			book := states[vid][0]
			if len(book.FileIDs) != 1 || book.FileIDs[0] != fid {
				t.Fatalf("copy %d not fulfilled independently: %+v", vid, book)
			}
		}
	}
	assertOwners(bwFile)

	// An upgrade of the default copy preserves the pinned colored copy.
	better := add("ONE PIECE v01 (Premium).cbz", map[string]bool{"digital": true, "premium": true})
	result, err = im.ApplyUpgrades(ctx, sid, better, []store.BookRef{bwSlot}, false)
	if err != nil || len(result.Replaced) != 1 || result.Replaced[0].FileID != bwFile || result.Replaced[0].VersionID != bw.ID {
		t.Fatalf("wrong copy displaced: %+v, %v", result, err)
	}
	if err := im.OrganizeReviewedFile(ctx, sid, better); err != nil {
		t.Fatal(err)
	}
	assertOwners(better)
	// Even force cannot let a profile-refused file evict the accepted copy.
	refused := add("ONE PIECE v01 (Colored Premium).cbz", map[string]bool{"digital": true, "premium": true, "colored": true})
	result, err = im.ApplyUpgrades(ctx, sid, refused, []store.BookRef{bwSlot}, true)
	if err != nil || len(result.Replaced) != 0 {
		t.Fatalf("refused file displaced an accepted copy: %+v, %v", result, err)
	}
	assertOwners(better)
	files, err := st.ListFiles(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[int64]string{}
	for _, f := range files {
		paths[f.ID] = f.Path
		if _, err := os.Stat(f.Path); err != nil {
			t.Fatalf("lost bytes of file %d: %v", f.ID, err)
		}
	}
	if filepath.Dir(paths[better]) == filepath.Dir(paths[colorFile]) || strings.Contains(paths[better], "/review/") || strings.Contains(paths[colorFile], "/review/") || !strings.Contains(paths[bwFile], "/review/") {
		t.Fatalf("copies not placed independently: %v", paths)
	}
	contents, err := st.Contents(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	preserved := false
	for _, c := range contents {
		if c.FileID == bwFile && c.BookID == books[0].ID && c.Verified {
			preserved = true
		}
	}
	if !preserved {
		t.Fatal("upgrade erased the displaced file's verified contents")
	}
	// A later copy can reuse that known asset without another download.
	originalProfile := addProfile(profile.Profile{Name: "Original digital", Refused: []int64{colored, premium}})
	original := store.CollectedVersion{EditionID: edition.ID, Monitored: true, ProfileID: originalProfile}
	original.ID, err = lib.SaveVersion(ctx, sid, original)
	if err != nil {
		t.Fatal(err)
	}
	states, err := lib.BooksByVersion(ctx, sid)
	if err != nil || len(states[original.ID][0].FileIDs) != 1 || states[original.ID][0].FileIDs[0] != bwFile {
		t.Fatalf("later copy could not reuse verified contents: %+v, %v", states[original.ID], err)
	}
	if err := im.OrganizeReviewedFile(ctx, sid, bwFile); err != nil {
		t.Fatal(err)
	}
}

func TestUpgradeScopesPinsAndBothVersionsOfTheSameBook(t *testing.T) {
	lib, st, sid, edition, books, first := modelFixture(t)
	ctx := context.Background()
	im := NewImporter(st, provider.NewRegistry(), t.TempDir(), nil)
	digital, err := st.AddFormat(ctx, profile.Format{Name: "Digital", Conditions: []profile.Condition{{Type: profile.CondFlag, Value: "digital"}}})
	if err != nil {
		t.Fatal(err)
	}
	versions := []store.CollectedVersion{first, {EditionID: edition.ID}}
	for i := range versions {
		versions[i].ProfileID, err = st.AddProfile(ctx, profile.Profile{Name: []string{"Primary", "Archive"}[i], UpgradesEnabled: true, MinUpgradeScore: 1, MinFormatScore: profile.MinScoreOff, FormatScores: map[int64]int{digital: 100}})
		if err != nil {
			t.Fatal(err)
		}
		versions[i].Monitored = true
		versions[i].ID, err = lib.SaveVersion(ctx, sid, versions[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	slots := []store.BookRef{modelRef(edition, books[0], versions[0]), modelRef(edition, books[0], versions[1])}
	add := func(name string, digital bool) int64 {
		t.Helper()
		path := filepath.Join(im.libraryRoot, name)
		writeCBZ(t, path, name)
		id, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: path, Facts: store.AssetFacts{FactsKnown: true, Flags: map[string]bool{"digital": digital}}}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	old := add("ONE PIECE v01.cbz", false)
	if _, err := im.ApplyUpgrades(ctx, sid, old, slots, false); err != nil {
		t.Fatal(err)
	}
	if err := st.PinCoverage(ctx, sid, store.AssetCoverage{FileID: old, VersionID: versions[1].ID, BookID: books[0].ID, Pinned: true}); err != nil {
		t.Fatal(err)
	}
	better := add("ONE PIECE v01 (Digital).cbz", true)
	result, err := im.ApplyUpgrades(ctx, sid, better, slots, false)
	if err != nil || len(result.Replaced) != 1 || len(result.Kept) != 1 {
		t.Fatalf("both copies were not evaluated: %+v, %v", result, err)
	}
	if result.Replaced[0].VersionID != versions[0].ID || result.Kept[0].VersionID != versions[1].ID {
		t.Fatalf("pin crossed copies: %+v", result)
	}
	for vid, want := range map[int64]int64{versions[0].ID: better, versions[1].ID: old} {
		owners, err := st.BookOwners(ctx, vid, books[0].ID)
		if err != nil || len(owners) != 1 || owners[0].FileID != want {
			t.Fatalf("copy %d owners: %+v, %v", vid, owners, err)
		}
	}
}
