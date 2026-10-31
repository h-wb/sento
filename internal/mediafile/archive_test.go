package mediafile

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func zipBook(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(entries[name]))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
}

func entries(t *testing.T, path string) map[string]string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	out := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		out[f.Name] = string(data)
	}
	return out
}

// A processed book is the same pages in a clean container: folder
// flattened, junk gone, metadata merged over what the archive carried,
// unknown elements and the original ComicInfo kept.
func TestProcess(t *testing.T) {
	dir := t.TempDir()
	src, dest := filepath.Join(dir, "src.cbz"), filepath.Join(dir, "out", "book.cbz")
	zipBook(t, src, map[string]string{
		"Title v01 (Digital)/001.jpg":       "page 1",
		"Title v01 (Digital)/002.png":       "page 2",
		"Title v01 (Digital)/credits.txt":   "scanned by someone",
		"Title v01 (Digital)/Thumbs.db":     "junk",
		"__MACOSX/Title v01/._001.jpg":      "resource fork",
		"Title v01 (Digital)/ComicInfo.xml": `<?xml version="1.0"?><ComicInfo><Series>Wrong Name</Series><Number>2005</Number><Colorist>Someone</Colorist><Pages><Page Image="0" Type="FrontCover"/></Pages></ComicInfo>`,
	})
	before, err := Fingerprint(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := Process(src, dest, &ComicInfo{Series: "Title", Number: "1", Volume: "1", Notes: "sento:v1"}); err != nil {
		t.Fatal(err)
	}
	got := entries(t, dest)
	for _, name := range []string{"001.jpg", "002.png", "credits.txt", "ComicInfo.xml", "ComicInfo.orig.xml"} {
		if _, ok := got[name]; !ok {
			t.Errorf("missing %s in %v", name, got)
		}
	}
	if len(got) != 5 {
		t.Errorf("unexpected entries: %v", got)
	}
	info, err := ParseComicInfo([]byte(got["ComicInfo.xml"]))
	if err != nil {
		t.Fatal(err)
	}
	if info.Series != "Title" || info.Number != "1" || info.PageCount != "2" {
		t.Errorf("merged ComicInfo = %+v", info)
	}
	if !strings.Contains(got["ComicInfo.xml"], "<Colorist>Someone</Colorist>") || !strings.Contains(got["ComicInfo.xml"], `Type="FrontCover"`) {
		t.Errorf("unmodelled elements lost:\n%s", got["ComicInfo.xml"])
	}
	if !strings.Contains(got["ComicInfo.orig.xml"], "Wrong Name") {
		t.Errorf("original ComicInfo not preserved")
	}
	if after, _ := Fingerprint(dest); after != before {
		t.Errorf("fingerprint changed by processing: %s → %s", before, after)
	}

	// Re-tagging in place: new values win over the original, not over the
	// first tag (whose Volume this one leaves out), the original stays the
	// original, and the fingerprint still does not move.
	if err := Process(dest, dest, &ComicInfo{Series: "Title", Number: "1", Summary: "Now with a summary", Notes: "sento:v1"}); err != nil {
		t.Fatal(err)
	}
	got = entries(t, dest)
	if !strings.Contains(got["ComicInfo.xml"], "Now with a summary") || strings.Contains(got["ComicInfo.xml"], "<Volume>") || !strings.Contains(got["ComicInfo.xml"], "<Colorist>Someone</Colorist>") || !strings.Contains(got["ComicInfo.orig.xml"], "Wrong Name") || len(got) != 5 {
		t.Errorf("re-tag result: %v", got)
	}
	if after, _ := Fingerprint(dest); after != before {
		t.Errorf("fingerprint changed by re-tagging")
	}
}

func TestProcessRejectsNonBooks(t *testing.T) {
	dir := t.TempDir()
	garbage := filepath.Join(dir, "garbage.cbz")
	if err := os.WriteFile(garbage, []byte("not an archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Process(garbage, filepath.Join(dir, "a.cbz"), nil); err == nil {
		t.Error("a non-archive was accepted")
	}
	empty := filepath.Join(dir, "empty.cbz")
	zipBook(t, empty, map[string]string{"readme.txt": "no pages"})
	if err := Process(empty, filepath.Join(dir, "b.cbz"), nil); err == nil {
		t.Error("an archive without pages was accepted")
	}
	left, _ := filepath.Glob(filepath.Join(dir, ".sento-import-*"))
	if len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}
}

// The fingerprint follows the pages, not the wrapping.
func TestFingerprint(t *testing.T) {
	dir := t.TempDir()
	a, b, c, raw := filepath.Join(dir, "a.cbz"), filepath.Join(dir, "b.cbz"), filepath.Join(dir, "c.cbz"), filepath.Join(dir, "raw.bin")
	zipBook(t, a, map[string]string{"001.jpg": "p1", "002.jpg": "p2"})
	zipBook(t, b, map[string]string{"x/b.jpg": "p2", "x/a.jpg": "p1", "ComicInfo.xml": "<ComicInfo/>", "notes.txt": "hi"})
	zipBook(t, c, map[string]string{"001.jpg": "p1", "002.jpg": "different"})
	_ = os.WriteFile(raw, []byte("bytes"), 0o644)
	fa, _ := Fingerprint(a)
	fb, _ := Fingerprint(b)
	fc, _ := Fingerprint(c)
	fr, err := Fingerprint(raw)
	if fa == "" || fa != fb {
		t.Errorf("same pages, different fingerprints: %s %s", fa, fb)
	}
	if fa == fc {
		t.Error("different pages share a fingerprint")
	}
	if err != nil || len(fr) != 64 {
		t.Errorf("a plain file should fall back to its byte hash: %q %v", fr, err)
	}
}
