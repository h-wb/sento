package service

import (
	"context"
	"errors"
	"testing"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
)

func TestFileScoreTracksCurrentProfileAndFileCorrections(t *testing.T) {
	t.Parallel()
	lib, st, sid, col, books, version := factsClaimsFixture(t)
	ctx := context.Background()
	fid := addFactFile(t, st, sid, col, books[0], version, "Volume 1.cbz",
		store.AssetFacts{Title: "Volume 1.cbz", FactsKnown: true},
		store.AssetFacts{FactsKnown: true, Group: "LuCaZ", Flags: map[string]bool{"digital": true}})
	check := func(want int, matches int) *FileScore {
		t.Helper()
		score, err := lib.FileScore(ctx, sid, version.ID, fid)
		if err != nil {
			t.Fatal(err)
		}
		if score.Score != want || len(score.Formats) != matches || !score.FactsKnown || score.ProfileID != version.ProfileID || score.ProfileName != "Standard" {
			t.Fatalf("unexpected score: %+v", score)
		}
		sum := 0
		for _, f := range score.Formats {
			sum += f.Score
		}
		if sum != score.Score || bookStatus(t, lib, sid, version, books[0]).CurrentScore != score.Score {
			t.Fatalf("breakdown or ledger disagrees with file score: %+v", score)
		}
		return score
	}
	check(250, 2)
	prof, err := st.GetProfile(ctx, version.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	formats, err := st.ListFormats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range formats {
		if f.Name == "Known group" {
			prof.FormatScores[f.ID] = -100
		}
	}
	if err = st.UpdateProfile(ctx, *prof); err != nil {
		t.Fatal(err)
	}
	check(0, 2) // Negative contributions remain visible when the total is zero.
	empty := ""
	if err = st.CorrectFileFacts(ctx, sid, fid, store.AssetFactPatch{Group: &empty}); err != nil {
		t.Fatal(err)
	}
	check(100, 1) // Manually cleared facts must not inherit the offer's group.

	other, err := st.AddProfile(ctx, profile.Profile{Name: "Another profile", MinFormatScore: profile.MinScoreOff})
	if err != nil {
		t.Fatal(err)
	}
	otherVersion, err := lib.SaveVersion(ctx, sid, store.CollectedVersion{EditionID: col.ID, ProfileID: other})
	if err != nil {
		t.Fatal(err)
	}
	score, err := lib.FileScore(ctx, sid, otherVersion, fid)
	if err != nil || score.ProfileID != other || score.Score != 0 || len(score.Formats) != 1 {
		t.Fatalf("selected version did not use its own profile: %+v, %v", score, err)
	}
	version.ProfileID = 0
	if _, err = lib.SaveVersion(ctx, sid, version); err != nil {
		t.Fatal(err)
	}
	if err = st.SetSeriesProfile(ctx, sid, prof.ID); err != nil {
		t.Fatal(err)
	}
	score, err = lib.FileScore(ctx, sid, version.ID, fid)
	if err != nil || score.ProfileID != prof.ID || score.Score != 100 {
		t.Fatalf("series profile not inherited: %+v, %v", score, err)
	}
	if err = st.SetSeriesProfile(ctx, sid, 0); err != nil {
		t.Fatal(err)
	}
	if err = NewProfiles(st).SetDefaultProfile(ctx, other); err != nil {
		t.Fatal(err)
	}
	score, err = lib.FileScore(ctx, sid, version.ID, fid)
	if err != nil || score.ProfileID != other || score.Score != 0 {
		t.Fatalf("instance profile not inherited: %+v, %v", score, err)
	}
}

func TestFileScoreRejectsOtherSeriesAndMissingFiles(t *testing.T) {
	t.Parallel()
	lib, st, sid, col, books, version := factsClaimsFixture(t)
	fid := addFactFile(t, st, sid, col, books[0], version, "Volume 1.cbz", store.AssetFacts{}, store.AssetFacts{})
	for _, ids := range [][3]int64{{sid + 1000, version.ID, fid}, {sid, version.ID, fid + 1000}, {sid, version.ID + 1000, fid}} {
		if _, err := lib.FileScore(context.Background(), ids[0], ids[1], ids[2]); !errors.Is(err, apperr.ErrNotFound) {
			t.Fatalf("FileScore%v = %v, want not found", ids, err)
		}
	}
}

func TestFileScoreUsesConfirmedEditionAndMarksUnknownFacts(t *testing.T) {
	t.Parallel()
	lib, st, sid, _, _, _ := modelFixture(t)
	ctx := context.Background()
	colorID, err := st.AddFormat(ctx, profile.Format{Name: "Colored", Conditions: []profile.Condition{{Type: profile.CondFlag, Value: "colored"}}})
	if err != nil {
		t.Fatal(err)
	}
	pid, err := st.AddProfile(ctx, profile.Profile{Name: "Color", FormatScores: map[int64]int{colorID: 75}, Required: []int64{colorID}, MinFormatScore: profile.MinScoreOff})
	if err != nil {
		t.Fatal(err)
	}
	eid, err := st.AddCustomEdition(ctx, store.Edition{SeriesID: sid, EditionName: "Full Color Edition", Language: "fr", Type: "volume"}, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	vid, err := lib.SaveVersion(ctx, sid, store.CollectedVersion{EditionID: eid, ProfileID: pid})
	if err != nil {
		t.Fatal(err)
	}
	fid, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: "unknown.cbz", Facts: store.AssetFacts{Language: "en"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	score, err := lib.FileScore(ctx, sid, vid, fid)
	if err != nil || score.FactsKnown || score.Score != 0 || score.Reason == "" || len(score.Formats) != 0 {
		t.Fatalf("unmapped unknown file: %+v, %v", score, err)
	}
	books, err := st.ListBooks(ctx, eid)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.MapFile(ctx, sid, fid, []store.BookRef{{VersionID: vid, EditionID: eid, BookID: books[0].ID, Ledger: "volume", Number: 100}}); err != nil {
		t.Fatal(err)
	}
	score, err = lib.FileScore(ctx, sid, vid, fid)
	if err != nil || score.Score != 75 || score.Reason != "" || len(score.Formats) != 1 {
		t.Fatalf("confirmed edition context not used: %+v, %v", score, err)
	}
}
