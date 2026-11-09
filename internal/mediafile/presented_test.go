package mediafile

import "testing"

// Presented fields are written as Sento renders them, even empty: a name
// a source's own ComicInfo gave the series does not survive beside it.
func TestMergeWritesPresentedFieldsExactly(t *testing.T) {
	existing := ComicInfo{Series: "One Piece (Digital)", SeriesGroup: "Shonen Jump", Writer: "Oda"}
	out := Merge(existing, ComicInfo{Series: "ONE PIECE"})
	if out.Series != "ONE PIECE" || out.SeriesGroup != "" || out.Writer != "Oda" {
		t.Fatalf("merged = %+v", out)
	}
}
