package parse

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// Num is a fixed-point chapter/volume number in hundredths: 34.5 → 3450.
// Alpha sub-chapters map a→+10, b→+20 (Mihon convention: 4a ≈ 4.1).
type Num int64

// MarshalJSON renders plain books (3450 → 34.5) for corpus readability.
func (n Num) MarshalJSON() ([]byte, error) {
	whole := int64(n) / 100
	frac := int64(n) % 100
	if frac == 0 {
		return []byte(strconv.FormatInt(whole, 10)), nil
	}
	s := strconv.FormatFloat(float64(n)/100, 'f', -1, 64)
	return []byte(s), nil
}

func (n *Num) UnmarshalJSON(b []byte) error {
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	*n = Num(f*100 + 0.5)
	return nil
}

// parseNum converts "34", "034", "34.5" to a Num. Returns false on overflow
// or empty input.
func parseNum(s string) (Num, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	whole, frac, _ := strings.Cut(s, ".")
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || w > 1<<40 {
		return 0, false
	}
	n := Num(w * 100)
	if frac != "" {
		if len(frac) > 2 {
			frac = frac[:2]
		}
		f, err := strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, false
		}
		if len(frac) == 1 {
			f *= 10
		}
		n += Num(f)
	}
	return n, true
}

// alphaSuffixNum maps a single alpha sub-chapter suffix: a→10, b→20 … i→90.
func alphaSuffixNum(c byte) Num {
	if c < 'a' || c > 'i' {
		return 0
	}
	return Num(int64(c-'a'+1) * 10)
}

// Range is an inclusive span of Nums; a single number is From == To.
// JSON form is the pair [from,to] (corpus readability).
type Range struct{ From, To Num }

func (r Range) MarshalJSON() ([]byte, error) {
	from, _ := r.From.MarshalJSON()
	to, _ := r.To.MarshalJSON()
	return []byte("[" + string(from) + "," + string(to) + "]"), nil
}

func (r *Range) UnmarshalJSON(b []byte) error {
	var pair [2]Num
	if err := json.Unmarshal(b, &pair); err != nil {
		return err
	}
	r.From, r.To = pair[0], pair[1]
	return nil
}

// mergeRanges returns rs sorted by start, with overlapping or near-adjacent
// ranges coalesced — "near" meaning within one whole book, so [1,2], [3,3]
// and [4,5] all fuse into [1,5]. Distinct fractional books never fill gaps.
// Coalescing is transitive: because the input
// is sorted first, a range that bridges two others merges all three (the old
// unsorted single-pass version left them split).
func mergeRanges(rs []Range) []Range {
	if len(rs) < 2 {
		return rs
	}
	sorted := make([]Range, len(rs))
	copy(sorted, rs)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].From != sorted[j].From {
			return sorted[i].From < sorted[j].From
		}
		return sorted[i].To < sorted[j].To
	})
	out := make([]Range, 0, len(sorted))
	out = append(out, sorted[0])
	for _, r := range sorted[1:] {
		last := &out[len(out)-1]
		if r.From <= last.To || (r.From%100 == 0 && last.To%100 == 0 && r.From == last.To+100) { // overlap or consecutive whole books
			if r.To > last.To {
				last.To = r.To
			}
		} else {
			out = append(out, r)
		}
	}
	return out
}
