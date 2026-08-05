package render

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

// TableStyle colors the parts of a table. A nil function leaves its part
// unstyled. Styling is applied after layout, so it never affects column
// widths.
type TableStyle struct {
	Header func(string) string
	Rule   func(string) string
	// Cells styles data cells per column index; entries may be nil.
	Cells []func(string) string
}

func (s TableStyle) cell(col int, v string) string {
	if col < len(s.Cells) && s.Cells[col] != nil {
		return s.Cells[col](v)
	}
	return v
}

func paint(fn func(string) string, s string) string {
	if fn == nil {
		return s
	}
	return fn(s)
}

// Table prints a simple two-space-separated table with a header underline.
// Widths are computed from content and clamped to the terminal width.
func Table(w io.Writer, headers []string, rows [][]string, style TableStyle) {
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "(no results)")
		return
	}

	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				n := utf8.RuneCountInString(cell)
				if n > widths[i] {
					widths[i] = n
				}
			}
		}
	}

	termWidth := 120
	if tw, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && tw > 0 {
		termWidth = tw
	}
	total := len(widths) - 1
	for _, wv := range widths {
		total += wv + 2
	}
	if total > termWidth && len(widths) > 1 {
		excess := total - termWidth
		last := len(widths) - 1
		if widths[last] > excess+10 {
			widths[last] -= excess
		}
	}

	var head, sep strings.Builder
	for i, h := range headers {
		if i > 0 {
			head.WriteString("  ")
			sep.WriteString("  ")
		}
		head.WriteString(padRight(h, widths[i]))
		sep.WriteString(strings.Repeat("─", widths[i]))
	}
	_, _ = fmt.Fprintln(w, paint(style.Header, head.String()))
	_, _ = fmt.Fprintln(w, paint(style.Rule, sep.String()))

	for _, row := range rows {
		var line strings.Builder
		for i := 0; i < len(headers); i++ {
			if i > 0 {
				line.WriteString("  ")
			}
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			padded := padRight(truncate(cell, widths[i]), widths[i])
			if cell != "" {
				padded = style.cell(i, padded)
			}
			line.WriteString(padded)
		}
		_, _ = fmt.Fprintln(w, line.String())
	}
}

func padRight(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	if max <= 3 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}
