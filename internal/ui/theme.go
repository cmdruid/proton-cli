package ui

import (
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// The glyph vocabulary. One symbol per meaning, used everywhere that meaning
// appears, so a reader learns each mark once.
const (
	GlyphSuccess    = "✓" // a mutation succeeded
	GlyphUnread     = "●" // an unread message
	GlyphStarred    = "★" // a starred message
	GlyphAttachment = "📎" // the item carries attachments
	GlyphRule       = "─" // a horizontal rule, under table headers
	GlyphBarFilled  = "━" // progress, done
	GlyphBarPending = "─" // progress, remaining
)

// Proton's carbon accents in 24-bit, each with its nearest xterm-256 index.
var (
	colorHint    = color{"109;105;125", 244} // text-hint: headers, footers, labels
	colorRule    = color{"74;70;88", 239}    // border-norm: the header rule
	colorID      = color{"150;125;255", 141} // primary-major-1: Proton IDs
	colorAccent  = color{"138;110;255", 99}  // primary: markers, the progress bar
	colorSuccess = color{"53;177;145", 72}   // signal-success: confirmations
	colorDanger  = color{"220;79;66", 167}   // signal-danger: errors
)

type color struct {
	rgb  string
	x256 uint8
}

// Theme styles one stream. It is deliberately narrow: colour is a courtesy for
// a human at a terminal, never part of the data. The zero Theme is disabled and
// every method returns its input unchanged, so the bytes a pipe receives are
// identical either way.
type Theme struct {
	enabled bool
	// wide is true for 24-bit terminals; 256-colour terminals get the nearest
	// palette index instead.
	wide bool
}

// NoColor reports whether colour is suppressed regardless of destination,
// honouring the NO_COLOR convention (https://no-color.org) and TERM=dumb.
func NoColor() bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return true
	}
	switch os.Getenv("TERM") {
	case "dumb", "":
		return true
	}
	return false
}

// ThemeFor returns the palette to use when writing to w. Colour is enabled only
// for a real terminal; PROTON_CLI_FORCE_TTY deliberately does not apply, so
// captured output stays plain.
func ThemeFor(w io.Writer) Theme {
	if NoColor() {
		return Theme{}
	}
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return Theme{}
	}
	return Theme{enabled: true, wide: wideTerminal()}
}

func wideTerminal() bool {
	switch os.Getenv("COLORTERM") {
	case "truecolor", "24bit":
		return true
	}
	return strings.Contains(os.Getenv("TERM"), "truecolor")
}

// Enabled reports whether this palette emits escape sequences.
func (t Theme) Enabled() bool { return t.enabled }

func (t Theme) paint(c color, s string) string {
	if !t.enabled || s == "" {
		return s
	}
	if t.wide {
		return "\x1b[38;2;" + c.rgb + "m" + s + "\x1b[0m"
	}
	return "\x1b[38;5;" + strconv.Itoa(int(c.x256)) + "m" + s + "\x1b[0m"
}

func (t Theme) Hint(s string) string    { return t.paint(colorHint, s) }
func (t Theme) Rule(s string) string    { return t.paint(colorRule, s) }
func (t Theme) ID(s string) string      { return t.paint(colorID, s) }
func (t Theme) Accent(s string) string  { return t.paint(colorAccent, s) }
func (t Theme) Success(s string) string { return t.paint(colorSuccess, s) }
func (t Theme) Danger(s string) string  { return t.paint(colorDanger, s) }
