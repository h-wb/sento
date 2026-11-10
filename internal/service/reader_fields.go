package service

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/store"
)

// readerTemplate is the template a presented field renders with for an
// edition: the edition's own, else the media settings', else the field's
// default. An empty template inherits.
func readerTemplate(f mediafile.PresentedField, ms MediaSettings, e store.Edition) string {
	for _, t := range []string{e.ReaderFields[f.Key], ms.ReaderFields[f.Key]} {
		if strings.TrimSpace(t) != "" {
			return t
		}
	}
	return f.Default
}

// readerValues renders every presented field for a copy. A template that
// renders empty for it falls back to the field's default, so a copy always
// has a series name.
func readerValues(ms MediaSettings, series *store.Series, e store.Edition, v *store.CollectedVersion) map[string]string {
	tokens := baseCopyTokens(series, e, v)
	out := map[string]string{}
	for _, f := range mediafile.PresentedFields {
		value := mediafile.RenderText(readerTemplate(f, ms, e), tokens)
		if value == "" {
			value = mediafile.RenderText(f.Default, tokens)
		}
		out[f.Key] = value
	}
	return out
}

// validateReaderFields rejects templates for fields Sento does not
// present, templates naming one book (a presented field holds for every
// book of a copy), templates naming themselves, misspelled tokens, and
// templates that render nothing for the given copy; empty templates
// inherit and always pass.
func validateReaderFields(fields map[string]string, series *store.Series, e store.Edition, v *store.CollectedVersion) error {
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		template := fields[key]
		if strings.TrimSpace(template) == "" {
			continue
		}
		f := presentedField(key)
		if f == nil {
			return apperr.Invalidf("unknown reader field %q", key)
		}
		if names := mediafile.UnknownTokens(template); len(names) > 0 {
			return apperr.Invalidf("the %s in readers uses an unknown token {%s}", f.Label, names[0])
		}
		for _, t := range mediafile.TemplateTokens(template) {
			if t.PerBook {
				return apperr.Invalidf("the %s in readers holds for every book, so it cannot use {%s}", f.Label, t.Name)
			}
			if t.Name == mediafile.ReaderSeriesToken {
				return apperr.Invalidf("the %s in readers cannot use {%s}: that token is the name being set here", f.Label, t.Name)
			}
		}
		if mediafile.RenderText(template, baseCopyTokens(series, e, v)) == "" {
			return apperr.Invalidf("the %s in readers renders empty", f.Label)
		}
	}
	return nil
}

func presentedField(key string) *mediafile.PresentedField {
	for i, f := range mediafile.PresentedFields {
		if f.Key == key {
			return &mediafile.PresentedFields[i]
		}
	}
	return nil
}

// CopyReaderNames is what readers show for one collected version of an
// edition: each presented field as it renders.
type CopyReaderNames struct {
	EditionID int64             `json:"editionId"`
	VersionID int64             `json:"versionId"`
	Fields    map[string]string `json:"fields"`
}

// ReaderNamesRequest previews an edition's own templates before they are
// saved. Without one, the names are what is saved.
type ReaderNamesRequest struct {
	EditionID    int64             `json:"editionId,omitempty"`
	ReaderFields map[string]string `json:"readerFields,omitempty"`
}

// ReaderNames renders the presented fields of every collected version of
// a series under the current media settings.
func (l *Library) ReaderNames(ctx context.Context, seriesID int64, req ReaderNamesRequest) ([]CopyReaderNames, error) {
	series, err := l.store.GetSeries(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	editions, err := l.store.ListEditions(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	versions, err := l.store.ListVersions(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	ms := LoadMediaSettings(ctx, l.store)
	out := []CopyReaderNames{}
	for _, e := range editions {
		if e.ID == req.EditionID {
			if err := validateReaderFields(req.ReaderFields, series, e, nil); err != nil {
				return nil, err
			}
			e.ReaderFields = req.ReaderFields
		}
		for i, v := range versions {
			if v.EditionID == e.ID {
				out = append(out, CopyReaderNames{EditionID: e.ID, VersionID: v.ID, Fields: readerValues(ms, series, e, &versions[i])})
			}
		}
	}
	return out, nil
}
