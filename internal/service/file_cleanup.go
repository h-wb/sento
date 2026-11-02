package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/store"
)

const fileTrashDir = ".sento-trash"

type CleanupFile struct {
	FileID   int64    `json:"fileId"`
	Path     string   `json:"path"`
	Size     int64    `json:"size"`
	Editions []string `json:"editions"`
	Category string   `json:"category"`
	Reason   string   `json:"reason"`
	Eligible bool     `json:"eligible"`
}
type CleanupSkipped struct {
	FileID int64  `json:"fileId"`
	Reason string `json:"reason"`
}
type CleanupResult struct {
	Moved   []int64          `json:"moved"`
	Skipped []CleanupSkipped `json:"skipped"`
}

// CleanupPreview describes intent, never changes it. A paused edition supplies
// candidates only; explicit selection is required for every file moved.
func (im *Importer) CleanupPreview(ctx context.Context, sid, eid int64) ([]CleanupFile, error) {
	files, err := im.store.ListFiles(ctx, sid)
	if err != nil {
		return nil, err
	}
	editions, err := im.store.ListEditions(ctx, sid)
	if err != nil {
		return nil, err
	}
	if eid != 0 && editionByID(editions, eid) == nil {
		return nil, store.ErrNotFound
	}
	books, err := im.store.SeriesBooks(ctx, sid)
	if err != nil {
		return nil, err
	}
	contents, err := im.store.Contents(ctx, sid)
	if err != nil {
		return nil, err
	}
	coverage, err := im.store.Coverage(ctx, sid)
	if err != nil {
		return nil, err
	}
	versions, err := im.store.ListVersions(ctx, sid)
	if err != nil {
		return nil, err
	}
	jobs, err := im.store.SeriesQueueItems(ctx, sid)
	if err != nil {
		return nil, err
	}
	bookEdition := map[int64]int64{}
	for _, b := range books {
		bookEdition[b.ID] = b.EditionID
	}
	collected := map[int64]bool{}
	monitored := map[int64]bool{}
	for _, v := range versions {
		monitored[v.ID] = v.Monitored
		collected[v.EditionID] = collected[v.EditionID] || v.Monitored
	}
	mapped := map[int64]map[int64]bool{}
	unverified := map[int64]bool{}
	for _, c := range contents {
		if mapped[c.FileID] == nil {
			mapped[c.FileID] = map[int64]bool{}
		}
		mapped[c.FileID][bookEdition[c.BookID]] = true
		unverified[c.FileID] = unverified[c.FileID] || !c.Verified
	}
	selected, pinned := map[int64]bool{}, map[int64]bool{}
	for _, c := range coverage {
		selected[c.FileID] = selected[c.FileID] || monitored[c.VersionID]
		pinned[c.FileID] = pinned[c.FileID] || c.Pinned
	}
	// Pins on deselected copies still protect the asset.
	pins, err := im.store.PinnedFiles(ctx, sid)
	if err != nil {
		return nil, err
	}
	for _, id := range pins {
		pinned[id] = true
	}
	pending := map[string]bool{}
	for _, j := range jobs {
		switch j.Status {
		case store.QueueImported, store.QueueCancelled, store.QueueFailed:
			continue
		}
		if j.ReleaseKey != "" {
			pending[j.ReleaseKey] = true
		}
		if j.ExternalID != "" {
			pending[j.ExternalID] = true
		}
	}
	out := []CleanupFile{}
	for _, f := range files {
		if eid != 0 && !mapped[f.ID][eid] {
			continue
		}
		c := CleanupFile{FileID: f.ID, Path: f.Path, Size: f.Size, Editions: []string{}}
		active := false
		for _, e := range editions {
			if mapped[f.ID][e.ID] {
				c.Editions = append(c.Editions, e.EditionName)
				active = active || collected[e.ID]
			}
		}
		switch {
		case pinned[f.ID]:
			c.Reason = "Pinned by a collected version"
		case selected[f.ID]:
			c.Reason = "Still selected by a collected version"
		case f.NeedsReview || unverified[f.ID]:
			c.Reason = "Finish reviewing this file first"
		case pending[f.Facts.ReleaseKey] || pending[f.Claims.ReleaseKey]:
			c.Reason = "Part of an unfinished download"
		case f.Presence != "present":
			c.Reason = "Recheck file storage before cleanup"
		default:
			c.Eligible = true
			switch {
			case len(mapped[f.ID]) == 0:
				c.Category = "unmapped"
				c.Reason = "Unmapped file, already reviewed"
			case !active:
				c.Category = "uncollected"
				c.Reason = "Only used by editions you are not collecting"
			default:
				c.Category = "unselected"
				c.Reason = "No collected version selects this copy"
			}
		}
		out = append(out, c)
	}
	return out, nil
}

func (im *Importer) CleanupFiles(ctx context.Context, sid, eid int64, ids []int64, days int) (*CleanupResult, error) {
	if len(ids) == 0 || len(ids) > 500 {
		return nil, apperr.Invalidf("select between 1 and 500 files")
	}
	if days < 1 || days > 365 {
		return nil, apperr.Invalidf("trash retention must be between 1 and 365 days")
	}
	im.mu.Lock()
	defer im.mu.Unlock()
	preview, err := im.CleanupPreview(ctx, sid, eid)
	if err != nil {
		return nil, err
	}
	byID := map[int64]CleanupFile{}
	for _, f := range preview {
		byID[f.FileID] = f
	}
	result := &CleanupResult{Moved: []int64{}, Skipped: []CleanupSkipped{}}
	for _, id := range slices.Compact(slices.Sorted(slices.Values(ids))) {
		f, ok := byID[id]
		if !ok || !f.Eligible {
			reason := f.Reason
			if !ok {
				reason = "File is no longer in this preview"
			}
			result.Skipped = append(result.Skipped, CleanupSkipped{id, reason})
			continue
		}
		if err = im.trashFile(ctx, sid, f, days); err != nil {
			result.Skipped = append(result.Skipped, CleanupSkipped{id, err.Error()})
			continue
		}
		result.Moved = append(result.Moved, id)
	}
	return result, nil
}

func (im *Importer) trashFile(ctx context.Context, sid int64, f CleanupFile, days int) error {
	info, err := os.Lstat(f.Path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return apperr.Conflictf("only regular archive files can be moved to trash")
	}
	dir := filepath.Join(filepath.Dir(f.Path), fileTrashDir, strconv.FormatInt(f.FileID, 10))
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for _, p := range []string{filepath.Dir(dir), dir} {
		st, e := os.Lstat(p)
		if e != nil {
			return e
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return apperr.Conflictf("trash folder must be a regular directory")
		}
	}
	dest := filepath.Join(dir, filepath.Base(f.Path))
	digest, err := fileHash(f.Path)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	format := "2006-01-02 15:04:05"
	if err = im.store.BeginFileTrash(ctx, sid, f.FileID, f.Path, dest, digest, now.Format(format), now.AddDate(0, 0, days).Format(format)); err != nil {
		return err
	}
	// Link then unlink on the same filesystem: no overwrite, no payload rewrite.
	if err = moveArchive(f.Path, dest); err != nil {
		if rollback := im.store.RestoreFileRecord(context.WithoutCancel(ctx), f.FileID); rollback != nil {
			return fmt.Errorf("move failed: %v; restore the retained trash entry: %w", err, rollback)
		}
		return err
	}
	_ = im.store.AddDecision(ctx, sid, store.DecisionBody{Event: "file_trashed", Title: filepath.Base(f.Path), Note: fmt.Sprintf("Recoverable for %d days. %s", days, f.Reason)})
	return nil
}

func moveArchive(from, to string) error {
	if err := os.Link(from, to); err != nil {
		return err
	}
	if err := os.Remove(from); err != nil {
		_ = os.Remove(to)
		return err
	}
	return nil
}

func (im *Importer) RestoreTrashedFile(ctx context.Context, sid, fid int64) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	f, err := im.store.GetFileTrash(ctx, fid)
	if err != nil {
		return err
	}
	if f.SeriesID != sid {
		return store.ErrNotFound
	}
	trashInfo, trashErr := os.Lstat(f.TrashPath)
	originalInfo, originalErr := os.Lstat(f.Path)
	switch {
	case trashErr == nil && originalErr == nil && os.SameFile(trashInfo, originalInfo):
		// A crash between link and unlink left both names for the same archive.
		if !trashInfo.Mode().IsRegular() {
			return apperr.Conflictf("trash entry is not a regular file")
		}
		digest, e := fileHash(f.Path)
		if e != nil {
			return e
		}
		if digest != f.Digest {
			return apperr.Conflictf("retained bytes changed; mappings cannot be restored")
		}
		if err = os.Remove(f.TrashPath); err != nil {
			return err
		}
	case errors.Is(trashErr, fs.ErrNotExist):
		// A crash before the move, or after a restore, left the original intact.
		if originalErr != nil {
			return originalErr
		}
		if !originalInfo.Mode().IsRegular() {
			return apperr.Conflictf("original path is not a regular file")
		}
		digest, e := fileHash(f.Path)
		if e != nil {
			return e
		}
		if digest != f.Digest {
			return apperr.Conflictf("original path contains different bytes; retained mappings cannot be restored")
		}
	default:
		if trashErr != nil {
			return trashErr
		}
		if !trashInfo.Mode().IsRegular() {
			return apperr.Conflictf("trash entry is not a regular file")
		}
		digest, e := fileHash(f.TrashPath)
		if e != nil {
			return e
		}
		if digest != f.Digest {
			return apperr.Conflictf("trash contents changed; retained mappings cannot be restored")
		}
		taken, e := im.store.PathTaken(ctx, f.Path, fid)
		if e != nil {
			return e
		}
		if taken {
			return apperr.Conflictf("the original path is used by another file")
		}
		if err = os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
			return err
		}
		if err = moveArchive(f.TrashPath, f.Path); err != nil {
			return fmt.Errorf("original path must be free before restoring: %w", err)
		}
	}
	if err = im.store.RestoreFileRecord(ctx, fid); err != nil {
		return err
	}
	_ = os.Remove(filepath.Dir(f.TrashPath))
	_ = os.Remove(filepath.Dir(filepath.Dir(f.TrashPath)))
	return im.store.AddDecision(ctx, sid, store.DecisionBody{Event: "file_restored", Title: filepath.Base(f.Path), Note: "Restored from trash with its original mappings; collection settings unchanged."})
}

func (im *Importer) PurgeTrashedFile(ctx context.Context, sid, fid int64) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.purgeTrashedFile(ctx, sid, fid)
}
func (im *Importer) purgeTrashedFile(ctx context.Context, sid, fid int64) error {
	f, err := im.store.GetFileTrash(ctx, fid)
	if err != nil {
		return err
	}
	if f.SeriesID != sid {
		return store.ErrNotFound
	}
	trashInfo, trashErr := os.Lstat(f.TrashPath)
	originalInfo, originalErr := os.Lstat(f.Path)
	if errors.Is(trashErr, fs.ErrNotExist) {
		if !errors.Is(originalErr, fs.ErrNotExist) {
			return apperr.Conflictf("original file still exists; restore the interrupted move first")
		}
	} else if trashErr != nil {
		return trashErr
	} else {
		if originalErr == nil && os.SameFile(trashInfo, originalInfo) {
			return apperr.Conflictf("original file still exists; restore the interrupted move first")
		}
		if err = os.Remove(f.TrashPath); err != nil {
			return err
		}
	}
	if _, err = im.store.DeleteFile(ctx, fid); err != nil {
		return err
	}
	_ = os.Remove(filepath.Dir(f.TrashPath))
	_ = os.Remove(filepath.Dir(filepath.Dir(f.TrashPath)))
	return im.store.AddDecision(ctx, sid, store.DecisionBody{Event: "file_deleted", Title: filepath.Base(f.Path), Note: "Permanently deleted from library trash. Download-client payloads were not touched."})
}

// PurgeExpiredTrash is run by housekeeping. A failure retains its record for
// another pass, and never falls back to deleting the original library path.
func (im *Importer) PurgeExpiredTrash(ctx context.Context) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	files, err := im.store.ListFileTrash(ctx, 0)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	var failures []error
	for _, f := range files {
		if f.PurgeAfter <= now {
			if err = im.purgeTrashedFile(ctx, f.SeriesID, f.FileID); err != nil {
				failures = append(failures, fmt.Errorf("trash file %d: %w", f.FileID, err))
			}
		}
	}
	return errors.Join(failures...)
}
func inFileTrash(path string) bool {
	return slices.Contains(strings.Split(filepath.ToSlash(filepath.Clean(path)), "/"), fileTrashDir)
}
