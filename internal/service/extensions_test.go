package service

import (
	"context"
	"encoding/json"
	"io"
	"slices"
	"testing"

	"github.com/h-wb/sento/internal/extensions"
	"github.com/h-wb/sento/internal/store"
)

// fakeExtHost implements Host + ExtensionManager: a catalog of available
// packages and a mutable installed set.
type fakeExtHost struct {
	catalog   []string
	installed map[string]bool
	installs  []string
	repos     []string
}

func (f *fakeExtHost) Name() string                                         { return "fake" }
func (f *fakeExtHost) Sources(context.Context) ([]extensions.Source, error) { return nil, nil }
func (f *fakeExtHost) Search(context.Context, string, string) ([]extensions.Manga, error) {
	return nil, nil
}
func (f *fakeExtHost) Chapters(context.Context, string, string) ([]extensions.Chapter, error) {
	return nil, nil
}
func (f *fakeExtHost) Pages(context.Context, string, string) ([]string, error) { return nil, nil }
func (f *fakeExtHost) Page(context.Context, string, string) (io.ReadCloser, error) {
	return nil, nil
}

func (f *fakeExtHost) Extensions(context.Context) ([]extensions.ExtensionInfo, error) {
	out := make([]extensions.ExtensionInfo, 0, len(f.catalog))
	for _, pkg := range f.catalog {
		info := extensions.ExtensionInfo{PkgName: pkg, AvailableVersion: "1.0"}
		if f.installed[pkg] {
			info.InstalledVersion = "1.0"
		}
		out = append(out, info)
	}
	return out, nil
}

func (f *fakeExtHost) InstallExtension(_ context.Context, pkg string) error {
	f.installed[pkg] = true
	f.installs = append(f.installs, pkg)
	return nil
}

func (f *fakeExtHost) UninstallExtension(_ context.Context, pkg string) error {
	delete(f.installed, pkg)
	return nil
}

func (f *fakeExtHost) ExtensionSources(_ context.Context, pkg string) ([]extensions.Source, error) {
	return []extensions.Source{{ID: "src-" + pkg, Name: pkg}}, nil
}

func (f *fakeExtHost) Repos(context.Context) ([]string, error) { return f.repos, nil }

func (f *fakeExtHost) RepoStatus(context.Context) ([]extensions.RepoStatus, error) {
	out := make([]extensions.RepoStatus, 0, len(f.repos))
	for _, u := range f.repos {
		out = append(out, extensions.RepoStatus{URL: u, Name: "Fake Repo", Extensions: len(f.catalog)})
	}
	return out, nil
}

func (f *fakeExtHost) SetRepos(_ context.Context, urls []string) error {
	f.repos = urls
	return nil
}

// addSourceEntry registers a source indexer entry referencing a package.
func addSourceEntry(t *testing.T, st *store.Store, name, sourceID, pkg string) int64 {
	t.Helper()
	settings, _ := json.Marshal(SourceSettings{SourceID: sourceID, PkgName: pkg})
	id, err := st.AddIndexer(context.Background(), store.Indexer{
		Name: name, Protocol: store.IndexerSource,
		Enabled: true, UseVolumes: true, UseChapters: true, Settings: settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Indexer entries drive the extension lifecycle; a wiped host converges
// back to whatever the entries reference.
func TestExtensionReconcile(t *testing.T) {
	t.Parallel()
	_, st := setup(t)
	ctx := context.Background()

	host := &fakeExtHost{
		catalog:   []string{"ext.a", "ext.b", "ext.c"},
		installed: map[string]bool{},
	}
	svc := NewExtensionService(st, func() extensions.Host { return host })

	// The repo list is user-added (nothing ships by default) and pushed
	// to the host on set.
	if err := svc.SetRepos(ctx, []string{"https://repo.example/index.min.json", ""}); err != nil {
		t.Fatal(err)
	}
	if len(host.repos) != 1 {
		t.Fatalf("host repos = %v", host.repos)
	}
	if err := svc.SetRepos(ctx, []string{"not-a-url"}); err == nil {
		t.Fatal("want validation error for scheme-less repo url")
	}

	// An indexer entry referencing the extension makes it desired.
	addSourceEntry(t, st, "A", "src-ext.a", "ext.a")
	views, err := svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	desired := 0
	for _, v := range views {
		if v.Desired {
			desired++
		}
	}
	if desired != 1 {
		t.Fatalf("want 1 desired, got %+v", views)
	}

	// The host gets wiped (fresh pod); reconcile restores the repo list
	// first, then reinstalls only what entries reference.
	host.installed = map[string]bool{}
	host.installs = nil
	host.repos = nil
	if err := svc.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if len(host.repos) != 1 {
		t.Fatalf("reconcile did not restore repos: %v", host.repos)
	}
	if len(host.installs) != 1 || host.installs[0] != "ext.a" {
		t.Fatalf("reconcile installs = %v, want [ext.a]", host.installs)
	}
	// Idempotent: nothing more to do.
	if err := svc.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if len(host.installs) != 1 {
		t.Fatalf("reconcile not idempotent: %v", host.installs)
	}

	// Garbage edition: an installed extension no entry references
	// (abandoned Add-Indexer pick) is removed on the next reconcile.
	host.installed["ext.c"] = true
	if err := svc.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if host.installed["ext.c"] {
		t.Fatal("reconcile should uninstall unreferenced extensions")
	}
	if !host.installed["ext.a"] {
		t.Fatal("reconcile must keep referenced extensions")
	}

	// CleanupExtension uninstalls only when no entry references the pkg.
	svc.CleanupExtension(ctx, "ext.a")
	if !host.installed["ext.a"] {
		t.Fatal("cleanup must not uninstall a referenced extension")
	}
	host.installed["ext.b"] = true
	svc.CleanupExtension(ctx, "ext.b")
	if host.installed["ext.b"] {
		t.Fatal("cleanup should uninstall an unreferenced extension")
	}
}

// Without a host, reconcile is a no-op and the management API
// unavailable.
func TestExtensionsNoManager(t *testing.T) {
	t.Parallel()
	_, st := setup(t)
	svc := NewExtensionService(st, func() extensions.Host { return nil })
	if svc.HasManager() {
		t.Fatal("nil host should have no manager")
	}
	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Install(context.Background(), "x"); err == nil {
		t.Fatal("install without manager should fail")
	}
}

// fakePrefHost adds the preference capability on top of fakeExtHost.
type fakePrefHost struct {
	fakeExtHost
	prefs  map[string]map[string]json.RawMessage // sourceID → key → current
	pushes []string
}

func (f *fakePrefHost) SourcePreferences(_ context.Context, sid string) ([]extensions.Preference, error) {
	var out []extensions.Preference
	for key, cur := range f.prefs[sid] {
		out = append(out, extensions.Preference{Key: key, ValueType: "String", Current: cur})
	}
	return out, nil
}

func (f *fakePrefHost) SetSourcePreference(_ context.Context, sid, key string, value json.RawMessage) error {
	if f.prefs[sid] == nil {
		f.prefs[sid] = map[string]json.RawMessage{}
	}
	f.prefs[sid][key] = value
	f.pushes = append(f.pushes, sid+"/"+key)
	return nil
}

// Stored preferences replay onto fresh hosts; unchanged values are not
// re-pushed.
func TestPreferenceReconcile(t *testing.T) {
	t.Parallel()
	_, st := setup(t)
	ctx := context.Background()

	host := &fakePrefHost{prefs: map[string]map[string]json.RawMessage{
		"42": {"username": json.RawMessage(`"old"`)},
	}}
	svc := NewExtensionService(st, func() extensions.Host { return host })

	if err := svc.SetPreference(ctx, "42", "username", json.RawMessage(`"hugo"`)); err != nil {
		t.Fatal(err)
	}
	if string(host.prefs["42"]["username"]) != `"hugo"` {
		t.Fatalf("push missing: %v", host.prefs)
	}

	// Host wiped: prefs reset to defaults.
	host.prefs = map[string]map[string]json.RawMessage{"42": {"username": json.RawMessage(`""`)}}
	host.pushes = nil
	if err := svc.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if len(host.pushes) != 1 || string(host.prefs["42"]["username"]) != `"hugo"` {
		t.Fatalf("reconcile did not replay: pushes=%v prefs=%v", host.pushes, host.prefs)
	}
	// Converged: nothing re-pushed.
	host.pushes = nil
	if err := svc.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if len(host.pushes) != 0 {
		t.Fatalf("re-pushed unchanged prefs: %v", host.pushes)
	}
}

// Every stored repo is reported, with what the host gets from it — or,
// when the host cannot be asked, with that reason instead of vanishing.
func TestRepoStatus(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, st := setup(t)
	host := &fakeExtHost{catalog: []string{"ext.a", "ext.b"}, installed: map[string]bool{}}
	svc := NewExtensionService(st, func() extensions.Host { return host })

	if got := svc.RepoStatus(ctx); len(got) != 0 {
		t.Fatalf("no repos stored, got %+v", got)
	}
	const url = "https://repo.example/index.min.json"
	if err := svc.SetRepos(ctx, []string{url}); err != nil {
		t.Fatal(err)
	}
	got := svc.RepoStatus(ctx)
	if len(got) != 1 || got[0].URL != url || got[0].Name != "Fake Repo" || got[0].Extensions != 2 || got[0].Error != "" {
		t.Fatalf("status = %+v, want the host's view of the repo", got)
	}

	// The host forgot the repo (fresh sidecar, not reconciled yet).
	host.repos = nil
	if got := svc.RepoStatus(ctx); len(got) != 1 || got[0].Error == "" {
		t.Fatalf("status = %+v, want the stored repo reported as not loaded", got)
	}

	// No host at all: still listed, with the reason.
	none := NewExtensionService(st, func() extensions.Host { return nil })
	if got := none.RepoStatus(ctx); len(got) != 1 || got[0].URL != url || got[0].Error == "" {
		t.Fatalf("status without a host = %+v, want the repo listed with an error", got)
	}
}

// A repo list given by the deployment's configuration replaces the stored
// one, cannot be edited, and is what the sidecar ends up with — nothing
// more.
func TestConfiguredReposAreAuthoritative(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, st := setup(t)
	host := &fakeExtHost{catalog: []string{"ext.a"}, installed: map[string]bool{}}
	svc := NewExtensionService(st, func() extensions.Host { return host })
	if err := svc.SetRepos(ctx, []string{"https://old.example/index.min.json"}); err != nil {
		t.Fatal(err)
	}

	managed := []string{"https://repo.example/index.min.json"}
	svc.ManageRepos(managed)
	if got := svc.Repos(ctx); !slices.Equal(got, managed) || !svc.ReposManaged() {
		t.Fatalf("repos = %v, want the configured list", got)
	}
	if err := svc.SetRepos(ctx, []string{"https://other.example/index.min.json"}); err == nil {
		t.Fatal("a configured repo list must not be editable")
	}
	host.repos = []string{"https://old.example/index.min.json", managed[0]}
	if err := svc.reconcileRepos(ctx, host); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(host.repos, managed) {
		t.Fatalf("sidecar repos = %v, want exactly the configured list", host.repos)
	}
}
