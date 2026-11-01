//go:build e2e

package main

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"encoding/json"

	"github.com/h-wb/sento/internal/qbittorrent"
)

const scenarioSource = "scenario-en-1"
const scenarioPackage = "eu.kanade.tachiyomi.extension.en.scenario"
const directoryRoot = "/Directory Kingdom"
const digitalFolder = directoryRoot + "/!(Digital) [1r0n]"
const namesFolder = directoryRoot + "/!ED-Japanese Names (Digital) [1r0n, Bingbong]"

var scenarioNames = []string{"healthy", "nested-listings", "page-503", "rate-limit", "long-cooldown", "permanent-401", "source-outage", "truncated-page"}

// Controls exist only in the e2e binary and only when -control is passed.
// Changing a fault plan preserves attempts and the request journal. Reset
// is separate: a backend restart must not silently reset the upstream.
type scenarioControl struct {
	sync.Mutex
	enabled  bool
	name     string
	attempts map[string]int
	events   []scenarioEvent
	clients  map[string]map[string]fixtureTorrent
}

type scenarioEvent struct {
	At          time.Time `json:"at"`
	Kind        string    `json:"kind"`
	Client      string    `json:"client,omitempty"`
	Key         string    `json:"key,omitempty"`
	Attempt     int       `json:"attempt,omitempty"`
	Status      int       `json:"status"`
	Category    string    `json:"category,omitempty"`
	DeleteFiles bool      `json:"deleteFiles"`
	Truncated   bool      `json:"truncated,omitempty"`
	RetryAfter  string    `json:"retryAfter,omitempty"`
}

type fixtureTorrent struct {
	qbittorrent.Torrent
	Category string `json:"category"`
}

var scenarios = &scenarioControl{name: "healthy", attempts: map[string]int{}, events: []scenarioEvent{}, clients: map[string]map[string]fixtureTorrent{}}

func (s *scenarioControl) record(e scenarioEvent) {
	// Caller holds s.Mutex. Never journal credentials, full URLs or bodies.
	e.At = time.Now().UTC()
	if len(s.events) == 5000 {
		s.events = s.events[1:]
	}
	s.events = append(s.events, e)
}

func mountScenarios(mux *http.ServeMux) {
	if !scenarios.enabled {
		return
	}
	mux.HandleFunc("GET /_control", func(w http.ResponseWriter, r *http.Request) {
		scenarios.Lock()
		defer scenarios.Unlock()
		writeJSON(w, map[string]any{"service": "sento-e2estub", "scenario": scenarios.name, "scenarios": scenarioNames, "attempts": scenarios.attempts, "events": scenarios.events, "clients": scenarios.clients})
	})
	mux.HandleFunc("POST /_control/scenario", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || !slices.Contains(scenarioNames, body.Name) {
			http.Error(w, "unknown scenario; see GET /_control", 400)
			return
		}
		scenarios.Lock()
		scenarios.name = body.Name
		scenarios.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /_control/reset", func(w http.ResponseWriter, r *http.Request) {
		scenarios.Lock()
		defer scenarios.Unlock()
		scenarios.name = "healthy"
		scenarios.attempts = map[string]int{}
		scenarios.events = []scenarioEvent{}
		scenarios.clients = map[string]map[string]fixtureTorrent{}
		w.WriteHeader(http.StatusNoContent)
	})
	// Simulate a client pausing, stalling, losing or finishing a torrent.
	// No disk data is invented: finishing with no files exercises Review.
	mux.HandleFunc("POST /_control/torrent", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Client string `json:"client"`
			fixtureTorrent
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || (body.Client != "regular" && body.Client != "vpn") || body.Hash == "" || body.Progress < 0 || body.Progress > 1 {
			http.Error(w, "client must be regular/vpn, with hash and progress in 0..1", 400)
			return
		}
		scenarios.Lock()
		defer scenarios.Unlock()
		if scenarios.clients[body.Client] == nil {
			scenarios.clients[body.Client] = map[string]fixtureTorrent{}
		}
		scenarios.clients[body.Client][body.Hash] = body.fixtureTorrent
		scenarios.record(scenarioEvent{Kind: "torrent-state", Client: body.Client, Key: body.Hash, Status: 200})
		w.WriteHeader(http.StatusNoContent)
	})
}

func fixtureSource() map[string]any {
	return map[string]any{"id": scenarioSource, "name": "Scenario Source", "lang": "en", "nsfw": false}
}

// These fixtures model the adapter protocol, not Madokami's HTML parser.
// Real directory parsing remains covered by the JVM fixture/contract tests.
func scenarioSourceResponse(w http.ResponseWriter, r *http.Request, kind string) bool {
	if !scenarios.enabled || r.PathValue("sid") != scenarioSource {
		return false
	}
	switch kind {
	case "search":
		writeJSON(w, []map[string]string{{"key": directoryRoot, "title": "Directory Kingdom"}, {"key": "recovery", "title": "Recovery Chapters"}})
	case "listings":
		out := []map[string]string{}
		if r.URL.Query().Get("manga") == directoryRoot {
			out = []map[string]string{{"key": digitalFolder, "title": "Directory Kingdom"}, {"key": namesFolder, "title": "Directory Kingdom"}, {"key": digitalFolder, "title": "Directory Kingdom"}}
		}
		writeJSON(w, out)
	case "chapters":
		key := r.URL.Query().Get("manga")
		out := []map[string]any{}
		switch key {
		case "recovery":
			for i := 1; i <= 3; i++ {
				out = append(out, map[string]any{"key": fmt.Sprintf("recovery/ch-%d", i), "name": fmt.Sprintf("Chapter %d (Digital)", i), "number": i})
			}
		case directoryRoot, digitalFolder, namesFolder:
			for i := 1; i <= 2; i++ {
				name := fmt.Sprintf("Directory Kingdom v%02d (Scan) (Root)", i)
				if key == digitalFolder {
					name = fmt.Sprintf("Directory Kingdom v%02d (Digital) (1r0n)", i)
				}
				if key == namesFolder {
					name = fmt.Sprintf("Directory Kingdom v%02d (Japanese Names) (Digital) (1r0n, Bingbong)", i)
				}
				out = append(out, map[string]any{"key": fmt.Sprintf("%s/v%d", key, i), "name": name, "number": i})
			}
		}
		writeJSON(w, out)
	case "pages":
		refs := []string{}
		for i := 1; i <= 5; i++ {
			refs = append(refs, fmt.Sprintf("%s/page-%d", r.URL.Query().Get("chapter"), i))
		}
		writeJSON(w, refs)
	case "page":
		ref := r.URL.Query().Get("ref")
		scenarios.Lock()
		scenarios.attempts[ref]++
		n, name := scenarios.attempts[ref], scenarios.name
		status, retryAfter, truncated := 200, "", false
		if strings.HasSuffix(ref, "/page-5") {
			switch name {
			case "page-503":
				if n <= 2 {
					status = 503
				}
			case "rate-limit":
				if n == 1 {
					status = 429
					retryAfter = "2"
				}
			case "long-cooldown":
				if n == 1 {
					status = 429
					retryAfter = "60"
				}
			case "permanent-401":
				status = 401
			case "source-outage":
				status = 503
			case "truncated-page":
				truncated = n <= 2
			}
		}
		scenarios.record(scenarioEvent{Kind: "page", Key: ref, Attempt: n, Status: status, Truncated: truncated, RetryAfter: retryAfter})
		scenarios.Unlock()
		if status != 200 {
			if retryAfter != "" {
				w.Header().Set("Retry-After", retryAfter)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(502) // same gateway envelope as the actual sidecar
			writeJSON(w, map[string]any{"error": fmt.Sprintf("HTTP error %d", status), "upstreamStatus": status, "retryable": status != 401, "retryAfter": retryAfter})
			return true
		}
		w.Header().Set("Content-Type", "image/gif")
		if truncated {
			w.Header().Set("Content-Length", "1024")
		}
		_, _ = w.Write(fixturePage)
	}
	return true
}

var fixturePage = []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\xff\xff\xff\x00\x00\x00!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;")

// A real Torznab feed is needed to exercise Sento's actual search/dispatch
// pipeline. UI route interception would hide mistakes in client routing.
func mountTorznab(mux *http.ServeMux) {
	mux.HandleFunc("GET /torznab/api", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write([]byte(`<caps><searching><search available="yes" supportedParams="q"/></searching><categories><category id="7000" name="Books"><subcat id="7030" name="Comics"/></category></categories></caps>`))
			return
		}
		_, _ = fmt.Fprint(w, `<rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>`)
		for i := 1; i <= 3; i++ {
			title := fmt.Sprintf("%s v%02d (Digital) (Fixture)", firstWords(r.URL.Query().Get("q")), i)
			hash := fmt.Sprintf("%040d", i)
			magnet := "magnet:?" + url.Values{"xt": {"urn:btih:" + hash}, "dn": {title}}.Encode()
			_, _ = fmt.Fprintf(w, `<item><title>%s</title><guid>%s</guid><size>120000000</size><torznab:attr name="infohash" value="%s"/><torznab:attr name="magneturl" value="%s"/><torznab:attr name="seeders" value="20"/><torznab:attr name="category" value="7030"/></item>`, html.EscapeString(title), hash, hash, html.EscapeString(magnet))
		}
		_, _ = fmt.Fprint(w, `</channel></rss>`)
	})
}

func mountTorrentClient(mux *http.ServeMux, prefix, client string) {
	mux.HandleFunc("POST "+prefix+"/api/v2/auth/login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "stub-session"})
		_, _ = w.Write([]byte("Ok."))
	})
	mux.HandleFunc("GET "+prefix+"/api/v2/app/version", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("v4.6.0")) })
	mux.HandleFunc("POST "+prefix+"/api/v2/torrents/add", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", 400)
			return
		}
		magnet, err := url.Parse(strings.TrimSpace(r.Form.Get("urls")))
		if err != nil || magnet.Scheme != "magnet" {
			http.Error(w, "fixture expects a magnet", 400)
			return
		}
		hash := strings.ToLower(strings.TrimPrefix(magnet.Query().Get("xt"), "urn:btih:"))
		if hash == "" {
			http.Error(w, "missing infohash", 400)
			return
		}
		scenarios.Lock()
		defer scenarios.Unlock()
		if scenarios.clients[client] == nil {
			scenarios.clients[client] = map[string]fixtureTorrent{}
		}
		if _, exists := scenarios.clients[client][hash]; !exists {
			scenarios.clients[client][hash] = fixtureTorrent{Torrent: qbittorrent.Torrent{Hash: hash, Name: magnet.Query().Get("dn"), State: "downloading", Progress: 0.2}, Category: r.Form.Get("category")}
		}
		scenarios.record(scenarioEvent{Kind: "torrent-add", Client: client, Key: hash, Category: r.Form.Get("category"), Status: 200})
		_, _ = w.Write([]byte("Ok."))
	})
	mux.HandleFunc("GET "+prefix+"/api/v2/torrents/info", func(w http.ResponseWriter, r *http.Request) {
		scenarios.Lock()
		defer scenarios.Unlock()
		items := []fixtureTorrent{}
		for _, t := range scenarios.clients[client] {
			if cat := r.URL.Query().Get("category"); cat == "" || cat == t.Category {
				items = append(items, t)
			}
		}
		slices.SortFunc(items, func(a, b fixtureTorrent) int { return strings.Compare(a.Hash, b.Hash) })
		writeJSON(w, items)
	})
	mux.HandleFunc("GET "+prefix+"/api/v2/torrents/files", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, []any{}) })
	mux.HandleFunc("POST "+prefix+"/api/v2/torrents/delete", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", 400)
			return
		}
		deleteFiles, _ := strconv.ParseBool(r.Form.Get("deleteFiles"))
		scenarios.Lock()
		defer scenarios.Unlock()
		for _, hash := range strings.Split(r.Form.Get("hashes"), "|") {
			delete(scenarios.clients[client], hash)
			scenarios.record(scenarioEvent{Kind: "torrent-delete", Client: client, Key: hash, DeleteFiles: deleteFiles, Status: 200})
		}
		w.WriteHeader(http.StatusOK)
	})
}
