package mediafile

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A torrent payload imported twice ends up with dest already hardlinked to
// src. Re-importing must not write through that link: truncating dest would
// empty the seeding payload too.
func TestHardlinkOrCopyKeepsSourceWhenDestIsItsHardlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "downloads", "v01.cbz")
	dest := filepath.Join(dir, "library", "Series v01.cbz")
	writeFile(t, src, "payload")

	for range 2 {
		if err := HardlinkOrCopy(src, dest); err != nil {
			t.Fatal(err)
		}
	}
	if got := readFile(t, src); got != "payload" {
		t.Fatalf("source payload = %q, want it untouched", got)
	}
	if got := readFile(t, dest); got != "payload" {
		t.Fatalf("dest = %q, want payload", got)
	}
	if err := CopyFile(src, dest); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, src); got != "payload" {
		t.Fatalf("source payload after CopyFile = %q, want it untouched", got)
	}
}

func TestHardlinkOrCopyReplacesAStaleDest(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "downloads", "v01.cbz")
	dest := filepath.Join(dir, "library", "Series v01.cbz")
	writeFile(t, src, "new release")
	writeFile(t, dest, "old file")

	if err := HardlinkOrCopy(src, dest); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, dest); got != "new release" {
		t.Fatalf("dest = %q, want the new release", got)
	}
	if got := readFile(t, src); got != "new release" {
		t.Fatalf("source = %q, want it untouched", got)
	}
	assertNoTemps(t, filepath.Dir(dest))
}

func TestCopyFileIsIndependentAndReadable(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.cbz")
	dest := filepath.Join(dir, "library", "dest.cbz")
	writeFile(t, src, "payload")

	if err := CopyFile(src, dest); err != nil {
		t.Fatal(err)
	}
	si, _ := os.Stat(src)
	di, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(si, di) {
		t.Fatal("CopyFile produced a hardlink, want an independent copy")
	}
	if di.Mode().Perm() != 0o644 {
		t.Fatalf("dest mode = %v, want 0644", di.Mode().Perm())
	}
	assertNoTemps(t, filepath.Dir(dest))
}

func TestCopyFileMissingSourceLeavesNoDest(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "dest.cbz")
	if err := CopyFile(filepath.Join(dir, "missing.cbz"), dest); err == nil {
		t.Fatal("want an error for a missing source")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("dest exists after a failed copy (stat err = %v)", err)
	}
}

func cbz(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// With the ComicInfo rewrite off the archive is stored verbatim — and must
// still be readable by the media server, not left at CreateTemp's 0600.
func TestImportCBZVerbatimIsWorldReadable(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "Series", "c001.cbz")
	if _, err := ImportCBZ(bytes.NewReader(cbz(t, "0001.jpg")), dest, nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %v, want 0644", info.Mode().Perm())
	}
	assertNoTemps(t, filepath.Dir(dest))
}

func TestImportCBZWritesComicInfoAndRejectsImageless(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "c001.cbz")
	if _, err := ImportCBZ(bytes.NewReader(cbz(t, "0001.jpg")), dest, &ComicInfo{Series: "Frieren"}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	found := false
	for _, f := range zr.File {
		found = found || f.Name == "ComicInfo.xml"
	}
	if !found {
		t.Fatal("ComicInfo.xml missing from the imported archive")
	}

	empty := filepath.Join(dir, "empty.cbz")
	if _, err := ImportCBZ(bytes.NewReader(cbz(t, "readme.txt")), empty, &ComicInfo{}); err == nil {
		t.Fatal("want an error for an archive without images")
	}
	if _, err := os.Stat(empty); !os.IsNotExist(err) {
		t.Fatalf("rejected archive left a file behind (stat err = %v)", err)
	}
}

func assertNoTemps(t *testing.T, dir string) {
	t.Helper()
	left, _ := filepath.Glob(filepath.Join(dir, ".sento-import-*"))
	if len(left) > 0 {
		t.Fatalf("temp files left behind: %v", left)
	}
}
