package match

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Frieren: Beyond Journey’s End": "frieren beyond journeys end",
		"Steins;Gate 0":                 "steins gate 0",
		"Pokémon RéBURST":               "pokemon reburst",
		"Tate & Yari":                   "tate and yari",
		"ONE PIECE":                     "one piece",
		"HUNTER×HUNTER":                 "hunter x hunter",
		"Hunter x Hunter":               "hunter x hunter",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestContainmentTrap(t *testing.T) {
	ix := NewIndex()
	ix.Add(1, "Berserk")
	ix.Add(2, "Berserk of Gluttony")

	got := ix.Lookup("berserk of gluttony")
	if len(got) == 0 || got[0].SeriesID != 2 {
		t.Fatalf("longest title must win: %+v", got)
	}
	got = ix.Lookup("berserk")
	if len(got) == 0 || got[0].SeriesID != 1 {
		t.Fatalf("exact match must win: %+v", got)
	}
}

// TestSimilarityProperties locks the three behaviors validated against a
// pool of ~1100 real source titles (cmd/matchcheck): token-set order
// tolerance, romanization tolerance, and extra-token dilution.
func TestSimilarityProperties(t *testing.T) {
	// Order tolerance — token-set similarity ignores word order.
	if s := Similarity(Normalize("Attack on Titan"), Normalize("Titan, Attack on")); s < 0.99 {
		t.Errorf("order tolerance: got %.3f, want ~1.0", s)
	}
	// Romanization tolerance — long-vowel variants are the SAME work and
	// must auto-match (the feature; its cost is rare 1-char false positives
	// like "Fish Boy"/"Fist Boy", an inherent fuzzy-matching tradeoff).
	if s := Similarity(Normalize("Kaiju Kamui"), Normalize("Kaijuu Kamui")); s < AutoThreshold {
		t.Errorf("romanization variant must clear auto threshold: got %.3f", s)
	}
	// Dilution — an extra marker token drops a 2-of-3 overlap below the
	// manual floor (why parse trims "One Piece Special" → "One Piece").
	if s := Similarity(Normalize("One Piece"), Normalize("One Piece Special")); s >= ManualThreshold {
		t.Errorf("dilution: got %.3f, want < %.2f", s, ManualThreshold)
	}
}

func TestFuzzyLookup(t *testing.T) {
	ix := NewIndex()
	ix.Add(1, "Frieren: Beyond Journey's End", "Sousou no Frieren")
	ix.Add(2, "One Piece")

	got := ix.Lookup("frieren beyond journeys end")
	if len(got) == 0 || got[0].SeriesID != 1 || got[0].Score < AutoThreshold {
		t.Fatalf("fuzzy match should clear auto threshold: %+v", got)
	}
	if c := ix.Lookup("solo leveling"); len(c) != 0 {
		t.Fatalf("unrelated title must not match: %+v", c)
	}
}
