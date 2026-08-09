package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A profile is the binding between a name and an account. `account login`
// creates it, the session file is where it lives, and PROTON_PROFILE or
// --profile chooses between them. Nothing else can name an account, which is
// what these tests are here to hold.

// TestProfileFromEnv: PROTON_PROFILE selects the profile with no --profile flag.
// The harness runs every command that way, so this asserts the account it lands
// on is the one the variable names.
func TestProfileFromEnv(t *testing.T) {
	acct := runJSONSecondary(t, "account", "get")
	email, _ := acct["email"].(string)
	if !strings.EqualFold(email, secondaryEmail()) {
		t.Errorf("PROTON_PROFILE=%s acted as %q, want %q", secondary, email, secondaryEmail())
	}
}

// TestProfileNotSignedInIsRefusedLocally: a profile nobody signed in acts as
// nobody. It used to fall back to whatever credentials were in the environment,
// which meant a mistyped profile name quietly reached a different account.
//
// The refusal happens before the network, so it holds even with no session and
// no connection.
func TestProfileNotSignedInIsRefusedLocally(t *testing.T) {
	_, stderr, code := runWithEnv(t, map[string]string{"PROTON_PROFILE": "no-such-" + testID()},
		"mail", "messages", "list")
	if code != 2 {
		t.Errorf("expected exit 2 for a profile with no session, got %d", code)
	}
	assertContains(t, stderr, "not signed in")
	assertContains(t, stderr, "account login")
}

// TestProfileSessionsAreSeparateFiles: each profile keeps its own session, so
// two accounts on one machine cannot clobber each other.
func TestProfileSessionsAreSeparateFiles(t *testing.T) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("user config dir: %v", err)
	}
	for _, name := range []string{primary, secondary} {
		path := filepath.Join(configDir, "proton-cli", "sessions", name+".json")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected a session file for %q at %s: %v", name, path, err)
		}
	}
}

// TestProfilesListNamesEverySignedInAccount: the session directory is the whole
// truth about who is signed in here, so listing it needs no API call and cannot
// disagree with what a command would act as.
func TestProfilesListNamesEverySignedInAccount(t *testing.T) {
	rows := runJSONArray(t, "account", "profiles", "list")
	seen := map[string]string{}
	for _, r := range rows {
		m := r.(map[string]interface{})
		name, _ := m["name"].(string)
		email, _ := m["email"].(string)
		seen[name] = email
	}
	for name, want := range map[string]string{primary: selfEmail(), secondary: secondaryEmail()} {
		if got, ok := seen[name]; !ok {
			t.Errorf("profile %q is signed in but missing from the list", name)
		} else if !strings.EqualFold(got, want) {
			t.Errorf("profile %q lists %q, want %q", name, got, want)
		}
	}
}
