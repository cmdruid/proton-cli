package kit

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// Walking a collection the CLI already holds.
//
// Proton pages and orders mail itself, so `mail messages list` hands --page and
// --sort straight to the server. Nothing else it stores can be asked for in
// pieces: contacts arrive as one encrypted export, Pass items as one batch per
// vault, a Drive folder as its whole listing. Those collections are decrypted
// locally and therefore ordered and paged locally too.
//
// That distinction is the whole reason this is a separate thing from the flags:
// paging a result the server ordered is a different act from paging one this
// process is holding, and only the second can also sort. Offering --sort on a
// collection whose pages arrive pre-cut would sort one page and call it the
// answer.

// Page is the position in a collection this invocation asked for.
type Page struct {
	Number int
	Size   int
}

// Register adds --page and --page-size.
func (p *Page) Register(c *cobra.Command, noun string) {
	c.Flags().IntVar(&p.Number, "page", 0, "Which page of results, counting from zero")
	c.Flags().IntVar(&p.Size, "page-size", 50, "How many "+noun+" per page")
}

// Slice cuts rows down to the page that was asked for and reports how many there
// were in total, which is what lets the footer say "50 of 3000" rather than
// leaving a reader to wonder whether that was all of them.
//
// A page past the end is empty rather than an error: it is the honest answer, and
// it is what a script walking pages until they run out needs.
func Slice[T any](p Page, rows []T) ([]T, int) {
	total := len(rows)
	if p.Size <= 0 {
		return rows, total
	}
	start := p.Number * p.Size
	if start >= total {
		return nil, total
	}
	end := min(start+p.Size, total)
	return rows[start:end], total
}

// Order is the ordering this invocation asked for.
type Order struct {
	Desc bool
	key  *Enum
}

// Register adds --sort and --desc, offering only the keys this collection has.
//
// The key is an Enum, so a key that cannot be sorted by is refused from the
// command line before anything is fetched, its domain is printed when it is, and
// shell completion offers it - the three things a fixed set of values owes,
// discharged by the one declaration. The first key is the default.
func (o *Order) Register(c *cobra.Command, keys ...string) {
	o.key = &Enum{Name: "sort", Usage: "Order by", Values: keys, Default: keys[0]}
	o.key.Register(c)
	c.Flags().BoolVar(&o.Desc, "desc", false, "Reverse the order")
}

// Comparators is how one collection may be ordered: a comparison per key it
// declared. Sorting reads from here, so a key offered by --sort and a key that
// can actually be applied are the same set.
type Comparators[T any] map[string]func(a, b T) int

// Sort orders rows in place.
//
// The key was already checked against the declared domain before the command
// body ran, so a comparator missing here is this CLI disagreeing with itself
// about what it offers - a bug rather than bad input, and it says so.
func Sort[T any](o Order, rows []T, by Comparators[T]) error {
	key, err := o.key.Value()
	if err != nil {
		return err
	}
	cmp, ok := by[key]
	if !ok {
		return Fail("--sort offers %q but this collection cannot order by it", key)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if o.Desc {
			return cmp(rows[i], rows[j]) > 0
		}
		return cmp(rows[i], rows[j]) < 0
	})
	return nil
}

// Fold compares two strings the way a person reading a list would: without
// caring about case, and falling back to the exact bytes so the order never
// depends on which of two equal-looking names arrived first.
func Fold(a, b string) int {
	if c := strings.Compare(strings.ToLower(a), strings.ToLower(b)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

// Ints compares two numbers, for a size or a timestamp.
func Ints(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
