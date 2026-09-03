package parse

import "testing"

func TestPreClean(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"archive extension stripped", "Berserk v01.cbz", "Berserk v01"},
		{"extension case-insensitive", "Berserk v01.CBZ", "Berserk v01"},
		{"unknown extension kept", "One Piece v1.5", "One Piece v1.5"},
		{"extension-only name untouched", ".cbz", ".cbz"},
		{"underscores become spaces", "Solo_Leveling_Ch_110", "Solo Leveling Ch 110"},
		{"full-width folded to ASCII", "ＯＮＥ　ＰＩＥＣＥ ｖ０１", "ONE PIECE v01"},
		{"nfkc compatibility forms", "Berserk ①", "Berserk 1"},
		{"whitespace collapsed", "  One \t Piece  v01 ", "One Piece v01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := preClean(tc.in); got != tc.want {
				t.Errorf("preClean(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
