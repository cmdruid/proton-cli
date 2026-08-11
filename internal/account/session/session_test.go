package session

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roman-16/proton-cli/internal/profile"
)

// A session without a sealed key password is one that still needs the account
// password to decrypt anything; Unlocked is how the account commands report the
// difference.
func TestUnlockedTracksTheSealedKeyPassword(t *testing.T) {
	if (&Session{UID: "u"}).Unlocked() {
		t.Error("a session with no blob is not unlocked")
	}
	if !(&Session{UID: "u", EncKeyBlob: "blob"}).Unlocked() {
		t.Error("a session with a blob is unlocked")
	}
	var none *Session
	if none.Unlocked() {
		t.Error("a missing session is not unlocked")
	}
}

func TestPathInStaysOneElementUnderTheSessionDirectory(t *testing.T) {
	d := t.TempDir()
	for _, tc := range []struct{ in, want string }{
		{"work", "work.json"},
		{"", "default.json"},
		{"default", "default.json"},
		{"my-work.2", "my-work.2.json"},
	} {
		name, err := profile.Parse(tc.in)
		if err != nil {
			t.Fatalf("profile.Parse(%q): %v", tc.in, err)
		}
		got := pathIn(d, name)
		want := filepath.Join(d, "sessions", tc.want)
		if got != want {
			t.Errorf("pathIn(%q) = %q, want %q", tc.in, got, want)
		}
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
