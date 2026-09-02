package kit

import (
	"context"
	"strings"

	"github.com/cmdruid/proton-cli/internal/errs"
	"github.com/cmdruid/proton-cli/internal/ui"
)

// Lookup resolves references against a collection small enough to fetch whole:
// labels, filters, calendars, vaults, albums.
//
// It exists because a reference is a promise. Wherever the tree says REF, the
// CLI undertakes to accept a full ID, a short ID, or something human - and a
// command that passes its arguments straight to an endpoint keeps only the
// first third of that. Lookup is how the other two thirds are kept without
// every collection inventing its own resolver.
//
// The collection is loaded on first use and kept, so naming three labels costs
// one request rather than three. A handle matches case-insensitively; several
// matches is an error rather than a guess, which is the promise Expand already
// makes for a short ID.
type Lookup[T any] struct {
	// Kind is the singular noun for what is being looked for, used in errors.
	Kind string
	// Load fetches the whole collection.
	Load func(context.Context) ([]T, error)
	// ID is the reference a mutation acts on.
	ID func(T) string
	// Handle is the name a person would use. Leave it nil for a collection
	// whose members have no name of their own, such as photos.
	Handle func(T) string

	rows   []T
	loaded bool
}

// Find resolves one reference to its row.
func (l *Lookup[T]) Find(ctx context.Context, ref string) (T, error) {
	var zero T
	if err := l.load(ctx); err != nil {
		return zero, err
	}
	for _, row := range l.rows {
		if l.ID(row) == ref {
			return row, nil
		}
	}
	if l.Handle == nil {
		return zero, &errs.NotFound{Kind: l.Kind, Ref: ref}
	}

	var matches []T
	for _, row := range l.rows {
		if strings.EqualFold(strings.TrimSpace(l.Handle(row)), strings.TrimSpace(ref)) {
			matches = append(matches, row)
		}
	}
	switch len(matches) {
	case 0:
		return zero, &errs.NotFound{Kind: l.Kind, Ref: ref}
	case 1:
		return matches[0], nil
	}
	candidates := make([]errs.Candidate, 0, len(matches))
	for _, row := range matches {
		candidates = append(candidates, errs.Candidate{ID: l.ID(row), Label: l.Handle(row)})
	}
	return zero, &errs.Ambiguous{Kind: l.Kind, Ref: ref, Candidates: candidates}
}

// Rows returns the whole collection, loading it if it has not been loaded.
func (l *Lookup[T]) Rows(ctx context.Context) ([]T, error) {
	if err := l.load(ctx); err != nil {
		return nil, err
	}
	return l.rows, nil
}

func (l *Lookup[T]) load(ctx context.Context) error {
	if l.loaded {
		return nil
	}
	rows, err := l.Load(ctx)
	if err != nil {
		return err
	}
	l.rows, l.loaded = rows, true
	return nil
}

// SelectFrom resolves the command's references against a small collection and
// returns them as a selection, so the mutation that follows can show what it is
// about to touch.
//
// It is Select for the collections that have no filters: the references are all
// there is to resolve, but they still have to become rows, because a count is
// not something a person can approve.
func SelectFrom[T any](c *Invocation, noun string, cols []ui.Column[T], l *Lookup[T]) (Selection[T], error) {
	return Select(c, Selector[T]{
		Noun:    noun,
		Columns: cols,
		IDOf:    l.ID,
		ByRef:   l.Find,
	})
}
