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
	"github.com/h-wb/sento/internal/profile"
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
	t.Parallel()
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
			(info.Volume != "1" && info.Volume != "2") || info.Number != "" ||
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
	t.Parallel()
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
	t.Parallel()
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
	if len(before) != 2 || !strings.Contains(before[0].Path, "editions") {
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
	t.Parallel()
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

func TestEditionFolders(t *testing.T) {
	t.Parallel()
	root := filepath.Join(string(filepath.Separator), "library")
	std := store.Edition{ID: 1, EditionName: "Standard Edition", Language: "en", Type: "volume"}
	color := store.Edition{ID: 2, EditionName: "Digital Colored Comics", Language: "en", Type: "volume"}
	stdFR := store.Edition{ID: 3, EditionName: "Standard Edition", Language: "fr", Type: "volume", Publisher: "Glénat"}
	stdKana := store.Edition{ID: 4, EditionName: "Standard Edition", Language: "fr", Type: "volume", Publisher: "Kana"}
	chapters := store.Edition{ID: 5, EditionName: "Standard Edition", Language: "en", Type: "chapter"}
	editions := []store.Edition{stdKana, color, std, stdFR, chapters}
	series := func(path string) *store.Series {
		return &store.Series{ID: 1, Title: "ONE PIECE", MangaBakaID: 377, Path: path}
	}
	perEdition := DefaultMediaSettings()
	perEdition.SeriesFolder = "{Series Title} [mangabaka-{seriesId}] [{edition}][{lang}]"
	split := perEdition
	split.VolumesRoot, split.ChaptersRoot = "Volumes", "Chapters"
	perVersion := split
	perVersion.SeriesFolder = SeriesFolderByEdition // names the version too
	name := "ONE PIECE [mangabaka-377]"

	got := editionFolders(root, series(""), DefaultMediaSettings(), editions, nil)
	for k, dir := range got {
		if dir != filepath.Join(root, name) {
			t.Errorf("without edition tokens, edition %d goes to %q", k.EditionID, dir)
		}
	}
	colored := []store.CollectedVersion{
		{ID: 10, EditionID: 1, Name: "Default", IsDefault: true},
		{ID: 11, EditionID: 1, Name: "Colored"},
		{ID: 12, EditionID: 1, Name: "Colored"}, // same name as 11: told apart
		{ID: 13, EditionID: 5, Name: "Colored"}, // a chapter edition's version
	}
	for _, tc := range []struct {
		label string
		ms    MediaSettings
		path  string
		want  map[folderKey]string
	}{
		{"one folder per edition", split, "", map[folderKey]string{
			{1, 0}: filepath.Join(root, "Volumes", name+" [Standard Edition][en]"),
			{2, 0}: filepath.Join(root, "Volumes", name+" [Digital Colored Comics][en]"),
			{3, 0}: filepath.Join(root, "Volumes", name+" [Standard Edition][fr]"),
			// Same render as edition 3: the folder is shared.
			{4, 0}: filepath.Join(root, "Volumes", name+" [Standard Edition][fr]"),
			{5, 0}: filepath.Join(root, "Chapters", name+" [Standard Edition][en]"),
		}},
		{"one folder per edition and version", perVersion, "", map[folderKey]string{
			{1, 0}:  filepath.Join(root, "Volumes", name+" [Standard Edition][en]"),
			{1, 10}: filepath.Join(root, "Volumes", name+" [Standard Edition][en]"), // the default version keeps the edition's
			{1, 11}: filepath.Join(root, "Volumes", name+" [Standard Edition][en] (Colored)"),
			// Same render as version 11: the folder is shared.
			{1, 12}: filepath.Join(root, "Volumes", name+" [Standard Edition][en] (Colored)"),
			{2, 0}:  filepath.Join(root, "Volumes", name+" [Digital Colored Comics][en]"),
			{5, 13}: filepath.Join(root, "Chapters", name+" [Standard Edition][en] (Colored)"),
		}},
		{"a kept folder name survives", perEdition, filepath.Join(root, "My One Piece"), map[folderKey]string{
			{1, 0}: filepath.Join(root, "My One Piece [Standard Edition][en]"),
			{2, 0}: filepath.Join(root, "My One Piece [Digital Colored Comics][en]"),
			// Volumes and chapters of one edition may share a folder.
			{5, 0}: filepath.Join(root, "My One Piece [Standard Edition][en]"),
		}},
		{"a kept folder name survives with versions", perVersion, filepath.Join(root, "My One Piece"), map[folderKey]string{
			{1, 11}: filepath.Join(root, "Volumes", "My One Piece [Standard Edition][en] (Colored)"),
		}},
		{"adopted from outside the library", perEdition, filepath.Join(string(filepath.Separator), "mnt", "OP"), map[folderKey]string{
			{1, 0}: filepath.Join(string(filepath.Separator), "mnt", "OP"),
			{2, 0}: filepath.Join(string(filepath.Separator), "mnt", "OP"),
		}},
	} {
		got := editionFolders(root, series(tc.path), tc.ms, editions, colored)
		for k, want := range tc.want {
			if got[k] != want {
				t.Errorf("%s: edition %d version %d in %q, want %q", tc.label, k.EditionID, k.VersionID, got[k], want)
			}
		}
	}
	// Grouping is by rendered name, so a version token needs no edition
	// token: the default version renders like the series, a named one
	// gets a sibling folder.
	if err := ValidateMediaSettings(func() MediaSettings { m := perVersion; m.SeriesFolder = "{Series Title} ({version})"; return m }()); err != nil {
		t.Errorf("a version token without an edition token was rejected: %v", err)
	}
	if err := ValidateMediaSettings(perVersion); err != nil {
		t.Errorf("a version token beside an edition token was rejected: %v", err)
	}

	// Folders group by the name in readers: {Reader Series} renders the
	// presented Series field, so a copy with its own name gets a sibling.
	byReader := DefaultMediaSettings()
	byReader.SeriesFolder = "{Reader Series} [mangabaka-{seriesId}]"
	fanTL := std
	fanTL.ID = 6
	fanTL.EditionName = "Fan Translation"
	fanTL.ReaderFields = store.ReaderFields{"series": "{Series Title} {edition} {version}"}
	got = editionFolders(root, series(""), byReader, append(slices.Clone(editions), fanTL), colored)
	if got[folderKey{1, 0}] != filepath.Join(root, name) {
		t.Errorf("the default copy stays in the series folder, got %q", got[folderKey{1, 0}])
	}
	if want := filepath.Join(root, "ONE PIECE Fan Translation [mangabaka-377]"); got[folderKey{6, 0}] != want {
		t.Errorf("own-name edition in %q, want %q", got[folderKey{6, 0}], want)
	}
	if want := filepath.Join(root, "ONE PIECE Colored [mangabaka-377]"); got[folderKey{1, 11}] != want {
		t.Errorf("colored version in %q, want %q", got[folderKey{1, 11}], want)
	}
}

// A series folder template that names the version puts each collected
// version's books in a top folder beside its edition's own.
func TestBookDirNamesVersionFolders(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	lib, st, sid, standard, _, version := modelFixture(t)
	colored, err := st.AddProfile(ctx, profile.Profile{Name: "Colored", FormatScores: map[int64]int{}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := lib.SaveVersion(ctx, sid, store.CollectedVersion{EditionID: standard.ID, Monitored: true, ProfileID: colored})
	if err != nil {
		t.Fatal(err)
	}
	series, err := st.GetSeries(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	library := t.TempDir()
	im := NewImporter(st, provider.NewRegistry(&payloadProvider{}), library, nil)
	cat := newCatalogCache(st, sid)
	ms := DefaultMediaSettings()
	ms.SeriesFolder, ms.BookFolder, ms.VolumesRoot = SeriesFolderByEdition, BookFolderByVersion, "Volumes"
	base := filepath.Join(library, "Volumes", "ONE PIECE [mangabaka-377] [Standard Edition][en]")
	ref := store.BookRef{Ledger: "volume", EditionID: standard.ID, Number: 100}
	ref.VersionID = version.ID
	if dir := im.bookDir(ctx, cat, ms, "anywhere", series, ref); dir != base {
		t.Fatalf("default version in %q, want %q", dir, base)
	}
	ref.VersionID = second
	if dir := im.bookDir(ctx, cat, ms, "anywhere", series, ref); dir != base+" (Colored)" {
		t.Fatalf("colored version in %q, want %q", dir, base+" (Colored)")
	}
}

// A {version} token in the file templates names the version in the file
// name too, for readers that group series by the parsed file name.
func TestBookFileNameNamesVersion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	lib, st, sid, standard, _, version := modelFixture(t)
	colored, err := st.AddProfile(ctx, profile.Profile{Name: "Colored", FormatScores: map[int64]int{}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := lib.SaveVersion(ctx, sid, store.CollectedVersion{EditionID: standard.ID, Monitored: true, ProfileID: colored})
	if err != nil {
		t.Fatal(err)
	}
	series, err := st.GetSeries(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	im := NewImporter(st, provider.NewRegistry(&payloadProvider{}), t.TempDir(), nil)
	cat := newCatalogCache(st, sid)
	ms := DefaultMediaSettings()
	ms.VolumeFile = "{Series Title} {version} v{volume:00} [{edition}][{lang}].cbz"
	ref := func(vid int64) []store.BookRef {
		return []store.BookRef{{Ledger: "volume", EditionID: standard.ID, VersionID: vid, Number: 100}}
	}
	if name := im.bookFileName(ctx, cat, ms, series, ref(version.ID), "", ".cbz"); name != "ONE PIECE v01 [Standard Edition][en].cbz" {
		t.Fatalf("default version named %q, want %q", name, "ONE PIECE v01 [Standard Edition][en].cbz")
	}
	if name := im.bookFileName(ctx, cat, ms, series, ref(second), "", ".cbz"); name != "ONE PIECE Colored v01 [Standard Edition][en].cbz" {
		t.Fatalf("colored version named %q, want %q", name, "ONE PIECE Colored v01 [Standard Edition][en].cbz")
	}
}

// A collected version beside the default names itself in ComicInfo, so a
// reader that files by metadata rather than folders keeps the copies apart.
func TestFileInfoNamesVersion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	lib, st, sid, c, ws, version := modelFixture(t)
	colored, err := st.AddProfile(ctx, profile.Profile{Name: "Colored", FormatScores: map[int64]int{}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := lib.SaveVersion(ctx, sid, store.CollectedVersion{EditionID: c.ID, Monitored: true, ProfileID: colored})
	if err != nil {
		t.Fatal(err)
	}
	secondV, err := st.GetVersion(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	series, err := st.GetSeries(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	im := NewImporter(st, nil, t.TempDir(), nil)
	cat := newCatalogCache(st, sid)
	ms := DefaultMediaSettings()
	if info := im.fileInfo(ctx, cat, ms, series, []store.BookRef{modelRef(c, ws[1], version)}, tagSource{}); info.Series != "ONE PIECE" || info.SeriesGroup != "ONE PIECE" {
		t.Fatalf("default version Series, SeriesGroup = %q, %q, want ONE PIECE for both", info.Series, info.SeriesGroup)
	}
	if info := im.fileInfo(ctx, cat, ms, series, []store.BookRef{modelRef(c, ws[1], *secondV)}, tagSource{}); info.Series != "ONE PIECE Colored" || info.SeriesGroup != "ONE PIECE" {
		t.Fatalf("colored version Series, SeriesGroup = %q, %q, want %q, %q", info.Series, info.SeriesGroup, "ONE PIECE Colored", "ONE PIECE")
	}
}

// The series name readers see follows the library template, an edition's
// own replaces it, and a collected version still renders its name in it.
func TestFileInfoReaderFieldTemplates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	lib, st, sid, c, ws, version := modelFixture(t)
	colored, err := st.AddProfile(ctx, profile.Profile{Name: "Colored", FormatScores: map[int64]int{}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := lib.SaveVersion(ctx, sid, store.CollectedVersion{EditionID: c.ID, Monitored: true, ProfileID: colored})
	if err != nil {
		t.Fatal(err)
	}
	secondV, err := st.GetVersion(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	series, err := st.GetSeries(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	im := NewImporter(st, nil, t.TempDir(), nil)
	named := func(ms MediaSettings, v store.CollectedVersion) string {
		// A fresh cache per call: an organize pass snapshots the catalog
		// once, so one snapshot must not see the edition change.
		return im.fileInfo(ctx, newCatalogCache(st, sid), ms, series, []store.BookRef{modelRef(c, ws[1], v)}, tagSource{}).Series
	}
	ms := DefaultMediaSettings()
	ms.ReaderFields["series"] = "{Series Title} ({lang})"
	if got := named(ms, version); got != "ONE PIECE (en)" {
		t.Fatalf("library template: %q, want %q", got, "ONE PIECE (en)")
	}
	if err := st.SetEditionReaderFields(ctx, sid, c.ID, store.ReaderFields{"series": "{Series Title} {edition} {version}"}); err != nil {
		t.Fatal(err)
	}
	if got := named(ms, version); got != "ONE PIECE Standard Edition" {
		t.Fatalf("edition template: %q, want %q", got, "ONE PIECE Standard Edition")
	}
	if got := named(ms, *secondV); got != "ONE PIECE Standard Edition Colored" {
		t.Fatalf("edition template, colored version: %q, want %q", got, "ONE PIECE Standard Edition Colored")
	}
	// A template that renders nothing for this copy falls back to the default.
	if err := st.SetEditionReaderFields(ctx, sid, c.ID, store.ReaderFields{"series": "({version})"}); err != nil {
		t.Fatal(err)
	}
	if got := named(DefaultMediaSettings(), version); got != "ONE PIECE" {
		t.Fatalf("empty render: %q, want %q", got, "ONE PIECE")
	}
}

// With the edition in the series folder template, organising moves an
// edition's books out of the series folder into the edition's own folder,
// flat, and a scan still finds them there.
func TestOrganizeMovesBooksIntoEditionFolders(t *testing.T) {
	t.Parallel()
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
	im := NewImporter(st, provider.NewRegistry(&payloadProvider{dir: downloads, files: files}), library, nil)
	queueVolumes(t, st, seriesID, standard, "abc123", 100, 200)
	if n, err := im.ProcessQueue(ctx); err != nil || n != 1 {
		t.Fatalf("import: %d %v", n, err)
	}

	ms := DefaultMediaSettings()
	ms.SeriesFolder, ms.BookFolder, ms.VolumesRoot = SeriesFolderByEdition, BookFolderByVersion, "Volumes"
	if err := SaveMediaSettings(ctx, st, ms); err != nil {
		t.Fatal(err)
	}
	edition := filepath.Join("Volumes", "ONE PIECE [mangabaka-377] [Standard Edition][en]")
	plan, err := im.PlanOrganize(ctx, seriesID)
	if err != nil || len(plan) != 2 {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	for _, c := range plan {
		if !c.Rename || filepath.Dir(c.To) != edition {
			t.Fatalf("planned change: %+v", c)
		}
	}
	if res, err := im.Organize(ctx, seriesID, true, true); err != nil || res.Renamed != 2 || len(res.Errors) != 0 {
		t.Fatalf("organize: %+v %v", res, err)
	}
	for _, f := range mustFiles(t, st, seriesID) {
		if _, err := os.Stat(f.Path); err != nil || filepath.Dir(f.Path) != filepath.Join(library, edition) {
			t.Errorf("file not flat in its edition folder: %s (%v)", f.Path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(library, "ONE PIECE [mangabaka-377]")); !os.IsNotExist(err) {
		t.Error("the old series folder was left behind")
	}
	if plan, err = im.PlanOrganize(ctx, seriesID); err != nil || len(plan) != 0 {
		t.Fatalf("second plan: %+v %v", plan, err)
	}
	res, err := NewScanner(st, library).ScanSeriesFolder(ctx, seriesID, "")
	if err != nil || len(res.Skipped) != 2 {
		t.Fatalf("scan of the edition folders: %+v %v", res, err)
	}
}

func mustFiles(t *testing.T, st *store.Store, seriesID int64) []store.File {
	t.Helper()
	files, err := st.ListFiles(context.Background(), seriesID)
	if err != nil || len(files) == 0 {
		t.Fatalf("files: %+v %v", files, err)
	}
	return files
}

// An archive holding several books is named and titled for its range,
// not for the first book in it.
func TestMultiBookFileIsNamedForItsRange(t *testing.T) {
	t.Parallel()
	_, st, id, c, ws, version := modelFixture(t)
	ctx := context.Background()
	im := NewImporter(st, nil, t.TempDir(), nil)
	series, _ := st.GetSeries(ctx, id)
	ms := DefaultMediaSettings()
	slots := []store.BookRef{modelRef(c, ws[1], version), modelRef(c, ws[0], version)}
	cat := newCatalogCache(st, id)
	if got := im.bookFileName(ctx, cat, ms, series, slots, "", ".cbz"); !strings.Contains(got, "v01-02") {
		t.Errorf("range name = %q", got)
	}
	if got := im.bookFileName(ctx, cat, ms, series, slots[:1], "", ".cbz"); !strings.Contains(got, "v02") || strings.Contains(got, "-") {
		t.Errorf("single name = %q", got)
	}
	info := im.fileInfo(ctx, cat, ms, series, slots, tagSource{})
	if info.Title != "Volumes 1–2" || info.Number != "" {
		t.Errorf("pack info = %+v", info)
	}
}

// The files inside a release grabbed for an edition are placed by their
// numbers: a name with no title is the release's series, and an edition
// word the series has no edition for does not send the file to review.
// One that another edition answers to still does.
func TestFilesInsideAGrabTrustItsEdition(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		files  []string
		review bool
	}{
		{"numbers alone", []string{"Volume 1.cbz", "Volume 2.cbz"}, false},
		{"an edition word nothing answers to", []string{"ONE PIECE v01 (Collector's Edition).cbz", "ONE PIECE v02 (Collector's Edition).cbz"}, false},
		{"an edition the series has", []string{"ONE PIECE v01 (Omnibus Edition).cbz", "ONE PIECE v02 (Omnibus Edition).cbz"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lib, st := setup(t)
			seriesID, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
			if err != nil {
				t.Fatal(err)
			}
			cols, _ := st.ListEditions(ctx, seriesID)
			var standard int64
			for _, c := range cols {
				if c.EditionName == "Standard Edition" && c.Type == "volume" && c.Language == "en" {
					standard = c.ID
				}
			}
			downloads := t.TempDir()
			for _, name := range tc.files {
				writeCBZ(t, filepath.Join(downloads, name), name)
			}
			im := NewImporter(st, provider.NewRegistry(&payloadProvider{dir: downloads, files: tc.files}), t.TempDir(), nil)
			queueVolumes(t, st, seriesID, standard, "abc123", 100, 200)
			_, _ = im.ProcessQueue(ctx)
			files, _ := st.ListFiles(ctx, seriesID)
			reviewed := 0
			for _, f := range files {
				if f.NeedsReview {
					reviewed++
				}
			}
			if len(files) != 2 || (reviewed > 0) != tc.review {
				t.Fatalf("%d files, %d for review; want review=%v", len(files), reviewed, tc.review)
			}
		})
	}
}
