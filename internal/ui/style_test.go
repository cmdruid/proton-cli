package ui

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

// roleNames is every meaning the CLI has, by the name it is written with.
func roleNames() map[string]Role {
	return map[string]Role{
		"plain": Plain, "muted": Muted, "accent": Accent,
		"success": Success, "caution": Caution, "danger": Danger,
	}
}

// The zero Style is the contract that makes piped output byte-identical to
// terminal output minus the escapes: every method has to return its input.
func TestZeroStyleIsTransparent(t *testing.T) {
	var zero Style
	if zero.Enabled() {
		t.Error("the zero Style must be disabled")
	}
	for name, role := range roleNames() {
		if got := zero.Paint(role, "text"); got != "text" {
			t.Errorf("%s on a disabled style returned %q", name, got)
		}
	}
	if got := zero.Swatch("#8080FF", GlyphSwatch); got != GlyphSwatch {
		t.Errorf("a swatch on a disabled style returned %q", got)
	}
}

// pinnedColor is a sequence that names an exact colour rather than one of the
// terminal's own: the 256-palette form and the 24-bit form.
var pinnedColor = regexp.MustCompile(`\x1b\[[34]8[;:][25]`)

// The point of the whole palette: nothing the CLI says about its own output
// pins a colour value. Every role is one of ECMA-48's colour names, so the
// shade is the reader's to choose - a terminal themed in green renders Proton's
// purple as their green, and a light terminal stays legible without the CLI
// ever guessing at its background.
func TestRolesNameColoursRatherThanPinningThem(t *testing.T) {
	style := Style{enabled: true, direct: true}
	for name, role := range roleNames() {
		got := style.Paint(role, "x")
		if pinnedColor.MatchString(got) {
			t.Errorf("%s pins an exact colour: %q", name, got)
		}
	}
}

// Every painted role opens with its own sequence and closes surgically, leaving
// any background or attribute around it alone.
func TestRolesAreDistinctAndCloseThemselves(t *testing.T) {
	style := Style{enabled: true}
	seen := map[string]string{}
	for name, role := range roleNames() {
		got := style.Paint(role, "x")
		if role == Plain {
			if got != "x" {
				t.Errorf("plain text should gain nothing, got %q", got)
			}
			continue
		}
		open, _, ok := strings.Cut(got, "x")
		if !ok || open == "" {
			t.Errorf("%s emitted no opening sequence: %q", name, got)
			continue
		}
		if prev, dup := seen[open]; dup {
			t.Errorf("%q opens both %s and %s", open, prev, name)
		}
		seen[open] = name
		if strings.HasSuffix(got, "\x1b[0m") {
			t.Errorf("%s closes with a full reset, which would clobber a background: %q", name, got)
		}
	}
	// An empty string gains nothing, so padding never ends up inside an escape.
	if got := style.Paint(Danger, ""); got != "" {
		t.Errorf("empty input should stay empty, got %q", got)
	}
}

// Muted is an intensity, not a colour. The slot a theme usually puts its grey
// in is also the slot several of them reserve for a background, so dimming with
// one would make headers vanish on those; faint dims against whatever the
// reader is reading on.
func TestMutedDimsRatherThanColouring(t *testing.T) {
	if got := (Style{enabled: true}).Paint(Muted, "x"); got != "\x1b[2mx\x1b[22m" {
		t.Errorf("muted should be faint, got %q", got)
	}
}

// A swatch is the one exception, and it is not the CLI choosing a colour: the
// hex is the value a label, folder, calendar or group was given, so redrawing
// it from the reader's palette would misreport a field rather than respect a
// theme.
func TestSwatchCarriesTheStoredColour(t *testing.T) {
	direct := Style{enabled: true, direct: true}
	if got := direct.Swatch("#8080FF", GlyphSwatch); got != "\x1b[38;2;128;128;255m"+GlyphSwatch+"\x1b[39m" {
		t.Errorf("24-bit swatch not applied: %q", got)
	}
	indexed := Style{enabled: true}
	if got := indexed.Swatch("#8080FF", GlyphSwatch); !strings.HasPrefix(got, "\x1b[38;5;") {
		t.Errorf("256-colour fallback not applied: %q", got)
	}
	// An unparseable hex is printed plainly rather than painted from nonsense.
	for _, hex := range []string{"", "purple", "#8080F", "#GGGGGG"} {
		if got := direct.Swatch(hex, GlyphSwatch); got != GlyphSwatch {
			t.Errorf("Swatch(%q) should leave the glyph alone, got %q", hex, got)
		}
	}
}

// The 256-colour fallback never matches against the first sixteen entries, for
// the same reason every role is one of them: those are the reader's to redefine,
// so a swatch resolved to one would come out as their theme rather than as the
// colour Proton stores.
func TestSwatchFallbackAvoidsTheThemeableEntries(t *testing.T) {
	for _, hex := range []string{"#000000", "#FF0000", "#8080FF", "#FFFFFF", "#6D697D"} {
		c, ok := parseHex(hex)
		if !ok {
			t.Fatalf("parseHex(%q) failed", hex)
		}
		if idx := c.x256(); idx < 16 {
			t.Errorf("%s resolved to index %d, which the terminal may redefine", hex, idx)
		}
	}
}

func TestNoColorConventions(t *testing.T) {
	t.Run("NO_COLOR set to anything disables colour", func(t *testing.T) {
		t.Setenv("TERM", "xterm-256color")
		t.Setenv("NO_COLOR", "")
		if !NoColor() {
			t.Error("NO_COLOR present, even empty, must disable colour")
		}
	})
	t.Run("TERM=dumb disables colour", func(t *testing.T) {
		unsetenv(t, "NO_COLOR")
		t.Setenv("TERM", "dumb")
		if !NoColor() {
			t.Error("TERM=dumb must disable colour")
		}
	})
	t.Run("a normal terminal allows colour", func(t *testing.T) {
		unsetenv(t, "NO_COLOR")
		t.Setenv("TERM", "xterm-256color")
		if NoColor() {
			t.Error("a normal TERM should allow colour")
		}
	})
}

// 24-bit capability decides how faithfully a swatch is drawn and nothing else,
// since every role is a name any colour terminal can resolve.
func TestDirectColorDetection(t *testing.T) {
	for _, tc := range []struct {
		colorterm, term string
		want            bool
	}{
		{"truecolor", "xterm-256color", true},
		{"24bit", "xterm-256color", true},
		{"", "xterm-direct", true},
		{"", "xterm-256color", false},
		{"", "screen", false},
		{"8bit", "xterm-256color", false},
	} {
		t.Setenv("COLORTERM", tc.colorterm)
		t.Setenv("TERM", tc.term)
		if got := directColor(); got != tc.want {
			t.Errorf("COLORTERM=%q TERM=%q: got %v, want %v", tc.colorterm, tc.term, got, tc.want)
		}
	}
}

// unsetenv removes a variable for the duration of the test. t.Setenv cannot do
// it: NO_COLOR counts as set even when empty, which is the case under test.
func unsetenv(t *testing.T, key string) {
	t.Helper()
	prev, had := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, prev)
			return
		}
		_ = os.Unsetenv(key)
	})
}

// A buffer is not a terminal, so nothing written to one is ever coloured. This
// is what keeps captured output and golden files stable.
func TestStyleForNonTerminalIsDisabled(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	unsetenv(t, "NO_COLOR")
	if StyleFor(&bytes.Buffer{}).Enabled() {
		t.Error("a buffer must never be coloured")
	}
}
