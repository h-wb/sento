package mediafile

import (
	"encoding/xml"
)

// ComicInfo is the Anansi-project ComicInfo.xml subset Sento writes
// (spec.md §11.3).
type ComicInfo struct {
	XMLName     xml.Name `xml:"ComicInfo"`
	Title       string   `xml:"Title,omitempty"`
	Series      string   `xml:"Series,omitempty"`
	Number      string   `xml:"Number,omitempty"`
	Volume      string   `xml:"Volume,omitempty"`
	Count       string   `xml:"Count,omitempty"`
	Summary     string   `xml:"Summary,omitempty"`
	Writer      string   `xml:"Writer,omitempty"`
	Penciller   string   `xml:"Penciller,omitempty"`
	Translator  string   `xml:"Translator,omitempty"`
	Genre       string   `xml:"Genre,omitempty"`
	Web         string   `xml:"Web,omitempty"`
	LanguageISO string   `xml:"LanguageISO,omitempty"`
	AgeRating   string   `xml:"AgeRating,omitempty"`
	Notes       string   `xml:"Notes,omitempty"`
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
func Merge(existing, sento ComicInfo) ComicInfo {
	out := existing
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&out.Title, sento.Title)
	set(&out.Series, sento.Series)
	set(&out.Number, sento.Number)
	set(&out.Volume, sento.Volume)
	set(&out.Count, sento.Count)
	set(&out.Summary, sento.Summary)
	set(&out.Writer, sento.Writer)
	set(&out.Penciller, sento.Penciller)
	set(&out.Genre, sento.Genre)
	set(&out.Web, sento.Web)
	set(&out.LanguageISO, sento.LanguageISO)
	set(&out.AgeRating, sento.AgeRating)
	set(&out.Notes, sento.Notes)
	return out
}
