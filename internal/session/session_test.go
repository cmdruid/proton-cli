package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathInNamedProfile(t *testing.T) {
	d := t.TempDir()
	got := pathIn(d, "work")
	want := filepath.Join(d, "sessions", "work.json")
	if got != want {
		t.Errorf("pathIn(work) = %q, want %q", got, want)
	}
}

func TestPathInEmptyTreatedAsDefault(t *testing.T) {
	d := t.TempDir()
	got := pathIn(d, "")
	want := filepath.Join(d, "sessions", "default.json")
	if got != want {
		t.Errorf("pathIn(\"\") = %q, want %q", got, want)
	}
}

func TestPathInDefaultNoFiles(t *testing.T) {
	d := t.TempDir()
	got := pathIn(d, "default")
	want := filepath.Join(d, "sessions", "default.json")
	if got != want {
		t.Errorf("pathIn(default) with no files = %q, want %q", got, want)
	}
}

func TestPathInDefaultLegacyMigration(t *testing.T) {
	d := t.TempDir()
	legacy := filepath.Join(d, "session.json")
	if err := os.WriteFile(legacy, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := pathIn(d, "default"); got != legacy {
		t.Errorf("pathIn(default) should return legacy path %q, got %q", legacy, got)
	}
}

func TestPathInDefaultPrefersNewOverLegacy(t *testing.T) {
	d := t.TempDir()
	newPath := filepath.Join(d, "sessions", "default.json")
	if err := os.MkdirAll(filepath.Dir(newPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "session.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := pathIn(d, "default"); got != newPath {
		t.Errorf("pathIn(default) should prefer new path %q over legacy, got %q", newPath, got)
	}
}

func TestPathInNamedNeverUsesLegacy(t *testing.T) {
	d := t.TempDir()
	// A legacy file must never be picked up for a non-default profile.
	if err := os.WriteFile(filepath.Join(d, "session.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	got := pathIn(d, "work")
	if got != filepath.Join(d, "sessions", "work.json") {
		t.Errorf("named profile must ignore legacy file, got %q", got)
	}
}
