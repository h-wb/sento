package parse

import (
	"slices"
	"testing"
)

// Pins the segments contract: flat scan, first closer wins, unbalanced
// openers degrade to free text, free runs emitted separately.
func TestSegments(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []segment
	}{
		{"free text only", "Berserk", []segment{
			{text: "Berserk", delim: 0, pos: 0},
		}},
		{"leading group, trailing tag", "[Kamizye] Berserk - 375 (2023)", []segment{
			{text: "Kamizye", delim: '[', pos: 0},
			{text: "Berserk - 375", delim: 0, pos: 1},
			{text: "2023", delim: '(', pos: 2},
		}},
		{"unbalanced opener degrades to free text", "Berserk [incomplete", []segment{
			{text: "Berserk [incomplete", delim: 0, pos: 0},
		}},
		{"mixed delimiters inside a segment stay inner text", "[a (b) c]", []segment{
			{text: "a (b) c", delim: '[', pos: 0},
		}},
		{"no nesting: first closer wins", "[[x] y]", []segment{
			{text: "[x", delim: '[', pos: 0},
			{text: "y]", delim: 0, pos: 1},
		}},
		{"free runs emitted separately", "Title [tag] more", []segment{
			{text: "Title", delim: 0, pos: 0},
			{text: "tag", delim: '[', pos: 1},
			{text: "more", delim: 0, pos: 2},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := segments(tc.in); !slices.Equal(got, tc.want) {
				t.Errorf("segments(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}
