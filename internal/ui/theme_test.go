package ui

import (
	"bytes"
	"os"
	"strings"
	"testing"
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

// Every glyph carries exactly one meaning, and no two meanings share a glyph.
func TestGlyphVocabularyIsDistinct(t *testing.T) {
	glyphs := map[string]string{
		"success": GlyphSuccess, "unread": GlyphUnread, "starred": GlyphStarred,
		"attachment": GlyphAttachment, "rule": GlyphRule,
		"bar filled": GlyphBarFilled,
	}
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
