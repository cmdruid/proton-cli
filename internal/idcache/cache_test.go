package idcache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	// Add one more - head should be pruned.
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
