package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/config"
	"github.com/h-wb/sento/internal/db/dbtest"
	"github.com/h-wb/sento/internal/metadata"
	"github.com/h-wb/sento/internal/service"
)

// noMetadata is a metadata provider that knows nothing.
type noMetadata struct{}

func (noMetadata) Search(context.Context, string) ([]metadata.SeriesRef, error) { return nil, nil }
func (noMetadata) GetSeries(context.Context, int64) (*metadata.Series, error) {
	return nil, context.DeadlineExceeded
}
func (noMetadata) GetCollections(context.Context, int64) ([]metadata.Collection, error) {
	return nil, nil
}
func (noMetadata) GetEditions(context.Context) ([]metadata.Edition, error) { return nil, nil }
func (noMetadata) GetWorks(context.Context, int64) (map[string][]metadata.Work, error) {
	return nil, nil
}

func testServer(t *testing.T) (http.Handler, string) {
	t.Helper()
	static := t.TempDir()
	for path, body := range map[string]string{
		"index.html":           "<html>app</html>",
		"favicon.svg":          "<svg/>",
		"assets/index-abc1.js": "console.log(1)",
	} {
		full := filepath.Join(static, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Default()
	cfg.Server.StaticDir = static
	cfg.Library.Path = t.TempDir()
	cfg.Downloads.Dir = t.TempDir()
	conn, dialect := dbtest.Open(t)
	return New(cfg, conn, dialect, noMetadata{}).Handler(), static
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

// A browser that caches index.html keeps loading the previous release's
// bundle after an upgrade; the fingerprinted assets are the opposite case.
func TestStaticCaching(t *testing.T) {
	h, _ := testServer(t)
	for path, want := range map[string]string{
		"/":                     "no-cache",
		"/series/12":            "no-cache", // SPA route → index.html
		"/favicon.svg":          "no-cache",
		"/assets/index-abc1.js": "public, max-age=31536000, immutable",
	} {
		rec := do(t, h, http.MethodGet, path, "")
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != want {
			t.Errorf("GET %s Cache-Control = %q, want %q", path, got, want)
		}
	}
	if rec := do(t, h, http.MethodGet, "/series/12", ""); !strings.Contains(rec.Body.String(), "app") {
		t.Errorf("SPA route did not fall back to index.html: %q", rec.Body.String())
	}
}

func TestStaticDoesNotEscapeItsRoot(t *testing.T) {
	h, static := testServer(t)
	secret := filepath.Join(filepath.Dir(static), "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(secret) }()
	for _, path := range []string{"/../secret.txt", "/assets/../../secret.txt", "/%2e%2e/secret.txt"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.URL.Path = path
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if strings.Contains(rec.Body.String(), "top secret") {
			t.Errorf("GET %s leaked a file outside the static dir", path)
		}
	}
}

func TestAPIErrorShapes(t *testing.T) {
	h, _ := testServer(t)

	if rec := do(t, h, http.MethodGet, "/api/v1/system/health", ""); rec.Code != http.StatusOK {
		t.Fatalf("health = %d", rec.Code)
	}
	// Unknown API routes answer JSON 404, never the SPA shell.
	rec := do(t, h, http.MethodGet, "/api/v1/nope", "")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Errorf("unknown api route = %d %q, want a JSON 404", rec.Code, rec.Body.String())
	}
	// Service errors map by kind: a missing series is a 404, not a 500.
	if rec := do(t, h, http.MethodGet, "/api/v1/series/999", ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing series = %d, want 404", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/series/abc", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("non-numeric id = %d, want 400", rec.Code)
	}
	// A failing metadata provider is the upstream's fault: 502.
	if rec := do(t, h, http.MethodPost, "/api/v1/series", `{"mangabakaId":1}`); rec.Code != http.StatusBadGateway {
		t.Errorf("add with a failing provider = %d, want 502", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/series", `{not json`); rec.Code != http.StatusBadRequest {
		t.Errorf("malformed body = %d, want 400", rec.Code)
	}
}

func TestIndexerSearchDiagnosticEndpoint(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write([]byte(`<caps><categories><category id="7000" name="Books"/></categories></caps>`))
			return
		}
		if r.URL.Query().Get("q") != "Example" || r.URL.Query().Get("cat") != "7000" {
			t.Errorf("unexpected diagnostic query: %v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`<rss><channel><item><title>Example v01</title></item></channel></rss>`))
	}))
	defer upstream.Close()
	h, _ := testServer(t)
	data, err := json.Marshal(map[string]any{
		"name": "Diagnostic fixture", "protocol": "torznab", "enabled": true,
		"settings": map[string]string{"url": upstream.URL, "apiKey": "fixture-key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	created := do(t, h, http.MethodPost, "/api/v1/indexers", string(data))
	if created.Code != http.StatusCreated {
		t.Fatalf("create indexer: %d %s", created.Code, created.Body.String())
	}
	var ix struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &ix); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/indexers/" + strconv.FormatInt(ix.ID, 10) + "/search-test"
	response := do(t, h, http.MethodPost, path, `{"query":"Example"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("search diagnostic: %d %s", response.Code, response.Body.String())
	}
	var result service.IndexerSearchCheck
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.BooksFound != 1 || len(result.Samples) != 1 || result.Samples[0].Title != "Example v01" {
		t.Fatalf("diagnostic = %+v", result)
	}
	if response := do(t, h, http.MethodPost, path, `{"query":" "}`); response.Code != http.StatusBadRequest {
		t.Fatalf("empty query status = %d", response.Code)
	}
	if response := do(t, h, http.MethodPost, "/api/v1/indexers/99999/search-test", `{"query":"Example"}`); response.Code != http.StatusNotFound {
		t.Fatalf("unknown indexer status = %d", response.Code)
	}
}

func TestRequestBodiesAreCapped(t *testing.T) {
	h, _ := testServer(t)
	huge := `{"name":"` + strings.Repeat("a", maxBodyBytes) + `"}`
	if rec := do(t, h, http.MethodPost, "/api/v1/profiles", huge); rec.Code != http.StatusBadRequest {
		t.Errorf("oversized body = %d, want 400", rec.Code)
	}
}

// The health report is served from cache and re-run once a change to what
// it checks invalidates it.
func TestHealthChecksEndpoint(t *testing.T) {
	h, _ := testServer(t)
	issues := func(method string) []service.HealthIssue {
		t.Helper()
		rec := do(t, h, method, "/api/v1/system/checks", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s checks = %d: %s", method, rec.Code, rec.Body.String())
		}
		var report service.HealthReport
		if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil || report.CheckedAt == "" {
			t.Fatalf("report = %s (%v)", rec.Body.String(), err)
		}
		return report.Issues
	}
	if got := issues(http.MethodGet); len(got) != 1 || got[0].Source != "indexer" {
		t.Fatalf("fresh install = %+v", got)
	}
	body := `{"name":"Dead","protocol":"torznab","enabled":true,"useVolumes":true,"settings":{"url":"http://127.0.0.1:1"}}`
	if rec := do(t, h, http.MethodPost, "/api/v1/indexers", body); rec.Code != http.StatusCreated {
		t.Fatalf("add indexer = %d: %s", rec.Code, rec.Body.String())
	}
	got := issues(http.MethodGet)
	if len(got) != 2 || got[0].Level != service.HealthError || got[0].Message != "Indexer Dead is unavailable" {
		t.Fatalf("after adding a dead indexer = %+v", got)
	}
	if got := issues(http.MethodPost); len(got) != 2 {
		t.Fatalf("re-run = %+v", got)
	}
}

func TestQueueAnotherRequiresSelectedDownloads(t *testing.T) {
	h, _ := testServer(t)
	for _, body := range []string{`{}`, `{"ids":[]}`} {
		if r := do(t, h, http.MethodPost, "/api/v1/queue/another", body); r.Code != http.StatusBadRequest {
			t.Fatalf("empty selection: %d %s", r.Code, r.Body.String())
		}
	}
	if r := do(t, h, http.MethodPost, "/api/v1/queue/another", `{"ids":[999]}`); r.Code != http.StatusNotFound {
		t.Fatalf("missing job: %d %s", r.Code, r.Body.String())
	}
}

func TestActivityScopesAndEmptyCollections(t *testing.T) {
	h, _ := testServer(t)
	for _, path := range []string{"/api/v1/activity", "/api/v1/activity?seriesId=0"} {
		r := do(t, h, http.MethodGet, path, "")
		if r.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, r.Code, r.Body.String())
		}
		var body struct {
			Downloads []any `json:"downloads"`
			Reviews   []any `json:"reviews"`
			Events    []any `json:"events"`
		}
		if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Downloads == nil || body.Reviews == nil || body.Events == nil {
			t.Fatalf("empty lists must be arrays: %s", r.Body.String())
		}
	}
	for _, path := range []string{"/api/v1/activity?seriesId=bad", "/api/v1/activity?seriesId=-1"} {
		if r := do(t, h, http.MethodGet, path, ""); r.Code != http.StatusBadRequest {
			t.Fatalf("invalid scope = %d", r.Code)
		}
	}
	if r := do(t, h, http.MethodGet, "/api/v1/activity?seriesId=999", ""); r.Code != http.StatusNotFound {
		t.Fatalf("missing scope = %d", r.Code)
	}
}
