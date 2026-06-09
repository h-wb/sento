// Package match resolves parsed title guesses to library series
// (parser.md §6): normalization, a title index over all titles/aliases,
// and token-set similarity with Jaro-Winkler per token.
package match

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Thresholds per parser.md §6 [default].
const (
	AutoThreshold   = 0.90
	ManualThreshold = 0.75
)

// Normalize folds a title for comparison: NFKC, lowercase, diacritics
// stripped, punctuation → space, whitespace collapsed, '&' → "and".
func Normalize(s string) string {
	s = norm.NFKC.String(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\'' || r == '’' || r == '`' || r == '´':
			// Drop apostrophes entirely: "journey's" → "journeys", so
			// possessives don't shed stray "s" tokens.
		case r == '&':
			b.WriteString(" and ")
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if r > 127 {
				dec := []rune(norm.NFD.String(string(r)))
				if len(dec) > 0 && dec[0] < 128 {
					r = dec[0]
				}
			}
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

type entry struct {
	seriesID int64
	title    string // normalized
}

// Index maps normalized titles/aliases to series.
type Index struct {
	exact   map[string][]int64
	entries []entry
}

func NewIndex() *Index {
	return &Index{exact: map[string][]int64{}}
}

func (ix *Index) Add(seriesID int64, titles ...string) {
	for _, t := range titles {
		n := Normalize(t)
		if n == "" {
			continue
		}
		ix.exact[n] = append(ix.exact[n], seriesID)
		ix.entries = append(ix.entries, entry{seriesID: seriesID, title: n})
	}
}

type Candidate struct {
	SeriesID int64
	Title    string
	Score    float64
}

// Lookup returns candidates sorted by score desc. Exact normalized matches
// score 1.0; otherwise token-set similarity over all entries. The longest
// title wins ties (containment rule, parser.md §5.9).
func (ix *Index) Lookup(guess string) []Candidate {
	n := Normalize(guess)
	if n == "" {
		return nil
	}
	seen := map[int64]Candidate{}
	for _, id := range ix.exact[n] {
		seen[id] = Candidate{SeriesID: id, Title: n, Score: 1}
	}
	for _, e := range ix.entries {
		score := Similarity(n, e.title)
		if score < ManualThreshold {
			continue
		}
		cur, ok := seen[e.seriesID]
		if !ok || score > cur.Score || (score == cur.Score && len(e.title) > len(cur.Title)) {
			seen[e.seriesID] = Candidate{SeriesID: e.seriesID, Title: e.title, Score: score}
		}
	}
	out := make([]Candidate, 0, len(seen))
	for _, c := range seen {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return len(out[i].Title) > len(out[j].Title)
	})
	return out
}

// Similarity is an order-tolerant token-set score: each token of the
// shorter set is matched to its best Jaro-Winkler counterpart.
func Similarity(a, b string) float64 {
	if a == b {
		return 1
	}
	ta, tb := strings.Fields(a), strings.Fields(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	if len(ta) > len(tb) {
		ta, tb = tb, ta
	}
	used := make([]bool, len(tb))
	var sum float64
	for _, x := range ta {
		best, bestIdx := 0.0, -1
		for i, y := range tb {
			if used[i] {
				continue
			}
			if s := jaroWinkler(x, y); s > best {
				best, bestIdx = s, i
			}
		}
		if bestIdx >= 0 {
			used[bestIdx] = true
		}
		sum += best
	}
	// Average over the longer set: extra tokens dilute the score.
	return sum / float64(len(tb))
}

func jaroWinkler(a, b string) float64 {
	j := jaro(a, b)
	if j < 0.7 {
		return j
	}
	prefix := 0
	for i := 0; i < len(a) && i < len(b) && i < 4; i++ {
		if a[i] != b[i] {
			break
		}
		prefix++
	}
	return j + float64(prefix)*0.1*(1-j)
}

func jaro(a, b string) float64 {
	la, lb := len(a), len(b)
	if la == 0 || lb == 0 {
		return 0
	}
	if a == b {
		return 1
	}
	window := max(la, lb)/2 - 1
	if window < 0 {
		window = 0
	}
	ma := make([]bool, la)
	mb := make([]bool, lb)
	matches := 0
	for i := 0; i < la; i++ {
		lo := max(0, i-window)
		hi := min(lb-1, i+window)
		for j := lo; j <= hi; j++ {
			if !mb[j] && a[i] == b[j] {
				ma[i], mb[j] = true, true
				matches++
				break
			}
		}
	}
	if matches == 0 {
		return 0
	}
	transpositions := 0
	k := 0
	for i := 0; i < la; i++ {
		if !ma[i] {
			continue
		}
		for !mb[k] {
			k++
		}
		if a[i] != b[k] {
			transpositions++
		}
		k++
	}
	m := float64(matches)
	return (m/float64(la) + m/float64(lb) + (m-float64(transpositions)/2)/m) / 3
}
