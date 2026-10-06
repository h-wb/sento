package mediafile

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	cleanStaleTemps(filepath.Dir(destPath))

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
	} else if err := rewriteArchive(tmpPath, destPath, *sentoInfo); err != nil {
		return 0, err
	}
	info, err := os.Stat(destPath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// cleanStaleTemps removes leftover temp files from imports interrupted by
// a crash/kill (the defer never ran). Anything older than an hour is junk.
func cleanStaleTemps(dir string) {
	matches, err := filepath.Glob(filepath.Join(dir, ".sento-import-*"))
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-time.Hour)
	for _, m := range matches {
		if info, err := os.Stat(m); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(m)
		}
	}
}

var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
	".avif": true, ".jxl": true, ".bmp": true,
}

// rewriteArchive copies srcZip to dest, replacing/inserting ComicInfo.xml.
func rewriteArchive(srcZip, dest string, sentoInfo ComicInfo) error {
	zr, err := zip.OpenReader(srcZip)
	if err != nil {
		return fmt.Errorf("verify: not a readable zip: %w", err)
	}
	defer func() { _ = zr.Close() }()

	images := 0
	var existing *ComicInfo
	for _, f := range zr.File {
		if imageExts[strings.ToLower(filepath.Ext(f.Name))] {
			images++
		}
		if strings.EqualFold(filepath.Base(f.Name), "ComicInfo.xml") {
			rc, err := f.Open()
			if err == nil {
				data, _ := io.ReadAll(io.LimitReader(rc, 4<<20))
				_ = rc.Close()
				existing, _ = ParseComicInfo(data)
			}
		}
	}
	if images == 0 {
		return fmt.Errorf("verify: archive contains no images")
	}

	merged := sentoInfo
	if existing != nil {
		merged = Merge(*existing, sentoInfo)
	}
	infoXML, err := merged.Marshal()
	if err != nil {
		return err
	}

	tmpDest := dest + ".sento-tmp"
	out, err := os.Create(tmpDest)
	if err != nil {
		return err
	}
	// A no-op once the rename below has moved it into place.
	defer func() { _ = os.Remove(tmpDest) }()
	zw := zip.NewWriter(out)

	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		if strings.EqualFold(base, "ComicInfo.xml") || strings.EqualFold(base, "ComicInfo.orig.xml") {
			// Preserve the pre-Sento ComicInfo once.
			if existing != nil && strings.EqualFold(base, "ComicInfo.xml") {
				if err := copyZipEntryAs(zw, f, "ComicInfo.orig.xml"); err != nil {
					_ = out.Close()
					return err
				}
			}
			continue
		}
		if err := copyZipEntry(zw, f); err != nil {
			_ = out.Close()
			return err
		}
	}
	w, err := zw.Create("ComicInfo.xml")
	if err != nil {
		_ = out.Close()
		return err
	}
	if _, err := w.Write(infoXML); err != nil {
		_ = out.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmpDest, dest)
}

func copyZipEntry(zw *zip.Writer, f *zip.File) error {
	return copyZipEntryAs(zw, f, f.Name)
}

func copyZipEntryAs(zw *zip.Writer, f *zip.File, name string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	hdr := f.FileHeader
	hdr.Name = name
	w, err := zw.CreateHeader(&hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, rc)
	return err
}

// HardlinkOrCopy links src into dest, falling back to a copy across
// filesystems (torrent imports must keep seeding — spec.md §7). An existing
// dest is replaced atomically, never written through: after a re-import it
// can be a hardlink to src itself, and truncating it in place would wipe
// the seeding payload along with the library file.
func HardlinkOrCopy(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if sameFile(src, dest) {
		return nil
	}
	if err := os.Link(src, dest); err == nil {
		return nil
	}
	// dest exists (or the filesystem refuses links): link under a temp name
	// and rename over, so readers only ever see a complete file.
	tmp, err := tempName(dest)
	if err != nil {
		return err
	}
	if err := os.Link(src, tmp); err == nil {
		if err := os.Rename(tmp, dest); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		return nil
	}
	return CopyFile(src, dest)
}

// CopyFile copies src to dest, creating parent directories. The bytes land
// in a temp file renamed into place: a failed copy leaves no partial dest,
// and an existing dest is replaced rather than truncated.
func CopyFile(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if sameFile(src, dest) {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.CreateTemp(filepath.Dir(dest), ".sento-import-*")
	if err != nil {
		return err
	}
	tmp := out.Name()
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// CreateTemp makes the file 0600; library files should be readable by
	// the media server like any other.
	if err := os.Chmod(tmp, 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// sameFile reports whether both paths already name one inode.
func sameFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

// tempName reserves a unique sibling path of dest that does not exist yet
// (os.Link needs a free name).
func tempName(dest string) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(dest), ".sento-import-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	_ = f.Close()
	if err := os.Remove(name); err != nil {
		return "", err
	}
	return name, nil
}
