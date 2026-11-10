// Package mediafile handles file organization: naming templates, ComicInfo,
// and the import pipeline (spec.md §11).
package mediafile

import (
	"fmt"
	"regexp"
	"strings"
)

// Naming templates use {Token} and {token:000} padding syntax (spec.md
// §11.2). Volume/chapter numbers are passed in hundredths and rendered in
// plain books; padding applies to the integer part.
type NamingConfig struct {
	SeriesFolder string `yaml:"series_folder"`
	VolumeFile   string `yaml:"volume_file"`
	ChapterFile  string `yaml:"chapter_file"`
	ChapterDir   string `yaml:"chapter_dir"`
}

func DefaultNaming() NamingConfig {
	return NamingConfig{
		SeriesFolder: "{Series Title} [mangabaka-{seriesId}]",
		VolumeFile:   "{Series Title} v{volume:00} [{edition}][{lang}].cbz",
		ChapterFile:  "{Series Title} c{chapter:000} [{group}][{lang}].cbz",
		ChapterDir:   "chapters",
	}
}

type NameTokens struct {
	SeriesTitle string
	SeriesID    int64
	Volume      int64 // hundredths; <0 = absent
	Chapter     int64 // hundredths; <0 = absent
	// VolumeEnd and ChapterEnd make the number a range ("c001-008") for a
	// file that holds several books; at or below the start = a single one.
	VolumeEnd  int64
	ChapterEnd int64
	Edition    string
	Lang       string
	Group      string
	ISBN       string
	Year       int
	// ReaderSeries is the copy's presented Series field (the name readers
	// file it under), so folder and file templates can use {Reader Series}.
	ReaderSeries string
	// Folder-layout tokens: what tells two copies of a book apart.
	Publisher string
	Medium    string
	Version   string // collected version name; empty for the default one
	Type      string // "Volumes" or "Chapters"
	EditionID int64
	VersionID int64
}

var tokenRe = regexp.MustCompile(`\{([A-Za-z ]+?)(?::(0+))?\}`)

// RenderFolder fills a folder template that may nest with "/" and returns
// its clean path segments. Segments that render empty are dropped, so an
// empty template is the series folder itself.
func RenderFolder(template string, t NameTokens) []string {
	var out []string
	for _, segment := range strings.Split(template, "/") {
		if s := Sanitize(Render(segment, t)); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Render fills a template; empty values collapse their bracket/paren wrapper
// ("[{group}]" with no group disappears). Values lose the characters a
// path cannot hold.
func Render(template string, t NameTokens) string {
	return render(template, t, sanitize)
}

// RenderText fills a template for text that is not a path (a ComicInfo
// field): values are kept as they are, colons and all.
func RenderText(template string, t NameTokens) string {
	return render(template, t, strings.TrimSpace)
}

func render(template string, t NameTokens, clean func(string) string) string {
	out := tokenRe.ReplaceAllStringFunc(template, func(m string) string {
		sub := tokenRe.FindStringSubmatch(m)
		if tok := lookupToken(sub[1]); tok != nil {
			return tok.value(t, len(sub[2]), clean)
		}
		return ""
	})

	// Drop empty [] () {} wrappers left by absent tokens, collapse spaces.
	for _, pair := range []string{"[]", "()", "{}"} {
		out = strings.ReplaceAll(out, pair, "")
	}
	out = strings.Join(strings.Fields(out), " ")
	return strings.TrimSpace(out)
}

// RenderPlainNum renders a hundredths number in plain books ("34", "34.5").
func RenderPlainNum(hundredths int64) string {
	return renderNum(hundredths, 0)
}

// renderSpan renders a number, or "first-last" when end lies beyond it.
func renderSpan(start, end int64, pad int) string {
	if start < 0 || end <= start {
		return renderNum(start, pad)
	}
	return renderNum(start, pad) + "-" + renderNum(end, pad)
}

func renderNum(hundredths int64, pad int) string {
	if hundredths < 0 {
		return ""
	}
	whole := hundredths / 100
	frac := hundredths % 100
	s := fmt.Sprintf("%0*d", pad, whole)
	if frac != 0 {
		if frac%10 == 0 {
			s += fmt.Sprintf(".%d", frac/10)
		} else {
			s += fmt.Sprintf(".%02d", frac)
		}
	}
	return s
}

var unsafeChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

// Sanitize makes a string safe as a path component.
func Sanitize(s string) string {
	s = unsafeChars.ReplaceAllString(s, "")
	return strings.TrimSpace(strings.Trim(s, "."))
}

func sanitize(s string) string { return Sanitize(s) }

func slug(s string) string {
	s = strings.ToLower(sanitize(s))
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
