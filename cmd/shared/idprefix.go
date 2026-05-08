package shared

import (
	"errors"
	"fmt"
	"strings"

	"github.com/roman-16/proton-cli/internal/app"
	"github.com/roman-16/proton-cli/internal/idcache"
)

// ResolvePrefix expands an 8+-character short-ID prefix to its full ID by
// consulting the local cache. Returns the input unchanged when:
//
//   - It is already a full Proton ID (60+ chars, ends "==").
//   - It is not short-ID-shaped (e.g. contains a space).
//   - It is short-ID-shaped but not in the cache (cache miss — the
//     input may be a search term, vault name, contact name, etc.; we
//     hand it back to the caller so service-layer keyword-search can
//     try to resolve it).
//
// Returns an *app.ExitError only when the prefix is AMBIGUOUS in the
// cache (two or more full IDs share the prefix); the user must then
// disambiguate with a longer prefix or `--full-ids`.
//
// Apply this to every cmd-layer arg that may carry a Proton ID. It is
// a no-op for non-ID and unknown-prefix inputs and so can be called
// uniformly.
func ResolvePrefix(a *app.App, ref string) (string, error) {
	if ref == "" || idcache.IsFullID(ref) || !idcache.IsShortID(ref) {
		return ref, nil
	}
	if a == nil || a.IDCache == nil {
		return ref, nil
	}
	full, err := a.IDCache.Resolve(ref)
	if err == nil {
		return full, nil
	}
	var amb *idcache.AmbiguousError
	if errors.As(err, &amb) {
		return "", app.Exit(4, fmt.Errorf(
			"ambiguous: %d IDs match prefix %q:\n  %s",
			len(amb.Candidates), ref, strings.Join(amb.Candidates, "\n  ")))
	}
	if errors.Is(err, idcache.ErrNotFound) {
		// Cache miss: pass the input through. The service layer's
		// Resolve / search-by-keyword path may still match it as a
		// title, name, vault label, etc. If nothing matches there
		// either, the user gets a clean "no X matching Y" error
		// (exit 3) from the service layer.
		return ref, nil
	}
	// Unexpected error (e.g. file-system) — surface unwrapped.
	return "", err
}

// ResolvePrefixes runs ResolvePrefix on each of refs and returns the
// expanded slice. Stops at the first error.
func ResolvePrefixes(a *app.App, refs []string) ([]string, error) {
	out := make([]string, len(refs))
	for i, r := range refs {
		full, err := ResolvePrefix(a, r)
		if err != nil {
			return nil, err
		}
		out[i] = full
	}
	return out, nil
}
