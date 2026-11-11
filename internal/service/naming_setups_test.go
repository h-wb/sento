package service

import (
	"strings"
	"testing"
)

// TestNamingSetupsAreValidAndRecognised: every offered setup saves, and is
// recognised again once applied.
func TestNamingSetupsAreValidAndRecognised(t *testing.T) {
	t.Parallel()
	for _, s := range NamingSetups {
		m := s.Apply(DefaultMediaSettings())
		if err := ValidateMediaSettings(m); err != nil {
			t.Errorf("%s: %v", s.Key, err)
		}
		if !s.Matches(m) {
			t.Errorf("%s: not recognised after applying it", s.Key)
		}
	}
	if !NamingSetups[1].Matches(DefaultMediaSettings()) {
		t.Error("the default settings should be Sento's default setup")
	}
}

// TestPreviewPathsTagEachLevel: the sample paths name the setting behind
// each level, and show where a second copy parts ways on disk.
func TestPreviewPathsTagEachLevel(t *testing.T) {
	t.Parallel()
	m := DefaultMediaSettings()
	m.VolumesRoot = "Volumes"
	paths := PreviewPaths(m)
	if len(paths) != 3 {
		t.Fatalf("want 3 sample paths, got %d", len(paths))
	}
	var got []string
	for _, p := range paths[0].Parts {
		got = append(got, p.Setting+"="+p.Text)
	}
	want := "volumesRoot=Volumes seriesFolder=ONE PIECE [mangabaka-377] bookFolder=Standard Edition [en] volumeFile=ONE PIECE v112 [Standard Edition][en].cbz"
	if strings.Join(got, " ") != want {
		t.Errorf("volume path:\n got %s\nwant %s", strings.Join(got, " "), want)
	}
	colored := paths[1].Parts
	if colored[2].Text != "Standard Edition [en] (Colored)" {
		t.Errorf("colored copy's book folder: %q", colored[2].Text)
	}
	chapter := paths[2].Parts
	if chapter[0].Setting != "seriesFolder" || chapter[len(chapter)-1].Setting != "chapterFile" {
		t.Errorf("chapter path without a chapters top folder: %+v", chapter)
	}
}

// TestCheckReaders: the default nests both copies in one series folder,
// which Kavita supports only loosely; the readers setup gives each its own
// folder, and the same names merge the copies.
func TestCheckReaders(t *testing.T) {
	t.Parallel()
	fine := func(m MediaSettings) (kavita, komga bool) {
		c := CheckReaders(m)
		return c[0].Fine, c[1].Fine
	}
	if kavita, komga := fine(DefaultMediaSettings()); kavita || !komga {
		t.Errorf("default: kavita %v komga %v, want a Kavita warning and Komga fine", kavita, komga)
	}
	if kavita, komga := fine(NamingSetups[0].Apply(DefaultMediaSettings())); !kavita || !komga {
		t.Errorf("readers setup: kavita %v komga %v, want both fine", kavita, komga)
	}
	same := NamingSetups[0].Apply(DefaultMediaSettings())
	same.ReaderFields["series"] = "{Series Title}"
	same.BookFolder = ""
	if kavita, komga := fine(same); kavita || komga {
		t.Errorf("one name for both copies in one folder: kavita %v komga %v, want both warned", kavita, komga)
	}
}
