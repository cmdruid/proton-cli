package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFromPartsPrefersEncKeyBlob(t *testing.T) {
	s := FromParts("uid", "acc", "ref", "encrypted-blob", "legacy-cleartext", "App", "https://x")
	if s.UID != "uid" || s.AccessToken != "acc" || s.RefreshToken != "ref" {
		t.Errorf("tokens not carried: %+v", s)
	}
	if s.EncKeyBlob != "encrypted-blob" {
		t.Errorf("EncKeyBlob = %q, want encrypted-blob", s.EncKeyBlob)
	}
	if s.SaltedKeyPass != "" {
		t.Errorf("once a blob exists, cleartext must be dropped; got %q", s.SaltedKeyPass)
	}
}

func TestFromPartsPreservesLegacyUntilMigrated(t *testing.T) {
	s := FromParts("uid", "acc", "ref", "", "legacy-cleartext", "App", "https://x")
	if s.EncKeyBlob != "" {
		t.Errorf("EncKeyBlob should be empty, got %q", s.EncKeyBlob)
	}
	if s.SaltedKeyPass != "legacy-cleartext" {
		t.Errorf("legacy cleartext should be preserved until migrated, got %q", s.SaltedKeyPass)
	}
}

func TestFromPartsNoKeyMaterial(t *testing.T) {
	s := FromParts("uid", "acc", "ref", "", "", "App", "https://x")
	if s.EncKeyBlob != "" || s.SaltedKeyPass != "" {
		t.Errorf("expected no key material, got blob=%q skp=%q", s.EncKeyBlob, s.SaltedKeyPass)
	}
}

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

func TestSessionMarshalUsesEncKeyBlobNotCleartext(t *testing.T) {
	b, err := json.Marshal(Session{UID: "u", AccessToken: "a", RefreshToken: "r", EncKeyBlob: "blob"})
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if !strings.Contains(out, `"enc_key_blob":"blob"`) {
		t.Errorf("expected enc_key_blob in JSON, got: %s", out)
	}
	if strings.Contains(out, "salted_key_pass") {
		t.Errorf("salted_key_pass must be omitted when empty, got: %s", out)
	}
}

func TestSessionUnmarshalLegacyCleartext(t *testing.T) {
	var s Session
	if err := json.Unmarshal([]byte(`{"uid":"u","access_token":"a","refresh_token":"r","salted_key_pass":"legacy"}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.SaltedKeyPass != "legacy" {
		t.Errorf("legacy salted_key_pass should still parse, got %q", s.SaltedKeyPass)
	}
	if s.EncKeyBlob != "" {
		t.Errorf("EncKeyBlob should be empty for a legacy file, got %q", s.EncKeyBlob)
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
