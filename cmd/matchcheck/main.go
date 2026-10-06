//go:build matchcheck

// Command matchcheck pulls a broad pool of real manga titles from the
// running sento-extensions sidecar, builds a match.Index treating each distinct
// title as a library series, and stress-tests internal/match:
//
//   - collisions: distinct real series that score >= AutoThreshold against
//     each other (a wrong auto-grab risk);
//   - self-match: every title must resolve to itself at 1.0;
//   - round-trip: blind-parse "<title> v05" and confirm the guess still
//     matches the right series (flags blind-parse title hazards).
//
// Manual tool — never built by CI.
//
//	go run -tags matchcheck ./cmd/matchcheck 2> /tmp/matchcheck.log
package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/h-wb/sento/internal/extensions"
	"github.com/h-wb/sento/internal/match"
	"github.com/h-wb/sento/internal/parse"
)

const hostURL = "http://localhost:8901"

// Common words that appear across many distinct titles — maximizes the
// diversity (and near-collisions) of the harvested pool.
var queries = []string{
	"love", "war", "school", "demon", "magic", "world", "king", "death",
	"dragon", "hero", "girl", "boy", "dark", "blood", "sword", "star",
	"moon", "night", "blue", "devil", "saint", "knight", "ghost", "wolf",
	"one piece", "solo", "kaiju", "frieren", "dandadan", "gachiakuta",
}

func main() {
	c := extensions.NewClient(hostURL)
	ctx := context.Background()

	srcs, err := c.Sources(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sources:", err)
		os.Exit(1)
	}
	// One reliable MangaDex source per language + Weeb Central.
	wantLang := map[string]bool{"en": true, "es": true, "fr": true, "ja": true, "it": true, "pt-BR": true}
	var pick []extensions.Source
	seen := map[string]bool{}
	for _, s := range srcs {
		if s.Name == "Weeb Central" {
			pick = append(pick, s)
			continue
		}
		if s.Name == "MangaDex" && wantLang[s.Lang] && !seen[s.Lang] {
			seen[s.Lang] = true
			pick = append(pick, s)
		}
	}
	fmt.Fprintf(os.Stderr, "harvesting titles from %d sources\n", len(pick))

	// Collect a pool of distinct titles.
	var mu sync.Mutex
	pool := map[string]string{} // normalized -> original
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for _, s := range pick {
		for _, q := range queries {
			s, q := s, q
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				sctx, cancel := context.WithTimeout(ctx, 15*time.Second)
				defer cancel()
				ms, err := c.Search(sctx, s.ID, q)
				if err != nil {
					return
				}
				mu.Lock()
				for _, m := range ms {
					n := match.Normalize(m.Title)
					if n != "" && len(strings.Fields(n)) >= 2 { // skip 1-token noise
						pool[n] = m.Title
					}
				}
				mu.Unlock()
			}()
		}
	}
	wg.Wait()

	// Build the index: each distinct normalized title is a series.
	titles := make([]string, 0, len(pool))
	for n := range pool {
		titles = append(titles, n)
	}
	sort.Strings(titles)
	ix := match.NewIndex()
	for i, n := range titles {
		ix.Add(int64(i), pool[n])
	}
	fmt.Fprintf(os.Stderr, "pool: %d distinct titles\n\n", len(titles))

	// 1) Collisions: distinct series scoring >= AutoThreshold.
	type collision struct {
		a, b  string
		score float64
	}
	var collisions []collision
	selfFail, rtFail, rtNumbered := 0, 0, 0
	for i, n := range titles {
		for _, cand := range ix.Lookup(pool[n]) {
			if cand.SeriesID == int64(i) {
				continue
			}
			if cand.Score >= match.AutoThreshold {
				if int(cand.SeriesID) > i { // report each pair once
					collisions = append(collisions, collision{pool[n], pool[titles[cand.SeriesID]], cand.Score})
				}
			}
		}
		// 2) Self-match must be exact.
		top := ix.Lookup(pool[n])
		if len(top) == 0 || top[0].SeriesID != int64(i) || top[0].Score < 1 {
			selfFail++
		}
		// 3) Round-trip: blind-parse "<title> v05" → match back.
		guess := parse.Parse(pool[n]+" v05", parse.ChannelTorrent).Title
		hit := false
		for _, cand := range ix.Lookup(guess) {
			if cand.SeriesID == int64(i) && cand.Score >= match.ManualThreshold {
				hit = true
				break
			}
		}
		if !hit {
			rtFail++
			hasNum := strings.ContainsAny(n, "0123456789")
			if hasNum {
				rtNumbered++
			}
			if !hasNum && rtFail-rtNumbered <= 12 { // sample non-numbered failures
				fmt.Fprintf(os.Stderr, "  round-trip miss: %-40q -> guess %q\n", pool[n], guess)
			}
		}
	}

	sort.Slice(collisions, func(i, j int) bool { return collisions[i].score > collisions[j].score })
	fmt.Fprintf(os.Stderr, "\n=== collisions (distinct titles >= AutoThreshold %.2f): %d ===\n", match.AutoThreshold, len(collisions))
	for i, cl := range collisions {
		if i >= 30 {
			fmt.Fprintf(os.Stderr, "  ... and %d more\n", len(collisions)-30)
			break
		}
		fmt.Fprintf(os.Stderr, "  %.3f  %q  ==  %q\n", cl.score, cl.a, cl.b)
	}
	fmt.Fprintf(os.Stderr, "\nself-match failures: %d / %d\n", selfFail, len(titles))
	fmt.Fprintf(os.Stderr, "round-trip misses: %d (%d on numbered titles = known blind-parse limit)\n", rtFail, rtNumbered)
}
