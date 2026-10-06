//go:build harvest

// Command harvest pulls real chapter names from the running sento-extensions
// sidecar and runs them through internal/parse, flagging cases worth
// turning into corpus fixtures. Manual tool — never built by CI.
//
//	go run -tags harvest ./cmd/harvest > /tmp/harvest.jsonl 2> /tmp/harvest.log
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/h-wb/sento/internal/extensions"
	"github.com/h-wb/sento/internal/parse"
)

const hostURL = "http://localhost:8901"

// Well-known titles likely to exist across languages, chosen to exercise
// markers, decimals, specials and long runs.
var queries = []string{
	"one piece", "berserk", "naruto", "solo leveling", "chainsaw man",
	"vinland saga", "jujutsu kaisen", "kingdom", "vagabond", "bleach",
}

type record struct {
	Source     string   `json:"source"`
	Lang       string   `json:"lang"`
	Title      string   `json:"title"`
	Query      string   `json:"query"`
	Number     float64  `json:"number"`
	Name       string   `json:"name"`
	Chapters   [][2]int `json:"chapters"`
	Volumes    [][2]int `json:"volumes"`
	Extra      bool     `json:"extra,omitempty"`
	Confidence float64  `json:"confidence"`
	Flags      []string `json:"flags,omitempty"`
}

func rngs(rs []parse.Range) [][2]int {
	out := make([][2]int, 0, len(rs))
	for _, r := range rs {
		out = append(out, [2]int{int(r.From), int(r.To)})
	}
	return out
}

func main() {
	c := extensions.NewClient(hostURL)
	ctx := context.Background()

	srcs, err := c.Sources(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sources:", err)
		os.Exit(1)
	}
	// One source per (name,lang) to spread coverage; skip nsfw.
	seen := map[string]bool{}
	var pick []extensions.Source
	for _, s := range srcs {
		if s.NSFW {
			continue
		}
		k := s.Name + "|" + s.Lang
		if seen[k] {
			continue
		}
		seen[k] = true
		pick = append(pick, s)
	}
	fmt.Fprintf(os.Stderr, "harvesting %d sources\n", len(pick))

	var (
		mu   sync.Mutex
		recs []record
		sem  = make(chan struct{}, 6)
		wg   sync.WaitGroup
	)
	emit := func(r record) {
		mu.Lock()
		recs = append(recs, r)
		mu.Unlock()
	}

	for _, s := range pick {
		s := s
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			for _, q := range queries {
				sctx, cancel := context.WithTimeout(ctx, 15*time.Second)
				mangas, err := c.Search(sctx, s.ID, q)
				cancel()
				if err != nil || len(mangas) == 0 {
					continue
				}
				m := mangas[0] // best match only, keep it light
				cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
				chs, err := c.Chapters(cctx, s.ID, m.Key)
				cancel()
				if err != nil {
					continue
				}
				if len(chs) > 40 {
					chs = chs[:40] // cap per manga
				}
				for _, ch := range chs {
					if strings.TrimSpace(ch.Name) == "" {
						continue
					}
					res := parse.ParseAnchored(ch.Name, parse.ChannelSource, []string{m.Title})
					rec := record{
						Source: s.Name, Lang: s.Lang, Title: m.Title, Query: q,
						Number: ch.Number, Name: ch.Name,
						Chapters: rngs(res.Chapters), Volumes: rngs(res.Volumes),
						Extra: res.Extra, Confidence: res.Confidence,
					}
					rec.Flags = flagsFor(res, ch.Number)
					emit(rec)
				}
				break // one matched manga per source is enough
			}
		}()
	}
	wg.Wait()

	sort.Slice(recs, func(i, j int) bool {
		if recs[i].Source != recs[j].Source {
			return recs[i].Source < recs[j].Source
		}
		return recs[i].Name < recs[j].Name
	})
	enc := json.NewEncoder(os.Stdout)
	buckets := map[string]int{}
	for _, r := range recs {
		_ = enc.Encode(r)
		for _, f := range r.Flags {
			buckets[f]++
		}
	}
	fmt.Fprintf(os.Stderr, "records: %d\n", len(recs))
	for f, n := range buckets {
		fmt.Fprintf(os.Stderr, "  flag %-22s %d\n", f, n)
	}
}

// flagsFor marks records worth a human look: parse disagreeing with the
// source's own number, or structurally interesting names.
func flagsFor(res parse.Result, num float64) []string {
	var f []string
	src := int(num*100 + 0.5) // source number in hundredths
	single := len(res.Chapters) == 1 && res.Chapters[0].From == res.Chapters[0].To

	if num > 0 && single && int(res.Chapters[0].From) != src {
		f = append(f, "number-mismatch")
	}
	if num > 0 && len(res.Chapters) == 0 && len(res.Volumes) == 0 {
		f = append(f, "parsed-nothing")
	}
	if len(res.Volumes) > 0 {
		f = append(f, "volume-on-source")
	}
	for _, r := range res.Chapters {
		if r.From%100 != 0 || r.To%100 != 0 {
			f = append(f, "decimal-chapter")
			break
		}
		if r.From != r.To {
			f = append(f, "range")
			break
		}
	}
	if res.Extra {
		f = append(f, "extra")
	}
	if res.Confidence < 0.7 {
		f = append(f, "low-confidence")
	}
	return f
}
