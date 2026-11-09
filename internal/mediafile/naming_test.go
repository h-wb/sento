package mediafile

import "testing"

func TestRender(t *testing.T) {
	naming := DefaultNaming()

	got := Render(naming.SeriesFolder, NameTokens{SeriesTitle: "One Piece", SeriesID: 377, Volume: -1, Chapter: -1})
	if got != "One Piece [mangabaka-377]" {
		t.Errorf("series folder: %q", got)
	}

	got = Render(naming.VolumeFile, NameTokens{
		SeriesTitle: "One Piece", SeriesID: 377, Volume: 100, Chapter: -1,
		Edition: "Standard Edition", Lang: "en",
	})
	if got != "One Piece v01 [Standard Edition][en].cbz" {
		t.Errorf("volume file: %q", got)
	}

	// Absent tokens collapse their wrappers.
	got = Render(naming.ChapterFile, NameTokens{
		SeriesTitle: "One Piece", SeriesID: 377, Volume: -1, Chapter: 118400,
	})
	if got != "One Piece c1184 .cbz" && got != "One Piece c1184.cbz" {
		t.Errorf("chapter file: %q", got)
	}

	// Decimal chapter numbers survive padding.
	got = Render("{Series Title} c{chapter:000}.cbz", NameTokens{SeriesTitle: "X", Volume: -1, Chapter: 11050})
	if got != "X c110.5.cbz" {
		t.Errorf("decimal chapter: %q", got)
	}

	// Path-hostile characters are stripped.
	got = Render("{Series Title}", NameTokens{SeriesTitle: `Fate/stay night: UBW?`, Volume: -1, Chapter: -1})
	if got != "Fatestay night UBW" {
		t.Errorf("sanitize: %q", got)
	}

	// Every token is a path component: a custom collection's free-text
	// language must not be able to climb out of the series folder.
	got = Render("{Series Title} [{lang}].cbz", NameTokens{
		SeriesTitle: "X", Lang: "../../etc", Volume: -1, Chapter: -1,
	})
	if got != "X [etc].cbz" {
		t.Errorf("lang traversal: %q", got)
	}
}

// A file holding several books is named for the range it spans.
func TestRenderRange(t *testing.T) {
	tmpl := "{Series Title} c{chapter:000} [{lang}].cbz"
	base := NameTokens{SeriesTitle: "HUNTER×HUNTER", Lang: "en", Volume: -1}
	for name, tc := range map[string]struct {
		start, end int64
		want       string
	}{
		"single":            {100, 0, "HUNTER×HUNTER c001 [en].cbz"},
		"end equals start":  {100, 100, "HUNTER×HUNTER c001 [en].cbz"},
		"range":             {100, 800, "HUNTER×HUNTER c001-008 [en].cbz"},
		"fractional bounds": {1250, 1400, "HUNTER×HUNTER c012.5-014 [en].cbz"},
	} {
		tokens := base
		tokens.Chapter, tokens.ChapterEnd = tc.start, tc.end
		if got := Render(tmpl, tokens); got != tc.want {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}
	vol := NameTokens{SeriesTitle: "X", Volume: 100, VolumeEnd: 3800, Chapter: -1}
	if got := Render("{Series Title} v{volume:00}.cbz", vol); got != "X v01-38.cbz" {
		t.Errorf("volume range: %q", got)
	}
}

// Text for a ComicInfo field keeps what a path cannot hold.
func TestRenderTextKeepsPunctuation(t *testing.T) {
	tokens := NameTokens{SeriesTitle: "Dragon Quest: The Adventure of Dai", Edition: "Fan Translation", Volume: -1, Chapter: -1}
	if got := RenderText("{Series Title} {edition} {version}", tokens); got != "Dragon Quest: The Adventure of Dai Fan Translation" {
		t.Fatalf("RenderText = %q", got)
	}
	if got := Render("{Series Title}", tokens); got != "Dragon Quest The Adventure of Dai" {
		t.Fatalf("Render = %q", got)
	}
}
