package library

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ImportCBZ writes a CBZ stream to destPath (atomically via temp file),
// verifying it is a readable archive with ≥1 image, and rewrites its
// ComicInfo.xml with sentoInfo merged over any existing one (original
// preserved once as ComicInfo.orig.xml). Returns the final file size.
func ImportCBZ(src io.Reader, destPath string, sentoInfo ComicInfo) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return 0, err
	}

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

	if err := rewriteArchive(tmpPath, destPath, sentoInfo); err != nil {
		return 0, err
	}
	info, err := os.Stat(destPath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
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
// filesystems (torrent imports must keep seeding — spec.md §7).
func HardlinkOrCopy(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.Link(src, dest); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
