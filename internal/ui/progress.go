package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/roman-16/proton-cli/internal/progress"
	"github.com/roman-16/proton-cli/internal/units"
	"golang.org/x/term"
)

// barWidth is the drawn length of the progress bar, chosen to leave room for the
// label and the byte counter on an 80-column terminal.
const barWidth = 30

// Progress draws a transfer bar on a terminal and nothing anywhere else. It
// implements progress.Sink so services report through the interface and never
// import this package.
//
// The bar is built from the same box-drawing glyphs as a table rule, so a
// transfer looks like part of the same interface rather than a borrowed
// ASCII widget.
type Progress struct {
	w      io.Writer
	theme  Theme
	active bool

	total   int64
	label   string
	current int64
}

// NewProgress returns a sink drawing on the UI's stderr, or a no-op when output
// is not an interactive terminal or --quiet was given. Callers can therefore
// hand the result straight to a service without checking.
func NewProgress(u *UI) progress.Sink {
	if u.Quiet || u.Format.Machine() {
		return progress.Nop{}
	}
	f, ok := u.Err.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return progress.Nop{}
	}
	return &Progress{w: u.Err, theme: u.errTheme, active: true}
}

func (p *Progress) Start(total int64, label string) {
	p.total, p.label, p.current = total, label, 0
	p.draw()
}

func (p *Progress) Add(n int64) {
	p.current += n
	p.draw()
}

// Done closes the line so whatever prints next starts fresh.
func (p *Progress) Done() {
	if !p.active {
		return
	}
	_, _ = fmt.Fprintln(p.w)
	p.active = false
}

func (p *Progress) draw() {
	if !p.active {
		return
	}
	// Encryption adds per-block overhead, so the byte counter can run past the
	// source size. Report the size the user asked about.
	done := p.current
	if p.total > 0 && done > p.total {
		done = p.total
	}
	ratio := 0.0
	if p.total > 0 {
		ratio = float64(done) / float64(p.total)
	}
	filled := int(ratio * barWidth)
	bar := strings.Repeat(GlyphBarFilled, filled) + strings.Repeat(GlyphBarPending, barWidth-filled)

	_, _ = fmt.Fprintf(p.w, "\r%s %s %s / %s (%.0f%%)",
		p.label, p.theme.Accent(bar), units.Size(done), units.Size(p.total), ratio*100)
}
