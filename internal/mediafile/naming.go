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
// plain units; padding applies to the integer part.
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
	Edition     string
	Lang        string
	Group       string
	ISBN        string
	Year        int
	// Folder-layout tokens: what tells two copies of a book apart.
	Publisher    string
	Medium       string
	Version      string // collected version name; empty for the default one
	Type         string // "Volumes" or "Chapters"
	CollectionID int64
	VersionID    int64
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
// ("[{group}]" with no group disappears).
func Render(template string, t NameTokens) string {
	out := tokenRe.ReplaceAllStringFunc(template, func(m string) string {
		sub := tokenRe.FindStringSubmatch(m)
		name, pad := sub[1], len(sub[2])
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "series title":
			return sanitize(t.SeriesTitle)
		case "series titleslug":
			return slug(t.SeriesTitle)
		case "seriesid":
			return fmt.Sprintf("%d", t.SeriesID)
		case "volume":
			return renderNum(t.Volume, pad)
		case "chapter":
			return renderNum(t.Chapter, pad)
		case "edition":
			return sanitize(t.Edition)
		case "lang":
			return sanitize(t.Lang)
		case "group":
			return sanitize(t.Group)
		case "isbn":
			return sanitize(t.ISBN)
		case "publisher":
			return sanitize(t.Publisher)
		case "medium":
			return sanitize(t.Medium)
		case "version":
			return sanitize(t.Version)
		case "type":
			return sanitize(t.Type)
		case "collectionid":
			return fmt.Sprintf("%d", t.CollectionID)
		case "versionid":
			return fmt.Sprintf("%d", t.VersionID)
		case "year":
			if t.Year == 0 {
				return ""
			}
			return fmt.Sprintf("%d", t.Year)
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

// RenderPlainNum renders a hundredths number in plain units ("34", "34.5").
func RenderPlainNum(hundredths int64) string {
	return renderNum(hundredths, 0)
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
