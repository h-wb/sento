package provider

import "testing"

func TestCategoryLanguage(t *testing.T) {
	names := map[int]string{
		7000: "Books", 1: "Literature - English-translated", 2: "Literature - Non-English-translated",
		3: "Literature - Raw", 4: "BDs & Comics & Manga",
	}
	for _, tc := range []struct {
		name string
		ids  []int
		want string
	}{
		{"english section", []int{7000, 1}, "en"},
		{"raw section", []int{7000, 3}, "ja"},
		{"non-english says nothing", []int{7000, 2}, ""},
		{"a plain section says nothing", []int{7000, 4}, ""},
		{"disagreeing sections say nothing", []int{1, 3}, ""},
		{"unknown ids", []int{999}, ""},
	} {
		if got := categoryLanguage(tc.ids, names); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
