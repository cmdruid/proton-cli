package render

import (
	"bytes"
	"strings"
	"testing"
)

func TestColorsZeroValueIsPassthrough(t *testing.T) {
	var c Colors
	if c.Enabled() {
		t.Error("zero Colors reports enabled")
	}
	for name, got := range map[string]string{
		"Hint":    c.Hint("x"),
		"Rule":    c.Rule("x"),
		"ID":      c.ID("x"),
		"Accent":  c.Accent("x"),
		"Success": c.Success("x"),
	} {
		if got != "x" {
			t.Errorf("%s on disabled Colors = %q, want %q", name, got, "x")
		}
	}
}

func TestColorsPaint(t *testing.T) {
	tests := []struct {
		name string
		c    Colors
		want string
	}{
		{"truecolor", Colors{enabled: true, wide: true}, "\x1b[38;2;150;125;255mabc\x1b[0m"},
		{"256 color", Colors{enabled: true}, "\x1b[38;5;141mabc\x1b[0m"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.ID("abc"); got != tc.want {
				t.Errorf("ID() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestColorsPaintEmptyStringUnchanged(t *testing.T) {
	c := Colors{enabled: true, wide: true}
	if got := c.Hint(""); got != "" {
		t.Errorf("Hint(\"\") = %q, want empty", got)
	}
}

func TestColorsForNonTerminalIsDisabled(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	if ColorsFor(&bytes.Buffer{}).Enabled() {
		t.Error("ColorsFor(buffer) is enabled, want disabled for a non-terminal")
	}
}

func TestNoColor(t *testing.T) {
	tests := []struct {
		name    string
		noColor string
		set     bool
		term    string
		want    bool
	}{
		{"NO_COLOR set", "1", true, "xterm-256color", true},
		{"NO_COLOR empty but present", "", true, "xterm-256color", true},
		{"TERM=dumb", "", false, "dumb", true},
		{"TERM unset", "", false, "", true},
		{"regular terminal", "", false, "xterm-256color", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv("NO_COLOR", tc.noColor)
			}
			t.Setenv("TERM", tc.term)
			if got := NoColor(); got != tc.want {
				t.Errorf("NoColor() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWideTerminal(t *testing.T) {
	tests := []struct {
		name      string
		colorterm string
		term      string
		want      bool
	}{
		{"COLORTERM=truecolor", "truecolor", "xterm-256color", true},
		{"COLORTERM=24bit", "24bit", "xterm-256color", true},
		{"TERM mentions truecolor", "", "xterm-truecolor", true},
		{"plain 256 color", "", "xterm-256color", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("COLORTERM", tc.colorterm)
			t.Setenv("TERM", tc.term)
			if got := wideTerminal(); got != tc.want {
				t.Errorf("wideTerminal() = %v, want %v", got, tc.want)
			}
		})
	}
}

// Color must never leak into what a pipe receives: styling only ever wraps a
// value, so stripping the escapes has to yield the original text.
func TestColorsWrapWithoutChangingText(t *testing.T) {
	c := Colors{enabled: true, wide: true}
	const in = "2026-04-15 14:32"
	got := c.Accent(in)
	if stripped := stripANSI(got); stripped != in {
		t.Errorf("stripANSI(%q) = %q, want %q", got, stripped, in)
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, "\x1b[")
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		end := strings.IndexByte(s[i:], 'm')
		if end < 0 {
			return b.String()
		}
		s = s[i+end+1:]
	}
}

func TestItoa(t *testing.T) {
	for in, want := range map[uint8]string{0: "0", 7: "7", 72: "72", 141: "141", 255: "255"} {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q, want %q", in, got, want)
		}
	}
}
