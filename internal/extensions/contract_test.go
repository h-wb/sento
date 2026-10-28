//go:build contract

// Contract tests against a live sento-extensions sidecar. Their job is to fail
// loudly when a Suwayomi bump (host/vendir.yml) breaks the vendored fake
// Android runtime or the APK loading path — the parts of host/ Sento does
// not own and cannot book-test. Network- and JVM-bound, so never part of
// `go test ./...`:
//
//	mise run extensions:contract
//
// Env:
//
//	SENTO_EXTENSIONS_URL       sidecar base URL (default http://localhost:8901)
//	SENTO_TEST_EXTENSION_REPO   Mihon-compatible repo index URL; unlocks the
//	                            install/search/pages tier (skipped if unset)
//	SENTO_TEST_EXTENSION_PKG    pkgName to install (default: first with a
//	                            source in the repo's language)
//	SENTO_TEST_SEARCH_QUERY     search term for the live source (default "one piece")
package extensions

import (
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func hostURL() string {
	if u := os.Getenv("SENTO_EXTENSIONS_URL"); u != "" {
		return u
	}
	return "http://localhost:8901"
}

// requireHost fails (rather than skips) when the sidecar is unreachable:
// these tests are only ever run deliberately, and a silent skip after a
// vendir bump would defeat the point.
func requireHost(t *testing.T) (*Client, context.Context) {
	t.Helper()
	base := hostURL()
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(base + "/health")
	if err != nil {
		t.Fatalf("sidecar unreachable at %s: %v (start it with: mise daemons start extensions)", base, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sidecar %s/health returned %d", base, resp.StatusCode)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	return NewClient(base), ctx
}

// TestContractBoot is the vendir-bump smoke test: a sidecar that starts and
// answers these three endpoints has loaded AndroidCompat and its Koin
// modules successfully.
func TestContractBoot(t *testing.T) {
	c, ctx := requireHost(t)

	// Sources may legitimately be empty (no extensions installed); the
	// contract is that the endpoint answers with a decodable array.
	if _, err := c.Sources(ctx); err != nil {
		t.Fatalf("Sources: %v", err)
	}
	if _, err := c.Extensions(ctx); err != nil {
		t.Fatalf("Extensions: %v", err)
	}
	if _, err := c.Repos(ctx); err != nil {
		t.Fatalf("Repos: %v", err)
	}
}

// TestContractRepoRoundTrip covers the reconciliation contract: Sento
// pushes its stored repo list onto a stateless host and reads it back.
func TestContractRepoRoundTrip(t *testing.T) {
	c, ctx := requireHost(t)

	before, err := c.Repos(ctx)
	if err != nil {
		t.Fatalf("Repos: %v", err)
	}
	t.Cleanup(func() {
		if err := c.SetRepos(context.Background(), before); err != nil {
			t.Errorf("restoring repos: %v", err)
		}
	})

	const probe = "https://example.invalid/index.min.json"
	if err := c.SetRepos(ctx, []string{probe}); err != nil {
		t.Fatalf("SetRepos: %v", err)
	}
	got, err := c.Repos(ctx)
	if err != nil {
		t.Fatalf("Repos after SetRepos: %v", err)
	}
	if len(got) != 1 || got[0] != probe {
		t.Fatalf("Repos round-trip = %v, want [%s]", got, probe)
	}

	// A repo that cannot be loaded is reported with its reason, not
	// silently counted as empty.
	statuses, err := c.RepoStatus(ctx)
	if err != nil {
		t.Fatalf("RepoStatus: %v", err)
	}
	if len(statuses) != 1 || statuses[0].URL != probe || statuses[0].Error == "" {
		t.Fatalf("RepoStatus for an unreachable repo = %+v, want one entry carrying an error", statuses)
	}
}

// TestContractExtensionLifecycle is the real proof that the vendored APK
// machinery (ExtensionGithubApi, PackageTools/dex2jar, the child-first
// classloader) still works: install a real extension and drive it.
func TestContractExtensionLifecycle(t *testing.T) {
	repo := os.Getenv("SENTO_TEST_EXTENSION_REPO")
	if repo == "" {
		t.Skip("SENTO_TEST_EXTENSION_REPO unset — skipping the install/search tier")
	}
	c, ctx := requireHost(t)

	before, err := c.Repos(ctx)
	if err != nil {
		t.Fatalf("Repos: %v", err)
	}
	t.Cleanup(func() { _ = c.SetRepos(context.Background(), before) })
	if err := c.SetRepos(ctx, []string{repo}); err != nil {
		t.Fatalf("SetRepos(%s): %v", repo, err)
	}

	exts, err := c.Extensions(ctx)
	if err != nil {
		t.Fatalf("Extensions: %v", err)
	}
	if len(exts) == 0 {
		t.Fatalf("no extensions listed from %s — the repo index format or ExtensionGithubApi changed", repo)
	}

	// The same answer per repo, which is what the Connections page shows.
	statuses, err := c.RepoStatus(ctx)
	if err != nil {
		t.Fatalf("RepoStatus: %v", err)
	}
	if len(statuses) != 1 || statuses[0].URL != repo {
		t.Fatalf("RepoStatus = %+v, want exactly %s", statuses, repo)
	}
	if st := statuses[0]; st.Error != "" || st.Extensions == 0 || st.Name == "" {
		t.Errorf("RepoStatus for a working repo = %+v, want a name, a count and no error", st)
	}

	pkg := os.Getenv("SENTO_TEST_EXTENSION_PKG")
	if pkg == "" {
		pkg = exts[0].PkgName
	}
	if err := c.InstallExtension(ctx, pkg); err != nil {
		// Distinguish the two failure modes: a transport error means the
		// repo moved its APKs (the index's apkUrl convention changed),
		// while anything else points at dex2jar or the classloader.
		t.Fatalf("InstallExtension(%s): %v\n"+
			"  HTTP 404 → the repo no longer serves APKs where the index implies;\n"+
			"  otherwise → APK conversion or class loading broke", pkg, err)
	}
	t.Cleanup(func() { _ = c.UninstallExtension(context.Background(), pkg) })

	srcs, err := c.ExtensionSources(ctx, pkg)
	if err != nil {
		t.Fatalf("ExtensionSources(%s): %v", pkg, err)
	}
	if len(srcs) == 0 {
		t.Fatalf("installed %s exposes no sources — SourceFactory/CatalogueSource reflection broke", pkg)
	}
	sid := srcs[0].ID

	// The catalogue declares an extension's sources before it is installed
	// (the Add Indexer flow picks from them). What it declared must be
	// what the installed extension really provides, or an indexer entry
	// would point at a source that never appears.
	have := map[string]bool{}
	for _, s := range srcs {
		have[s.ID] = true
	}
	for _, e := range exts {
		if e.PkgName != pkg {
			continue
		}
		if len(e.Sources) == 0 {
			t.Logf("the index declares no sources for %s; nothing to cross-check", pkg)
		}
		for _, s := range e.Sources {
			if !have[s.ID] {
				t.Errorf("index declares source %s (%s, %s) but installed %s does not provide it", s.ID, s.Name, s.Lang, pkg)
			}
		}
	}

	// Installed sources must also show up in the global list Sento reads.
	all, err := c.Sources(ctx)
	if err != nil {
		t.Fatalf("Sources: %v", err)
	}
	var found bool
	for _, s := range all {
		if s.ID == sid {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("source %s missing from Sources() after install", sid)
	}

	query := os.Getenv("SENTO_TEST_SEARCH_QUERY")
	if query == "" {
		query = "one piece"
	}
	mangas, err := c.Search(ctx, sid, query)
	if err != nil {
		t.Fatalf("Search(%s, %q): %v — the source's HTTP path (OkHttp/interceptors) broke", sid, query, err)
	}
	if len(mangas) == 0 {
		t.Skipf("source %s returned no results for %q; nothing to drive chapters with", sid, query)
	}

	chapters, err := c.Chapters(ctx, sid, mangas[0].Key)
	if err != nil {
		t.Fatalf("Chapters(%s): %v", mangas[0].Key, err)
	}
	if len(chapters) == 0 {
		t.Fatalf("no chapters for %q — chapter parsing broke", mangas[0].Title)
	}

	pages, err := c.Pages(ctx, sid, chapters[len(chapters)-1].Key)
	if err != nil {
		t.Fatalf("Pages: %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("no page refs — page-list parsing broke")
	}

	body, err := c.Page(ctx, sid, pages[0])
	if err != nil {
		t.Fatalf("Page: %v", err)
	}
	defer func() { _ = body.Close() }()
	// Enough bytes to prove real image data flowed through the source's
	// client, not an error page.
	n, err := io.Copy(io.Discard, io.LimitReader(body, 4096))
	if err != nil {
		t.Fatalf("reading page bytes: %v", err)
	}
	if n < 512 {
		t.Fatalf("page body was %d bytes; expected image data", n)
	}
}
