package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// writeCBZ writes a small valid book: one page per entry, wrapped in a
// folder with a junk file the way scene packs ship them. seed makes the
// pages, and so the fingerprint, distinct.
func writeCBZ(t *testing.T, path, seed string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"Book (Digital)/001.jpg": "page one of " + seed, "Book (Digital)/002.jpg": "page two of " + seed,
		"Book (Digital)/Thumbs.db": "junk",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// readBook returns a library book's entry names and its ComicInfo.
func readBook(t *testing.T, path string) ([]string, *mediafile.ComicInfo) {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("library file %s is not a readable cbz: %v", path, err)
	}
	defer func() { _ = zr.Close() }()
	var names []string
	var info *mediafile.ComicInfo
	for _, f := range zr.File {
		names = append(names, f.Name)
		if f.Name == "ComicInfo.xml" {
			rc, _ := f.Open()
			data, _ := io.ReadAll(rc)
			_ = rc.Close()
			if info, err = mediafile.ParseComicInfo(data); err != nil {
				t.Fatal(err)
			}
		}
	}
	return names, info
}

// payloadProvider is a download client whose every item is finished and
// sits in dir (the torrent shape: files left in place for seeding).
type payloadProvider struct {
	dir   string
	files []string
}

func (p *payloadProvider) ID() string       { return "torrent" }
func (p *payloadProvider) Protocol() string { return provider.ProtocolTorrent }
func (p *payloadProvider) Sites(context.Context) ([]provider.Site, error) {
	return nil, nil
}
func (p *payloadProvider) Search(context.Context, provider.SearchRequest) ([]provider.Candidate, error) {
	return nil, nil
}
func (p *payloadProvider) Grab(context.Context, provider.Candidate) (string, error) {
	return "", nil
}
func (p *payloadProvider) State(context.Context, string) (provider.DownloadState, error) {
	return provider.DownloadState{Done: true}, nil
}
func (p *payloadProvider) Fetch(context.Context, string) (*provider.Payload, error) {
	return &provider.Payload{Dir: p.dir, Files: p.files}, nil
}
func (p *payloadProvider) Cleanup(context.Context, string) error { return nil }

func queueVolumes(t *testing.T, st *store.Store, seriesID, editionID int64, hash string, numbers ...int64) int64 {
	t.Helper()
	slots := make([]store.BookRef, 0, len(numbers))
	for _, n := range numbers {
		versions, err := st.ListVersions(context.Background(), seriesID)
		if err != nil {
			t.Fatal(err)
		}
		works, err := st.ListBooks(context.Background(), editionID)
		if err != nil {
			t.Fatal(err)
		}
		var versionID, bookID int64
		for _, version := range versions {
			if version.EditionID == editionID && version.IsDefault {
				versionID = version.ID
			}
		}
		for _, work := range works {
			if work.SequenceNum == n {
				bookID = work.ID
			}
		}
		slots = append(slots, store.BookRef{Ledger: "volume", EditionID: editionID, Number: n, VersionID: versionID, BookID: bookID})
	}
	raw, _ := json.Marshal(slots)
	id, err := st.AddQueueItem(context.Background(), store.QueueItem{
		SeriesID: seriesID, Protocol: provider.ProtocolTorrent, ExternalID: hash,
		Title: "ONE PIECE v01-v02", Books: raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// The torrent path installs each payload book into the library as a clean,
// tagged CBZ. Grabbing the same release again (after a failed first
// import, or a manual re-grab) must converge: same library files, no
// duplicate rows, and — above all — the seeding payload left byte-for-byte
// intact.
func TestImportTorrentPayloadIsIdempotent(t *testing.T) {
	ctx := context.Background()
	lib, st := setup(t)
	seriesID, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}
	cols, _ := st.ListEditions(ctx, seriesID)
	var standard int64
	for _, c := range cols {
		if c.EditionName == "Standard Edition" {
			standard = c.ID
		}
	}

	downloads, library := t.TempDir(), t.TempDir()
	files := []string{"ONE PIECE v01.cbz", "ONE PIECE v02.cbz", "cover.jpg"}
	payload := map[string][]byte{}
	for _, name := range files[:2] {
		payload[name] = writeCBZ(t, filepath.Join(downloads, name), name)
	}
	if err := os.WriteFile(filepath.Join(downloads, "cover.jpg"), []byte("cover"), 0o644); err != nil {
		t.Fatal(err)
	}
	im := NewImporter(st, provider.NewRegistry(&payloadProvider{dir: downloads, files: files}), library, nil)

	for pass := 1; pass <= 2; pass++ {
		qid := queueVolumes(t, st, seriesID, standard, "abc123", 100, 200)
		n, err := im.ProcessQueue(ctx)
		if err != nil || n != 1 {
			t.Fatalf("pass %d: imported %d (err %v), want 1", pass, n, err)
		}
		items, _ := st.ListQueueItems(ctx, false)
		for _, it := range items {
			if it.ID == qid && it.Status != store.QueueImported {
				t.Fatalf("pass %d: queue item is %q (%s), want imported", pass, it.Status, it.Error)
			}
		}
		for _, name := range files[:2] {
			got, err := os.ReadFile(filepath.Join(downloads, name))
			if err != nil || !bytes.Equal(got, payload[name]) {
				t.Fatalf("pass %d: seeding payload %s changed (err %v), want it untouched", pass, name, err)
			}
		}
	}

	libFiles, err := st.ListFiles(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	if len(libFiles) != 2 {
		t.Fatalf("library has %d file rows, want 2: %+v", len(libFiles), libFiles)
	}
	for _, f := range libFiles {
		// Self-contained: readable layout and name, pages flattened, junk
		// gone, and a ComicInfo that identifies the book on its own.
		rel, _ := filepath.Rel(library, f.Path)
		if filepath.Dir(rel) != filepath.Join("ONE PIECE [mangabaka-377]", "Standard Edition [en]") || len(filepath.Base(rel)) > 60 {
			t.Errorf("library path %q does not follow the edition layout", rel)
		}
		names, info := readBook(t, f.Path)
		if strings.Join(names, ",") != "001.jpg,002.jpg,ComicInfo.xml" && strings.Join(names, ",") != "002.jpg,001.jpg,ComicInfo.xml" {
			t.Errorf("%s entries = %v", rel, names)
		}
		if info == nil || info.Series != "ONE PIECE" || info.LanguageISO != "en" || info.PageCount != "2" ||
			(info.Volume != "1" && info.Volume != "2") || info.Number != info.Volume ||
			!strings.Contains(info.Notes, "sento:v1") || !strings.Contains(info.Notes, `release:"ONE PIECE v01-v02"`) {
			t.Errorf("%s ComicInfo = %+v", rel, info)
		}
		// The library copy and the payload are the same book.
		src := filepath.Join(downloads, "ONE PIECE v0"+info.Volume+".cbz")
		if want, _ := mediafile.Fingerprint(src); f.Hash != want || f.Hash == "" {
			t.Errorf("%s fingerprint %q, payload %q", rel, f.Hash, want)
		}
	}
	for _, b := range booksOf(t, lib, st, seriesID, standard)[:2] {
		if len(b.OnDiskFileIDs) != 1 {
			t.Fatalf("volume %s has %d files, want 1", b.Book.SequenceStr, len(b.OnDiskFileIDs))
		}
	}
}

func TestMapPath(t *testing.T) {
	im := NewImporter(nil, nil, "", []string{"/data=/mnt/seedbox", "/data/complete=/mnt/complete", "bogus"})
	for remote, want := range map[string]string{
		"/data/complete/sento":   "/mnt/complete/sento", // longest prefix wins
		"/data/incoming/x":       "/mnt/seedbox/incoming/x",
		"/elsewhere/unmapped":    "/elsewhere/unmapped",
		"/data/complete":         "/mnt/complete",
		"/database/not-a-prefix": "/database/not-a-prefix",
	} {
		if got := im.mapPath(remote); got != want {
			t.Errorf("mapPath(%q) = %q, want %q", remote, got, want)
		}
	}
}

// Organising brings existing files in line with new settings: a layout
// change moves them, a metadata change rewrites them, and neither changes
// which book they are.
func TestOrganizeRenamesAndRetags(t *testing.T) {
	ctx := context.Background()
	lib, st := setup(t)
	seriesID, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}
	cols, _ := st.ListEditions(ctx, seriesID)
	var standard int64
	for _, c := range cols {
		if c.EditionName == "Standard Edition" {
			standard = c.ID
		}
	}
	downloads, library := t.TempDir(), t.TempDir()
	files := []string{"ONE PIECE v01.cbz", "ONE PIECE v02.cbz"}
	for _, name := range files {
		writeCBZ(t, filepath.Join(downloads, name), name)
	}
	// Imported the old way: hardlinked, untagged, id folders, hash in the name.
	old := DefaultMediaSettings()
	old.WriteComicInfo, old.BookFolder, old.HashInName = false, BookFolderByID, true
	if err := SaveMediaSettings(ctx, st, old); err != nil {
		t.Fatal(err)
	}
	im := NewImporter(st, provider.NewRegistry(&payloadProvider{dir: downloads, files: files}), library, nil)
	queueVolumes(t, st, seriesID, standard, "abc123", 100, 200)
	if n, err := im.ProcessQueue(ctx); err != nil || n != 1 {
		t.Fatalf("import: %d %v", n, err)
	}
	before, _ := st.ListFiles(ctx, seriesID)
	if len(before) != 2 || !strings.Contains(before[0].Path, filepath.Join("editions")) {
		t.Fatalf("old-layout import: %+v", before)
	}
	if _, info := readBook(t, before[0].Path); info != nil {
		t.Fatal("embedding was off, yet the file is tagged")
	}

	// New settings: nothing to do until asked, and the preview says what.
	if err := SaveMediaSettings(ctx, st, DefaultMediaSettings()); err != nil {
		t.Fatal(err)
	}
	plan, err := im.PlanOrganize(ctx, seriesID)
	if err != nil || len(plan) != 2 {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	for _, c := range plan {
		if !c.Rename || !c.Retag || c.Skip != "" || !strings.HasPrefix(c.To, filepath.Join("ONE PIECE [mangabaka-377]", "Standard Edition [en]")+string(filepath.Separator)) || c.To == c.From {
			t.Fatalf("planned change: %+v", c)
		}
	}
	res, err := im.Organize(ctx, seriesID, true, true)
	if err != nil || res.Renamed != 2 || res.Retagged != 2 || len(res.Errors) != 0 {
		t.Fatalf("organize: %+v %v", res, err)
	}
	after, _ := st.ListFiles(ctx, seriesID)
	for i, f := range after {
		if _, err := os.Stat(before[i].Path); !os.IsNotExist(err) {
			t.Errorf("old file still at %s", before[i].Path)
		}
		if filepath.Dir(f.Path) != filepath.Join(library, "ONE PIECE [mangabaka-377]", "Standard Edition [en]") {
			t.Errorf("new path %s", f.Path)
		}
		names, info := readBook(t, f.Path)
		if info == nil || info.Series != "ONE PIECE" || info.Volume == "" || len(names) != 3 {
			t.Errorf("%s: entries %v, ComicInfo %+v", f.Path, names, info)
		}
		if f.Hash != before[i].Hash {
			t.Errorf("identity changed: %s → %s", before[i].Hash, f.Hash)
		}
	}
	if _, err := os.Stat(filepath.Join(library, "ONE PIECE [mangabaka-377]", "editions")); !os.IsNotExist(err) {
		t.Error("emptied id folders were left behind")
	}
	// The seeding payload was hardlinked to the old library file: still intact.
	for _, name := range files {
		if names, _ := readBook(t, filepath.Join(downloads, name)); len(names) != 3 || !slices.Contains(names, "Book (Digital)/Thumbs.db") {
			t.Errorf("payload %s was modified: %v", name, names)
		}
	}
	for _, b := range booksOf(t, lib, st, seriesID, standard)[:2] {
		if len(b.OnDiskFileIDs) != 1 {
			t.Fatalf("volume %s lost its file", b.Book.SequenceStr)
		}
	}
	// Up to date now: a second look finds nothing to do.
	if plan, err = im.PlanOrganize(ctx, seriesID); err != nil || len(plan) != 0 {
		t.Fatalf("second plan: %+v %v", plan, err)
	}

	// Splitting the library by type moves the series under its top folder,
	// keeps its folder name, and records the new locations.
	split := DefaultMediaSettings()
	split.VolumesRoot, split.ChaptersRoot = "Volumes", "Chapters"
	if err := SaveMediaSettings(ctx, st, split); err != nil {
		t.Fatal(err)
	}
	if plan, err = im.PlanOrganize(ctx, seriesID); err != nil || len(plan) != 2 || plan[0].Retag ||
		!strings.HasPrefix(plan[0].To, filepath.Join("Volumes", "ONE PIECE [mangabaka-377]", "Standard Edition [en]")) {
		t.Fatalf("split plan: %+v %v", plan, err)
	}
	if sr, _ := st.GetSeries(ctx, seriesID); sr.ChapterPath != "" {
		t.Fatalf("a preview recorded a new chapter folder: %q", sr.ChapterPath)
	}
	if res, err = im.Organize(ctx, seriesID, true, true); err != nil || res.Renamed != 2 || res.Retagged != 0 || len(res.Errors) != 0 {
		t.Fatalf("split organize: %+v %v", res, err)
	}
	sr, _ := st.GetSeries(ctx, seriesID)
	if sr.Path != filepath.Join(library, "Volumes", "ONE PIECE [mangabaka-377]") || sr.ChapterPath != filepath.Join(library, "Chapters", "ONE PIECE [mangabaka-377]") {
		t.Fatalf("series folders: %q / %q", sr.Path, sr.ChapterPath)
	}
	moved, _ := st.ListFiles(ctx, seriesID)
	for _, f := range moved {
		if _, err := os.Stat(f.Path); err != nil || !strings.HasPrefix(f.Path, sr.Path+string(filepath.Separator)) {
			t.Errorf("file not under the volumes folder: %s (%v)", f.Path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(library, "ONE PIECE [mangabaka-377]")); !os.IsNotExist(err) {
		t.Error("the old series folder was left behind")
	}
	// And back to one folder per series.
	if err := SaveMediaSettings(ctx, st, DefaultMediaSettings()); err != nil {
		t.Fatal(err)
	}
	if res, err = im.Organize(ctx, seriesID, true, true); err != nil || res.Renamed != 2 || len(res.Errors) != 0 {
		t.Fatalf("merge back: %+v %v", res, err)
	}
	if sr, _ = st.GetSeries(ctx, seriesID); sr.Path != filepath.Join(library, "ONE PIECE [mangabaka-377]") || sr.ChapterPath != "" {
		t.Fatalf("series folders after merging back: %q / %q", sr.Path, sr.ChapterPath)
	}
	if _, err := os.Stat(filepath.Join(library, "Volumes")); !os.IsNotExist(err) {
		t.Error("the emptied Volumes folder was left behind")
	}
}

func TestSeriesFolders(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "library")
	base := DefaultMediaSettings()
	split := base
	split.VolumesRoot, split.ChaptersRoot = "Volumes", "Chapters"
	onlyChapters := base
	onlyChapters.ChaptersRoot = "Chapters"
	series := func(path string) *store.Series {
		return &store.Series{Title: "ONE PIECE", MangaBakaID: 377, Path: path}
	}
	name := "ONE PIECE [mangabaka-377]"
	for _, tc := range []struct {
		label            string
		ms               MediaSettings
		path             string
		volumes, chapter string
	}{
		{"new series, one folder", base, "", filepath.Join(root, name), filepath.Join(root, name)},
		{"new series, split by type", split, "", filepath.Join(root, "Volumes", name), filepath.Join(root, "Chapters", name)},
		{"only chapters apart", onlyChapters, "", filepath.Join(root, name), filepath.Join(root, "Chapters", name)},
		{"existing folder keeps its name when re-rooted", split, filepath.Join(root, "My One Piece"), filepath.Join(root, "Volumes", "My One Piece"), filepath.Join(root, "Chapters", "My One Piece")},
		{"already under its top folder", split, filepath.Join(root, "Volumes", "My One Piece"), filepath.Join(root, "Volumes", "My One Piece"), filepath.Join(root, "Chapters", "My One Piece")},
		{"nested folders stay nested", split, filepath.Join(root, "Shonen", "OP"), filepath.Join(root, "Volumes", "Shonen", "OP"), filepath.Join(root, "Chapters", "Shonen", "OP")},
		{"untouched without top folders", base, filepath.Join(root, "Shonen", "OP"), filepath.Join(root, "Shonen", "OP"), filepath.Join(root, "Shonen", "OP")},
		{"split series merged back", base, filepath.Join(root, "Volumes", "OP"), filepath.Join(root, "OP"), filepath.Join(root, "OP")},
		{"adopted from outside the library", split, filepath.Join(string(filepath.Separator), "mnt", "OP"), filepath.Join(string(filepath.Separator), "mnt", "OP"), filepath.Join(string(filepath.Separator), "mnt", "OP")},
	} {
		sr := series(tc.path)
		if tc.label == "split series merged back" {
			sr.ChapterPath = filepath.Join(root, "Chapters", "OP")
		}
		v, c := seriesFolders(root, sr, tc.ms)
		if v != tc.volumes || c != tc.chapter {
			t.Errorf("%s: got %q / %q, want %q / %q", tc.label, v, c, tc.volumes, tc.chapter)
		}
	}
	if err := ValidateMediaSettings(func() MediaSettings { m := base; m.VolumesRoot = "../outside"; return m }()); err == nil {
		t.Error("a top folder escaping the library was accepted")
	}
}

// An archive holding several books is named and titled for its range,
// not for the first book in it.
func TestMultiBookFileIsNamedForItsRange(t *testing.T) {
	_, st, id, c, ws, version := modelFixture(t)
	ctx := context.Background()
	im := NewImporter(st, nil, t.TempDir(), nil)
	series, _ := st.GetSeries(ctx, id)
	ms := DefaultMediaSettings()
	slots := []store.BookRef{modelRef(c, ws[1], version), modelRef(c, ws[0], version)}
	if got := im.bookFileName(ctx, ms, series, slots, "", ".cbz"); !strings.Contains(got, "v01-02") {
		t.Errorf("range name = %q", got)
	}
	if got := im.bookFileName(ctx, ms, series, slots[:1], "", ".cbz"); !strings.Contains(got, "v02") || strings.Contains(got, "-") {
		t.Errorf("single name = %q", got)
	}
	info := im.packInfo(ctx, series, slots, "", "")
	if info.Title != "Volumes 1–2" || info.Number != "" {
		t.Errorf("pack info = %+v", info)
	}
}
