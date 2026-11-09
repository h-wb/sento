package parse

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// TestTagPathConsistency locks Finding A: a tag means the same thing whether
// it sits in brackets (classifySegment) or in free text (parseInto), because
// both go through scanTags. Before the unification, the free-text forms below
// silently set nothing.
func TestTagPathConsistency(t *testing.T) {
	cases := []struct {
		desc      string
		bracketed string
		freetext  string
		set       func(Result) bool
	}{
		{"omnibus", "Berserk (2-in-1) v01", "Berserk 2-in-1 v01", func(r Result) bool { return r.Edition.Omnibus }},
		{"colored", "Berserk (Coloured) v01", "Berserk Coloured v01", func(r Result) bool { return r.Edition.Colored }},
		{"raw", "Berserk (RAW) v01", "Berserk RAW v01", func(r Result) bool { return r.Edition.Raw }},
		{"digital", "Berserk (Digital) v01", "Berserk Digital v01", func(r Result) bool { return r.Quality.Digital }},
		{"official", "Berserk (Viz) v01", "Berserk Viz v01", func(r Result) bool { return r.Edition.Official }},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			if !tc.set(Parse(tc.bracketed, ChannelTorrent)) {
				t.Errorf("bracketed %q did not set %s", tc.bracketed, tc.desc)
			}
			if !tc.set(Parse(tc.freetext, ChannelTorrent)) {
				t.Errorf("free-text %q did not set %s (path drift)", tc.freetext, tc.desc)
			}
		})
	}
}

// TestTagWordBoundary guards the word-boundary matching scanTags relies on:
// a tag must not fire inside a larger word.
func TestTagWordBoundary(t *testing.T) {
	if Parse("Berserk Drawback v01", ChannelTorrent).Edition.Raw {
		t.Error(`"raw" fired inside "drawback"`)
	}
	if Parse("Discolored Memories v01", ChannelTorrent).Edition.Colored {
		t.Error(`"color" fired inside "discolored"`)
	}
}

// TestGroupNameIsNotAClaim: a color word inside a group-named segment is
// the group, not a color claim (parser.md §5.4/§5.8). A standalone color
// tag still claims color, and so does the Digital Colored Comics imprint in
// free text or in a compound segment.
func TestGroupNameIsNotAClaim(t *testing.T) {
	r := Parse("Vagabond v01 (2008) (VIZBIG) (Scan) (HQ) (Colored Council)", ChannelTorrent)
	if r.Edition.Colored || r.Claim.Color {
		t.Errorf("group name claimed color: edition=%+v claim=%+v", r.Edition, r.Claim)
	}
	if r.Group != "Colored Council" {
		t.Errorf("group = %q, want the trailing segment", r.Group)
	}
	if !Parse("Berserk v01 (Colored) (Colored Council)", ChannelTorrent).Edition.Colored {
		t.Error("standalone (Colored) tag did not claim color")
	}
	if !Parse("One Piece v106 (FULL COLOR Digital Colored Comics)", ChannelTorrent).Edition.Colored {
		t.Error("imprint compound segment did not claim color")
	}
	if !Parse("Bleach - Digital Colored Comics v01-74 (2021) (Colored Council)", ChannelTorrent).Edition.Colored {
		t.Error("free-text imprint did not claim color")
	}
}

func TestKResolutionBounds(t *testing.T) {
	maxK := math.MaxInt / 1024
	cases := []struct {
		name       string
		token      string
		resolution int
	}{
		{"largest representable", strconv.Itoa(maxK) + "K", maxK * 1024},
		{"pixel conversion overflow", strconv.Itoa(maxK+1) + "K-Repack", 0},
		{"integer parsing overflow", strconv.FormatUint(uint64(math.MaxInt)+1, 10) + "K-Repack", 0},
		{"very long count", strings.Repeat("9", 100) + "K", 0},
		{"overflow preserves valid resolution", "1800px " + strings.Repeat("9", 100) + "K-Repack", 1800},
		{"zero", "0K", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := classifySegment(tc.token)
			if h.quality.Resolution != tc.resolution || h.recognized != (tc.resolution > 0) || h.version != 0 {
				t.Fatalf("%s: got resolution %d, recognized %v, revision %d; want resolution %d", tc.token, h.quality.Resolution, h.recognized, h.version, tc.resolution)
			}
		})
	}
}
