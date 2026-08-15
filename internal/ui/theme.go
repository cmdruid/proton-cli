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
//
// Every glyph is one code point of one terminal cell. That is a rule, not a
// coincidence: no monospace font carries an emoji, so a terminal asked to draw
// one substitutes a colour emoji font, and those are two cells wide. A single
// such glyph is enough to push every column after it out of line, which is why
// the attachment marker is a count rather than a paperclip.
const (
	GlyphSuccess    = "✓" // a mutation succeeded
	GlyphCaution    = "!" // it worked, and something about it is worth knowing
	GlyphUnread     = "●" // an unread message
	GlyphStarred    = "★" // a starred message
	GlyphSwatch     = "■" // the colour a label, folder or calendar is shown in
	GlyphRule       = "─" // a horizontal rule, under table headers
	GlyphBarFilled  = "━" // progress, done
	GlyphBarPending = "─" // progress, remaining
)

// Proton's own design tokens, read from the carbon and snow themes in
// WebClients (packages/colors/themes/src). Where the two themes differ, the
// carbon value is used: a terminal is dark far more often than not.
var (
	colorHint    = rgb(0x6D, 0x69, 0x7D) // text-hint: headers, footers, labels
	colorRule    = rgb(0x4A, 0x46, 0x58) // border-norm: the header rule
	colorID      = rgb(0x96, 0x7D, 0xFF) // primary-major-1: Proton IDs
	colorAccent  = rgb(0x8A, 0x6E, 0xFF) // primary: markers, the progress bar
	colorSuccess = rgb(0x1E, 0xA8, 0x85) // signal-success: confirmations
	colorCaution = rgb(0xFF, 0x99, 0x00) // signal-warning: caveats, and the star
	colorDanger  = rgb(0xDC, 0x32, 0x51) // signal-danger: errors
)

type color struct{ r, g, b uint8 }

func rgb(r, g, b uint8) color { return color{r, g, b} }

// parseHex reads the "#RRGGBB" Proton stores a label, folder, calendar or group
// colour as. Anything else reports false, so an unrecognised value is printed
// plainly rather than painted from nonsense.
func parseHex(s string) (color, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return color{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color{}, false
	}
	return color{uint8(v >> 16), uint8(v >> 8), uint8(v)}, true
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
		return "\x1b[38;2;" + strconv.Itoa(int(c.r)) + ";" +
			strconv.Itoa(int(c.g)) + ";" + strconv.Itoa(int(c.b)) + "m" + s + "\x1b[0m"
	}
	return "\x1b[38;5;" + strconv.Itoa(int(c.x256())) + "m" + s + "\x1b[0m"
}

// x256 is the nearest index in the xterm-256 palette, computed from the 6×6×6
// colour cube and the 24-step grey ramp. The first sixteen entries are left out
// on purpose: a terminal is free to redefine them, so a colour matched against
// them would come out as whatever the user's scheme says rather than as Proton's.
func (c color) x256() uint8 {
	best, bestDist := uint8(0), 1<<31-1
	consider := func(idx uint8, r, g, b int) {
		d := sq(int(c.r)-r) + sq(int(c.g)-g) + sq(int(c.b)-b)
		if d < bestDist {
			best, bestDist = idx, d
		}
	}
	levels := []int{0, 95, 135, 175, 215, 255}
	for r := 0; r < 6; r++ {
		for g := 0; g < 6; g++ {
			for b := 0; b < 6; b++ {
				consider(uint8(16+36*r+6*g+b), levels[r], levels[g], levels[b])
			}
		}
	}
	for i := 0; i < 24; i++ {
		v := 8 + 10*i
		consider(uint8(232+i), v, v, v)
	}
	return best
}

func sq(n int) int { return n * n }

func (t Theme) Hint(s string) string    { return t.paint(colorHint, s) }
func (t Theme) Rule(s string) string    { return t.paint(colorRule, s) }
func (t Theme) ID(s string) string      { return t.paint(colorID, s) }
func (t Theme) Accent(s string) string  { return t.paint(colorAccent, s) }
func (t Theme) Success(s string) string { return t.paint(colorSuccess, s) }
func (t Theme) Caution(s string) string { return t.paint(colorCaution, s) }
func (t Theme) Danger(s string) string  { return t.paint(colorDanger, s) }

// Paint applies a colour Proton chose rather than one this palette owns: the
// hex a label, folder, calendar or contact group is stored with. An unparseable
// value is returned untouched.
func (t Theme) Paint(hex, s string) string {
	c, ok := parseHex(hex)
	if !ok {
		return s
	}
	return t.paint(c, s)
}

// Tone is how much a value is worth noticing, and the one vocabulary tables and
// records share for it.
//
// It says what a value means, never which colour to use: that mapping lives here
// so a verdict reads the same wherever it appears, and so turning colour off
// changes nothing but the escapes.
type Tone int

const (
	// ToneNeutral is ordinary data, which is nearly all of it.
	ToneNeutral Tone = iota
	// ToneGood is a verdict in the reader's favour: a signature that verified.
	ToneGood
	// ToneCaution is true and worth noticing: a signature nobody could check.
	ToneCaution
	// ToneBad is a verdict against: a signature that is cryptographically wrong.
	ToneBad
	// ToneAccent is one of Proton's own markers rather than a verdict.
	ToneAccent
	// ToneMuted is context rather than content: a count beside a state.
	ToneMuted
)

// Mark is one glyph in a status column, paired with what it means.
type Mark struct {
	Glyph string
	Tone  Tone
}

// Marks is the run of glyphs a status column draws as a single cell.
//
// They are a list rather than a string because each mark means something
// different, and a cell painted one colour throughout cannot say so. A star is
// Proton's own orange wherever it appears; the count beside it is context, not
// a state, and is dimmed accordingly.
type Marks []Mark

// String is the plain text of the run, which is what the table measures,
// truncates and hands to a pipe.
func (m Marks) String() string {
	var b strings.Builder
	for _, mk := range m {
		b.WriteString(mk.Glyph)
	}
	return b.String()
}

func (t Theme) paintMarks(m Marks) string {
	var b strings.Builder
	for _, mk := range m {
		b.WriteString(t.tone(mk.Tone, mk.Glyph))
	}
	return b.String()
}

func (t Theme) tone(tone Tone, s string) string {
	switch tone {
	case ToneGood:
		return t.Success(s)
	case ToneCaution:
		return t.Caution(s)
	case ToneBad:
		return t.Danger(s)
	case ToneAccent:
		return t.Accent(s)
	case ToneMuted:
		return t.Hint(s)
	}
	return s
}
