package service

import (
	"fmt"
	"maps"
	"strings"
)

// NamingSetup is a known arrangement of the naming settings: where a copy's
// files go and the name readers file it under. Picking one on the Media
// page fills those settings; it is not stored, and a setup is recognised
// by the settings matching it. Top folders, file names and import options
// are left as they are.
type NamingSetup struct {
	Key          string            `json:"key"`
	Title        string            `json:"title"`
	Summary      string            `json:"summary"`
	SeriesFolder string            `json:"seriesFolder"`
	BookFolder   string            `json:"bookFolder"`
	ReaderFields map[string]string `json:"readerFields"`
	// Tips say what to set in each reader for the setup to show as meant.
	Tips []ReaderTip `json:"tips"`
}

// ReaderTip is one thing to know or set in a reader.
type ReaderTip struct {
	Reader string `json:"reader"`
	Tip    string `json:"tip"`
}

// SeriesFolderByReader gives every name in readers its own folder, so a
// copy readers show apart also sits apart.
const SeriesFolderByReader = "{Reader Series} [mangabaka-{seriesId}]"

// NamingSetups are the setups the Media page offers, first the one it
// recommends.
var NamingSetups = []NamingSetup{
	{
		Key:   "readers",
		Title: "Kavita or Komga",
		Summary: "One folder per name readers show, with the books directly in it. " +
			"An edition collected again as Colored gets its own name and folder, so both readers show it as its own series.",
		SeriesFolder: SeriesFolderByReader,
		BookFolder:   BookFolderFlat,
		ReaderFields: defaultReaderFields(),
		Tips: []ReaderTip{
			{"Kavita", "Kavita files a book under the Series name inside it and wants every series in a folder of its own, which this gives."},
			{"Kavita", "To keep chapters apart from volumes, give them a top folder and add it as a second Kavita library: one series must not sit in two sibling folders of a library."},
			{"Kavita", "Series group becomes a collection when the library's Manage Collections is on."},
			{"Komga", "Komga makes a series of every folder that holds books, and titles it with the Series name inside them when it imports ComicInfo."},
			{"Komga", "Turn off Append volume to series title in the library, or a manga series is titled after a volume number, such as ONE PIECE (12)."},
			{"Komga", "Series group becomes a collection."},
		},
	},
	{
		Key:   "sento",
		Title: "Sento's default",
		Summary: "One folder per series, with a folder per edition inside it. Easy to browse on disk; " +
			"Kavita copes with several series in one folder only loosely, and Komga shows each edition folder as a series.",
		SeriesFolder: mediaDefaults.SeriesFolder,
		BookFolder:   BookFolderByEdition,
		ReaderFields: defaultReaderFields(),
	},
}

var mediaDefaults = DefaultMediaSettings()

// Apply returns the settings with the setup's templates in place.
func (s NamingSetup) Apply(m MediaSettings) MediaSettings {
	m.SeriesFolder, m.BookFolder = s.SeriesFolder, s.BookFolder
	m.ReaderFields = maps.Clone(s.ReaderFields)
	return m
}

// Matches reports whether the settings are the setup's.
func (s NamingSetup) Matches(m MediaSettings) bool {
	if m.SeriesFolder != s.SeriesFolder || m.BookFolder != s.BookFolder {
		return false
	}
	for key, template := range s.ReaderFields {
		if m.ReaderFields[key] != template {
			return false
		}
	}
	return true
}

// ReaderCheck is what one reader makes of the sample edition and the same
// edition collected again as Colored: two series, as the user most likely
// wants, or not.
type ReaderCheck struct {
	Reader string `json:"reader"`
	// Fine is false when the reader would merge the two copies, or show
	// them in a way it does not support well.
	Fine bool   `json:"fine"`
	Says string `json:"says"`
}

// CheckReaders says what Kavita and Komga would show for the settings,
// from the same sample paths and names the page previews.
func CheckReaders(m MediaSettings) []ReaderCheck {
	paths := PreviewPaths(m)
	first, colored := paths[0], paths[1]
	names := PreviewReaderFields(m)["series"]
	// The series folder: the first level that is not a top folder.
	seriesFolder := func(p PathExample) string {
		for _, part := range p.Parts {
			if part.Setting == "seriesFolder" {
				return part.Text
			}
		}
		return ""
	}
	// The folder holding the book: everything but the file.
	holder := func(p PathExample) string {
		levels := make([]string, 0, len(p.Parts)-1)
		for _, part := range p.Parts[:len(p.Parts)-1] {
			levels = append(levels, part.Text)
		}
		return strings.Join(levels, "/")
	}
	two := fmt.Sprintf("%s and %s", names.First, names.Colored)

	kavita := ReaderCheck{Reader: "Kavita"}
	switch {
	case names.First == names.Colored:
		kavita.Says = fmt.Sprintf("Shows both copies as one series, %s: their books carry the same Series name.", names.First)
	case seriesFolder(first) == seriesFolder(colored):
		kavita.Says = fmt.Sprintf("Shows %s, but from one series folder, which Kavita supports only loosely.", two)
	default:
		kavita.Fine = true
		kavita.Says = fmt.Sprintf("Shows %s as two series, each in its own folder.", two)
	}

	komga := ReaderCheck{Reader: "Komga"}
	switch {
	case holder(first) == holder(colored):
		komga.Says = "Shows both copies as one series: Komga makes a series of each folder, and they share one."
	case m.BookFolder != "":
		komga.Fine = true
		komga.Says = fmt.Sprintf("Shows a series per book folder, such as %q, titled %s when it imports ComicInfo.", first.Parts[len(first.Parts)-2].Text, two)
	default:
		komga.Fine = true
		komga.Says = fmt.Sprintf("Shows %s as two series, one per folder.", two)
	}
	return []ReaderCheck{kavita, komga}
}
