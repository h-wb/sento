package service

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/store"
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
		"{Series Title}/{edition}":               false, // one folder name: nesting belongs to the book folder
		"../{Series Title}":                      false,
		`..\{Series Title}`:                      false,
	} {
		m := DefaultMediaSettings()
		m.SeriesFolder = template
		if err := ValidateMediaSettings(m); (err == nil) != ok {
			t.Errorf("%q: err = %v, want ok %v", template, err, ok)
		}
	}
}

// A stored series folder template that climbs out of the library (saved
// before templates were confined) still lands inside it.
func TestSeriesFolderStaysInsideTheLibrary(t *testing.T) {
	t.Parallel()
	root := filepath.Join(string(filepath.Separator), "library")
	ms := DefaultMediaSettings()
	ms.SeriesFolder = "../../{Series Title}"
	volumes, chapters := seriesFolders(root, &store.Series{Title: "ONE PIECE", MangaBakaID: 377}, ms)
	if want := filepath.Join(root, "ONE PIECE"); volumes != want || chapters != want {
		t.Fatalf("series folders = %q, %q; want %q", volumes, chapters, want)
	}
}

// A misspelled token is refused instead of silently rendering nothing, and
// the settings page gets the reason beside the field it belongs to.
func TestMediaSettingsNameUnknownTokens(t *testing.T) {
	t.Parallel()
	m := DefaultMediaSettings()
	m.VolumeFile = "{Seris Title} v{volume:00}.cbz"
	m.BookFolder = "{edition} [{language}]"
	if err := ValidateMediaSettings(m); err == nil || !strings.Contains(err.Error(), "{Seris Title}") {
		t.Fatalf("validate: %v", err)
	}
	problems := MediaSettingsProblems(m)
	if len(problems) != 2 || !strings.Contains(problems["volumeFile"], "{Seris Title}") || !strings.Contains(problems["bookFolder"], "{language}") {
		t.Fatalf("problems: %v", problems)
	}
	if problems := MediaSettingsProblems(DefaultMediaSettings()); len(problems) != 0 {
		t.Fatalf("the defaults have problems: %v", problems)
	}
	// Every token works in every path template: a file name may say the
	// publisher as a folder may.
	m = DefaultMediaSettings()
	m.VolumeFile = "{Series Title} v{volume:00} [{publisher}][{type}].cbz"
	if err := ValidateMediaSettings(m); err != nil {
		t.Fatal(err)
	}
}

// A reader template cannot name itself, name one book, or misspell a token.
func TestReaderFieldTemplatesAreChecked(t *testing.T) {
	t.Parallel()
	for template, want := range map[string]string{
		"{Series Title} {edition}":        "",
		"A fixed name":                    "",
		"{Reader Series} {edition}":       "{Reader Series}",
		"{Series Title} v{volume}":        "{volume}",
		"{Series Title} {editon}":         "unknown token {editon}",
		"{Series Title} [{group}]":        "{group}",
		"{version}":                       "renders empty",
		"{Series Title} {version} {lang}": "",
	} {
		m := DefaultMediaSettings()
		m.ReaderFields["series"] = template
		err := ValidateMediaSettings(m)
		if (want == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), want)) {
			t.Errorf("%q: err = %v, want %q", template, err, want)
		}
		if got := PreviewReaderFields(m)["series"].Error; (want == "") != (got == "") {
			t.Errorf("%q: preview error = %q, want %q", template, got, want)
		}
	}
}

// The settings page is built from the renderer's own lists.
func TestMediaSettingsSchema(t *testing.T) {
	t.Parallel()
	schema := MediaSettingsSchema()
	if len(schema.Tokens) == 0 || len(schema.ReaderFields) != 2 {
		t.Fatalf("schema: %+v", schema)
	}
	for _, f := range schema.ReaderFields {
		if f.Title == "" || f.Help == "" || f.Default != DefaultMediaSettings().ReaderFields[f.Key] {
			t.Errorf("reader field %+v", f)
		}
	}
	for _, tok := range schema.Tokens {
		if tok.PerBook != (tok.Syntax == "{volume:00}" || tok.Syntax == "{chapter:000}" || tok.Syntax == "{group}" || tok.Syntax == "{isbn}") {
			t.Errorf("token %+v", tok)
		}
	}
}
