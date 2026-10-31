package service

import (
	"context"
	"testing"

	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
)

// An edition is collected once per release profile; a second card is named
// after its profile; any way of collecting it can be removed but the last,
// and removing the default makes another the default.
func TestEditionCollectedOncePerProfile(t *testing.T) {
	lib, st, sid, standard, _, version := modelFixture(t)
	ctx := context.Background()
	colored, err := st.AddProfile(ctx, profile.Profile{Name: "Colored", FormatScores: map[int64]int{}})
	if err != nil {
		t.Fatal(err)
	}
	same := store.CollectedVersion{EditionID: standard.ID, Name: "Again", Monitored: true, ProfileID: version.ProfileID}
	if _, err = lib.SaveVersion(ctx, sid, same); err == nil {
		t.Fatal("a second card with the edition's profile was accepted")
	}
	second, err := lib.SaveVersion(ctx, sid, store.CollectedVersion{EditionID: standard.ID, Name: "Anything", Monitored: true, ProfileID: colored})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := st.GetVersion(ctx, second); got == nil || got.Name != "Colored" || got.ProfileName != "Colored" {
		t.Fatalf("the second card is not named after its profile: %+v", got)
	}
	// The default card can go while another remains; that one takes over.
	if err = st.DeleteVersion(ctx, sid, version.ID); err != nil {
		t.Fatalf("removing the default card: %v", err)
	}
	left, err := st.GetVersion(ctx, second)
	if err != nil || !left.IsDefault {
		t.Fatalf("the remaining card is not the default: %+v %v", left, err)
	}
	if err = st.DeleteVersion(ctx, sid, second); err == nil {
		t.Fatal("the last card of an edition was removed")
	}
}
