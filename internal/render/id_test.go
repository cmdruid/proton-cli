package render

import "testing"

func TestShortID(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		short bool
		want  string
	}{
		{"short=false passthrough", "abcdefghijklmnop==", false, "abcdefghijklmnop=="},
		{"short=true 88-char ID", "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnop+/+/AAAA==", true, "abcdefgh"},
		{"short=true exactly 8 chars", "abcdefgh", true, "abcdefgh"},
		{"short=true 7 chars (untouched)", "abcdefg", true, "abcdefg"},
		{"empty input", "", true, ""},
		{"short=true exactly 9 chars", "abcdefghi", true, "abcdefgh"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShortID(tc.id, tc.short); got != tc.want {
				t.Errorf("ShortID(%q,%v) = %q, want %q", tc.id, tc.short, got, tc.want)
			}
		})
	}
}
