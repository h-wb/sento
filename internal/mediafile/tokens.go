package mediafile

import (
	"fmt"
	"strings"
)

// Token is one {name} a naming template can use. Tokens is the one list of
// them: Render fills templates from it, validation names the ones a
// template misspells, and the settings page shows it. Adding a token is
// adding an entry here.
type Token struct {
	// Name is the token as written between the braces.
	Name string
	// Syntax is how the settings page shows it: "{volume:00}".
	Syntax string
	// Help says what it renders to.
	Help string
	// PerBook marks a token that names one book or file rather than a
	// copy, so a template that holds for a whole copy cannot use it.
	PerBook bool
	value   func(t NameTokens, pad int, clean func(string) string) string
}

// ReaderSeriesToken is the token for a copy's presented Series name.
const ReaderSeriesToken = "Reader Series"

func text(get func(NameTokens) string) func(NameTokens, int, func(string) string) string {
	return func(t NameTokens, _ int, clean func(string) string) string { return clean(get(t)) }
}

func id(get func(NameTokens) int64) func(NameTokens, int, func(string) string) string {
	return func(t NameTokens, _ int, _ func(string) string) string { return fmt.Sprintf("%d", get(t)) }
}

// Tokens lists every naming token, in the order the settings page shows.
var Tokens = []Token{
	{Name: "Series Title", Help: "ONE PIECE", value: text(func(t NameTokens) string { return t.SeriesTitle })},
	{Name: "Series TitleSlug", Help: "one-piece", value: func(t NameTokens, _ int, _ func(string) string) string { return slug(t.SeriesTitle) }},
	{Name: "seriesId", Help: "MangaBaka id: 377", value: id(func(t NameTokens) int64 { return t.SeriesID })},
	{Name: ReaderSeriesToken, Help: "the series name in readers (ComicInfo Series), e.g. ONE PIECE Colored", value: text(func(t NameTokens) string { return t.ReaderSeries })},
	{Name: "volume", Syntax: "{volume:00}", Help: "112; :00 pads the number", PerBook: true, value: func(t NameTokens, pad int, _ func(string) string) string {
		return renderSpan(t.Volume, t.VolumeEnd, pad)
	}},
	{Name: "chapter", Syntax: "{chapter:000}", Help: "1102.5; padding applies to the whole part", PerBook: true, value: func(t NameTokens, pad int, _ func(string) string) string {
		return renderSpan(t.Chapter, t.ChapterEnd, pad)
	}},
	{Name: "edition", Help: "Standard Edition", value: text(func(t NameTokens) string { return t.Edition })},
	{Name: "lang", Help: "en", value: text(func(t NameTokens) string { return t.Lang })},
	{Name: "group", Help: "release or scanlation group", PerBook: true, value: text(func(t NameTokens) string { return t.Group })},
	{Name: "isbn", Help: "ISBN when known", PerBook: true, value: text(func(t NameTokens) string { return t.ISBN })},
	{Name: "year", Help: "1997", value: func(t NameTokens, _ int, _ func(string) string) string {
		if t.Year == 0 {
			return ""
		}
		return fmt.Sprintf("%d", t.Year)
	}},
	{Name: "publisher", Help: "the edition's publisher", value: text(func(t NameTokens) string { return t.Publisher })},
	{Name: "medium", Help: "paperback, digital…", value: text(func(t NameTokens) string { return t.Medium })},
	{Name: "type", Help: "Volumes or Chapters", value: text(func(t NameTokens) string { return t.Type })},
	{Name: "version", Help: "profile of an edition collected with several; empty for the first", value: text(func(t NameTokens) string { return t.Version })},
	{Name: "editionId", Help: "internal id that never changes", value: id(func(t NameTokens) int64 { return t.EditionID })},
	{Name: "versionId", Help: "internal id that never changes", value: id(func(t NameTokens) int64 { return t.VersionID })},
}

// tokenAliases are earlier names of tokens, still rendered.
var tokenAliases = map[string]string{"collectionid": "editionid"}

var tokenIndex = func() map[string]*Token {
	out := map[string]*Token{}
	for i := range Tokens {
		t := &Tokens[i]
		if t.Syntax == "" {
			t.Syntax = "{" + t.Name + "}"
		}
		out[strings.ToLower(t.Name)] = t
	}
	for alias, name := range tokenAliases {
		out[alias] = out[name]
	}
	return out
}()

func lookupToken(name string) *Token {
	return tokenIndex[strings.ToLower(strings.TrimSpace(name))]
}

// TemplateTokens returns the tokens a template uses, each once, in the
// order they first appear. Names it does not know are left out.
func TemplateTokens(template string) []Token {
	var out []Token
	seen := map[*Token]bool{}
	for _, m := range tokenRe.FindAllStringSubmatch(template, -1) {
		if t := lookupToken(m[1]); t != nil && !seen[t] {
			seen[t] = true
			out = append(out, *t)
		}
	}
	return out
}

// UnknownTokens returns the {names} in a template that are not tokens, as
// written: they would render empty.
func UnknownTokens(template string) []string {
	var out []string
	for _, m := range tokenRe.FindAllStringSubmatch(template, -1) {
		if lookupToken(m[1]) == nil {
			out = append(out, strings.TrimSpace(m[1]))
		}
	}
	return out
}
