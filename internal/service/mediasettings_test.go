package service

import (
	"context"
	"strings"
	"testing"
)

func TestMediaSettingsRoundtrip(t *testing.T) {
	t.Parallel()
	_, st := setup(t)
	ctx := context.Background()

	// Unset → defaults.
	got := LoadMediaSettings(ctx, st)
	if got != DefaultMediaSettings() {
		t.Fatalf("want defaults when unset, got %+v", got)
	}
	if !got.RenameImports || !got.WriteComicInfo || got.AlwaysCopy {
		t.Fatalf("bad default flags: %+v", got)
	}

	got.VolumeFile = "{Series Title} - Vol {volume:000}.cbz"
	got.RenameImports = false
	if err := SaveMediaSettings(ctx, st, got); err != nil {
		t.Fatal(err)
	}
	if back := LoadMediaSettings(ctx, st); back != got {
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
