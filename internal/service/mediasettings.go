package service

import (
	"context"
	"encoding/json"
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
	BookFolderByID = "collections/{collectionId}/versions/{versionId}"
)

func DefaultMediaSettings() MediaSettings {
	n := mediafile.DefaultNaming()
	return MediaSettings{
		SeriesFolder: n.SeriesFolder, VolumeFile: n.VolumeFile,
		ChapterFile: n.ChapterFile, ChapterDir: n.ChapterDir,
		BookFolder:    BookFolderByEdition,
		RenameImports: true, WriteComicInfo: true,
	}
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
	if strings.Contains(m.BookFolder, "..") || strings.HasPrefix(strings.TrimSpace(m.BookFolder), "/") {
		return apperr.Invalidf("book folder template must stay inside the series folder")
	}
	for _, root := range []string{m.VolumesRoot, m.ChaptersRoot} {
		if strings.Contains(root, "..") || filepath.IsAbs(strings.TrimSpace(root)) {
			return apperr.Invalidf("top folders must be plain folder names inside the library")
		}
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

// PreviewMediaSettings renders the templates against a fixed sample so the
// UI can show live results (and validation can probe them).
func PreviewMediaSettings(m MediaSettings) map[string]string {
	sample := mediafile.NameTokens{
		SeriesTitle: "ONE PIECE", SeriesID: 377, Edition: "Standard Edition",
		Lang: "en", Group: "TCB Scans", Year: 1997,
	}
	folder := sample
	folder.Volume, folder.Chapter = -1, -1
	volume := sample
	volume.Volume, volume.Chapter = 11200, -1 // v112
	chapter := sample
	chapter.Volume, chapter.Chapter = -1, 110250 // ch 1102.5
	folder.CollectionID, folder.VersionID = 42, 81
	// Whole example paths: top folder, series folder, book folder, file.
	full := func(root, kind, file string) string {
		tokens := folder
		tokens.Type = kind
		parts := append(cleanRoot(root), mediafile.Render(m.SeriesFolder, folder))
		parts = append(parts, mediafile.RenderFolder(m.BookFolder, tokens)...)
		return strings.Join(append(parts, file), "/")
	}
	folder.Type = "Volumes"
	return map[string]string{
		"volumePath":   full(m.VolumesRoot, "Volumes", mediafile.Render(m.VolumeFile, volume)),
		"chapterPath":  full(m.ChaptersRoot, "Chapters", mediafile.Render(m.ChapterFile, chapter)),
		"bookFolder":   strings.Join(mediafile.RenderFolder(m.BookFolder, folder), "/"),
		"seriesFolder": mediafile.Render(m.SeriesFolder, folder),
		"volumeFile":   mediafile.Render(m.VolumeFile, volume),
		"chapterFile":  mediafile.Render(m.ChapterFile, chapter),
	}
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
