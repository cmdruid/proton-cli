package hv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A named helper is used instead of the embedded one, and is found even in a
// build that embeds nothing - which is the case the override exists for.
func TestANamedHelperIsUsedEvenWhenNoneIsEmbedded(t *testing.T) {
	if len(helperBinary) != 0 {
		t.Skip("this build embeds a helper; the override is checked before it either way")
	}
	want := executableFile(t, "helper")
	t.Setenv(EnvHelper, want)

	got, err := helperPath()
	if err != nil {
		t.Fatalf("helperPath() with %s set: %v", EnvHelper, err)
	}
	if got != want {
		t.Errorf("helperPath() = %q, want the named helper %q", got, want)
	}
}

// A relative path is made absolute, because the helper is exec'd later and from
// nowhere in particular.
func TestANamedHelperIsResolvedToAnAbsolutePath(t *testing.T) {
	abs := executableFile(t, "helper")
	t.Chdir(filepath.Dir(abs))
	t.Setenv(EnvHelper, "./"+filepath.Base(abs))

	got, err := helperPath()
	if err != nil {
		t.Fatalf("helperPath() with a relative %s: %v", EnvHelper, err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("helperPath() = %q, want an absolute path", got)
	}
}

// A wrong path reads as a wrong path. Without this it surfaces as human
// verification being unavailable, which sends somebody looking in the wrong
// place entirely.
func TestAHelperThatIsNotThereSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name  string
		path  func(t *testing.T) string
		wants string
	}{
		{"missing", func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent") }, "cannot be read"},
		{"a directory", func(t *testing.T) string { return t.TempDir() }, "is a directory"},
		{"not executable", func(t *testing.T) string {
			if runtime.GOOS == "windows" {
				t.Skip("permission bits do not carry this meaning on Windows")
			}
			p := filepath.Join(t.TempDir(), "helper")
			if err := os.WriteFile(p, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			return p
		}, "not executable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvHelper, tc.path(t))
			_, err := helperPath()
			if err == nil {
				t.Fatalf("helperPath() accepted a helper that is %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wants) || !strings.Contains(err.Error(), EnvHelper) {
				t.Errorf("error %q says neither %q nor %s", err, tc.wants, EnvHelper)
			}
		})
	}
}

func executableFile(t *testing.T, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}
