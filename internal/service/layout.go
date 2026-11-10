package service

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/store"
)

// Where a series' books go: the tokens templates render with and the
// folders a series, its editions and their collected versions live in
// (docs/domain/editions.md, spec.md §11.2).

func seriesTokens(series *store.Series) mediafile.NameTokens {
	return mediafile.NameTokens{SeriesTitle: series.Title, SeriesID: series.MangaBakaID, Volume: -1, Chapter: -1, Year: series.Year}
}

// seriesFolderTokens are what the series folder renders with for the series
// itself: the reader series name of a default copy, so a template using
// {Reader Series} and a kept folder name agree on the series' own folder.
func seriesFolderTokens(ms MediaSettings, series *store.Series) mediafile.NameTokens {
	t := seriesTokens(series)
	t.ReaderSeries = readerValues(ms, series, store.Edition{}, nil)["series"]
	return t
}

// editionTokens are the tokens a folder of one edition's books renders with.
func editionTokens(series *store.Series, e store.Edition) mediafile.NameTokens {
	t := seriesTokens(series)
	t.Edition, t.Lang, t.Publisher, t.Medium, t.EditionID = e.EditionName, e.Language, e.Publisher, e.Medium, e.ID
	t.Type = "Volumes"
	if e.Type == "chapter" {
		t.Type = "Chapters"
	}
	return t
}

// folderKey identifies the top folder a book goes in: its edition, and
// the collected version. VersionID 0 names the edition's own folder, the
// one its default version and unversioned books live in.
type folderKey struct{ EditionID, VersionID int64 }

// editionFolders maps each edition — and each collected version — to the
// top folder its books go in: the series folder template rendered for that
// copy, reader name included. Copies that render the same folder share it;
// a different render gets a sibling folder, keeping the series' own folder
// name where the render still starts with the series' plain one (a kept or
// adopted name survives). The default version keeps its edition's folder.
// A series adopted from outside the library root stays where it is.
func editionFolders(libraryRoot string, series *store.Series, ms MediaSettings, editions []store.Edition, versions []store.CollectedVersion) map[folderKey]string {
	volumes, chapters := seriesFolders(libraryRoot, series, ms)
	plain := mediafile.Render(ms.SeriesFolder, seriesFolderTokens(ms, series))
	sorted := slices.Clone(editions)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	ofEdition := map[int64][]store.CollectedVersion{}
	for _, v := range versions {
		ofEdition[v.EditionID] = append(ofEdition[v.EditionID], v)
	}
	out := map[folderKey]string{}
	taken := map[string]string{}
	folderFor := func(base, typ string, e store.Edition, v *store.CollectedVersion) string {
		name := mediafile.Render(ms.SeriesFolder, copyTokens(ms, series, e, v))
		if name == "" || name == plain {
			return base
		}
		if rest, ok := strings.CutPrefix(name, plain); ok && plain != "" {
			name = filepath.Base(base) + rest
		}
		dir := filepath.Join(filepath.Dir(base), mediafile.Sanitize(name))
		key := typ + "\x00" + dir
		if prev, ok := taken[key]; ok {
			return prev // the same rendered folder is shared
		}
		taken[key] = dir
		return dir
	}
	for _, e := range sorted {
		base := volumes
		if e.Type == "chapter" {
			base = chapters
		}
		out[folderKey{e.ID, 0}] = base
		if rel, err := filepath.Rel(libraryRoot, base); err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			continue // adopted from outside the library: left where it is
		}
		dir := folderFor(base, e.Type, e, nil)
		out[folderKey{e.ID, 0}] = dir
		for _, v := range ofEdition[e.ID] {
			if v.IsDefault {
				out[folderKey{e.ID, v.ID}] = dir
				continue
			}
			out[folderKey{e.ID, v.ID}] = folderFor(base, e.Type, e, &v)
		}
	}
	return out
}

// cleanRoot splits a top-folder setting into safe path segments.
func cleanRoot(root string) []string {
	var out []string
	for _, segment := range strings.Split(filepath.ToSlash(root), "/") {
		if s := mediafile.Sanitize(segment); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// seriesFolders is where a series' volumes and chapters belong under the
// current settings. The series keeps the folder it already has, re-rooted
// under the configured top folder; chapters get their own folder only when
// their top folder differs from the volumes'. A series adopted from
// outside the library root stays where it is.
func seriesFolders(libraryRoot string, series *store.Series, ms MediaSettings) (volumes, chapters string) {
	vr, cr := cleanRoot(ms.VolumesRoot), cleanRoot(ms.ChaptersRoot)
	// One folder name, whatever the stored template holds.
	rest := []string{mediafile.Sanitize(mediafile.Render(ms.SeriesFolder, seriesFolderTokens(ms, series)))}
	if series.Path != "" {
		rel, err := filepath.Rel(libraryRoot, series.Path)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return series.Path, series.Path
		}
		rest = strings.Split(filepath.ToSlash(rel), "/")
		// A series already split by type shows its old top folders: what
		// its two folders share from the end is the series' own part.
		if crel, err := filepath.Rel(libraryRoot, series.ChapterPath); series.ChapterPath != "" && err == nil && !strings.HasPrefix(crel, "..") {
			other := strings.Split(filepath.ToSlash(crel), "/")
			n := 0
			for n < len(rest) && n < len(other) && rest[len(rest)-1-n] == other[len(other)-1-n] {
				n++
			}
			if n > 0 {
				rest = rest[len(rest)-n:]
			}
		}
		// Drop a top folder the series already sits under before re-rooting.
		for _, root := range [][]string{vr, cr} {
			if len(root) > 0 && len(rest) > len(root) && slices.Equal(rest[:len(root)], root) {
				rest = rest[len(root):]
				break
			}
		}
	}
	join := func(root []string) string {
		return filepath.Join(append(append([]string{libraryRoot}, root...), rest...)...)
	}
	volumes = join(vr)
	chapters = volumes
	if !slices.Equal(vr, cr) {
		chapters = join(cr)
	}
	return volumes, chapters
}

// baseCopyTokens are what a copy's presented fields render with: its
// series, its edition and, beside the edition's default version, the
// version. They do not include the presented names themselves, so a
// presented-field template cannot name {Reader Series}.
func baseCopyTokens(series *store.Series, e store.Edition, v *store.CollectedVersion) mediafile.NameTokens {
	t := editionTokens(series, e)
	if v != nil {
		t.VersionID = v.ID
		if !v.IsDefault {
			t.Version = v.Name
		}
	}
	return t
}

// copyTokens are what a copy's paths render with: the base tokens plus its
// reader series name, so {Reader Series} in a folder or file template is
// the same name readers file the copy under.
func copyTokens(ms MediaSettings, series *store.Series, e store.Edition, v *store.CollectedVersion) mediafile.NameTokens {
	t := baseCopyTokens(series, e, v)
	t.ReaderSeries = readerValues(ms, series, e, v)["series"]
	return t
}
