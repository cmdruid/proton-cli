package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmdruid/proton-cli/internal/profile"
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

// A reader must never see a half-written session. Save replaces the file, so a
// command running alongside a save finds either the old session or the new one.
func TestSaveReplacesTheFileRatherThanRewritingIt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	name, err := profile.Parse("primary")
	if err != nil {
		t.Fatal(err)
	}

	if err := Save(name, &Session{UID: "uid", AccessToken: "first", RefreshToken: "r1"}); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 200 {
			_ = Save(name, &Session{
				UID:          "uid",
				AccessToken:  fmt.Sprintf("token-%d", i),
				RefreshToken: fmt.Sprintf("refresh-%d", i),
			})
		}
	}()
	for {
		select {
		case <-done:
			return
		default:
		}
		got, err := Load(name)
		if err != nil {
			t.Fatalf("Load while saving: %v", err)
		}
		if got == nil {
			t.Fatal("Load found no session while one was being saved")
		}
		if got.UID != "uid" || got.AccessToken == "" || got.RefreshToken == "" {
			t.Fatalf("Load saw a partial session: %+v", got)
		}
	}
}

func TestSaveLeavesNoLeftoverFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	name, err := profile.Parse("primary")
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := Save(name, &Session{UID: "uid", AccessToken: "a", RefreshToken: "r"}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(dir, "proton-cli", "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("the sessions directory holds %v, want just the session file", names)
	}
}

func TestSavedSessionIsReadableOnlyByItsOwner(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	name, err := profile.Parse("primary")
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(name, &Session{UID: "uid", AccessToken: "a", RefreshToken: "r"}); err != nil {
		t.Fatal(err)
	}
	p, err := Path(name)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600", perm)
	}
}
