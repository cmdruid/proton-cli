package kit

import (
	"context"
	"strings"

	"github.com/roman-16/proton-cli/internal/ui"
)

// One selection model, for every collection.
//
// Bulk commands answer the same question everywhere: which things? The answer is
// always the union of what the user named explicitly and what their filters
// matched, capped, and previewable. Mail, Drive and Pass share this one
// implementation, parameterised by what differs between them.

// Selector tells Select how a particular collection is addressed.
type Selector[T any] struct {
	// Noun is the collection's plural name, used in wording and in the preview.
	Noun string
	// Columns render the preview a dry run shows.
	Columns []ui.Column[T]
	// IDOf extracts the reference a mutation acts on. For a compound key, return
	// the joined form.
	IDOf func(T) string
	// ByRef resolves one explicit reference to a row.
	ByRef func(context.Context, string) (T, error)
	// ByFilter returns everything the filters matched. Leave it nil when the
	// user set no filters; that is how Select knows.
	ByFilter func(context.Context) ([]T, error)
	// FilterHint completes "pass a REF, or a filter such as ...", so the error a
	// user sees names the filters this particular command actually has.
	FilterHint string
	// Scope names what --all would cover when nothing narrows it, for the warning
	// that precedes an unbounded change.
	Scope string
}

// Selection is a resolved set of things to act on.
type Selection[T any] struct {
	Rows  []T
	IDs   []string
	noun  string
	cols  []ui.Column[T]
	idsOf func(T) string
}

// Len is how many things were selected.
func (s Selection[T]) Len() int { return len(s.IDs) }

// Preview renders the selection as the table its list command would show. A dry
// run uses it, so approving a bulk change means looking at the things themselves
// rather than at a count.
func (s Selection[T]) Preview() func(*ui.UI) error {
	if len(s.Rows) == 0 {
		return nil
	}
	rows, cols, noun := s.Rows, s.cols, s.noun
	return func(u *ui.UI) error {
		return ui.Table(u, ui.TableSpec[T]{
			Noun: noun, Columns: cols,
			Total: ui.Unknown, Page: ui.Unpaged,
		}, rows)
	}
}

// Select resolves the references in c.Args, unions them with whatever the
// filters matched, and returns the result.
//
// A bare invocation with neither references nor filters is refused rather than
// interpreted: "everything" is too consequential to be the default reading of an
// empty command line.
func Select[T any](c *Invocation, s Selector[T]) (Selection[T], error) {
	sel := Selection[T]{noun: s.Noun, cols: s.Columns, idsOf: s.IDOf}

	if len(c.Args) == 0 && s.ByFilter == nil {
		return sel, Fail("Nothing selected.").
			Hint("pass a REF, or a filter such as "+s.FilterHint+".",
				"Use --all to target "+s.Scope+".")
	}

	for _, ref := range c.Args {
		row, err := s.ByRef(c.Ctx, ref)
		if err != nil {
			return sel, err
		}
		sel.Rows = append(sel.Rows, row)
	}

	if s.ByFilter != nil {
		matched, err := s.ByFilter(c.Ctx)
		if err != nil {
			return sel, err
		}
		sel.Rows = append(sel.Rows, matched...)
	}

	// Deduplicate by reference, keeping the first row seen for each, so a thing
	// named explicitly and also matched by a filter is acted on once and
	// previewed once.
	seen := make(map[string]struct{}, len(sel.Rows))
	rows := make([]T, 0, len(sel.Rows))
	for _, row := range sel.Rows {
		id := s.IDOf(row)
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		rows = append(rows, row)
		sel.IDs = append(sel.IDs, id)
	}
	sel.Rows = rows
	return sel, nil
}

// ── the shared filter flags ──

// Range holds the age filters every collection shares. Apps embed it so
// --older-than means the same thing wherever it appears.
type Range struct {
	OlderThan string
	NewerThan string
}

// Register adds the age filters. subject completes "not modified within", so the
// help says what the age is actually measured against.
func (r *Range) Register(f Flags, subject string) {
	f.StringVar(&r.OlderThan, "older-than", "",
		"Match "+subject+" older than DURATION (e.g. 30d, 2w, 1h)")
	f.StringVar(&r.NewerThan, "newer-than", "",
		"Match "+subject+" newer than DURATION")
}

// Set reports whether either bound was given.
func (r *Range) Set() bool { return r.OlderThan != "" || r.NewerThan != "" }

// Flags is the slice of pflag.FlagSet the shared groups need. Declaring it as an
// interface keeps kit from importing pflag into every caller's mental model.
type Flags interface {
	StringVar(p *string, name, value, usage string)
	StringArrayVar(p *[]string, name string, value []string, usage string)
	BoolVar(p *bool, name string, value bool, usage string)
	IntVar(p *int, name string, value int, usage string)
}

// HintList joins flag names for a "such as" hint.
func HintList(flags ...string) string {
	for i, f := range flags {
		flags[i] = "--" + strings.TrimPrefix(f, "--")
	}
	switch len(flags) {
	case 0:
		return ""
	case 1:
		return flags[0]
	}
	return strings.Join(flags[:len(flags)-1], ", ") + " or " + flags[len(flags)-1]
}
