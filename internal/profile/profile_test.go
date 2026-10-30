package profile

import "testing"

func in() Input {
	return Input{
		Title:      "One Piece v01-v10 (Digital) (1r0n)",
		Group:      "1r0n",
		Language:   "fr-ca",
		Protocol:   "torrent",
		Site:       "Nyaa",
		Resolution: 1920,
		Flags:      map[string]bool{"digital": true, "complete": true},
	}
}

func TestMatchesORWithinTypeANDAcrossTypes(t *testing.T) {
	f := Format{Name: "f", Conditions: []Condition{
		{Type: CondTitle, Value: `\bdigital\b`},
		{Type: CondTitle, Value: `\bwebrip\b`}, // OR with the one above
		{Type: CondProtocol, Value: "torrent"}, // AND with the title group
	}}
	if !f.Matches(in()) {
		t.Error("digital OR webrip, AND torrent: should match")
	}
	f.Conditions[2].Value = "source"
	if f.Matches(in()) {
		t.Error("protocol group fails: must not match")
	}
}

func TestMatchesRequiredAndNegate(t *testing.T) {
	f := Format{Name: "f", Conditions: []Condition{
		{Type: CondFlag, Value: "digital"},
		{Type: CondTitle, Value: `\blq\b`, Negate: true, Required: true},
	}}
	if !f.Matches(in()) {
		t.Error("digital, not LQ: should match")
	}
	i := in()
	i.Title = "One Piece v01 (Digital) [LQ]"
	if f.Matches(i) {
		t.Error("required NOT-LQ violated: must not match")
	}
}

func TestMatchesFacts(t *testing.T) {
	cases := []struct {
		c    Condition
		want bool
	}{
		{Condition{Type: CondLanguage, Value: "FR"}, true}, // base lang, case-insensitive
		{Condition{Type: CondLanguage, Value: "en"}, false},
		{Condition{Type: CondGroup, Value: "^1r0n$"}, true},
		{Condition{Type: CondSite, Value: "nyaa"}, true},
		{Condition{Type: CondResolution, Value: "1600"}, true},
		{Condition{Type: CondResolution, Value: "2000"}, false},
		{Condition{Type: CondFlag, Value: "colored"}, false},
	}
	for _, tc := range cases {
		f := Format{Name: "f", Conditions: []Condition{tc.c}}
		if got := f.Matches(in()); got != tc.want {
			t.Errorf("%+v: got %v, want %v", tc.c, got, tc.want)
		}
	}
}

func TestEmptyFormatMatchesNothing(t *testing.T) {
	if (Format{Name: "empty"}).Matches(in()) {
		t.Error("a format without conditions must not match")
	}
}

func TestValidate(t *testing.T) {
	bad := []Condition{
		{Type: CondTitle, Value: "("},
		{Type: CondFlag, Value: "shiny"},
		{Type: CondProtocol, Value: "usenet"},
		{Type: CondResolution, Value: "abc"},
		{Type: "nope", Value: "x"},
		{Type: CondLanguage, Value: " "},
	}
	for _, c := range bad {
		if c.Validate() == nil {
			t.Errorf("%+v: want validation error", c)
		}
	}
	if err := (Condition{Type: CondFlag, Value: "Digital"}).Validate(); err != nil {
		t.Errorf("flag value should be case-insensitive: %v", err)
	}
}

// Patterns are compiled once and cached — including the ones that do not
// compile, which must keep matching nothing on every later call.
func TestRegexConditionsAreCachedSafely(t *testing.T) {
	in := Input{Title: "One Piece v01 (Digital) (1r0n)"}
	good := Format{Conditions: []Condition{{Type: CondTitle, Value: `\b1r0n\b`}}}
	bad := Format{Conditions: []Condition{{Type: CondTitle, Value: `(unclosed`}}}
	for range 3 {
		if !good.Matches(in) {
			t.Fatal("valid pattern stopped matching")
		}
		if bad.Matches(in) {
			t.Fatal("invalid pattern matched")
		}
	}
	// Case-insensitive, and distinct patterns do not share a cache book.
	if !(Format{Conditions: []Condition{{Type: CondTitle, Value: `DIGITAL`}}}).Matches(in) {
		t.Fatal("patterns must match case-insensitively")
	}
	if (Format{Conditions: []Condition{{Type: CondTitle, Value: `omnibus`}}}).Matches(in) {
		t.Fatal("unrelated pattern matched")
	}
}

// A profile scores what a release matches and holds it to its hard
// rules: every required format, no refused one.
func TestJudge(t *testing.T) {
	lucaz := Format{ID: 1, Name: "LuCaZ", Conditions: []Condition{{Type: CondGroup, Value: "^lucaz$"}}}
	stated := Format{ID: 2, Name: "Stated language", Conditions: []Condition{{Type: CondLanguage, Value: AnyLanguage}}}
	raw := Format{ID: 3, Name: "Raw", Conditions: []Condition{{Type: CondFlag, Value: "raw"}}}
	formats := []Format{lucaz, stated, raw}
	p := &Profile{Name: "LuCaZ only", FormatScores: map[int64]int{1: 100, 2: 10}, Required: []int64{1, 2}, Refused: []int64{3}}
	for _, tc := range []struct {
		name   string
		in     Input
		score  int
		broken string
	}{
		{"meets every rule", Input{Group: "LuCaZ", Language: "en"}, 110, ""},
		{"another group", Input{Group: "1r0n", Language: "en"}, 10, "does not match “LuCaZ”, which LuCaZ only requires"},
		{"no stated language", Input{Group: "LuCaZ"}, 100, "does not match “Stated language”, which LuCaZ only requires"},
		{"a refused format", Input{Group: "LuCaZ", Language: "en", Flags: map[string]bool{"raw": true}}, 110, "matches “Raw”, which LuCaZ only refuses"},
	} {
		if j := p.Judge(tc.in, formats); j.Score != tc.score || j.Broken != tc.broken {
			t.Errorf("%s: score %d %q, want %d %q", tc.name, j.Score, j.Broken, tc.score, tc.broken)
		}
	}
	// Another profile ranks the same group differently and requires nothing.
	other := &Profile{Name: "Any group", FormatScores: map[int64]int{1: -50}}
	if j := other.Judge(Input{Group: "LuCaZ"}, formats); j.Score != -50 || j.Broken != "" {
		t.Errorf("another profile: %+v", j)
	}
}
