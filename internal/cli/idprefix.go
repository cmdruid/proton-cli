package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/roman-16/proton-cli/internal/app"
	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/idcache"
)

// resolvePrefix expands an 8+-char short-ID prefix to its full ID via the
// local cache. It returns the input unchanged when it is already a full ID,
// not short-ID-shaped, or a cache miss (the input may be a search term the
// service layer will resolve). An ambiguous prefix exits 4.
func resolvePrefix(a *app.App, ref string) (string, error) {
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
		return "", errs.WithExit(4, fmt.Errorf(
			"ambiguous: %d IDs match prefix %q:\n  %s",
			len(amb.Candidates), ref, strings.Join(amb.Candidates, "\n  ")))
	}
	if errors.Is(err, idcache.ErrNotFound) {
		return ref, nil
	}
	return "", err
}

// resolvePrefixes runs resolvePrefix on each ref, stopping at the first error.
func resolvePrefixes(a *app.App, refs []string) ([]string, error) {
	out := make([]string, len(refs))
	for i, r := range refs {
		full, err := resolvePrefix(a, r)
		if err != nil {
			return nil, err
		}
		out[i] = full
	}
	return out, nil
}
