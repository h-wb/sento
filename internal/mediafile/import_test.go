package mediafile

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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

func TestCleanStaleTempsSweepsOldImportAndStageFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	old := now.Add(-2 * time.Hour)
	files := []struct {
		name  string
		mtime time.Time
		swept bool
	}{
		{".sento-stage-x", old, true},
		{".sento-import-x", old, true},
		{".sento-stage-y", now, false},
		{"unrelated.cbz", old, false},
	}
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, f.mtime, f.mtime); err != nil {
			t.Fatal(err)
		}
	}
	CleanStaleTemps(dir)
	for _, f := range files {
		_, err := os.Stat(filepath.Join(dir, f.name))
		if f.swept && !os.IsNotExist(err) {
			t.Errorf("%s survived the sweep (stat err = %v)", f.name, err)
		}
		if !f.swept && err != nil {
			t.Errorf("%s was removed: %v", f.name, err)
		}
	}
}
