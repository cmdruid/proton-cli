// Package idcache stores the set of full Proton IDs the user has seen
// recently, so that interactive listings can show the shortened form of an ID
// and the user can paste that back into any command that takes an ID. What is
// stored is always the whole ID; internal/ref owns both how it is shortened and
// how the short form is matched against it.
//
// The cache is a per-profile flat JSON array of full IDs at
// ~/.config/proton-cli/idcache/<profile>.json. Writes are atomic
// (write to tmp, rename) and best-effort: a missing or corrupt cache
// reads as empty without surfacing an error to the caller.
package idcache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/roman-16/proton-cli/internal/ref"
)

// MaxEntries caps the cache size. Older entries are pruned FIFO on write.
const MaxEntries = 10000

// ErrNotFound is returned by Resolve when no cached ID has the given prefix.
var ErrNotFound = errors.New("idcache: prefix not found")

// AmbiguousError is returned by Resolve when two or more cached IDs share
// the same prefix.
type AmbiguousError struct {
	Prefix     string
	Candidates []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("idcache: %d IDs match prefix %q", len(e.Candidates), e.Prefix)
}

// Cache is the per-profile ID cache. The zero value is unsafe; use New.
type Cache struct {
	path string
	mu   sync.Mutex
}

// New constructs a Cache backed by the given file path. Parent directories
// are created on first write.
func New(path string) *Cache { return &Cache{path: path} }

func (c *Cache) Path() string { return c.path }

// Save merges ids into the cache, dedupes (preserving insertion order),
// FIFO-prunes to MaxEntries, and atomically rewrites the file.
//
// Empty calls are a no-op. Failures are returned but ignored by callers in
// list-command paths so a cache hiccup never fails a user-visible command.
func (c *Cache) Save(ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	existing := c.load()
	seen := make(map[string]struct{}, len(existing)+len(ids))
	out := make([]string, 0, len(existing)+len(ids))
	for _, id := range existing {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) > MaxEntries {
		out = out[len(out)-MaxEntries:]
	}
	return c.writeAtomic(out)
}

// Resolve returns the full ID a short one names. Returns ErrNotFound on miss,
// *AmbiguousError when several cached IDs answer to the same short form.
func (c *Cache) Resolve(prefix string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ids := c.load()
	var hits []string
	for _, id := range ids {
		if ref.Matches(id, prefix) {
			hits = append(hits, id)
		}
	}
	switch len(hits) {
	case 0:
		return "", ErrNotFound
	case 1:
		return hits[0], nil
	default:
		return "", &AmbiguousError{Prefix: prefix, Candidates: hits}
	}
}

// Clear removes the cache file. Used by tests; surfaced for a future
// `proton cache clear` command.
func (c *Cache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := os.Remove(c.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// load reads the cache from disk. Missing or malformed file → empty slice,
// no error (cache is best-effort).
func (c *Cache) load() []string {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return nil
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil
	}
	return ids
}

// writeAtomic writes ids to a tmp file in the same directory, then renames
// over the target. A crash mid-write leaves the previous file intact.
func (c *Cache) writeAtomic(ids []string) error {
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".cache-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	enc := json.NewEncoder(tmp)
	if err := enc.Encode(ids); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0600); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, c.path)
}
