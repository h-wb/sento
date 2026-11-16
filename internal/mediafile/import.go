package mediafile

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ImportCBZ writes a CBZ stream to destPath (atomically via temp file),
// verifying it is a readable archive with ≥1 image, and rewrites its
// ComicInfo.xml with sentoInfo merged over any existing one (original
// preserved once as ComicInfo.orig.xml). Returns the final file size.
func ImportCBZ(src io.Reader, destPath string, sentoInfo *ComicInfo) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return 0, err
	}
	CleanStaleTemps(filepath.Dir(destPath))

	tmp, err := os.CreateTemp(filepath.Dir(destPath), ".sento-import-*")
	if err != nil {
		return 0, err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		return 0, fmt.Errorf("writing archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}

	if sentoInfo == nil {
		// No ComicInfo rewrite: verify the archive, store it verbatim.
		zr, err := zip.OpenReader(tmpPath)
		if err != nil {
			return 0, fmt.Errorf("verify: not a readable zip: %w", err)
		}
		_ = zr.Close()
		// CreateTemp makes the file 0600; the media server must read it.
		if err := os.Chmod(tmpPath, 0o644); err != nil {
			return 0, err
		}
		if err := os.Rename(tmpPath, destPath); err != nil {
			return 0, err
		}
	} else if err := Process(tmpPath, destPath, sentoInfo); err != nil {
		return 0, err
	}
	info, err := os.Stat(destPath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// CleanStaleTemps removes leftover import and stage files from operations
// interrupted by a crash/kill (the defer never ran). Anything older than an
// hour is junk.
func CleanStaleTemps(dir string) {
	cutoff := time.Now().Add(-time.Hour)
	for _, pattern := range []string{".sento-import-*", ".sento-stage-*"} {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			continue
		}
		for _, m := range matches {
			if info, err := os.Lstat(m); err == nil && info.Mode().IsRegular() && info.ModTime().Before(cutoff) {
				_ = os.Remove(m)
			}
		}
	}
}
