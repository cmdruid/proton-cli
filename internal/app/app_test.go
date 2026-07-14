package app

import "testing"

func TestDefaultUserAgent(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "proton-cli/dev"},
		{"dev", "proton-cli/dev"},
		{"1.2.3", "proton-cli/1.2.3"},
		{"v1.4.2", "proton-cli/v1.4.2"},
	}
	for _, tc := range tests {
		if got := defaultUserAgent(tc.in); got != tc.want {
			t.Errorf("defaultUserAgent(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDefaultAppVersion(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "external-proton_cli@0.0.0"},
		{"dev", "external-proton_cli@0.0.0"},
		{"e7e4ad5", "external-proton_cli@0.0.0"},
		{"1.2.3", "external-proton_cli@1.2.3"},
		{"v1.4.2", "external-proton_cli@1.4.2"},
		{"1.9.2-rc.1", "external-proton_cli@1.9.2"},
	}
	for _, tc := range tests {
		if got := defaultAppVersion(tc.in); got != tc.want {
			t.Errorf("defaultAppVersion(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
