//go:build e2e

// Command e2estub serves deterministic stand-ins for every upstream Sento
// talks to — MangaBaka, a sento-extensions sidecar, Prowlarr and qBittorrent —
// on one port, each under its own path prefix. The Playwright suite boots
// it alongside a real sento so browser tests exercise the whole Go stack
// without touching the network.
//
// Fixtures are fixed: /mangabaka/v1/series/search always returns the same
// three series, so assertions can name them. Never built by CI's release
// path — the e2e build tag keeps it out of ./... .
//
//	go run -tags e2e ./cmd/e2estub -addr :8899
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
)

func main() {
	addr := flag.String("addr", ":8899", "listen address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mountMangaBaka(mux)
	mountExtensions(mux)
	mountProwlarr(mux)
	mountQBittorrent(mux)

	log.Printf("e2estub listening on %s", *addr)
	srv := &http.Server{Addr: *addr, Handler: logging(mux)}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.RequestURI())
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("encoding response: %v", err)
	}
}

// envelope mirrors MangaBaka's {status, data, pagination} wrapper.
func envelope(data any) map[string]any {
	return map[string]any{"status": 200, "data": data, "pagination": nil}
}

// --- MangaBaka (SENTO_MANGABAKA_BASE_URL=<stub>/mangabaka/v1) ---

const standardEdition = "edition-standard"

func collectionID(series int64, medium string) string {
	if medium == "digital" {
		return fmt.Sprintf("col-%d-en", series)
	}
	return fmt.Sprintf("col-%d-en-%s", series, medium)
}

type stubSeries struct {
	ID            int64    `json:"id"`
	State         string   `json:"state"`
	Title         string   `json:"title"`
	NativeTitle   string   `json:"native_title"`
	Authors       []string `json:"authors"`
	Artists       []string `json:"artists"`
	Description   string   `json:"description"`
	Year          int      `json:"year"`
	Status        string   `json:"status"`
	ContentRating string   `json:"content_rating"`
	Type          string   `json:"type"`
	FinalVolume   string   `json:"final_volume"`
	TotalChapters string   `json:"total_chapters"`
	Genres        []string `json:"genres"`
	Cover         struct {
		Raw struct {
			URL string `json:"url"`
		} `json:"raw"`
		X350 struct {
			X1 string `json:"x1"`
		} `json:"x350"`
	} `json:"cover"`
	Titles []struct {
		Language string `json:"language"`
		Title    string `json:"title"`
		Primary  bool   `json:"is_primary"`
	} `json:"titles"`
	Publishers []map[string]string `json:"publishers"`
	// unlisted: the catalog knows the series but lists no collections or
	// works for it, as MangaBaka does for publishers it does not cover.
	unlisted bool
}

// unlistedSeries is a series whose volumes the catalog does not list: only
// the series record says who publishes it and how many volumes there are.
func unlistedSeries(id int64, title string, year, volumes int) stubSeries {
	s := newStubSeries(id, title, year, volumes)
	s.unlisted = true
	s.Publishers = []map[string]string{
		{"name": "Stub House", "language": "en", "type": "English", "note": "5 Volumes - Ongoing"},
		{"name": "Éditions Stub", "language": "fr", "type": "Other"},
	}
	return s
}

func newStubSeries(id int64, title string, year, volumes int) stubSeries {
	s := stubSeries{
		ID: id, State: "active", Title: title, NativeTitle: title,
		Authors: []string{"Stub Author"}, Artists: []string{"Stub Artist"},
		Description: "A deterministic fixture series served by e2estub.",
		Year:        year, Status: "completed", ContentRating: "safe", Type: "manga",
		FinalVolume: fmt.Sprint(volumes), TotalChapters: fmt.Sprint(volumes * 9),
		Genres: []string{"Action", "Adventure"},
	}
	// A real URL would make the browser hit the network; a data URI keeps
	// covers offline and the suite hermetic.
	const px = "data:image/gif;base64,R0lGODlhAQABAIAAAP///wAAACH5BAEAAAAALAAAAAABAAEAAAICRAEAOw=="
	s.Cover.Raw.URL, s.Cover.X350.X1 = px, px
	s.Titles = append(s.Titles, struct {
		Language string `json:"language"`
		Title    string `json:"title"`
		Primary  bool   `json:"is_primary"`
	}{Language: "en", Title: title, Primary: true})
	return s
}

// mbFixtures is the whole MangaBaka universe the browser suite sees.
var mbFixtures = map[int64]stubSeries{
	101: newStubSeries(101, "Stub Blade", 2011, 12),
	102: newStubSeries(102, "Stub Blade: Afterstory", 2019, 3),
	103: newStubSeries(103, "Deterministic Diaries", 2004, 7),
	// Not in mbOrder: reachable by id, never a search result.
	104: unlistedSeries(104, "Unlisted Volumes", 2015, 6),
}

// mbOrder fixes search-result ordering; map iteration would not.
var mbOrder = []int64{101, 102, 103}

func mountMangaBaka(mux *http.ServeMux) {
	mux.HandleFunc("GET /mangabaka/v1/series/search", func(w http.ResponseWriter, r *http.Request) {
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		rows := []stubSeries{}
		for _, id := range mbOrder {
			s := mbFixtures[id]
			if q == "" || strings.Contains(strings.ToLower(s.Title), q) {
				rows = append(rows, s)
			}
		}
		writeJSON(w, envelope(rows))
	})

	mux.HandleFunc("GET /mangabaka/v1/series/{id}", func(w http.ResponseWriter, r *http.Request) {
		s, ok := mbFixtures[pathID(r.PathValue("id"))]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, envelope(s))
	})

	mux.HandleFunc("GET /mangabaka/v1/series/{id}/collections", func(w http.ResponseWriter, r *http.Request) {
		id := pathID(r.PathValue("id"))
		s, ok := mbFixtures[id]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		// As MangaBaka lists them: one collection per medium, both of the
		// same edition.
		cols := []map[string]any{}
		for _, medium := range media(s) {
			cols = append(cols, map[string]any{
				"id":          collectionID(id, medium),
				"publisher":   map[string]string{"name": "Stub Publishing"},
				"edition":     map[string]string{"id": standardEdition, "name": "Standard Edition"},
				"language":    map[string]string{"iso": "en"},
				"type":        "volume",
				"format":      "paged",
				"medium":      medium,
				"status":      "releasing",
				"count_main":  volumeCount(s),
				"count_extra": 0,
				"count_other": 0,
				"start_date":  fmt.Sprintf("%d-01-01", s.Year),
				"end_date":    "",
			})
		}
		writeJSON(w, envelope(cols))
	})

	mux.HandleFunc("GET /mangabaka/v1/editions/all", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, envelope([]map[string]string{
			{"id": standardEdition, "name": "Standard Edition"},
			{"id": "edition-omnibus", "name": "Omnibus Edition"},
			{"id": "edition-deluxe", "name": "Deluxe Edition"},
		}))
	})

	mux.HandleFunc("GET /mangabaka/v1/series/{id}/works", func(w http.ResponseWriter, r *http.Request) {
		id := pathID(r.PathValue("id"))
		s, ok := mbFixtures[id]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		works := []map[string]any{}
		for m, medium := range media(s) {
			for i := 1; i <= volumeCount(s); i++ {
				works = append(works, map[string]any{
					"id":               fmt.Sprintf("work-%d-%d-%s", id, i, medium),
					"count_type":       "main",
					"release_date":     fmt.Sprintf("%d-%02d-01", s.Year+i/6, (i%12)+1),
					"sequence_string":  fmt.Sprint(i),
					"sequence_numeric": i,
					"pages":            192,
					"identifiers":      []map[string]string{{"id": fmt.Sprintf("97800%d000%04d", m, i), "name": "isbn"}},
					"collections":      []map[string]string{{"id": collectionID(id, medium)}},
				})
			}
		}
		writeJSON(w, envelope(works))
	})
}

func media(s stubSeries) []string {
	if s.unlisted {
		return nil
	}
	return []string{"digital", "paperback"}
}

func volumeCount(s stubSeries) int {
	var n int
	_, _ = fmt.Sscanf(s.FinalVolume, "%d", &n)
	return n
}

func pathID(s string) int64 {
	var id int64
	_, _ = fmt.Sscanf(s, "%d", &id)
	return id
}

// --- sento-extensions sidecar (SENTO_EXTENSIONS_URL=<stub>/host) ---

// hostState is the mutable slice of the sidecar contract: Sento reconciles
// repos and installed extensions onto a stateless host, so the stub has to
// remember what was pushed for those flows to read back correctly.
var hostState = struct {
	sync.Mutex
	repos     []string
	installed map[string]bool
}{installed: map[string]bool{}}

// repoWorks reports whether a pushed repo URL yields a catalogue; any URL
// containing "broken" stands for a repo that fails to load.
func repoWorks(url string) bool { return !strings.Contains(url, "broken") }

func mountExtensions(mux *http.ServeMux) {
	mux.HandleFunc("GET /host/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /host/api/v1/sources", func(w http.ResponseWriter, _ *http.Request) {
		sources := []map[string]any{
			{"id": "stub-en-1", "name": "Stub Source", "lang": "en", "nsfw": false},
		}
		hostState.Lock()
		if hostState.installed["eu.kanade.tachiyomi.extension.all.lazysource"] {
			sources = append(sources,
				map[string]any{"id": "lazy-en-1", "name": "Lazy Source", "lang": "en", "nsfw": false},
				map[string]any{"id": "lazy-fr-1", "name": "Lazy Source", "lang": "fr", "nsfw": false})
		}
		hostState.Unlock()
		writeJSON(w, sources)
	})

	mux.HandleFunc("GET /host/api/v1/sources/{sid}/search", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []map[string]any{
			{"key": "manga-101", "title": "Stub Blade"},
			{"key": "manga-103", "title": "Deterministic Diaries"},
		})
	})

	mux.HandleFunc("GET /host/api/v1/sources/{sid}/listings", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []map[string]any{}) // the stub sources have no nested listings
	})

	mux.HandleFunc("GET /host/api/v1/sources/{sid}/chapters", func(w http.ResponseWriter, _ *http.Request) {
		chs := []map[string]any{}
		for i := 1; i <= 10; i++ {
			chs = append(chs, map[string]any{
				"key":    fmt.Sprintf("ch-%d", i),
				"name":   fmt.Sprintf("Chapter %d", i),
				"number": i,
			})
		}
		writeJSON(w, chs)
	})

	mux.HandleFunc("GET /host/api/v1/sources/{sid}/pages", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []string{"page-1", "page-2", "page-3"})
	})

	// A 1x1 GIF stands in for page bytes: enough for CBZ assembly to run.
	mux.HandleFunc("GET /host/api/v1/sources/{sid}/page", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/gif")
		_, _ = w.Write([]byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\xff\xff\xff\x00\x00\x00!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;"))
	})

	mux.HandleFunc("GET /host/api/v1/sources/{sid}/preferences", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []map[string]any{{
			"key": "mirror", "type": "ListPreference", "title": "Mirror",
			"valueType": "String", "default": "a", "current": "a",
			"entries": []string{"Mirror A", "Mirror B"}, "entryValues": []string{"a", "b"},
		}})
	})
	mux.HandleFunc("POST /host/api/v1/sources/{sid}/preferences", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /host/api/v1/repos", func(w http.ResponseWriter, _ *http.Request) {
		hostState.Lock()
		defer hostState.Unlock()
		// Same envelope as the sidecar: {"urls": [...]}.
		writeJSON(w, map[string]any{"urls": append([]string{}, hostState.repos...)})
	})
	mux.HandleFunc("GET /host/api/v1/repos/status", func(w http.ResponseWriter, _ *http.Request) {
		hostState.Lock()
		defer hostState.Unlock()
		out := []map[string]any{}
		for _, u := range hostState.repos {
			st := map[string]any{"url": u, "name": "Stub Repo", "extensions": 2}
			if !repoWorks(u) {
				st = map[string]any{"url": u, "extensions": 0, "error": "fetch of " + u + " returned HTTP 404"}
			}
			out = append(out, st)
		}
		writeJSON(w, out)
	})
	mux.HandleFunc("PUT /host/api/v1/repos", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			URLs []string `json:"urls"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		hostState.Lock()
		hostState.repos = body.URLs
		hostState.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /host/api/v1/extensions", func(w http.ResponseWriter, _ *http.Request) {
		hostState.Lock()
		defer hostState.Unlock()
		const pkg = "eu.kanade.tachiyomi.extension.en.stubsource"
		ext := map[string]any{
			"pkgName": pkg, "name": "Stub Source", "lang": "en", "nsfw": false,
			"availableVersion": "1.4.0",
			"sources": []map[string]any{
				{"id": "stub-en-1", "name": "Stub Source", "lang": "en", "nsfw": false},
			},
		}
		if hostState.installed[pkg] {
			ext["installedVersion"] = "1.4.0"
		}
		// A second, never-installed extension with two sources: the Add
		// Indexer flow must offer them without installing anything.
		const lazy = "eu.kanade.tachiyomi.extension.all.lazysource"
		other := map[string]any{
			"pkgName": lazy, "name": "Lazy Source", "lang": "all", "nsfw": false,
			"availableVersion": "1.4.2",
			"sources": []map[string]any{
				{"id": "lazy-en-1", "name": "Lazy Source", "lang": "en", "nsfw": false},
				{"id": "lazy-fr-1", "name": "Lazy Source", "lang": "fr", "nsfw": false},
			},
		}
		if hostState.installed[lazy] {
			other["installedVersion"] = "1.4.2"
		}
		// Like the sidecar: the catalogue is what the configured repos
		// offer, plus whatever is installed. No working repo, nothing to
		// offer.
		offered := slices.ContainsFunc(hostState.repos, repoWorks)
		out := []map[string]any{}
		for _, e := range []map[string]any{ext, other} {
			if offered || e["installedVersion"] != nil {
				out = append(out, e)
			}
		}
		writeJSON(w, out)
	})
	mux.HandleFunc("GET /host/api/v1/extensions/{pkg}/sources", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []map[string]any{
			{"id": "stub-en-1", "name": "Stub Source", "lang": "en", "nsfw": false},
		})
	})
	mux.HandleFunc("POST /host/api/v1/extensions/{pkg}", func(w http.ResponseWriter, r *http.Request) {
		hostState.Lock()
		hostState.installed[r.PathValue("pkg")] = true
		hostState.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /host/api/v1/extensions/{pkg}", func(w http.ResponseWriter, r *http.Request) {
		hostState.Lock()
		delete(hostState.installed, r.PathValue("pkg"))
		hostState.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
}

// --- Prowlarr (SENTO_PROWLARR_URL=<stub>/prowlarr) ---

func mountProwlarr(mux *http.ServeMux) {
	mux.HandleFunc("GET /prowlarr/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []any{})
	})
	mux.HandleFunc("GET /prowlarr/api/v1/indexer", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []map[string]any{
			{"id": 1, "name": "Stub Tracker", "protocol": "torrent", "enable": true},
		})
	})
	mux.HandleFunc("GET /prowlarr/api/v1/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		rows := []map[string]any{}
		for i := 1; i <= 3; i++ {
			rows = append(rows, map[string]any{
				"guid":        fmt.Sprintf("stub-release-%d", i),
				"title":       fmt.Sprintf("%s v%02d (2019) (Digital) (Stub)", firstWords(q), i),
				"size":        int64(120_000_000 + i),
				"indexer":     "Stub Tracker",
				"indexerId":   1,
				"seeders":     40 - i,
				"leechers":    i,
				"downloadUrl": fmt.Sprintf("http://%s/prowlarr/download/%d.torrent", r.Host, i),
				"infoHash":    fmt.Sprintf("%040d", i),
				"protocol":    "torrent",
				"publishDate": "2024-01-0" + fmt.Sprint(i) + "T00:00:00Z",
			})
		}
		writeJSON(w, rows)
	})
}

// firstWords keeps release titles recognizable without echoing a whole query.
func firstWords(q string) string {
	if q == "" {
		return "Stub Blade"
	}
	if i := strings.Index(q, " v"); i > 0 {
		return q[:i]
	}
	return q
}

// --- qBittorrent (SENTO_QBITTORRENT_URL=<stub>/qbit) ---

func mountQBittorrent(mux *http.ServeMux) {
	mux.HandleFunc("POST /qbit/api/v2/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "stub-session"})
		_, _ = w.Write([]byte("Ok."))
	})
	mux.HandleFunc("GET /qbit/api/v2/app/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("v4.6.0"))
	})
	mux.HandleFunc("POST /qbit/api/v2/torrents/add", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("Ok."))
	})
	mux.HandleFunc("GET /qbit/api/v2/torrents/info", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []any{})
	})
	mux.HandleFunc("GET /qbit/api/v2/torrents/files", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []any{})
	})
}
