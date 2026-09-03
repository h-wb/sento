package parse

import (
	"slices"
	"testing"
)

func TestParseNum(t *testing.T) {
	cases := []struct {
		in   string
		want Num
		ok   bool
	}{
		{"34", 3400, true},
		{"034", 3400, true},    // leading zero
		{"34.5", 3450, true},   // one fractional digit → tenths
		{"34.55", 3455, true},  // two fractional digits → hundredths
		{"34.555", 3455, true}, // more than two truncated
		{"0", 0, true},
		{"  7  ", 700, true}, // surrounding whitespace trimmed
		{"", 0, false},
		{"abc", 0, false},
		{"3.x", 0, false}, // non-numeric fraction
	}
	for _, tc := range cases {
		got, ok := parseNum(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("parseNum(%q) = (%d, %v), want (%d, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestMergeRanges(t *testing.T) {
	r := func(from, to Num) Range { return Range{From: from, To: to} }
	cases := []struct {
		name string
		in   []Range
		want []Range
	}{
		{"empty", nil, nil},
		{"single", []Range{r(500, 500)}, []Range{r(500, 500)}},
		{"sorts by start", []Range{r(500, 500), r(100, 300)}, []Range{r(100, 300), r(500, 500)}},
		{"overlap fuses", []Range{r(100, 300), r(200, 400)}, []Range{r(100, 400)}},
		{"adjacent within one unit fuses", []Range{r(300, 300), r(400, 400)}, []Range{r(300, 400)}},
		{"gap kept", []Range{r(100, 100), r(500, 500)}, []Range{r(100, 100), r(500, 500)}},
		// The bug the rewrite fixes: a range bridging two others must fuse
		// all three. The old single-pass version returned [1,3] and [4,5].
		{"transitive bridge", []Range{r(100, 200), r(400, 500), r(300, 300)}, []Range{r(100, 500)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mergeRanges(tc.in); !slices.Equal(got, tc.want) {
				t.Errorf("mergeRanges(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
