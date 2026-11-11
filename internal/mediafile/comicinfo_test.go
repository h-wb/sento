package mediafile

import (
	"reflect"
	"strings"
	"testing"
)

// TestEveryComicInfoFieldIsDescribed keeps the settings page's account of
// ComicInfo whole: a field added to the struct needs a word on where its
// value comes from, and a description names only real fields.
func TestEveryComicInfoFieldIsDescribed(t *testing.T) {
	t.Parallel()
	real := map[string]bool{}
	typ := reflect.TypeOf(ComicInfo{})
	for i := range typ.NumField() {
		tag := strings.Split(typ.Field(i).Tag.Get("xml"), ",")[0]
		if tag == "" || tag == "ComicInfo" {
			continue // XMLName and the carried-over extras
		}
		real[tag] = true
	}
	described := map[string]bool{}
	for _, w := range WrittenFields {
		if w.From == "" {
			t.Errorf("%v: no source", w.Fields)
		}
		for _, f := range w.Fields {
			if !real[f] {
				t.Errorf("%s is described but is not a ComicInfo field", f)
			}
			described[f] = true
		}
	}
	for f := range real {
		if !described[f] {
			t.Errorf("ComicInfo field %s says nothing about where its value comes from", f)
		}
	}
	for _, p := range PresentedFields {
		var ci ComicInfo
		p.Set(&ci, "probe")
		if got := reflect.ValueOf(ci).FieldByName(p.Element); !got.IsValid() || got.String() != "probe" {
			t.Errorf("presented field %s says it writes %s, which it does not", p.Key, p.Element)
		}
		if !described[p.Element] {
			t.Errorf("presented field %s is not described", p.Key)
		}
	}
}
