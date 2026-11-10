package mediafile

import (
	"encoding/xml"
)

// ComicInfo is the ComicInfo.xml (Anansi v2.1) subset Sento writes
// (spec.md §11.3): enough for a reader to file and describe a book with no
// other source. Field order follows the schema.
type ComicInfo struct {
	XMLName         xml.Name `xml:"ComicInfo"`
	Title           string   `xml:"Title,omitempty"`
	Series          string   `xml:"Series,omitempty"`
	LocalizedSeries string   `xml:"LocalizedSeries,omitempty"`
	Number          string   `xml:"Number,omitempty"`
	Count           string   `xml:"Count,omitempty"`
	Volume          string   `xml:"Volume,omitempty"`
	Summary         string   `xml:"Summary,omitempty"`
	Notes           string   `xml:"Notes,omitempty"`
	Year            string   `xml:"Year,omitempty"`
	Month           string   `xml:"Month,omitempty"`
	Day             string   `xml:"Day,omitempty"`
	Writer          string   `xml:"Writer,omitempty"`
	Penciller       string   `xml:"Penciller,omitempty"`
	Translator      string   `xml:"Translator,omitempty"`
	Publisher       string   `xml:"Publisher,omitempty"`
	Imprint         string   `xml:"Imprint,omitempty"`
	Genre           string   `xml:"Genre,omitempty"`
	Tags            string   `xml:"Tags,omitempty"`
	Web             string   `xml:"Web,omitempty"`
	PageCount       string   `xml:"PageCount,omitempty"`
	LanguageISO     string   `xml:"LanguageISO,omitempty"`
	Format          string   `xml:"Format,omitempty"`
	Manga           string   `xml:"Manga,omitempty"`
	ScanInformation string   `xml:"ScanInformation,omitempty"`
	SeriesGroup     string   `xml:"SeriesGroup,omitempty"`
	AgeRating       string   `xml:"AgeRating,omitempty"`
	GTIN            string   `xml:"GTIN,omitempty"`
	// Extra keeps elements Sento does not model (Pages, Characters,
	// StoryArc…) so rewriting a file never loses what a tagger put there.
	Extra []ExtraElement `xml:",any"`
}

// ExtraElement is one unmodelled ComicInfo element, carried verbatim.
type ExtraElement struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Content string     `xml:",innerxml"`
}

func (ci ComicInfo) Marshal() ([]byte, error) {
	body, err := xml.MarshalIndent(ci, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), append(body, '\n')...), nil
}

func ParseComicInfo(data []byte) (*ComicInfo, error) {
	var ci ComicInfo
	if err := xml.Unmarshal(data, &ci); err != nil {
		return nil, err
	}
	return &ci, nil
}

// Merge overlays Sento's fields onto an existing ComicInfo: Sento wins
// where it has a value, existing values survive elsewhere (spec.md §11.3).
// Numbering and the presented fields Sento always says, including "none":
// a source's own guess (often a year mis-parse) must not survive a
// volume's absent Number or a chapter's absent Volume, nor a name it gave
// the series compete with the one chosen for readers.
func Merge(existing, sento ComicInfo) ComicInfo {
	out := existing
	out.Number, out.Volume = sento.Number, sento.Volume
	for _, f := range PresentedFields {
		*f.field(&out) = *f.field(&sento)
	}
	for _, f := range []struct {
		dst *string
		v   string
	}{
		{&out.Title, sento.Title}, {&out.LocalizedSeries, sento.LocalizedSeries},
		{&out.Count, sento.Count},
		{&out.Summary, sento.Summary}, {&out.Notes, sento.Notes},
		{&out.Year, sento.Year}, {&out.Month, sento.Month}, {&out.Day, sento.Day},
		{&out.Writer, sento.Writer}, {&out.Penciller, sento.Penciller}, {&out.Translator, sento.Translator},
		{&out.Publisher, sento.Publisher}, {&out.Imprint, sento.Imprint},
		{&out.Genre, sento.Genre}, {&out.Tags, sento.Tags}, {&out.Web, sento.Web},
		{&out.PageCount, sento.PageCount}, {&out.LanguageISO, sento.LanguageISO},
		{&out.Format, sento.Format}, {&out.Manga, sento.Manga},
		{&out.ScanInformation, sento.ScanInformation}, {&out.AgeRating, sento.AgeRating}, {&out.GTIN, sento.GTIN},
	} {
		if f.v != "" {
			*f.dst = f.v
		}
	}
	return out
}
