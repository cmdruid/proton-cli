package selfmanage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatched(t *testing.T) {
	standalone := "/home/alice/.local/bin/proton"
	cases := []struct {
		name    string
		path    string
		version string
		off     *string
		want    bool
	}{
		{name: "a standalone install that could replace itself", path: standalone, version: "2.4.1", want: true},
		{name: "a development build has nothing to compare", path: standalone, version: "dev"},
		{name: "a build with no version at all", path: standalone, version: ""},
		{name: "an install Nix owns", path: "/nix/store/abc-proton-cli/bin/proton", version: "2.4.1"},
		{name: "an install Homebrew owns", path: "/opt/homebrew/Caskroom/proton-cli/2.4.1/proton", version: "2.4.1"},
		{name: "switched off", path: standalone, version: "2.4.1", off: ptr("1")},
		{name: "switched off with nothing", path: standalone, version: "2.4.1", off: ptr("")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.off != nil {
				t.Setenv(disable, *tc.off)
			} else if _, set := os.LookupEnv(disable); set {
				t.Setenv(disable, "")
				if err := os.Unsetenv(disable); err != nil {
					t.Fatal(err)
				}
			}
			if got := Watched(tc.path, tc.version); got != tc.want {
				t.Errorf("Watched(%q, %q) = %v, want %v", tc.path, tc.version, got, tc.want)
			}
		})
	}
}

func ptr(s string) *string { return &s }

func TestCheckIsDue(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		last time.Time
		want bool
	}{
		{name: "never looked", want: true},
		{name: "just looked", last: now, want: false},
		{name: "an hour ago", last: now.Add(-time.Hour), want: false},
		{name: "a day ago", last: now.Add(-Interval), want: true},
		{name: "a week ago", last: now.Add(-7 * Interval), want: true},
		// A clock that jumped backwards would otherwise never come due again.
		{name: "stamped in the future", last: now.Add(Interval), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Check{CheckedAt: tc.last}).Due(now); got != tc.want {
				t.Errorf("Due = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCheckSurvivesARoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "update-check.json")
	if LoadCheck(path).Due(time.Now()) != true {
		t.Fatal("a check that was never written should read as due")
	}
	stamped := time.Now().Truncate(time.Second)
	if err := SaveCheck(path, Check{CheckedAt: stamped}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := LoadCheck(path); !got.CheckedAt.Equal(stamped) {
		t.Fatalf("CheckedAt = %v, want %v", got.CheckedAt, stamped)
	}
	if LoadCheck(path).Due(time.Now()) {
		t.Fatal("a check just written should not be due")
	}
}

// A courtesy that can fail a command is worse than no courtesy, so a file that
// makes no sense reads as one nobody has written yet.
func TestAnUnreadableCheckReadsAsNone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !LoadCheck(path).Due(time.Now()) {
		t.Fatal("a corrupt check should read as due")
	}
}
