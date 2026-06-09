package parse

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The regression corpus (parser.md §8.1): JSONL files, one case per line,
// partial expectations — only fields present in "want" are compared.
// CI gates on 100%: a failing line is a build failure.

type corpusCase struct {
	In     string          `json:"in"`
	Ch     Channel         `json:"ch"`
	Titles []string        `json:"titles"`
	Want   json.RawMessage `json:"want"`
}

func TestCorpus(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "corpus", "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no corpus files found")
	}
	total := 0
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			f, err := os.Open(file)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()

			sc := bufio.NewScanner(f)
			line := 0
			for sc.Scan() {
				line++
				raw := sc.Bytes()
				if len(raw) == 0 || raw[0] == '#' {
					continue
				}
				var c corpusCase
				if err := json.Unmarshal(raw, &c); err != nil {
					t.Fatalf("%s:%d: invalid case: %v", file, line, err)
				}
				total++

				var res Result
				if len(c.Titles) > 0 {
					res = ParseAnchored(c.In, c.Ch, c.Titles)
				} else {
					res = Parse(c.In, c.Ch)
				}

				gotJSON, err := json.Marshal(res)
				if err != nil {
					t.Fatalf("%s:%d: marshal: %v", file, line, err)
				}
				var got, want map[string]any
				if err := json.Unmarshal(gotJSON, &got); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(c.Want, &want); err != nil {
					t.Fatalf("%s:%d: invalid want: %v", file, line, err)
				}

				if diff := partialDiff(want, got, ""); diff != "" {
					t.Errorf("%s:%d: %q\n%s\nfull parse: %s", file, line, c.In, diff, gotJSON)
				}
			}
			if err := sc.Err(); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Logf("corpus: %d cases", total)
}

// partialDiff verifies every key present in want against got, recursively
// for objects; arrays and scalars compare exactly.
func partialDiff(want, got any, path string) string {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return fmt.Sprintf("  %s: want object, got %T (%v)", path, got, got)
		}
		out := ""
		for k, wv := range w {
			out += partialDiff(wv, g[k], path+"."+k)
		}
		return out
	default:
		// omitempty: an absent field equals its zero value.
		if got == nil && (want == false || want == 0.0 || want == "") {
			return ""
		}
		if !reflect.DeepEqual(want, got) {
			return fmt.Sprintf("  %s: want %v, got %v\n", path, want, got)
		}
		return ""
	}
}

func FuzzParse(f *testing.F) {
	seeds := []string{
		"One Piece v001-111 + 1134-1176 (2003-2026) (Digital) (1r0n)",
		"[Kamizye] Berserk - 375",
		"Vol.5 Ch.34: The Sword Master",
		"((((((", "[[[[", "第巻", "v999999999999999999999",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		for _, ch := range []Channel{ChannelTorrent, ChannelSource, ChannelFile} {
			res := Parse(name, ch)
			for _, r := range append(res.Volumes, res.Chapters...) {
				if r.From > r.To {
					t.Fatalf("invalid range %v in %q", r, name)
				}
				if r.From < 0 {
					t.Fatalf("negative number in %q", name)
				}
			}
		}
	})
}
