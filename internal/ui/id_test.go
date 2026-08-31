package ui

import (
	"testing"

	"github.com/roman-16/proton-cli/internal/ref"
)

func TestShort(t *testing.T) {
	const full = "5bH2mQxKT9wLpN4vRs8kZc1yXd7fGh3jAe6bUi0oQm2nWr5tYv=="
	const dashed = "-Qt-s7R_oGCru5u3Kv6Y8Q"
	for _, tc := range []struct {
		name  string
		in    string
		short bool
		want  string
	}{
		{"shortening off leaves it alone", full, false, full},
		{"shortening keeps the first eight", full, true, "5bH2mQxK"},
		{"already short is untouched", "abc", true, "abc"},
		{"exactly eight is untouched", "12345678", true, "12345678"},
		{"empty stays empty", "", true, ""},
		// A leading dash belongs to the ID, but a token starting with one belongs
		// to the flag parser, so the eight characters begin after it.
		{"a leading dash is skipped", dashed, true, "Qt-s7R_o"},
		{"shortening off keeps the dash", dashed, false, dashed},
		{"a dashed half of a compound", full + "/" + dashed, true, "5bH2mQxK/Qt-s7R_o"},
		// A compound reference is one pasteable token, so both halves shorten and
		// the separator survives.
		{"compound shortens each half", full + "/" + full, true, "5bH2mQxK/5bH2mQxK"},
		{"compound off", full + "/" + full, false, full + "/" + full},
		// An occurrence is what tells two rows of one series apart, so shortening
		// the reference must not take it with the rest of the event's ID.
		{"an occurrence survives", full + "/" + full + "@2026-04-16T09:00", true, "5bH2mQxK/5bH2mQxK@2026-04-16T09:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Short(tc.in, tc.short); got != tc.want {
				t.Errorf("Short(%q, %v) = %q, want %q", tc.in, tc.short, got, tc.want)
			}
		})
	}
}

// A short ID must stay copy-safe: no ellipsis, no marker, nothing a shell would
// have to quote and nothing a flag parser would claim.
func TestShortIsCopySafe(t *testing.T) {
	for _, id := range []string{
		"5bH2mQxKT9wLpN4vRs8kZc1yXd7fGh3jAe6bUi0oQm2nWr5tYv==",
		"-Qt-s7R_oGCru5u3Kv6Y8Q",
	} {
		got := Short(id, true)
		for _, bad := range []string{"…", "...", " ", "*"} {
			if contains(got, bad) {
				t.Errorf("short ID %q contains %q, which breaks pasting it back", got, bad)
			}
		}
		if got[0] == '-' {
			t.Errorf("short ID %q starts with a dash, which the flag parser would eat", got)
		}
		if len(got) != ref.ShortLen {
			t.Errorf("short ID length %d, want %d", len(got), ref.ShortLen)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
