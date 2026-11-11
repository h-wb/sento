package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
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
// something non-empty, and a {name} that is no token is a mistake.
func ValidateMediaSettings(m MediaSettings) error {
	if problems := mediaSettingsProblems(m); len(problems) > 0 {
		return apperr.Invalidf("%s", problems[0].message)
	}
	return validateReaderFields(m.ReaderFields, &previewSeries, previewVolumes, nil)
}

// MediaSettingsProblems says, per settings field, why saving would be
// refused, so the settings page can show it beside the field. Reader
// fields report theirs through PreviewReaderFields.
func MediaSettingsProblems(m MediaSettings) map[string]string {
	out := map[string]string{}
	for _, p := range mediaSettingsProblems(m) {
		if _, seen := out[p.field]; !seen {
			out[p.field] = p.message
		}
	}
	return out
}

type fieldProblem struct{ field, message string }

// mediaSettingsProblems checks the naming fields in the order the settings
// page shows them.
func mediaSettingsProblems(m MediaSettings) []fieldProblem {
	var out []fieldProblem
	add := func(field, format string, args ...any) {
		out = append(out, fieldProblem{field, fmt.Sprintf(format, args...)})
	}
	unknown := func(field, label, template string) {
		if names := mediafile.UnknownTokens(template); len(names) > 0 {
			add(field, "%s template uses an unknown token {%s}", label, names[0])
		}
	}

	unknown("seriesFolder", "series folder", m.SeriesFolder)
	switch {
	case strings.TrimSpace(m.SeriesFolder) == "":
		add("seriesFolder", "series folder template is empty")
	case strings.ContainsAny(m.SeriesFolder, `/\`):
		add("seriesFolder", "series folder template names one folder; nest with the top folders or the book folder instead")
	case mediafile.Sanitize(mediafile.Render(m.SeriesFolder, seriesFolderTokens(m, &previewSeries))) == "":
		add("seriesFolder", "series folder template renders empty (every series would share a folder)")
	}
	unknown("volumeFile", "volume file", m.VolumeFile)
	if !usesToken(m.VolumeFile, "volume") {
		add("volumeFile", "volume file template needs a {volume} token (files would overwrite each other)")
	}
	unknown("chapterFile", "chapter file", m.ChapterFile)
	if !usesToken(m.ChapterFile, "chapter") {
		add("chapterFile", "chapter file template needs a {chapter} token (files would overwrite each other)")
	}
	for field, root := range map[string]string{"volumesRoot": m.VolumesRoot, "chaptersRoot": m.ChaptersRoot} {
		if strings.Contains(root, "..") || filepath.IsAbs(strings.TrimSpace(root)) {
			add(field, "top folders must be plain folder names inside the library")
		}
	}
	unknown("bookFolder", "book folder", m.BookFolder)
	if strings.Contains(m.BookFolder, "..") || strings.HasPrefix(strings.TrimSpace(m.BookFolder), "/") {
		add("bookFolder", "book folder template must stay inside the series folder")
	}
	return out
}

// usesToken reports whether a template uses the named token.
func usesToken(template, name string) bool {
	return slices.ContainsFunc(mediafile.TemplateTokens(template), func(t mediafile.Token) bool {
		return strings.EqualFold(t.Name, name)
	})
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
	sample := copyTokens(m, &previewSeries, previewVolumes, nil)
	sample.Group = "TCB Scans"
	volume := sample
	volume.Volume, volume.Chapter = 11200, -1 // v112
	chapter := sample
	chapter.Volume, chapter.Chapter = -1, 110250 // ch 1102.5
	// Whole example paths: top folder, series folder, book folder, file.
	full := func(e store.Edition, file string) string {
		tokens := copyTokens(m, &previewSeries, e, nil)
		tokens.VersionID = 81
		parts := append([]string{folders[folderKey{e.ID, 0}]}, mediafile.RenderFolder(m.BookFolder, tokens)...)
		rel, _ := filepath.Rel(root, filepath.Join(append(parts, file)...))
		return filepath.ToSlash(rel)
	}
	book := copyTokens(m, &previewSeries, previewVolumes, nil)
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

// PathPart is one level of an example path and the setting that makes it:
// volumesRoot or chaptersRoot, seriesFolder, bookFolder, volumeFile or
// chapterFile.
type PathPart struct {
	Setting string `json:"setting"`
	Text    string `json:"text"`
}

// PathExample is where one sample book lands, level by level.
type PathExample struct {
	Label string     `json:"label"`
	Parts []PathPart `json:"parts"`
}

// PreviewPaths lays out where the sample books land, each level tagged with
// the setting behind it: a volume, the same volume collected again as
// "Colored" (where versions part ways on disk), and a chapter. They go
// through the folder functions real placement uses.
func PreviewPaths(m MediaSettings) []PathExample {
	root := string(filepath.Separator) + "library"
	first := store.CollectedVersion{ID: 81, EditionID: previewVolumes.ID, IsDefault: true}
	colored := store.CollectedVersion{ID: 82, EditionID: previewVolumes.ID, Name: "Colored"}
	chapters := store.CollectedVersion{ID: 83, EditionID: previewChapters.ID, IsDefault: true}
	folders := editionFolders(root, &previewSeries, m, []store.Edition{previewVolumes, previewChapters}, []store.CollectedVersion{first, colored, chapters})
	example := func(label string, e store.Edition, v store.CollectedVersion, rootSetting, fileSetting, fileTemplate string, volume, chapter int64) PathExample {
		out := PathExample{Label: label}
		rel, _ := filepath.Rel(root, folders[folderKey{e.ID, v.ID}])
		levels := strings.Split(filepath.ToSlash(rel), "/")
		for i, level := range levels {
			setting := "seriesFolder"
			if i < len(levels)-1 {
				setting = rootSetting
			}
			out.Parts = append(out.Parts, PathPart{setting, level})
		}
		tokens := copyTokens(m, &previewSeries, e, &v)
		for _, folder := range mediafile.RenderFolder(m.BookFolder, tokens) {
			out.Parts = append(out.Parts, PathPart{"bookFolder", folder})
		}
		tokens.Group = "TCB Scans"
		tokens.Volume, tokens.Chapter = volume, chapter
		out.Parts = append(out.Parts, PathPart{fileSetting, mediafile.Render(fileTemplate, tokens)})
		return out
	}
	return []PathExample{
		example("Volume 112", previewVolumes, first, "volumesRoot", "volumeFile", m.VolumeFile, 11200, -1),
		example("Volume 112, collected again as Colored", previewVolumes, colored, "volumesRoot", "volumeFile", m.VolumeFile, 11200, -1),
		example("Chapter 1102.5", previewChapters, chapters, "chaptersRoot", "chapterFile", m.ChapterFile, -1, 110250),
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

// NamingToken is a template token as the settings page lists it.
type NamingToken struct {
	Syntax string `json:"syntax"`
	Help   string `json:"help"`
	// PerBook tokens name one book, so reader templates cannot use them.
	PerBook bool `json:"perBook"`
}

// ReaderFieldInfo is a presented ComicInfo field as the settings page
// offers it.
type ReaderFieldInfo struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Help    string `json:"help"`
	Default string `json:"default"`
}

// ComicInfoSource is where the value of one or more ComicInfo fields Sento
// writes comes from (mediafile.WrittenFields).
type ComicInfoSource struct {
	Fields []string `json:"fields"`
	From   string   `json:"from"`
}

// MediaSchema is what the naming settings are made of: the tokens a
// template can use and the reader fields that have one. It comes from the
// lists the renderer itself uses (mediafile.Tokens, mediafile.PresentedFields).
type MediaSchema struct {
	Tokens       []NamingToken     `json:"tokens"`
	ReaderFields []ReaderFieldInfo `json:"readerFields"`
	// ComicInfo says where each field Sento writes into a book comes from.
	ComicInfo []ComicInfoSource `json:"comicInfo"`
	// Setups are the arrangements the page offers, recommended first.
	Setups []NamingSetup `json:"setups"`
}

func MediaSettingsSchema() MediaSchema {
	out := MediaSchema{Setups: NamingSetups}
	for _, w := range mediafile.WrittenFields {
		out.ComicInfo = append(out.ComicInfo, ComicInfoSource{Fields: w.Fields, From: w.From})
	}
	for _, t := range mediafile.Tokens {
		out.Tokens = append(out.Tokens, NamingToken{Syntax: t.Syntax, Help: t.Help, PerBook: t.PerBook})
	}
	for _, f := range mediafile.PresentedFields {
		out.ReaderFields = append(out.ReaderFields, ReaderFieldInfo{Key: f.Key, Title: f.Title, Help: f.Help, Default: f.Default})
	}
	return out
}
