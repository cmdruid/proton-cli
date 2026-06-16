// Package view is the declarative presentation layer. A command describes its
// list output as columns; Render handles text-table vs json/yaml, short-ID
// shortening, ID-cache population and footers uniformly - removing the
// per-command format branching that would otherwise be repeated everywhere.
package view

import (
	"fmt"

	"github.com/roman-16/proton-cli/internal/idcache"
	"github.com/roman-16/proton-cli/internal/render"
)

type Column[T any] struct {
	Header string
	Cell   func(T) string
	// ID marks the cell as a Proton ID: it is shortened to 8 chars on an
	// interactive terminal (unless full IDs are requested).
	ID bool
}

type List[T any] struct {
	Columns []Column[T]
	// CacheIDs returns the IDs of a row to persist to the short-ID cache.
	// nil disables caching.
	CacheIDs func(T) []string
	// Footer returns an optional stderr footer line given the row count;
	// printed only in text mode. nil disables it.
	Footer func(n int) string
	// JSON, when non-nil, overrides the marshalled value in json/yaml mode
	// (e.g. a paginated wrapper). nil marshals the items slice directly.
	JSON any
}

// Render emits items according to l, using r's format. short shortens IDs;
// cache (may be nil) receives the row IDs.
func Render[T any](r *render.Renderer, short bool, cache *idcache.Cache, l List[T], items []T) error {
	if cache != nil && l.CacheIDs != nil && len(items) > 0 {
		ids := make([]string, 0, len(items))
		for _, it := range items {
			ids = append(ids, l.CacheIDs(it)...)
		}
		_ = cache.Save(ids...)
	}

	if r.Format != render.FormatText {
		if l.JSON != nil {
			return r.Object(l.JSON)
		}
		return r.Object(items)
	}

	headers := make([]string, len(l.Columns))
	for i, c := range l.Columns {
		headers[i] = c.Header
	}
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		row := make([]string, len(l.Columns))
		for i, c := range l.Columns {
			v := c.Cell(it)
			if c.ID {
				v = render.ShortID(v, short)
			}
			row[i] = v
		}
		rows = append(rows, row)
	}
	render.Table(r.Stdout, headers, rows)
	if l.Footer != nil {
		if f := l.Footer(len(items)); f != "" {
			_, _ = fmt.Fprintln(r.Stderr, "\n"+f)
		}
	}
	return nil
}
