package mediafile

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/nwaples/rardecode/v2"
)

// junk reports archive entries no reader wants: OS droppings and resource
// forks that taggers and file managers leave behind.
func junk(name string) bool {
	base := strings.ToLower(path.Base(name))
	return base == "thumbs.db" || base == ".ds_store" || base == "desktop.ini" ||
		strings.HasPrefix(base, "._") || strings.HasPrefix(name, "__MACOSX/") || strings.Contains(name, "/__MACOSX/")
}

func isImage(name string) bool { return imageExts[strings.ToLower(path.Ext(name))] }

func isComicInfo(name string) bool { return strings.EqualFold(path.Base(name), "ComicInfo.xml") }

// member is one file inside a book archive, whatever the container.
type member struct {
	name string
	zip  *zip.File // nil for RAR members, which are read from the stream
}

// The formats Sento handles, in one place for scan, import and organize.
// Archives (zip/cbz, rar/cbr) can be read, fingerprinted and rewritten as
// a tagged CBZ. Documents are recognised and kept as they are.
var (
	archiveExts  = map[string]bool{".cbz": true, ".zip": true, ".cbr": true, ".rar": true}
	documentExts = map[string]bool{".pdf": true, ".epub": true}
)

// IsArchive reports whether name is a comic archive Process can rewrite.
func IsArchive(name string) bool { return archiveExts[strings.ToLower(filepath.Ext(name))] }

// IsBook reports whether name is any book file Sento keeps in a library.
func IsBook(name string) bool {
	return IsArchive(name) || documentExts[strings.ToLower(filepath.Ext(name))]
}

// walk visits every regular file of a zip or RAR archive in archive order.
// read returns the member's uncompressed content and is only valid during
// the visit.
func walk(src string, visit func(m member, read func() (io.ReadCloser, error)) error) error {
	if zr, err := zip.OpenReader(src); err == nil {
		defer func() { _ = zr.Close() }()
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
			}
			if err := visit(member{name: filepath.ToSlash(f.Name), zip: f}, f.Open); err != nil {
				return err
			}
		}
		return nil
	}
	rr, err := rardecode.OpenReader(src)
	if err != nil {
		return fmt.Errorf("not a readable zip or rar archive")
	}
	defer func() { _ = rr.Close() }()
	for {
		hdr, err := rr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading rar archive: %w", err)
		}
		if hdr.IsDir {
			continue
		}
		read := func() (io.ReadCloser, error) { return io.NopCloser(rr), nil }
		if err := visit(member{name: filepath.ToSlash(hdr.Name)}, read); err != nil {
			return err
		}
	}
}

// Fingerprint identifies a book by its pages alone: the SHA-256 of the
// sorted SHA-256s of its image files. Names, order, container format and
// ComicInfo.xml do not enter it, so a download and its processed library
// copy share a fingerprint, and re-tagging never changes it. A file that
// is not a book archive is identified by its bytes.
func Fingerprint(src string) (string, error) {
	var pages [][]byte
	err := walk(src, func(m member, read func() (io.ReadCloser, error)) error {
		if !isImage(m.name) || junk(m.name) {
			return nil
		}
		rc, err := read()
		if err != nil {
			return err
		}
		defer func() { _ = rc.Close() }()
		h := sha256.New()
		if _, err := io.Copy(h, rc); err != nil {
			return err
		}
		pages = append(pages, h.Sum(nil))
		return nil
	})
	if err != nil || len(pages) == 0 {
		return fileSHA256(src)
	}
	sort.Slice(pages, func(i, j int) bool { return bytes.Compare(pages[i], pages[j]) < 0 })
	h := sha256.New()
	for _, p := range pages {
		_, _ = h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func fileSHA256(src string) (string, error) {
	f, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// commonDir returns the single top-level folder every page lives under
// ("Title v01 (Digital)/001.jpg" → "Title v01 (Digital)/"), or "".
func commonDir(src string) string {
	prefix, first := "", true
	_ = walk(src, func(m member, _ func() (io.ReadCloser, error)) error {
		if !isImage(m.name) || junk(m.name) {
			return nil
		}
		dir := ""
		if i := strings.Index(m.name, "/"); i >= 0 {
			dir = m.name[:i+1]
		}
		if first {
			prefix, first = dir, false
		} else if dir != prefix {
			prefix = ""
		}
		return nil
	})
	return prefix
}

// Process writes src (a zip/cbz or rar/cbr book) to dest as a clean CBZ:
// pages byte-for-byte as they were, a wrapping folder flattened, junk
// entries dropped, and ComicInfo.xml set to info merged over whatever the
// archive already carried (kept once as ComicInfo.orig.xml). A nil info
// only normalises the container. dest is replaced atomically.
func Process(src, dest string, info *ComicInfo) error {
	strip := commonDir(src)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".sento-import-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // a no-op once renamed into place
	zw := zip.NewWriter(tmp)

	var existing, original []byte
	pages := 0
	seen := map[string]bool{}
	err = walk(src, func(m member, read func() (io.ReadCloser, error)) error {
		name := strings.TrimPrefix(m.name, strip)
		switch {
		case junk(m.name) || seen[name]:
			return nil
		case isComicInfo(name) || strings.EqualFold(path.Base(name), "ComicInfo.orig.xml"):
			rc, err := read()
			if err != nil {
				return err
			}
			data, _ := io.ReadAll(io.LimitReader(rc, 4<<20))
			_ = rc.Close()
			if isComicInfo(name) {
				existing = data
			} else {
				original = data
			}
			return nil
		}
		seen[name] = true
		if isImage(name) {
			pages++
		}
		if m.zip != nil {
			// Raw copy: the compressed bytes move as they are.
			hdr := m.zip.FileHeader
			hdr.Name = name
			w, err := zw.CreateRaw(&hdr)
			if err != nil {
				return err
			}
			raw, err := m.zip.OpenRaw()
			if err != nil {
				return err
			}
			_, err = io.Copy(w, raw)
			return err
		}
		rc, err := read()
		if err != nil {
			return err
		}
		// Images are already compressed; deflating them only costs time.
		method := zip.Deflate
		if isImage(name) {
			method = zip.Store
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		if err != nil {
			return err
		}
		_, err = io.Copy(w, rc)
		return err
	})
	if err == nil && pages == 0 {
		err = fmt.Errorf("verify: archive contains no images")
	}
	if err != nil {
		_ = tmp.Close()
		return err
	}

	write := func(name string, data []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	switch {
	case info == nil && existing != nil:
		err = write("ComicInfo.xml", existing)
	case info != nil:
		merged := *info
		if prior, perr := ParseComicInfo(existing); existing != nil && perr == nil {
			merged = Merge(*prior, *info)
		}
		merged.PageCount = strconv.Itoa(pages)
		var data []byte
		if data, err = merged.Marshal(); err == nil {
			err = write("ComicInfo.xml", data)
		}
		// The pre-Sento ComicInfo is preserved once, never overwritten by a
		// later re-tag.
		if err == nil && original == nil && existing != nil && !sentoTagged(existing) {
			original = existing
		}
	}
	if err == nil && original != nil {
		err = write("ComicInfo.orig.xml", original)
	}
	if err == nil {
		err = zw.Close()
	}
	if err == nil {
		// CreateTemp makes the file 0600; the media server must read it.
		err = tmp.Chmod(0o644)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpPath, dest)
}

// sentoTagged reports whether a ComicInfo was written by Sento, so a
// re-tag does not mistake its own earlier output for the original.
func sentoTagged(data []byte) bool {
	ci, err := ParseComicInfo(data)
	return err == nil && strings.Contains(ci.Notes, "sento:")
}

// ReadComicInfo returns the ComicInfo.xml inside a CBZ, or nil when the
// file has none or is not a zip.
func ReadComicInfo(src string) *ComicInfo {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return nil
	}
	defer func() { _ = zr.Close() }()
	for _, f := range zr.File {
		if f.Name != "ComicInfo.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil
		}
		data, _ := io.ReadAll(io.LimitReader(rc, 4<<20))
		_ = rc.Close()
		ci, err := ParseComicInfo(data)
		if err != nil {
			return nil
		}
		return ci
	}
	return nil
}
