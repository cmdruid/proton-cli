package idcache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func newTestCache(t *testing.T) *Cache {
	t.Helper()
	return New(filepath.Join(t.TempDir(), "cache.json"))
}

func TestSaveLoadRoundTrip(t *testing.T) {
	c := newTestCache(t)
	if err := c.Save("a", "b", "c"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := c.load()
	want := []string{"a", "b", "c"}
	if !equalSlice(got, want) {
		t.Errorf("load: got %v, want %v", got, want)
	}
}

func TestSaveDedupes(t *testing.T) {
	c := newTestCache(t)
	if err := c.Save("a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := c.Save("b", "c", "a"); err != nil {
		t.Fatal(err)
	}
	got := c.load()
	want := []string{"a", "b", "c"}
	if !equalSlice(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSaveDropsEmpty(t *testing.T) {
	c := newTestCache(t)
	if err := c.Save("", "a", ""); err != nil {
		t.Fatal(err)
	}
	got := c.load()
	if !equalSlice(got, []string{"a"}) {
		t.Errorf("empty IDs not dropped: %v", got)
	}
}

func TestSavePrunesAtMaxEntries(t *testing.T) {
	c := newTestCache(t)
	// Seed the cache with MaxEntries items.
	seed := make([]string, MaxEntries)
	for i := range seed {
		seed[i] = fmt.Sprintf("seed-%05d", i)
	}
	if err := c.Save(seed...); err != nil {
		t.Fatal(err)
	}
	if got := len(c.load()); got != MaxEntries {
		t.Fatalf("expected %d entries after seed, got %d", MaxEntries, got)
	}
	// Add one more — head should be pruned.
	if err := c.Save("new-entry"); err != nil {
		t.Fatal(err)
	}
	loaded := c.load()
	if got := len(loaded); got != MaxEntries {
		t.Errorf("expected %d entries after prune, got %d", MaxEntries, got)
	}
	if loaded[0] == "seed-00000" {
		t.Errorf("oldest entry should have been pruned; head=%q", loaded[0])
	}
	if loaded[len(loaded)-1] != "new-entry" {
		t.Errorf("newest entry not at tail; tail=%q", loaded[len(loaded)-1])
	}
}

func TestResolveExactMatch(t *testing.T) {
	c := newTestCache(t)
	_ = c.Save("abc12345xyz", "def67890qwerty")
	got, err := c.Resolve("abc12345")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "abc12345xyz" {
		t.Errorf("got %q, want abc12345xyz", got)
	}
}

func TestResolveNotFound(t *testing.T) {
	c := newTestCache(t)
	_ = c.Save("abc12345xyz")
	_, err := c.Resolve("nope")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestResolveAmbiguous(t *testing.T) {
	c := newTestCache(t)
	_ = c.Save("abc12345first", "abc12345second", "def67890other")
	_, err := c.Resolve("abc12345")
	var amb *AmbiguousError
	if !errors.As(err, &amb) {
		t.Fatalf("expected *AmbiguousError, got %v", err)
	}
	if amb.Prefix != "abc12345" {
		t.Errorf("Prefix = %q, want abc12345", amb.Prefix)
	}
	sort.Strings(amb.Candidates)
	want := []string{"abc12345first", "abc12345second"}
	if !equalSlice(amb.Candidates, want) {
		t.Errorf("Candidates = %v, want %v", amb.Candidates, want)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	c := newTestCache(t)
	if got := c.load(); len(got) != 0 {
		t.Errorf("expected empty slice on missing file, got %v", got)
	}
}

func TestLoadCorruptFileIsEmpty(t *testing.T) {
	c := newTestCache(t)
	if err := os.MkdirAll(filepath.Dir(c.path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := c.load(); len(got) != 0 {
		t.Errorf("expected empty slice on corrupt file, got %v", got)
	}
	// Save should still succeed and overwrite.
	if err := c.Save("a"); err != nil {
		t.Errorf("Save on corrupt file failed: %v", err)
	}
	if got := c.load(); !equalSlice(got, []string{"a"}) {
		t.Errorf("Save did not overwrite corrupt file: %v", got)
	}
}

func TestSaveAtomicWrite(t *testing.T) {
	// Verify the on-disk format is a JSON array of strings (so external
	// tools / future migrations can read it directly).
	c := newTestCache(t)
	_ = c.Save("a", "b", "c")
	data, err := os.ReadFile(c.path)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("on-disk format is not a JSON array: %v\n%s", err, string(data))
	}
	if !equalSlice(got, []string{"a", "b", "c"}) {
		t.Errorf("got %v, want [a b c]", got)
	}
}

func TestClear(t *testing.T) {
	c := newTestCache(t)
	_ = c.Save("a")
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if got := c.load(); len(got) != 0 {
		t.Errorf("expected empty after Clear, got %v", got)
	}
	// Clearing a missing file is fine.
	if err := c.Clear(); err != nil {
		t.Errorf("Clear on missing file errored: %v", err)
	}
}

func TestIsFullID(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{strings.Repeat("a", 86) + "==", true},
		{strings.Repeat("a", 88) + "==", true},
		{strings.Repeat("a", 58) + "==", true},  // exactly 60 chars (boundary, accepted)
		{strings.Repeat("a", 57) + "==", false}, // 59 chars: too short
		{strings.Repeat("a", 88), false},        // missing ==
		{"", false},
		{"some search term that is long enough though", false}, // no ==
	}
	for _, tc := range tests {
		t.Run(tc.in[:min(len(tc.in), 12)], func(t *testing.T) {
			if got := IsFullID(tc.in); got != tc.want {
				t.Errorf("IsFullID(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsShortID(t *testing.T) {
	// IsShortID is a loose shape check: any base64-charset string of the
	// right length matches. Disambiguation between "real ID prefix" and
	// "search term" happens at ResolvePrefix time — cache miss falls
	// through to the caller, ambiguous cache hits exit 4.
	tests := []struct {
		name string
		in   string
		want bool
	}{
		// Real Proton ID prefixes.
		{"8-char base64 mixed case", "AbC12345", true},
		{"realistic 8-char prefix", "NWM5AYGx", true},
		{"all lowercase 8 chars", "abc12345", true},
		{"30-char base64 with hyphen", "abc12345_xy7zSTUFF-base64body", true},
		// Search-term shapes that also match — ResolvePrefix handles them
		// via cache-miss fallthrough.
		{"capitalized name", "Personal", true},
		{"test fixture name", "proton-cli-test-1234-5678-ref", true},
		{"hyphenated lowercase name", "john-doe-2026", true},
		// Negatives by length / shape.
		{"7 chars too short", "AbC1234", false},
		{"60 chars too long for short", strings.Repeat("A", 60), false},
		{"ends with ==", "abc12345xyz==", false},
		{"contains space", "abc 1234", false},
		{"contains slash (not URL-safe)", "abc/1234", false},
		{"contains plus (not URL-safe)", "abc+1234", false},
		{"empty", "", false},
		{"all underscores", "________", true},
		{"all hyphens", "--------", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsShortID(tc.in); got != tc.want {
				t.Errorf("IsShortID(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestFullAndShortMutuallyExclusive(t *testing.T) {
	cases := []string{
		strings.Repeat("a", 88) + "==",
		"abc12345",
		strings.Repeat("a", 30),
		"",
		"hi",
	}
	for _, s := range cases {
		if IsFullID(s) && IsShortID(s) {
			t.Errorf("both predicates matched %q", s)
		}
	}
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
