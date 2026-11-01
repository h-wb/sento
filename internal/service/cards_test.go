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

// A card is judged by its own profile, else the series'.
func TestCardProfilesFor(t *testing.T) {
	series, own := &profile.Profile{ID: 1}, &profile.Profile{ID: 2}
	p := cardProfiles{Series: series, ByID: map[int64]*profile.Profile{1: series, 2: own}}
	for _, tc := range []struct {
		name string
		card store.CollectedVersion
		want *profile.Profile
	}{
		{"its own", store.CollectedVersion{ProfileID: 2}, own},
		{"inherited", store.CollectedVersion{}, series},
		{"a deleted profile falls back to the series'", store.CollectedVersion{ProfileID: 9}, series},
	} {
		if got := p.For(tc.card); got != tc.want {
			t.Errorf("%s: got %+v", tc.name, got)
		}
	}
}

// A profile that judges a card cannot be deleted from under it.
func TestProfileInUseByACardIsKept(t *testing.T) {
	lib, st, sid, standard, _, _ := modelFixture(t)
	ctx := context.Background()
	svc := NewProfiles(st)
	if err := svc.EnsureDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	colored, err := svc.CreateProfile(ctx, profile.Profile{Name: "Colored", MinFormatScore: profile.MinScoreOff})
	if err != nil {
		t.Fatal(err)
	}
	card, err := lib.SaveVersion(ctx, sid, store.CollectedVersion{EditionID: standard.ID, Name: "x", Monitored: true, ProfileID: colored.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.DeleteProfile(ctx, colored.ID); err == nil {
		t.Fatal("a profile judging a card was deleted")
	}
	if err = st.DeleteVersion(ctx, sid, card); err != nil {
		t.Fatal(err)
	}
	if err = svc.DeleteProfile(ctx, colored.ID); err != nil {
		t.Fatalf("an unused profile could not be deleted: %v", err)
	}
}

func TestKnownDigitalGroups(t *testing.T) {
	f := profile.Format{ID: 1, Conditions: []profile.Condition{{Type: profile.CondGroup, Value: knownDigitalGroups}}}
	for group, want := range map[string]bool{
		"1r0n": true, "1r0n, Bingbong": true, "LuCaZ": true, "danke-Empire": true, "LostNerevarine-Empire": true,
		"Oaky": false, "Asura": false, "Bingbong": false, "": false,
	} {
		if got := f.Matches(profile.Input{Group: group}); got != want {
			t.Errorf("%q: known = %v", group, got)
		}
	}
}
