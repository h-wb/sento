package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/store"
)

// MediaSettings is the post-processing configuration: naming templates,
// transfer mode and metadata writing. Stored in the settings table so
// changes apply without a restart; defaults when unset.
type MediaSettings struct {
	SeriesFolder string `json:"seriesFolder"`
	VolumeFile   string `json:"volumeFile"`
	ChapterFile  string `json:"chapterFile"`
	ChapterDir   string `json:"chapterDir"` // "" = no chapter subfolder
	// VolumesRoot and ChaptersRoot are optional top folders under the
	// library root, above the series folder: "Volumes" and "Chapters"
	// give Volumes/<series>/… and Chapters/<series>/…. Equal (both empty
	// by default) keeps one folder per series.
	VolumesRoot  string `json:"volumesRoot"`
	ChaptersRoot string `json:"chaptersRoot"`
	// BookFolder is the folder layout inside a series folder, one level
	// per "/" ("" = every book directly in the series folder).
	BookFolder string `json:"bookFolder"`
	// HashInName appends the book's content fingerprint to its file name.
	// Off, the fingerprint is only added when two different books would
	// otherwise share a path.
	HashInName     bool `json:"hashInName"`
	RenameImports  bool `json:"renameImports"`
	AlwaysCopy     bool `json:"alwaysCopy"` // copy even when a hardlink would work
	WriteComicInfo bool `json:"writeComicInfo"`
	// ReaderFields are the templates for the ComicInfo fields readers
	// file and group a copy by (mediafile.PresentedFields), keyed by
	// field; an edition may set its own.
	ReaderFields map[string]string `json:"readerFields"`
}

const mediaSettingsKey = "media"

// Folder layout presets offered by the UI; any template works.
const (
	// "Standard Edition [en]", plus "(Group copy)" for an additional version.
	BookFolderByEdition = "{edition} [{lang}] ({version})"
	BookFolderFlat      = ""
	// "Volumes/Standard Edition [en]": the type split inside the series.
	BookFolderByType = "{type}/{edition} [{lang}] ({version})"
	// Stable internal ids: paths never change when an edition is renamed.
	BookFolderByID = "editions/{editionId}/versions/{versionId}"
	// One top folder per edition, and per collected version of one, for
	// readers that group by folder. The default version renders without
	// its " ({version})".
	SeriesFolderByEdition = "{Series Title} [mangabaka-{seriesId}] [{edition}][{lang}] ({version})"
	// Its book folder: flat, the version already named in the series folder.
	BookFolderByVersion = ""
	// The version split inside one edition folder, when the series folder
	// does not already name versions.
	BookFolderOnlyVersion = "({version})"
)

func DefaultMediaSettings() MediaSettings {
	n := mediafile.DefaultNaming()
	return MediaSettings{
		SeriesFolder: n.SeriesFolder, VolumeFile: n.VolumeFile,
		ChapterFile: n.ChapterFile, ChapterDir: n.ChapterDir,
		BookFolder:    BookFolderByEdition,
		RenameImports: true, WriteComicInfo: true,
		ReaderFields: defaultReaderFields(),
	}
}

func defaultReaderFields() map[string]string {
	out := map[string]string{}
	for _, f := range mediafile.PresentedFields {
		out[f.Key] = f.Default
	}
	return out
}

// Naming adapts the settings to the template renderer's config.
func (m MediaSettings) Naming() mediafile.NamingConfig {
	return mediafile.NamingConfig{
		SeriesFolder: m.SeriesFolder, VolumeFile: m.VolumeFile,
		ChapterFile: m.ChapterFile, ChapterDir: m.ChapterDir,
	}
}

// LoadMediaSettings reads the stored settings, falling back to defaults
// when unset or unreadable (imports must not fail over a settings row).
func LoadMediaSettings(ctx context.Context, st *store.Store) MediaSettings {
	def := DefaultMediaSettings()
	raw, err := st.GetSetting(ctx, mediaSettingsKey)
	if err != nil {
		if err != store.ErrNotFound {
			slog.Warn("loading media settings failed, using defaults", "error", err)
		}
		return def
	}
	if err := json.Unmarshal(raw, &def); err != nil {
		slog.Warn("media settings unreadable, using defaults", "error", err)
		return DefaultMediaSettings()
	}
	return def
}

// SaveMediaSettings validates and persists the settings.
func SaveMediaSettings(ctx context.Context, st *store.Store, m MediaSettings) error {
	if err := ValidateMediaSettings(m); err != nil {
		return err
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return st.PutSetting(ctx, mediaSettingsKey, raw)
}

// ValidateMediaSettings rejects templates that would collide or vanish:
// file templates need their number token, every template must render to
// something non-empty.
func ValidateMediaSettings(m MediaSettings) error {
	if strings.TrimSpace(m.SeriesFolder) == "" {
		return apperr.Invalidf("series folder template is empty")
	}
	if !strings.Contains(strings.ToLower(m.VolumeFile), "{volume") {
		return apperr.Invalidf("volume file template needs a {volume} token (files would overwrite each other)")
	}
	if !strings.Contains(strings.ToLower(m.ChapterFile), "{chapter") {
		return apperr.Invalidf("chapter file template needs a {chapter} token (files would overwrite each other)")
	}
	tokens := templateTokens(m.SeriesFolder)
	if (tokens["version"] || tokens["versionid"]) && !tokens["edition"] && !tokens["editionid"] && !tokens["collectionid"] {
		return apperr.Invalidf("series folder template cannot use version tokens without {edition} or {editionId}; a version is separated inside its edition")
	}
	if editionAware(m.SeriesFolder) && !tokens["edition"] && !tokens["editionid"] && !tokens["collectionid"] {
		return apperr.Invalidf("series folder template needs {edition} or {editionId} so two editions never share a folder")
	}
	if mediafile.Render(m.SeriesFolder, seriesTokens(&previewSeries)) == "" {
		return apperr.Invalidf("series folder template renders empty without an edition (every series would share a folder)")
	}
	if strings.Contains(m.BookFolder, "..") || strings.HasPrefix(strings.TrimSpace(m.BookFolder), "/") {
		return apperr.Invalidf("book folder template must stay inside the series folder")
	}
	for _, root := range []string{m.VolumesRoot, m.ChaptersRoot} {
		if strings.Contains(root, "..") || filepath.IsAbs(strings.TrimSpace(root)) {
			return apperr.Invalidf("top folders must be plain folder names inside the library")
		}
	}
	if err := validateReaderFields(m.ReaderFields, &previewSeries, previewVolumes, nil); err != nil {
		return err
	}
	for name, rendered := range PreviewMediaSettings(m) {
		if name == "bookFolder" || name == "volumePath" || name == "chapterPath" {
			continue // empty book folder = books directly in the series folder
		}
		if strings.TrimSpace(strings.TrimSuffix(rendered, ".cbz")) == "" {
			return apperr.Invalidf("%s template renders empty", name)
		}
	}
	return nil
}

var (
	previewSeries   = store.Series{Title: "ONE PIECE", MangaBakaID: 377, Year: 1997}
	previewVolumes  = store.Edition{ID: 42, EditionName: "Standard Edition", Language: "en", Type: "volume"}
	previewChapters = store.Edition{ID: 43, EditionName: "Standard Edition", Language: "en", Type: "chapter"}
)

// PreviewMediaSettings renders the templates against a fixed sample so the
// UI can show live results (and validation can probe them). Folders go
// through the same functions as real placement, so the preview cannot
// promise tokens a real path would drop.
func PreviewMediaSettings(m MediaSettings) map[string]string {
	root := string(filepath.Separator) + "library"
	folders := editionFolders(root, &previewSeries, m, []store.Edition{previewVolumes, previewChapters}, nil)
	sample := mediafile.NameTokens{
		SeriesTitle: "ONE PIECE", SeriesID: 377, Edition: "Standard Edition",
		Lang: "en", Group: "TCB Scans", Year: 1997,
	}
	volume := sample
	volume.Volume, volume.Chapter = 11200, -1 // v112
	chapter := sample
	chapter.Volume, chapter.Chapter = -1, 110250 // ch 1102.5
	// Whole example paths: top folder, series folder, book folder, file.
	full := func(e store.Edition, file string) string {
		tokens := editionTokens(&previewSeries, e)
		tokens.VersionID = 81
		parts := append([]string{folders[folderKey{e.ID, 0}]}, mediafile.RenderFolder(m.BookFolder, tokens)...)
		rel, _ := filepath.Rel(root, filepath.Join(append(parts, file)...))
		return filepath.ToSlash(rel)
	}
	book := editionTokens(&previewSeries, previewVolumes)
	book.VersionID = 81
	return map[string]string{
		"volumePath":   full(previewVolumes, mediafile.Render(m.VolumeFile, volume)),
		"chapterPath":  full(previewChapters, mediafile.Render(m.ChapterFile, chapter)),
		"bookFolder":   strings.Join(mediafile.RenderFolder(m.BookFolder, book), "/"),
		"seriesFolder": filepath.Base(folders[folderKey{previewVolumes.ID, 0}]),
		"volumeFile":   mediafile.Render(m.VolumeFile, volume),
		"chapterFile":  mediafile.Render(m.ChapterFile, chapter),
	}
}

// ReaderPreview is what readers would show for the sample edition, and
// for a second card of it collected as "Colored"; Error says why saving
// the field's template would be refused.
type ReaderPreview struct {
	First   string `json:"first"`
	Colored string `json:"colored"`
	Error   string `json:"error,omitempty"`
}

// PreviewReaderFields renders each presented field for the sample copies.
func PreviewReaderFields(m MediaSettings) map[string]ReaderPreview {
	first := readerValues(m, &previewSeries, previewVolumes, nil)
	colored := readerValues(m, &previewSeries, previewVolumes, &store.CollectedVersion{ID: 82, Name: "Colored"})
	out := map[string]ReaderPreview{}
	for key := range first {
		p := ReaderPreview{First: first[key], Colored: colored[key]}
		if err := validateReaderFields(map[string]string{key: m.ReaderFields[key]}, &previewSeries, previewVolumes, nil); err != nil {
			p.Error = err.Error()
		}
		out[key] = p
	}
	return out
}

var reTemplateToken = regexp.MustCompile(`\{([A-Za-z ]+?)(?::0+)?\}`)

// templateTokens is the set of tokens a template uses, lowercased.
func templateTokens(template string) map[string]bool {
	out := map[string]bool{}
	for _, m := range reTemplateToken.FindAllStringSubmatch(template, -1) {
		out[strings.ToLower(strings.TrimSpace(m[1]))] = true
	}
	return out
}

// editionAware says the series folder template names the edition, which
// gives every edition of a series its own top folder.
func editionAware(template string) bool {
	for token := range templateTokens(template) {
		switch token {
		case "edition", "lang", "publisher", "medium", "type", "editionid", "collectionid":
			return true
		}
	}
	return false
}

// versionAware says the series folder template names the collected
// version, which gives every version of an edition a top folder beside
// the edition's own.
func versionAware(template string) bool {
	tokens := templateTokens(template)
	return tokens["version"] || tokens["versionid"]
}

func seriesTokens(series *store.Series) mediafile.NameTokens {
	return mediafile.NameTokens{SeriesTitle: series.Title, SeriesID: series.MangaBakaID, Volume: -1, Chapter: -1, Year: series.Year}
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
// the collected version when the series folder template names versions.
// VersionID 0 names the edition's own folder, the one its default
// version and unversioned books live in.
type folderKey struct{ EditionID, VersionID int64 }

// editionFolders maps each edition — and each collected version, when
// the series folder template names versions — to the top folder its
// books go in. Without edition tokens in the series folder template that
// is the series' own folder. With them, each edition gets a sibling
// folder: the series' folder name plus what the edition adds to the
// template, so a kept or adopted folder name survives. A version token
// adds one sibling folder per collected version of an edition; the
// default one keeps the edition's folder, and so does a version whose
// name renders empty. Two folders of a type that would still meet in one
// path are told apart by id, the newer one renamed.
func editionFolders(libraryRoot string, series *store.Series, ms MediaSettings, editions []store.Edition, versions []store.CollectedVersion) map[folderKey]string {
	volumes, chapters := seriesFolders(libraryRoot, series, ms)
	aware := editionAware(ms.SeriesFolder)
	perVersion := versionAware(ms.SeriesFolder)
	plain := mediafile.Render(ms.SeriesFolder, seriesTokens(series))
	sorted := slices.Clone(editions)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	ofEdition := map[int64][]store.CollectedVersion{}
	for _, v := range versions {
		ofEdition[v.EditionID] = append(ofEdition[v.EditionID], v)
	}
	out := map[folderKey]string{}
	taken := map[string]bool{}
	for _, e := range sorted {
		base := volumes
		if e.Type == "chapter" {
			base = chapters
		}
		out[folderKey{e.ID, 0}] = base
		if rel, err := filepath.Rel(libraryRoot, base); !aware || err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			continue // adopted from outside the library: left where it is
		}
		dir := base
		name := mediafile.Render(ms.SeriesFolder, editionTokens(series, e))
		if name != plain {
			if rest, ok := strings.CutPrefix(name, plain); ok {
				name = filepath.Base(base) + rest
			}
			dir = filepath.Join(filepath.Dir(base), mediafile.Sanitize(name))
			key := e.Type + "\x00" + dir
			if taken[key] {
				dir = fmt.Sprintf("%s (edition %d)", dir, e.ID)
			}
			taken[key] = true
		}
		out[folderKey{e.ID, 0}] = dir
		if !perVersion {
			continue
		}
		for _, v := range ofEdition[e.ID] {
			out[folderKey{e.ID, v.ID}] = dir
			if v.IsDefault {
				continue
			}
			tokens := editionTokens(series, e)
			tokens.Version, tokens.VersionID = v.Name, v.ID
			vname := mediafile.Render(ms.SeriesFolder, tokens)
			if vname == name {
				continue // renders like the edition folder: stays in it
			}
			if rest, ok := strings.CutPrefix(vname, plain); ok {
				vname = filepath.Base(base) + rest
			}
			vdir := filepath.Join(filepath.Dir(base), mediafile.Sanitize(vname))
			vkey := e.Type + "\x00" + vdir
			if taken[vkey] {
				vdir = fmt.Sprintf("%s (version %d)", vdir, v.ID)
			}
			taken[vkey] = true
			out[folderKey{e.ID, v.ID}] = vdir
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
	rest := []string{mediafile.Render(ms.SeriesFolder, mediafile.NameTokens{
		SeriesTitle: series.Title, SeriesID: series.MangaBakaID, Volume: -1, Chapter: -1, Year: series.Year,
	})}
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
