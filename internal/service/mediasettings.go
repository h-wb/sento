package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/store"
)

// MediaSettings is the post-processing configuration: naming templates,
// transfer mode and metadata writing. Stored in the settings table so
// changes apply without a restart; defaults when unset.
type MediaSettings struct {
	SeriesFolder   string `json:"seriesFolder"`
	VolumeFile     string `json:"volumeFile"`
	ChapterFile    string `json:"chapterFile"`
	ChapterDir     string `json:"chapterDir"` // "" = no chapter subfolder
	RenameImports  bool   `json:"renameImports"`
	AlwaysCopy     bool   `json:"alwaysCopy"` // copy even when a hardlink would work
	WriteComicInfo bool   `json:"writeComicInfo"`
}

const mediaSettingsKey = "media"

func DefaultMediaSettings() MediaSettings {
	n := mediafile.DefaultNaming()
	return MediaSettings{
		SeriesFolder: n.SeriesFolder, VolumeFile: n.VolumeFile,
		ChapterFile: n.ChapterFile, ChapterDir: n.ChapterDir,
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
		return fmt.Errorf("series folder template is empty")
	}
	if !strings.Contains(strings.ToLower(m.VolumeFile), "{volume") {
		return fmt.Errorf("volume file template needs a {volume} token (files would overwrite each other)")
	}
	if !strings.Contains(strings.ToLower(m.ChapterFile), "{chapter") {
		return fmt.Errorf("chapter file template needs a {chapter} token (files would overwrite each other)")
	}
	for name, rendered := range PreviewMediaSettings(m) {
		if strings.TrimSpace(strings.TrimSuffix(rendered, ".cbz")) == "" {
			return fmt.Errorf("%s template renders empty", name)
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
	return map[string]string{
		"seriesFolder": mediafile.Render(m.SeriesFolder, folder),
		"volumeFile":   mediafile.Render(m.VolumeFile, volume),
		"chapterFile":  mediafile.Render(m.ChapterFile, chapter),
	}
}
