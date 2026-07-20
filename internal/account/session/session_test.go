package session

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestFromPartsCarriesEncKeyBlob(t *testing.T) {
	s := FromParts("uid", "acc", "ref", "encrypted-blob", "App", "https://x")
	if s.UID != "uid" || s.AccessToken != "acc" || s.RefreshToken != "ref" {
		t.Errorf("tokens not carried: %+v", s)
	}
	if s.EncKeyBlob != "encrypted-blob" {
		t.Errorf("EncKeyBlob = %q, want encrypted-blob", s.EncKeyBlob)
	}
}

func TestFromPartsNoKeyMaterial(t *testing.T) {
	s := FromParts("uid", "acc", "ref", "", "App", "https://x")
	if s.EncKeyBlob != "" {
		t.Errorf("expected no key material, got blob=%q", s.EncKeyBlob)
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

func TestPathInDefault(t *testing.T) {
	d := t.TempDir()
	got := pathIn(d, "default")
	want := filepath.Join(d, "sessions", "default.json")
	if got != want {
		t.Errorf("pathIn(default) = %q, want %q", got, want)
	}
}

func TestSessionMarshalUsesEncKeyBlob(t *testing.T) {
	b, err := json.Marshal(Session{UID: "u", AccessToken: "a", RefreshToken: "r", EncKeyBlob: "blob"})
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if !strings.Contains(out, `"enc_key_blob":"blob"`) {
		t.Errorf("expected enc_key_blob in JSON, got: %s", out)
	}
	if strings.Contains(out, "salted_key_pass") {
		t.Errorf("salted_key_pass field should not exist, got: %s", out)
	}
}
