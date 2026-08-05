package render

import (
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// Colors styles interactive output. It is deliberately narrow: color is a
// courtesy for a human reading a terminal, never part of the data. A zero
// Colors is disabled and every method returns its input unchanged, so the
// bytes a pipe or a redirect receives are identical either way.
type Colors struct {
	enabled bool
	// wide is true for 24-bit terminals; 256-color terminals get the nearest
	// palette index instead.
	wide bool
}

// Proton's carbon accents (24-bit) with their closest xterm-256 index.
var (
	colorHint    = color{"109;105;125", 244} // text-hint: table headers, footers
	colorRule    = color{"74;70;88", 239}    // border-norm: header underline
	colorID      = color{"150;125;255", 141} // primary-major-1: Proton IDs
	colorAccent  = color{"138;110;255", 99}  // primary: flags, progress bar
	colorSuccess = color{"53;177;145", 72}   // signal-success: ✓ confirmations
)

type color struct {
	rgb  string
	x256 uint8
}

// NoColor reports whether color is suppressed regardless of the destination,
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

// ColorsFor returns the palette to use when writing to w. Color is enabled
// only for a real terminal: PROTON_CLI_FORCE_TTY deliberately does not apply,
// so captured output stays plain.
func ColorsFor(w io.Writer) Colors {
	if NoColor() {
		return Colors{}
	}
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return Colors{}
	}
	return Colors{enabled: true, wide: wideTerminal()}
}

func wideTerminal() bool {
	switch os.Getenv("COLORTERM") {
	case "truecolor", "24bit":
		return true
	}
	return strings.Contains(os.Getenv("TERM"), "truecolor")
}

// Enabled reports whether this palette emits escape sequences.
func (c Colors) Enabled() bool { return c.enabled }

func (c Colors) paint(col color, s string) string {
	if !c.enabled || s == "" {
		return s
	}
	if c.wide {
		return "\x1b[38;2;" + col.rgb + "m" + s + "\x1b[0m"
	}
	return "\x1b[38;5;" + itoa(col.x256) + "m" + s + "\x1b[0m"
}

func (c Colors) Hint(s string) string    { return c.paint(colorHint, s) }
func (c Colors) Rule(s string) string    { return c.paint(colorRule, s) }
func (c Colors) ID(s string) string      { return c.paint(colorID, s) }
func (c Colors) Accent(s string) string  { return c.paint(colorAccent, s) }
func (c Colors) Success(s string) string { return c.paint(colorSuccess, s) }

func itoa(n uint8) string {
	if n == 0 {
		return "0"
	}
	var b [3]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = '0' + n%10
		n /= 10
	}
	return string(b[i:])
}
