package match

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Frieren: Beyond Journey’s End": "frieren beyond journeys end",
		"Steins;Gate 0":                 "steins gate 0",
		"Pokémon RéBURST":               "pokemon reburst",
		"Tate & Yari":                   "tate and yari",
		"ONE PIECE":                     "one piece",
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
