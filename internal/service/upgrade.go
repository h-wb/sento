package service

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
)

// Replacement is one book whose owning file was displaced by a better one.
type Replacement struct {
	Path      string `json:"path"`
	FileID    int64  `json:"fileId"`
	BookID    int64  `json:"bookId"`
	VersionID int64  `json:"versionId"`
	FromScore int    `json:"fromScore"`
	ToScore   int    `json:"toScore"`
}

// KeptClaim is one version's book the mapped file did not take because
// its current holder outranks it or is pinned.
type KeptClaim struct {
	BookID       int64  `json:"bookId"`
	VersionID    int64  `json:"versionId"`
	HolderFileID int64  `json:"holderFileId"`
	HolderPath   string `json:"holderPath"`
	HolderScore  int    `json:"holderScore"`
	Score        int    `json:"score"`
}

// FileMappingResult reports what a mapping save did beyond recording the
// file's own claims: which existing owners it displaced, and which books
// it left to their current holder.
type FileMappingResult struct {
	Notes    []string      `json:"notes,omitempty"`
	Replaced []Replacement `json:"replaced,omitempty"`
	Kept     []KeptClaim   `json:"kept,omitempty"`
}

// ApplyUpgrades maps the file to its books (atomically, like MapFile) and
// then applies the upgrade path per book.
func (im *Importer) ApplyUpgrades(ctx context.Context, seriesID, fileID int64, slots []store.BookRef, force bool) (*FileMappingResult, error) {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.applyUpgrades(ctx, seriesID, fileID, slots, force)
}

// applyUpgrades runs with im.mu held, including during a bulk review.
func (im *Importer) applyUpgrades(ctx context.Context, seriesID, fileID int64, slots []store.BookRef, force bool) (*FileMappingResult, error) {
	if err := im.store.MapFile(ctx, seriesID, fileID, slots); err != nil {
		return nil, err
	}
	return im.DisplaceOwners(ctx, seriesID, fileID, slots, force)
}

// DisplaceOwners enforces one owner per collected version's book: when another
// verified, present file already holds a claimed book, the better-scoring
// file takes the hold and the other is released and moved out of the final
// directory to review/ (its bytes stay; nothing is deleted). Automatic
// replacement follows the version profile's upgrade rules and never touches
// pinned holds; force — a person's explicit choice in review — overrides
// both. A file that does not outscore the holder gives up that version's
// selection, preserving its verified contents and other selections.
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
	_, ok := byID[fileID]
	if !ok {
		return nil, store.ErrNotFound
	}
	res := &FileMappingResult{}
	seen := map[string]bool{}
	affected := map[int64]bool{fileID: true}
	present := map[int64]bool{}
	for id, f := range byID {
		present[id] = f.Presence == "present" && !f.NeedsReview
	}
	for _, sl := range slots {
		key := store.BookKey(sl.VersionID, sl.BookID)
		if seen[key] {
			continue
		}
		seen[key] = true
		version, err := im.store.GetVersion(ctx, sl.VersionID)
		if err != nil {
			return nil, err
		}
		col, err := im.store.GetEdition(ctx, sl.EditionID)
		if err != nil {
			return nil, err
		}
		prof := profiles.For(*version)
		owners, err := im.store.BookOwners(ctx, sl.VersionID, sl.BookID)
		if err != nil {
			return nil, err
		}
		var cov []store.AssetCoverage
		for _, o := range owners {
			cov = append(cov, store.AssetCoverage{FileID: o.FileID, VersionID: sl.VersionID, BookID: sl.BookID, Evidence: o.Evidence, Verified: o.Verified, Pinned: o.Pinned})
		}
		judged := judgeCardFiles(*col, prof, profiles.Formats, cov, byID, present)
		eligible := map[int64]bool{}
		for _, id := range judged.accepted[sl.BookID] {
			eligible[id] = true
		}
		if !eligible[fileID] {
			continue // an observed profile refusal cannot displace an accepted file
		}
		toScore := judged.scores[fileID]
		var beaten, blockers []store.BookOwner
		for _, o := range owners {
			if o.FileID == fileID || !eligible[o.FileID] {
				continue
			}
			fromScore := judged.scores[o.FileID]
			if force || autoUpgrade(prof, o, fromScore, toScore) {
				beaten = append(beaten, o)
			} else {
				blockers = append(blockers, o)
			}
		}
		if len(blockers) > 0 {
			// This version keeps its pinned or better holder. Other versions
			// may still select the incoming file.
			b := blockers[0]
			if err := im.store.DeselectBook(ctx, fileID, sl.VersionID, sl.BookID); err != nil {
				return nil, err
			}
			res.Kept = append(res.Kept, KeptClaim{
				BookID: sl.BookID, VersionID: sl.VersionID, HolderFileID: b.FileID, HolderPath: b.Path,
				HolderScore: judged.scores[b.FileID], Score: toScore,
			})
			continue
		}
		for _, o := range beaten {
			if err := im.store.DeselectBook(ctx, o.FileID, sl.VersionID, sl.BookID); err != nil {
				return nil, err
			}
			affected[o.FileID] = true
			res.Replaced = append(res.Replaced, Replacement{
				FileID: o.FileID, Path: o.Path, BookID: sl.BookID, VersionID: sl.VersionID,
				FromScore: judged.scores[o.FileID], ToScore: toScore,
			})
		}
	}
	coverage, err := im.store.Coverage(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	versions, err := im.store.ListVersions(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	editions, err := im.store.ListEditions(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	for _, id := range preferredFileOwners(versions, editions, coverage, byID, profiles) {
		delete(affected, id)
	}
	for id := range affected {
		im.exileToReview(ctx, seriesID, byID[id])
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
// until-score ceiling, and beaten by at least MinUpgradeScore. Known means
// the file's own facts or its release's claims say enough to judge it.
func autoUpgrade(prof *profile.Profile, owner store.BookOwner, fromScore, toScore int) bool {
	if prof == nil || !prof.UpgradesEnabled || owner.Pinned {
		return false
	}
	if !backedFacts(owner.Facts, owner.Claims).FactsKnown {
		return false
	}
	if prof.UpgradeUntilScore != 0 && fromScore >= prof.UpgradeUntilScore {
		return false
	}
	return toScore-fromScore >= max(prof.MinUpgradeScore, 1)
}
