package service

import (
	"context"
	"io"
	"testing"

	"github.com/h-wb/sento/internal/sourcehost"
)

// fakeExtHost implements Host + ExtensionManager: a catalog of available
// packages and a mutable installed set.
type fakeExtHost struct {
	catalog   []string
	installed map[string]bool
	installs  []string
}

func (f *fakeExtHost) Name() string                                         { return "fake" }
func (f *fakeExtHost) Sources(context.Context) ([]sourcehost.Source, error) { return nil, nil }
func (f *fakeExtHost) Search(context.Context, string, string) ([]sourcehost.Manga, error) {
	return nil, nil
}
func (f *fakeExtHost) Chapters(context.Context, string, string) ([]sourcehost.Chapter, error) {
	return nil, nil
}
func (f *fakeExtHost) Pages(context.Context, string, string) ([]string, error) { return nil, nil }
func (f *fakeExtHost) Page(context.Context, string, string) (io.ReadCloser, error) {
	return nil, nil
}

func (f *fakeExtHost) Extensions(context.Context) ([]sourcehost.ExtensionInfo, error) {
	out := make([]sourcehost.ExtensionInfo, 0, len(f.catalog))
	for _, pkg := range f.catalog {
		info := sourcehost.ExtensionInfo{PkgName: pkg, AvailableVersion: "1.0"}
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

// Sento owns the desired list; a wiped host converges back to it.
func TestExtensionReconcile(t *testing.T) {
	_, st := setup(t)
	ctx := context.Background()

	host := &fakeExtHost{
		catalog:   []string{"ext.a", "ext.b", "ext.c"},
		installed: map[string]bool{},
	}
	svc := NewExtensionService(st, func() sourcehost.Host { return host })

	// Installing records desire and installs on the host.
	if err := svc.Install(ctx, "ext.a"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Install(ctx, "ext.b"); err != nil {
		t.Fatal(err)
	}
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
	if desired != 2 {
		t.Fatalf("want 2 desired, got %+v", views)
	}

	// Uninstall removes the desire too.
	if err := svc.Uninstall(ctx, "ext.b"); err != nil {
		t.Fatal(err)
	}

	// The host gets wiped (fresh pod); reconcile reinstalls only what is
	// desired and missing.
	host.installed = map[string]bool{}
	host.installs = nil
	if err := svc.Reconcile(ctx); err != nil {
		t.Fatal(err)
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
}

// Backends without management (Suwayomi) make reconcile a no-op and the
// management API unavailable.
func TestExtensionsNoManager(t *testing.T) {
	_, st := setup(t)
	svc := NewExtensionService(st, func() sourcehost.Host { return nil })
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
