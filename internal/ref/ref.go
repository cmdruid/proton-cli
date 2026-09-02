// Package ref resolves a user-supplied reference (a search term that matched
// some candidates) to a single resource, producing the CLI's standard typed
// NotFound (exit 3) and Ambiguous (exit 4) errors. Centralizing this guarantees
// every service reports the same shapes for "no match" and "too many matches".
package ref

import "github.com/cmdruid/proton-cli/internal/errs"

// Pick selects the single match for r among matches. It returns:
//
//   - the sole match and nil, when exactly one matched;
//   - the zero value and *errs.NotFound, when none matched;
//   - the zero value and *errs.Ambiguous (with a candidate list built from
//     id/label), when more than one matched.
//
// kind names the resource ("message", "contact", …) for the error text. id and
// label extract each candidate's full ID and a human-readable hint.
func Pick[T any](kind, r string, matches []T, id func(T) string, label func(T) string) (T, error) {
	var zero T
	switch len(matches) {
	case 0:
		return zero, &errs.NotFound{Kind: kind, Ref: r}
	case 1:
		return matches[0], nil
	default:
		cands := make([]errs.Candidate, 0, len(matches))
		for _, m := range matches {
			cands = append(cands, errs.Candidate{ID: id(m), Label: label(m)})
		}
		return zero, &errs.Ambiguous{Kind: kind, Ref: r, Candidates: cands}
	}
}
