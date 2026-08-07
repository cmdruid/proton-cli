package ui

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

// Column describes one column of a collection. Cell extracts the text; the
// flags say how it is treated.
type Column[T any] struct {
	// Header is the column title: one uppercase word, or SNAKE_CASE. Never a
	// glyph - a reader should be able to say the name out loud.
	Header string
	Cell   func(T) string
	// ID marks the cell as a Proton reference: shortened on a terminal unless
	// full IDs were asked for, and coloured as an ID.
	ID bool
	// Accent highlights the cell, for compact status markers.
	Accent bool
	// Flex allows the column to be narrowed when the table is wider than the
	// terminal. Columns without it keep their natural width.
	Flex bool
	// Right aligns the cell to the right, for counts and sizes.
	Right bool
}

// TableSpec describes a collection: the columns to draw, and the facts the
// footer and the JSON envelope need.
type TableSpec[T any] struct {
	// Noun is the collection's plural name. It is the JSON envelope key and the
	// word the footer uses, so the two can never disagree.
	Noun    string
	Columns []Column[T]

	// Total, Page, PageSize and Limit describe the request; see FooterSpec.
	Total    int
	Page     int
	PageSize int
	Limit    int

	// Rows, when set, replaces the marshalled items in machine formats. Use it
	// when the wire shape differs from the table's row type.
	Rows any
	// Extra adds fields to the JSON envelope.
	Extra map[string]any
}

// flexFloor is the narrowest a flexible column may be squeezed before the table
// is allowed to overflow instead. Below this, truncation destroys more than it
// saves.
const flexFloor = 12

// Table renders a collection. In text mode it draws an aligned table on Out and
// a one-line summary on Err; in a machine format it writes the envelope on Out.
//
// An empty collection produces no bytes on Out at all - just the summary on Err
// - so a redirect yields an empty file rather than a stray header.
func Table[T any](u *UI, spec TableSpec[T], items []T) error {
	if u.Format.Machine() {
		return u.encode(envelope(spec, items))
	}

	if len(items) > 0 {
		writeTable(u, spec, items)
	}
	u.Hint(Footer(FooterSpec{
		Noun: spec.Noun, Count: len(items), Total: spec.Total,
		Page: spec.Page, PageSize: spec.PageSize, Limit: spec.Limit,
	}))
	return nil
}

// envelope builds the machine-format object. Every collection has the same
// shape: the rows under their plural noun, plus the facts that were actually
// established. Fields the request did not involve are omitted rather than
// reported as zero.
// An empty collection is an empty array, never null: a nil slice is how Go
// spells "none", not how the contract does, and `.items[]` has to keep working.
func envelope[T any](spec TableSpec[T], items []T) map[string]any {
	if items == nil {
		items = []T{}
	}
	var rows any = items
	if spec.Rows != nil {
		rows = spec.Rows
	}
	env := map[string]any{
		spec.Noun: rows,
		"count":   len(items),
	}
	if spec.Total != Unknown {
		env["total"] = spec.Total
	}
	if spec.Page != Unpaged && spec.PageSize > 0 {
		env["page"] = spec.Page
		env["page_size"] = spec.PageSize
		env["has_more"] = spec.Total != Unknown && (spec.Page+1)*spec.PageSize < spec.Total
	}
	if spec.Limit > 0 {
		env["limited"] = len(items) >= spec.Limit
	}
	for k, v := range spec.Extra {
		env[k] = v
	}
	return env
}

func writeTable[T any](u *UI, spec TableSpec[T], items []T) {
	cols := spec.Columns
	short := u.ShortIDs()

	rows := make([][]string, 0, len(items))
	for _, it := range items {
		row := make([]string, len(cols))
		for i, c := range cols {
			v := c.Cell(it)
			if c.ID {
				v = Short(v, short)
			}
			row[i] = v
		}
		rows = append(rows, row)
	}

	widths := layout(cols, rows, u.width())
	theme := u.theme

	heads := make([]string, len(cols))
	rules := make([]string, len(cols))
	for i, c := range cols {
		heads[i] = pad(c.Header, widths[i], c.Right)
		rules[i] = strings.Repeat(GlyphRule, widths[i])
	}
	_, _ = fmt.Fprintln(u.Out, theme.Hint(strings.TrimRight(strings.Join(heads, "  "), " ")))
	_, _ = fmt.Fprintln(u.Out, theme.Rule(strings.Join(rules, "  ")))

	for _, row := range rows {
		// Cells are assembled first so trailing empty ones can be dropped whole.
		// Trimming the finished line instead would be wrong: styling wraps a cell,
		// so padding can end up inside an escape sequence where no trim reaches it.
		cells := make([]string, len(cols))
		last := -1
		for i, c := range cols {
			if row[i] == "" {
				cells[i] = strings.Repeat(" ", widths[i])
				continue
			}
			last = i
			cell := pad(truncate(row[i], widths[i]), widths[i], c.Right)
			switch {
			case c.ID:
				cell = theme.ID(cell)
			case c.Accent:
				cell = theme.Accent(cell)
			}
			cells[i] = cell
		}
		if last < 0 {
			_, _ = fmt.Fprintln(u.Out)
			continue
		}
		// The rightmost populated cell needs no padding after it.
		if !cols[last].Right {
			cells[last] = pad(truncate(row[last], widths[last]), widths[last], false)
			cells[last] = strings.TrimRight(cells[last], " ")
			switch {
			case cols[last].ID:
				cells[last] = theme.ID(cells[last])
			case cols[last].Accent:
				cells[last] = theme.Accent(cells[last])
			}
		}
		_, _ = fmt.Fprintln(u.Out, strings.Join(cells[:last+1], "  "))
	}
}

// layout sizes every column to its content, then, while the table is wider than
// maxWidth, shaves a character off the widest flexible column. Narrowing the
// widest column keeps the loss where there is most to spare, unlike shrinking a
// fixed position such as the last column.
//
// maxWidth <= 0 means unlimited: nothing is truncated.
//
// Widths are counted in runes. A few glyphs the flag column uses render two
// cells wide, which can nudge the columns after them; the flag column is
// therefore always last, where nothing follows to misalign.
func layout[T any](cols []Column[T], rows [][]string, maxWidth int) []int {
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = utf8.RuneCountInString(c.Header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if n := utf8.RuneCountInString(cell); n > widths[i] {
				widths[i] = n
			}
		}
	}
	if maxWidth <= 0 {
		return widths
	}

	total := func() int {
		n := 2 * (len(widths) - 1)
		for _, w := range widths {
			n += w
		}
		return n
	}
	for total() > maxWidth {
		victim, best := -1, flexFloor
		for i, c := range cols {
			if c.Flex && widths[i] > best {
				victim, best = i, widths[i]
			}
		}
		if victim < 0 {
			break
		}
		widths[victim]--
	}
	return widths
}

// width is the column budget for a table, or 0 for no budget at all.
//
// Truncation is a courtesy for a human whose terminal would otherwise wrap. It
// is never applied to a pipe or a file: there, a shortened subject is not a
// tidier table, it is corrupted data. So a non-terminal destination gets its
// full natural width, and only an explicit --width or a real terminal imposes
// one.
func (u *UI) width() int {
	if u.Width > 0 {
		return u.Width
	}
	f, ok := u.Out.(*os.File)
	if !ok {
		return 0
	}
	cols, _, err := term.GetSize(int(f.Fd()))
	if err != nil || cols <= 0 {
		return 0
	}
	return cols
}

func pad(s string, width int, right bool) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	fill := strings.Repeat(" ", width-n)
	if right {
		return fill + s
	}
	return s + fill
}

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}
