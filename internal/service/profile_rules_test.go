package service

import (
	"context"
	"testing"

	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
)

func TestCopyRuleMigrationPreservesProfilesAndIsRepeatable(t *testing.T) {
	_, st, sid, _, _, version := modelFixture(t)
	ctx := context.Background()
	svc := NewProfiles(st)
	if err := svc.EnsureDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	profiles, err := st.ListProfiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	base := profiles[0]
	version.RequiredGroup = "^LuCaZ$"
	version.AllowUnknown = false
	version.ProfileID = base.ID
	if _, err = st.SaveVersion(ctx, sid, version); err != nil {
		t.Fatal(err)
	}
	other := version
	other.ID = 0
	other.Name = "Another copy"
	oid, err := st.SaveVersion(ctx, sid, other)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.MigrateCopyRules(ctx); err != nil {
		t.Fatal(err)
	}
	migrated, err := st.GetVersion(ctx, version.ID)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := st.GetVersion(ctx, oid)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.RequiredGroup != "" || !migrated.AllowUnknown || migrated.ProfileID == base.ID || duplicate.ProfileID != migrated.ProfileID {
		t.Fatalf("migration lost or duplicated policy: %+v %+v", migrated, duplicate)
	}
	p, err := st.GetProfile(ctx, migrated.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	formats, err := st.ListFormats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Required) != 2 || p.MinFormatScore != base.MinFormatScore || p.Volume != base.Volume {
		t.Fatalf("lost inherited settings: %+v", p)
	}
	if j := p.Judge(profile.Input{Group: "lucaz", Language: "en"}, formats); j.Broken != "" {
		t.Fatal(j.Broken)
	}
	for _, in := range []profile.Input{{Group: "Other", Language: "en"}, {Group: "LuCaZ"}} {
		if j := p.Judge(in, formats); j.Broken == "" {
			t.Fatalf("lost legacy restriction: %+v", in)
		}
	}
	before, _ := st.ListProfiles(ctx)
	if err = st.MigrateCopyRules(ctx); err != nil {
		t.Fatal(err)
	}
	after, _ := st.ListProfiles(ctx)
	if len(before) != len(after) {
		t.Fatal("rerun duplicated profiles")
	}
}

func TestProfileRulesRoundTripAndFileEvaluation(t *testing.T) {
	lib, st, sid, c, books, version := modelFixture(t)
	ctx := context.Background()
	svc := NewProfiles(st)
	group, err := svc.CreateFormat(ctx, profile.Format{Name: "Group A", Conditions: []profile.Condition{{Type: profile.CondGroup, Value: "^A$"}}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := svc.CreateFormat(ctx, profile.Format{Name: "Raw", Conditions: []profile.Condition{{Type: profile.CondFlag, Value: "raw"}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.CreateProfile(ctx, profile.Profile{Name: "A only", MinFormatScore: profile.MinScoreOff, Required: []int64{group.ID}, Refused: []int64{raw.ID}, FormatScores: map[int64]int{group.ID: 100}})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := svc.ExportProfile(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	doc.Name = "Imported"
	imported, err := svc.ImportProfile(ctx, *doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Required) != 1 || len(imported.Refused) != 1 {
		t.Fatalf("rules lost: %+v", imported)
	}
	version.ProfileID = imported.ID
	if _, err = st.SaveVersion(ctx, sid, version); err != nil {
		t.Fatal(err)
	}
	fid := ownedModelFile(t, st, sid, c, books[0], version)
	for _, tc := range []struct {
		group string
		raw   bool
		state string
	}{{"A", false, "owned"}, {"B", false, "unsatisfactory"}, {"A", true, "unsatisfactory"}} {
		if err = st.SetAssetFacts(ctx, fid, store.AssetFacts{Group: tc.group, Language: "en", FactsKnown: true, Flags: map[string]bool{"raw": tc.raw}}, ""); err != nil {
			t.Fatal(err)
		}
		statuses, err := lib.BooksByVersion(ctx, sid)
		if err != nil {
			t.Fatal(err)
		}
		if got := statuses[version.ID][0]; got.State != tc.state {
			t.Fatalf("%+v: %+v", tc, got)
		}
	}
	formats, _ := svc.ListFormats(ctx)
	candidate := ReleaseCandidate{Title: "Release"}
	candidate.Parse.Group = "B"
	candidate.evalFormats(imported, formats)
	candidate.explain()
	if candidate.Decision.CanForce || candidate.Decision.CanDownload {
		t.Fatalf("hard rule bypassed: %+v", candidate.Decision)
	}
	bad := *p
	bad.Refused = bad.Required
	if err = svc.UpdateProfile(ctx, bad); err == nil {
		t.Fatal("conflicting rules accepted")
	}
	if err = svc.DeleteFormat(ctx, group.ID); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetProfile(ctx, imported.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Required) != 0 {
		t.Fatal("deleted format left dangling rule")
	}
}
