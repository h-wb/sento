package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/h-wb/sento/internal/store"
)

// twoCopies is one book held twice: a file for the edition's default
// version and one for a second version collected as "Colored". It returns
// the importer, the two file ids and where the current settings put each.
func twoCopies(t *testing.T) (im *Importer, st *store.Store, sid int64, standard, colored int64, toStandard, toColored string) {
	t.Helper()
	_, st, sid, c, ws, v := modelFixture(t)
	ctx := context.Background()
	other := v
	other.ID, other.Name = 0, "Colored"
	var err error
	if other.ID, err = st.SaveVersion(ctx, sid, other); err != nil {
		t.Fatal(err)
	}
	library := t.TempDir()
	im = NewImporter(st, nil, library, nil)
	add := func(name string, version store.CollectedVersion) int64 {
		path := filepath.Join(library, "incoming", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		writeCBZ(t, path, name)
		hash, err := fileHash(path)
		if err != nil {
			t.Fatal(err)
		}
		fid, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: path, Hash: hash}, nil, []store.BookLink{{EditionID: c.ID, BookID: ws[0].ID, VersionID: version.ID}})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.MapFile(ctx, sid, fid, []store.BookRef{modelRef(c, ws[0], version)}); err != nil {
			t.Fatal(err)
		}
		return fid
	}
	standard, colored = add("standard.cbz", v), add("colored.cbz", other)
	plans, err := im.PlanOrganize(ctx, sid)
	if err != nil || len(plans) != 2 {
		t.Fatalf("plan: %+v %v", plans, err)
	}
	for _, p := range plans {
		if p.FileID == standard {
			toStandard = filepath.Join(library, p.To)
		} else {
			toColored = filepath.Join(library, p.To)
		}
	}
	return im, st, sid, standard, colored, toStandard, toColored
}

// moveFile puts a recorded file somewhere else on disk and in the store.
func moveFile(t *testing.T, st *store.Store, sid, fileID int64, dest string) {
	t.Helper()
	ctx := context.Background()
	for _, f := range mustFiles(t, st, sid) {
		if f.ID != fileID {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(f.Path, dest); err != nil {
			t.Fatal(err)
		}
		if err := st.SetFileLocation(ctx, f.ID, dest, f.Size, f.Hash); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatalf("no file %d", fileID)
}

func pathOf(t *testing.T, st *store.Store, sid, fileID int64) store.File {
	t.Helper()
	for _, f := range mustFiles(t, st, sid) {
		if f.ID == fileID {
			return f
		}
	}
	t.Fatalf("no file %d", fileID)
	return store.File{}
}

// One run converges: a file whose clean name is held by a sibling that
// moves later in the same run still ends on that name (#60).
func TestOrganizeFreesADestinationBeforeUsingIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	im, st, sid, standard, colored, toStandard, toColored := twoCopies(t)
	// The colored copy sits on the standard's name; the standard one took
	// the fingerprinted name beside it, which sorts first.
	hash := pathOf(t, st, sid, standard).Hash
	moveFile(t, st, sid, colored, toStandard)
	moveFile(t, st, sid, standard, assetPath(filepath.Dir(toStandard), filepath.Base(toStandard), hash))

	res, err := im.Organize(ctx, sid, true, true)
	if err != nil || len(res.Errors) != 0 || res.Renamed != 2 {
		t.Fatalf("organize: %+v %v", res, err)
	}
	if got := pathOf(t, st, sid, standard).Path; got != toStandard {
		t.Errorf("standard copy at %s, want %s", got, toStandard)
	}
	if got := pathOf(t, st, sid, colored).Path; got != toColored {
		t.Errorf("colored copy at %s, want %s", got, toColored)
	}
	if plan, err := im.PlanOrganize(ctx, sid); err != nil || len(plan) != 0 {
		t.Fatalf("a second run has work left: %+v %v", plan, err)
	}
}

// Two files on each other's names both end in place after one run.
func TestOrganizeSwapsTwoFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	im, st, sid, standard, colored, toStandard, toColored := twoCopies(t)
	moveFile(t, st, sid, standard, toColored)
	moveFile(t, st, sid, colored, toStandard)

	res, err := im.Organize(ctx, sid, true, true)
	if err != nil || len(res.Errors) != 0 || res.Renamed != 2 {
		t.Fatalf("organize: %+v %v", res, err)
	}
	for id, want := range map[int64]string{standard: toStandard, colored: toColored} {
		f := pathOf(t, st, sid, id)
		if _, err := os.Stat(f.Path); err != nil || f.Path != want {
			t.Errorf("file %d at %s (%v), want %s", id, f.Path, err, want)
		}
	}
	if plan, err := im.PlanOrganize(ctx, sid); err != nil || len(plan) != 0 {
		t.Fatalf("a second run has work left: %+v %v", plan, err)
	}
}

// A file on its fingerprinted name because another book keeps the plain
// one is in place: it is neither planned nor counted as renamed again.
func TestOrganizeLeavesAFallbackNameWhoseOwnerStays(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	im, st, sid, _, _, _, _ := twoCopies(t)
	// Flat, version-blind templates: both copies render one path.
	ms := DefaultMediaSettings()
	ms.BookFolder = BookFolderFlat
	if err := SaveMediaSettings(ctx, st, ms); err != nil {
		t.Fatal(err)
	}
	res, err := im.Organize(ctx, sid, true, true)
	if err != nil || len(res.Errors) != 0 || res.Renamed != 2 {
		t.Fatalf("organize: %+v %v", res, err)
	}
	files := mustFiles(t, st, sid)
	if filepath.Dir(files[0].Path) != filepath.Dir(files[1].Path) || files[0].Path == files[1].Path {
		t.Fatalf("copies should share the folder under two names: %s, %s", files[0].Path, files[1].Path)
	}
	if plan, err := im.PlanOrganize(ctx, sid); err != nil || len(plan) != 0 {
		t.Fatalf("the copy on its fallback name is planned again: %+v %v", plan, err)
	}
	if res, err = im.Organize(ctx, sid, true, true); err != nil || res.Renamed != 0 || res.Retagged != 0 {
		t.Fatalf("a second run moved something: %+v %v", res, err)
	}
}
