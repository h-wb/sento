package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
)

// factsClaimsFixture is a series whose default version judges files on a
// Digital flag (100) and a known group (150), with upgrades on below 250.
func factsClaimsFixture(t *testing.T) (*Library, *store.Store, int64, store.Edition, []store.Book, store.CollectedVersion) {
	t.Helper()
	lib, st, sid, col, ws, version := modelFixture(t)
	ctx := context.Background()
	digital, err := st.AddFormat(ctx, profile.Format{Name: "Digital", Conditions: []profile.Condition{{Type: profile.CondFlag, Value: "digital"}}})
	if err != nil {
		t.Fatal(err)
	}
	group, err := st.AddFormat(ctx, profile.Format{Name: "Known group", Conditions: []profile.Condition{{Type: profile.CondGroup, Value: "LuCaZ"}}})
	if err != nil {
		t.Fatal(err)
	}
	prof := profile.Profile{
		Name: "Standard", UpgradesEnabled: true, UpgradeUntilScore: 250, MinUpgradeScore: 50,
		MinFormatScore: profile.MinScoreOff, FormatScores: map[int64]int{digital: 100, group: 150},
	}
	if prof.ID, err = st.AddProfile(ctx, prof); err != nil {
		t.Fatal(err)
	}
	version.ProfileID = prof.ID
	if _, err = lib.SaveVersion(ctx, sid, version); err != nil {
		t.Fatal(err)
	}
	return lib, st, sid, col, ws, version
}

func addFactFile(t *testing.T, st *store.Store, sid int64, col store.Edition, w store.Book, version store.CollectedVersion, name string, facts, claims store.AssetFacts) int64 {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("archive bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	fid, err := st.AddFile(context.Background(), store.File{SeriesID: sid, Path: path, Size: 13, Facts: facts, Claims: claims}, nil,
		[]store.BookLink{{Ledger: col.Type, EditionID: col.ID, Number: w.SequenceNum, VersionID: version.ID, BookID: w.ID}})
	if err != nil {
		t.Fatal(err)
	}
	return fid
}

func bookStatus(t *testing.T, lib *Library, sid int64, version store.CollectedVersion, w store.Book) BookStatus {
	t.Helper()
	status, err := lib.BooksByVersion(context.Background(), sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, sl := range status[version.ID] {
		if sl.Book.ID == w.ID {
			return sl
		}
	}
	t.Fatal("book missing from its version's status")
	return BookStatus{}
}

// A file organized to a bare name keeps the score its release earned: the
// claims it was imported with back the digital flag and group its name no
// longer carries, so a good copy is not flagged upgrade-eligible (#15).
func TestOrganizedFileKeepsItsReleaseScoreFromClaims(t *testing.T) {
	lib, st, sid, col, ws, version := factsClaimsFixture(t)
	addFactFile(t, st, sid, col, ws[0], version, "Volume 1.cbz",
		store.AssetFacts{Title: "Volume 1.cbz", FactsKnown: true, Protocol: "torrent"},
		store.AssetFacts{Title: "[LuCaZ] ONE PIECE v01 [Digital]", Group: "LuCaZ", FactsKnown: true, Protocol: "torrent", Digital: true, Flags: map[string]bool{"digital": true}})
	sl := bookStatus(t, lib, sid, version, ws[0])
	if sl.CurrentScore != 250 {
		t.Fatalf("organized file lost its release score: currentScore = %d", sl.CurrentScore)
	}
	if sl.State != StateOwned || sl.Wanted {
		t.Fatalf("good copy flagged for upgrade: state = %s, wanted = %v, reason = %q", sl.State, sl.Wanted, sl.Reason)
	}
}

// A file without claims scores from its own filename only: nothing is
// invented, and a bare name stays conservatively upgrade-eligible.
func TestClaimlessFileStillScoresFromItsFilenameOnly(t *testing.T) {
	lib, st, sid, col, ws, version := factsClaimsFixture(t)
	addFactFile(t, st, sid, col, ws[0], version, "Volume 1.cbz",
		store.AssetFacts{Title: "Volume 1.cbz", FactsKnown: true, Protocol: "torrent"},
		store.AssetFacts{})
	sl := bookStatus(t, lib, sid, version, ws[0])
	if sl.CurrentScore != 0 {
		t.Fatalf("claimless file scored %d, want 0", sl.CurrentScore)
	}
	if sl.State != StateUpgradeEligible || !sl.Wanted {
		t.Fatalf("claimless bare file not conservatively eligible: state = %s, wanted = %v", sl.State, sl.Wanted)
	}
}

// The file's own facts win over the release's claims: a group its name
// states is not overwritten by the offer's, only the missing digital flag
// is backed.
func TestFileFactsWinOverReleaseClaims(t *testing.T) {
	lib, st, sid, col, ws, version := factsClaimsFixture(t)
	addFactFile(t, st, sid, col, ws[0], version, "ONE PIECE v01 [SomeScans].cbz",
		store.AssetFacts{Title: "ONE PIECE v01 [SomeScans].cbz", Group: "SomeScans", FactsKnown: true, Protocol: "torrent"},
		store.AssetFacts{Title: "[LuCaZ] ONE PIECE v01 [Digital]", Group: "LuCaZ", FactsKnown: true, Protocol: "torrent", Digital: true, Flags: map[string]bool{"digital": true}})
	sl := bookStatus(t, lib, sid, version, ws[0])
	if sl.CurrentScore != 100 {
		t.Fatalf("observed group overridden by claims: currentScore = %d, want 100 (digital only)", sl.CurrentScore)
	}
	if sl.State != StateUpgradeEligible || !sl.Wanted {
		t.Fatalf("below-cutoff copy not flagged: state = %s, wanted = %v", sl.State, sl.Wanted)
	}
}

// Displacement judges the owner on the same evidence: claims alone make an
// otherwise-unknown owner known, and an owner with neither stays put.
func TestAutoUpgradeJudgesOwnerOnClaimsWhenFactsUnknown(t *testing.T) {
	prof := &profile.Profile{UpgradesEnabled: true, MinUpgradeScore: 10}
	if !autoUpgrade(prof, store.BookOwner{Claims: store.AssetFacts{FactsKnown: true}}, 0, 100) {
		t.Fatal("claims-known owner treated as unknown")
	}
	if autoUpgrade(prof, store.BookOwner{}, 0, 100) {
		t.Fatal("unknown owner displaced automatically")
	}
}
