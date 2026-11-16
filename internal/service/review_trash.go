package service

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/store"
)

// A committed choice may be interrupted before journaling, linking, unlinking,
// or recording completion. Every stage retains the reviewed fingerprint.
func (lf *LibraryFiles) finishReviewTrash(ctx context.Context) {
	var afterID int64
	for ctx.Err() == nil {
		files, err := lf.store.PendingReviewTrash(ctx, afterID)
		if err != nil {
			slog.Warn("review trash recovery listing pending files failed", "afterFile", afterID, "error", err)
			return
		}
		if len(files) == 0 {
			return
		}
		for _, f := range files {
			if ctx.Err() != nil {
				return
			}
			afterID = f.ID
			if err = lf.resumeReviewTrash(ctx, f); err != nil {
				slog.Warn("review trash recovery pending", "file", f.ID, "error", err)
			}
		}
	}
}

func (lf *LibraryFiles) reopenChangedReviewTrash(ctx context.Context, fid int64) error {
	return errors.Join(apperr.Conflictf("archive contents changed; review this file again before moving it to trash"), lf.store.ReopenReviewTrash(ctx, fid))
}

func (lf *LibraryFiles) resumeReviewTrash(ctx context.Context, f store.File) error {
	entry, err := lf.store.GetFileTrash(ctx, f.ID)
	if errors.Is(err, store.ErrNotFound) {
		if f.Hash == "" {
			return lf.reopenChangedReviewTrash(ctx, f.ID)
		}
		return lf.trashFile(ctx, f.SeriesID, CleanupFile{FileID: f.ID, Path: f.Path, Reason: "Unused incoming file rejected in review"}, 30, f.Hash)
	}
	if err != nil {
		return err
	}
	if f.Hash == "" || entry.Digest != f.Hash {
		return apperr.Conflictf("trash evidence changed; restore the retained archive before reviewing it again")
	}
	if err = checkFileTrashDirectory(filepath.Dir(entry.TrashPath)); err != nil {
		return err
	}
	original, originalErr := os.Lstat(f.Path)
	trash, trashErr := os.Lstat(entry.TrashPath)
	switch {
	case errors.Is(trashErr, fs.ErrNotExist):
		if originalErr != nil {
			return originalErr
		}
		if !original.Mode().IsRegular() {
			return apperr.Conflictf("original path is not a regular archive")
		}
		digest, e := fingerprint(f.Path)
		if e != nil {
			return e
		}
		if digest != f.Hash {
			// No destination exists, so the unstarted journal can be removed
			// safely and the changed original made available for fresh review.
			return lf.reopenChangedReviewTrash(ctx, f.ID)
		}
		if err = moveArchive(f.Path, entry.TrashPath); err != nil {
			return err
		}
	case trashErr != nil:
		return trashErr
	default:
		if !trash.Mode().IsRegular() {
			return apperr.Conflictf("trash entry is not a regular archive")
		}
		digest, e := fingerprint(entry.TrashPath)
		if e != nil {
			return e
		}
		if digest != f.Hash {
			return apperr.Conflictf("retained archive changed; restore and review it before cleanup")
		}
		if originalErr == nil {
			if !os.SameFile(original, trash) {
				return apperr.Conflictf("original path contains another archive; keep both files for recovery")
			}
			// Link succeeded before the interruption; only its original name
			// remains to be removed. The retained bytes have just been checked.
			if err = os.Remove(f.Path); err != nil {
				return err
			}
		} else if !errors.Is(originalErr, fs.ErrNotExist) {
			return originalErr
		}
	}
	return lf.store.CompleteReviewTrash(ctx, f.ID)
}
