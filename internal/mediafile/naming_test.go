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
