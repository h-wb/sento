package mediafile

import (
	"reflect"
	"testing"
)

func TestProvenanceRoundTrip(t *testing.T) {
	for _, p := range []Provenance{
		{MangaBaka: 15456, Edition: "Deluxe Edition", Ledger: "volume", Books: []string{"1", "2"},
			Release: `Nausicaa Deluxe v01-02 (Digital) "1r0n"`, Via: "torrent", Site: "Nyaa", Flags: []string{"deluxe", "digital"}},
		{MangaBaka: 377, Edition: "Standard Edition", CatalogEdition: "e-12", Ledger: "chapter", Books: []string{"10.5"}, Works: []string{"w1", "w2"}, Via: "source"},
		{MangaBaka: 1},
	} {
		got, ok := ReadProvenance(p.Notes())
		if !ok || !reflect.DeepEqual(got, p) {
			t.Errorf("%q read back as %+v", p.Notes(), got)
		}
	}
}

// Lines Sento wrote before the keys grew still read; notes without the
// tag are not Sento's.
func TestReadProvenanceOlderAndForeignNotes(t *testing.T) {
	got, ok := ReadProvenance(`sento:v1 mangabaka:377 edition:"Standard Edition" work:abc release:"ONE PIECE v01" future:"key" bare`)
	want := Provenance{MangaBaka: 377, Edition: "Standard Edition", Works: []string{"abc"}, Release: "ONE PIECE v01"}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	if _, ok := ReadProvenance("Tagged with ComicTagger 1.5 using info from Comic Vine"); ok {
		t.Fatal("foreign notes read as Sento's")
	}
}
