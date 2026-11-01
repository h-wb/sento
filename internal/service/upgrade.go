package service

import (
	"context"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"

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

// KeptClaim is one book the mapped file did not take because a current
// holder outranks it (or is pinned): the file stays an extra instead.
type KeptClaim struct {
	BookID       int64  `json:"bookId"`
	HolderFileID int64  `json:"holderFileId"`
	HolderPath   string `json:"holderPath"`
	HolderScore  int    `json:"holderScore"`
	Score        int    `json:"score"`
}

// FileMappingResult reports what a mapping save did beyond recording the
// file's own claims: which existing owners it displaced, and which books
// it left to their current holder.
type FileMappingResult struct {
	Replaced []Replacement `json:"replaced,omitempty"`
	Kept     []KeptClaim   `json:"kept,omitempty"`
}

// ApplyUpgrades maps the file to its books (atomically, like MapFile) and
// then applies the upgrade path per book.
func (im *Importer) ApplyUpgrades(ctx context.Context, seriesID, fileID int64, slots []store.BookRef, force bool) (*FileMappingResult, error) {
	if err := im.store.MapFile(ctx, seriesID, fileID, slots); err != nil {
		return nil, err
	}
	return im.DisplaceOwners(ctx, seriesID, fileID, slots, force)
}

// DisplaceOwners enforces one owner per book, Sonarr-style: when another
// verified, present file already holds a claimed book, the better-scoring
// file takes the hold and the other is released and moved out of the final
// directory to review/ (its bytes stay; nothing is deleted). Automatic
// replacement follows the version profile's upgrade rules and never touches
// pinned holds; force — a person's explicit choice in review — overrides
// both. A file that does not outscore the current holder keeps no claim on
// the book and stays an extra.
func (im *Importer) DisplaceOwners(ctx context.Context, seriesID, fileID int64, slots []store.BookRef, force bool) (*FileMappingResult, error) {
	profiles, err := NewProfiles(im.store).seriesProfiles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	files, err := im.store.ListFiles(ctx, seriesID)
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
		version, err := im.store.GetVersion(ctx, sl.VersionID)
		if err != nil {
			return nil, err
		}
		col, err := im.store.GetEdition(ctx, sl.EditionID)
		if err != nil {
			return nil, err
		}
		prof := profiles.For(*version)
		owners, err := im.store.BookOwners(ctx, sl.BookID)
		if err != nil {
			return nil, err
		}
		toScore := scoredFacts(newFile.Facts, "manual", *col, prof, profiles.Formats)
		var beaten, blockers []store.BookOwner
		for _, o := range owners {
			if o.FileID == fileID || !o.Verified || !o.Present {
				continue
			}
			fromScore := scoredFacts(o.Facts, o.Evidence, *col, prof, profiles.Formats)
			if force || autoUpgrade(prof, o, fromScore, toScore) {
				beaten = append(beaten, o)
			} else {
				blockers = append(blockers, o)
			}
		}
		if len(blockers) > 0 {
			// A pinned or better holder keeps the book; this file stays an
			// extra instead of sharing the final directory with it.
			b := blockers[0]
			if _, err := im.store.ReleaseBook(ctx, fileID, sl.BookID); err != nil {
				return nil, err
			}
			res.Kept = append(res.Kept, KeptClaim{
				BookID: sl.BookID, HolderFileID: b.FileID, HolderPath: b.Path,
				HolderScore: scoredFacts(b.Facts, b.Evidence, *col, prof, profiles.Formats), Score: toScore,
			})
			continue
		}
		for _, o := range beaten {
			holdless, err := im.store.ReleaseBook(ctx, o.FileID, sl.BookID)
			if err != nil {
				return nil, err
			}
			if holdless {
				im.exileToReview(ctx, seriesID, byID[o.FileID])
			}
			res.Replaced = append(res.Replaced, Replacement{
				FileID: o.FileID, Path: o.Path, BookID: sl.BookID,
				FromScore: scoredFacts(o.Facts, o.Evidence, *col, prof, profiles.Formats), ToScore: toScore,
			})
		}
	}
	return res, nil
}

// exileToReview moves a file that no longer holds any book out of the
// series' final folders into its review/ folder. Bytes are never deleted;
// a file already in review/ or outside the series tree is left alone.
func (im *Importer) exileToReview(ctx context.Context, seriesID int64, f store.File) {
	if f.ID == 0 || f.Presence != "present" {
		return
	}
	ms := LoadMediaSettings(ctx, im.store)
	series, err := im.store.GetSeries(ctx, seriesID)
	if err != nil {
		return
	}
	seriesDir, _ := seriesFolders(im.libraryRoot, series, ms)
	sep := string(filepath.Separator)
	if !strings.HasPrefix(f.Path, seriesDir+sep) {
		return
	}
	reviewDir := filepath.Join(seriesDir, "review")
	if strings.HasPrefix(f.Path, reviewDir+sep) {
		return
	}
	hash := f.Hash
	if hash == "" {
		if h, err := fileHash(f.Path); err == nil {
			hash = h
		}
	}
	dest := assetPath(reviewDir, filepath.Base(f.Path), hash)
	if dest == f.Path {
		return
	}
	// The files.path index is unique across the table: never move onto a
	// path another row already records, whatever is on disk.
	if taken, err := im.store.PathTaken(ctx, dest, f.ID); err != nil || taken {
		return
	}
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		return
	}
	if err := os.Rename(f.Path, dest); err != nil {
		return
	}
	if err := im.store.SetFileLocation(ctx, f.ID, dest, f.Size, hash); err != nil {
		// Roll the move back: a DB row still pointing at the old path is
		// what makes a later placement collide with the unique index.
		_ = os.Rename(dest, f.Path)
		slog.Warn("exiling displaced file to review failed", "file", f.ID, "error", err)
	}
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
