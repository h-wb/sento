package service

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestMediaSettingsRoundtrip(t *testing.T) {
	t.Parallel()
	_, st := setup(t)
	ctx := context.Background()

	// Unset → defaults.
	got := LoadMediaSettings(ctx, st)
	if !reflect.DeepEqual(got, DefaultMediaSettings()) {
		t.Fatalf("want defaults when unset, got %+v", got)
	}
	if !got.RenameImports || !got.WriteComicInfo || got.AlwaysCopy {
		t.Fatalf("bad default flags: %+v", got)
	}
	// Settings saved before reader fields existed get their defaults.
	if err := st.PutSetting(ctx, mediaSettingsKey, []byte(`{"seriesFolder":"{Series Title}","volumeFile":"{Series Title} v{volume}.cbz"}`)); err != nil {
		t.Fatal(err)
	}
	if old := LoadMediaSettings(ctx, st); !reflect.DeepEqual(old.ReaderFields, defaultReaderFields()) {
		t.Fatalf("older settings: reader fields %v", old.ReaderFields)
	}

	got.VolumeFile = "{Series Title} - Vol {volume:000}.cbz"
	got.RenameImports = false
	if err := SaveMediaSettings(ctx, st, got); err != nil {
		t.Fatal(err)
	}
	if back := LoadMediaSettings(ctx, st); !reflect.DeepEqual(back, got) {
		t.Fatalf("roundtrip lost data: %+v != %+v", back, got)
	}

	// File templates without their number token would overwrite files.
	bad := got
	bad.VolumeFile = "{Series Title}.cbz"
	if err := SaveMediaSettings(ctx, st, bad); err == nil {
		t.Fatal("want validation error for volume template without {volume}")
	}
	bad = got
	bad.ChapterFile = "{group}.cbz"
	if err := SaveMediaSettings(ctx, st, bad); err == nil {
		t.Fatal("want validation error for chapter template without {chapter}")
	}
}

func TestMediaSettingsPreview(t *testing.T) {
	t.Parallel()
	p := PreviewMediaSettings(DefaultMediaSettings())
	if p["seriesFolder"] != "ONE PIECE [mangabaka-377]" {
		t.Fatalf("seriesFolder preview: %q", p["seriesFolder"])
	}
	if p["volumeFile"] != "ONE PIECE v112 [Standard Edition][en].cbz" {
		t.Fatalf("volumeFile preview: %q", p["volumeFile"])
	}
	if !strings.Contains(p["chapterFile"], "c1102.5") {
		t.Fatalf("chapterFile preview: %q", p["chapterFile"])
	}
}

func TestMediaSettingsPreviewEditionFolders(t *testing.T) {
	t.Parallel()
	m := DefaultMediaSettings()
	m.SeriesFolder, m.BookFolder, m.VolumesRoot = SeriesFolderByEdition, BookFolderByVersion, "Volumes"
	p := PreviewMediaSettings(m)
	if p["seriesFolder"] != "ONE PIECE [mangabaka-377] [Standard Edition][en]" {
		t.Fatalf("seriesFolder preview: %q", p["seriesFolder"])
	}
	if p["volumePath"] != "Volumes/ONE PIECE [mangabaka-377] [Standard Edition][en]/ONE PIECE v112 [Standard Edition][en].cbz" {
		t.Fatalf("volumePath preview: %q", p["volumePath"])
	}
	// Tokens a real series folder never gets are not previewed either.
	m.SeriesFolder = "{Series Title} [{group}]"
	if p = PreviewMediaSettings(m); p["seriesFolder"] != "ONE PIECE" {
		t.Fatalf("seriesFolder preview with a book token: %q", p["seriesFolder"])
	}
}

func TestValidateEditionFolderTemplates(t *testing.T) {
	t.Parallel()
	for template, ok := range map[string]bool{
		SeriesFolderByEdition:                    true,
		"{Series Title} [{editionId}]":           true,
		"{Series Title} [{lang}]":                true, // same-lang editions share it: folders group by rendered name
		"{Series Title} ({version})":             true, // the default version renders like the series, a named one gets a sibling
		"{Reader Series} [mangabaka-{seriesId}]": true,
		"{edition} [{lang}]":                     false, // renders empty for the series itself: every series would share it
		"{Series Title} [mangabaka-{seriesId}]":  true,
		"{Series Title} {type} [{edition}]":      true,
	} {
		m := DefaultMediaSettings()
		m.SeriesFolder = template
		if err := ValidateMediaSettings(m); (err == nil) != ok {
			t.Errorf("%q: err = %v, want ok %v", template, err, ok)
		}
	}
}
