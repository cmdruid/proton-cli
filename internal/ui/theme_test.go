package ui

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

// The zero Theme is the contract that makes piped output byte-identical to
// terminal output minus the escapes: every method has to return its input.
func TestZeroThemeIsTransparent(t *testing.T) {
	var zero Theme
	if zero.Enabled() {
		t.Error("the zero Theme must be disabled")
	}
	for name, fn := range map[string]func(string) string{
		"Hint": zero.Hint, "Rule": zero.Rule, "ID": zero.ID,
		"Accent": zero.Accent, "Success": zero.Success, "Danger": zero.Danger,
	} {
		if got := fn("text"); got != "text" {
			t.Errorf("%s on a disabled theme returned %q", name, got)
		}
	}
}

func TestThemeEmitsEscapes(t *testing.T) {
	wide := Theme{enabled: true, wide: true}
	if got := wide.ID("x"); !strings.HasPrefix(got, "\x1b[38;2;") || !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("24-bit colour not applied: %q", got)
	}
	narrow := Theme{enabled: true}
	if got := narrow.ID("x"); !strings.HasPrefix(got, "\x1b[38;5;") {
		t.Errorf("256-colour fallback not applied: %q", got)
	}
	// An empty string gains nothing, so padding never ends up inside an escape.
	if got := wide.ID(""); got != "" {
		t.Errorf("empty input should stay empty, got %q", got)
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
func TestThemeForNonTerminalIsDisabled(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	unsetenv(t, "NO_COLOR")
	if ThemeFor(&bytes.Buffer{}).Enabled() {
		t.Error("a buffer must never be coloured")
	}
}

// vocabulary is every glyph the CLI draws, by what it means.
func vocabulary() map[string]string {
	return map[string]string{
		"success": GlyphSuccess, "caution": GlyphCaution, "unread": GlyphUnread,
		"starred": GlyphStarred, "swatch": GlyphSwatch, "rule": GlyphRule,
		"bar filled": GlyphBarFilled,
	}
}

// Every glyph carries exactly one meaning, and no two meanings share a glyph.
func TestGlyphVocabularyIsDistinct(t *testing.T) {
	glyphs := vocabulary()
	seen := map[string]string{}
	for meaning, g := range glyphs {
		if g == "" {
			t.Errorf("%s has no glyph", meaning)
		}
		if prev, dup := seen[g]; dup {
			t.Errorf("%q means both %q and %q", g, prev, meaning)
		}
		seen[g] = meaning
	}
	// The progress bar's remaining segment reuses the rule glyph deliberately:
	// both are "a line", drawn in different weights.
	if GlyphBarPending != GlyphRule {
		t.Errorf("the bar's pending segment should be the rule glyph, got %q", GlyphBarPending)
	}
}

// Every glyph is one code point of one terminal cell, which is what keeps a
// column a column.
//
// The rule exists because an emoji cannot satisfy it. No monospace font carries
// one, so a terminal asked to draw an emoji substitutes a colour emoji font, and
// those glyphs are two cells wide - enough to push every column after them out
// of line. A paperclip was the CLI's one emoji, and working around its width is
// why the status column had to be pinned last; the attachment marker is a count
// now, and this test is what stops the next one arriving.
func TestEveryGlyphIsOneNarrowCell(t *testing.T) {
	for meaning, g := range vocabulary() {
		if n := utf8.RuneCountInString(g); n != 1 {
			t.Errorf("%s is %q, which is %d code points; a glyph is one", meaning, g, n)
			continue
		}
		r, _ := utf8.DecodeRuneInString(g)
		if r > 0xFFFF {
			t.Errorf("%s is %q (U+%04X), which is outside the BMP and so an emoji", meaning, g, r)
		}
		if w := Cells(g); w != 1 {
			t.Errorf("%s is %q, which occupies %d cells; a glyph occupies one", meaning, g, w)
		}
	}
}
