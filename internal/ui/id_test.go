package ui

import "testing"

func TestShort(t *testing.T) {
	const full = "5bH2mQxKT9wLpN4vRs8kZc1yXd7fGh3jAe6bUi0oQm2nWr5tYv=="
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
// have to quote.
func TestShortIsCopySafe(t *testing.T) {
	got := Short("5bH2mQxKT9wLpN4vRs8kZc1yXd7fGh3jAe6bUi0oQm2nWr5tYv==", true)
	for _, bad := range []string{"…", "...", " ", "*"} {
		if contains(got, bad) {
			t.Errorf("short ID %q contains %q, which breaks pasting it back", got, bad)
		}
	}
	if len(got) != ShortIDLen {
		t.Errorf("short ID length %d, want %d", len(got), ShortIDLen)
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
