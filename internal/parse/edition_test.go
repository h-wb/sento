package parse

import (
	"slices"
	"testing"
)

// A release name and an edition name are read into the same claim; the
// edition's name is kept as the source writes it.
func TestReadEdition(t *testing.T) {
	for _, tc := range []struct {
		name    string
		regroup []string
		own     string
		written string
		color   bool
		box     bool
	}{
		{"Nausicaä of the Valley of the Wind - Deluxe Edition (2012)", []string{"deluxe"}, "", "Deluxe Edition", false, false},
		{"20th Century Boys v01-02 (The Perfect Edition)", []string{"perfect"}, "", "Perfect Edition", false, false},
		{"Dragon Ball V13 - SoM Kanzenban English", []string{"perfect"}, "", "Kanzenban Edition", false, false},
		{"Neon Genesis Evangelion v01-05 {3-in-1 Edition}", []string{"3-in-1"}, "", "3-in-1 Edition", false, false},
		{"Two In One Edition", []string{"2-in-1"}, "", "Two In One Edition", false, false},
		{"Vagabond v01-03 (VizBig Omnibus)", []string{"omnibus", "vizbig"}, "", "VizBig Edition", false, false},
		{"Fullmetal Alchemist (Fullmetal Edition) Volume 01", nil, "fullmetal", "Fullmetal Edition", false, false},
		{"Akira (35th Anniversary Edition) (2017)", nil, "35th anniversary", "35th Anniversary Edition", false, false},
		{"Bleach - Digital Colored Comics v01-74", nil, "", "", true, false},
		{"Full Color Edition", nil, "", "Full Color Edition", true, false},
		// Colored alone is a colorization, not the color edition.
		{"Dragon Ball v13 (Colored) (Fan)", nil, "", "", true, false},
		{"[ENG] One Piece - Vol. 106 (FULL COLOR Digital Colored Comics)", nil, "", "", true, false},
		{"Deluxe Box Set Edition", []string{"deluxe"}, "", "Deluxe Edition", false, true},
		{"Box Set Edition", nil, "", "", false, true},
		{"Standard Edition", nil, "", "", false, false},
		{"Akira (Digital Edition) (Left to Right)", nil, "", "", false, false},
	} {
		c := ReadEdition(tc.name)
		if !slices.Equal(c.Regroup, tc.regroup) || c.Own != tc.own || c.Written != tc.written || c.Color != tc.color || c.Box != tc.box {
			t.Errorf("%s: %+v", tc.name, c)
		}
	}
	// Only the color edition's own name names it.
	for name, want := range map[string]bool{"Full Color Edition": true, "Dragon Ball - Full Color - Vols. 01 - 16": true, "Dragon Ball v13 (Colored)": false, "Bleach - Digital Colored Comics v01-74": false} {
		if got := ReadEdition(name).ColorEdition; got != want {
			t.Errorf("%s: color edition %v, want %v", name, got, want)
		}
	}
	// Around the title, not in it.
	if c := ParseAnchored("Perfect Edition Blue v01 (Digital)", ChannelTorrent, []string{"Perfect Edition Blue"}).Claim; c.Named() {
		t.Errorf("a title's own words became a claim: %+v", c)
	}
	if c := ParseAnchored("Perfect Edition Blue v01 (Deluxe Edition)", ChannelTorrent, []string{"Perfect Edition Blue"}).Claim; !slices.Equal(c.Regroup, []string{"deluxe"}) {
		t.Errorf("the claim around the title was lost: %+v", c)
	}
}
