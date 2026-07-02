package app

import "testing"

func TestProfileEnvSegment(t *testing.T) {
	tests := []struct{ in, want string }{
		{"work", "WORK"},
		{"default", "DEFAULT"},
		{"my-work", "MY_WORK"},
		{"personal.2", "PERSONAL_2"},
		{"a b", "A_B"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := profileEnvSegment(tc.in); got != tc.want {
			t.Errorf("profileEnvSegment(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEnvForProfile(t *testing.T) {
	t.Run("scoped wins over unscoped", func(t *testing.T) {
		t.Setenv("PROTON_USER", "plain@x.com")
		t.Setenv("PROTON_WORK_USER", "work@x.com")
		if got := envForProfile("work", "USER"); got != "work@x.com" {
			t.Errorf("got %q, want work@x.com", got)
		}
	})
	t.Run("falls back to unscoped when scoped unset", func(t *testing.T) {
		t.Setenv("PROTON_USER", "plain@x.com")
		if got := envForProfile("work", "USER"); got != "plain@x.com" {
			t.Errorf("got %q, want plain@x.com", got)
		}
	})
	t.Run("empty scoped falls through to unscoped", func(t *testing.T) {
		t.Setenv("PROTON_USER", "plain@x.com")
		t.Setenv("PROTON_WORK_USER", "")
		if got := envForProfile("work", "USER"); got != "plain@x.com" {
			t.Errorf("got %q, want plain@x.com", got)
		}
	})
	t.Run("segment normalization is applied", func(t *testing.T) {
		t.Setenv("PROTON_MY_WORK_API_URL", "https://scoped.example")
		if got := envForProfile("my-work", "API_URL"); got != "https://scoped.example" {
			t.Errorf("got %q, want https://scoped.example", got)
		}
	})
	t.Run("neither set returns empty", func(t *testing.T) {
		// A base no ambient env would ever define, so this is hermetic.
		if got := envForProfile("work", "DOES_NOT_EXIST_XYZ"); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}
