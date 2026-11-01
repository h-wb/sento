package service

import (
	"context"
	"maps"

	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
)

// Replacement is one book whose owning file was displaced by a better one.
type Replacement struct {
	Path      string `json:"path"`
	FileID    int64  `json:"fileId"`
	BookID    int64  `json:"bookId"`
	FromScore int    `json:"fromScore"`
	ToScore   int    `json:"toScore"`
}

// FileMappingResult reports what a mapping save did beyond recording the
// file's own claims: which existing owners it displaced.
type FileMappingResult struct {
	Replaced []Replacement `json:"replaced,omitempty"`
}

// ApplyUpgrades maps the file to its books (atomically, like MapFile) and
// then applies the upgrade path per book.
func ApplyUpgrades(ctx context.Context, st *store.Store, seriesID, fileID int64, slots []store.BookRef, force bool) (*FileMappingResult, error) {
	if err := st.MapFile(ctx, seriesID, fileID, slots); err != nil {
		return nil, err
	}
	return DisplaceOwners(ctx, st, seriesID, fileID, slots, force)
}

// DisplaceOwners applies the upgrade path per claimed book, Sonarr-style:
// when another verified, present file already holds the book, a
// better-scoring newcomer takes the hold and the old file is released (it
// stays on disk, now an extra). Automatic replacement follows the version
// profile's upgrade rules and never touches pinned holds; force — a person's
// explicit choice in review — overrides both. A file that does not outscore
// the current owner is kept as an extra instead of displacing it.
func DisplaceOwners(ctx context.Context, st *store.Store, seriesID, fileID int64, slots []store.BookRef, force bool) (*FileMappingResult, error) {
	profiles, err := NewProfiles(st).seriesProfiles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	files, err := st.ListFiles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	byID := map[int64]store.File{}
	for _, f := range files {
		byID[f.ID] = f
	}
	newFile, ok := byID[fileID]
	if !ok {
		return nil, store.ErrNotFound
	}
	res := &FileMappingResult{}
	seen := map[int64]bool{}
	for _, sl := range slots {
		if seen[sl.BookID] {
			continue
		}
		seen[sl.BookID] = true
		version, err := st.GetVersion(ctx, sl.VersionID)
		if err != nil {
			return nil, err
		}
		col, err := st.GetEdition(ctx, sl.EditionID)
		if err != nil {
			return nil, err
		}
		prof := profiles.For(*version)
		owners, err := st.BookOwners(ctx, sl.BookID)
		if err != nil {
			return nil, err
		}
		toScore := scoredFacts(newFile.Facts, "manual", *col, prof, profiles.Formats)
		for _, o := range owners {
			if o.FileID == fileID || !o.Verified || !o.Present {
				continue
			}
			fromScore := scoredFacts(o.Facts, o.Evidence, *col, prof, profiles.Formats)
			if !force && !autoUpgrade(prof, o, fromScore, toScore) {
				continue
			}
			if err := st.ReleaseBook(ctx, o.FileID, sl.BookID); err != nil {
				return nil, err
			}
			res.Replaced = append(res.Replaced, Replacement{
				FileID: o.FileID, Path: o.Path, BookID: sl.BookID,
				FromScore: fromScore, ToScore: toScore,
			})
		}
	}
	return res, nil
}

// autoUpgrade is the profile's rule for displacing a current owner without
// a person asking: upgrades on, owner's facts known, not pinned, below the
// until-score ceiling, and beaten by at least MinUpgradeScore.
func autoUpgrade(prof *profile.Profile, owner store.BookOwner, fromScore, toScore int) bool {
	if prof == nil || !prof.UpgradesEnabled || owner.Pinned {
		return false
	}
	if !owner.Facts.FactsKnown {
		return false
	}
	if prof.UpgradeUntilScore != 0 && fromScore >= prof.UpgradeUntilScore {
		return false
	}
	return toScore-fromScore >= max(prof.MinUpgradeScore, 1)
}

// scoredFacts is the file's profile score, with the same manual-mapping
// adjustments judgeCardFiles makes: a confirmed mapping counts the edition's
// language, and confirming a color edition confirms color.
func scoredFacts(f store.AssetFacts, evidence string, col store.Edition, prof *profile.Profile, formats []profile.Format) int {
	if evidence == "manual" {
		f.Language = col.Language
		if parse.ReadEdition(col.EditionName).Color {
			f.Flags = maps.Clone(f.Flags)
			if f.Flags == nil {
				f.Flags = map[string]bool{}
			}
			f.Flags["colored"] = true
		}
	}
	return assetJudgement(f, prof, formats).Score
}
