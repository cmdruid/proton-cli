package tests

import (
	"strings"
	"testing"
)

// The account tree: the account itself, the session this machine holds, and the
// profiles on it.

func TestAccountGetReportsBothHalves(t *testing.T) {
	t.Parallel()
	stdout := runOK(t, "account", "get")
	// The question people ask is "whose account, and can this machine act as it",
	// so one command has to answer both.
	for _, want := range []string{"Email:", "Storage:", "Profile:", "Session:", "Unlocked:"} {
		assertContains(t, stdout, want)
	}
}

func TestAccountGetJSON(t *testing.T) {
	t.Parallel()
	data := runJSON(t, "account", "get")
	for _, key := range []string{"email", "used_space", "max_space", "profile", "session", "unlocked"} {
		if _, ok := data[key]; !ok {
			t.Errorf("missing %q in %v", key, keysOf(data))
		}
	}
	if unlocked, ok := data["unlocked"].(bool); !ok {
		t.Errorf("unlocked should be a boolean, got %T", data["unlocked"])
	} else if !unlocked {
		t.Error("the suite runs with an unlocked session, so this should be true")
	}
}

// Storage is reported as a share of a total, which is how a person reads it.
func TestAccountGetStorageIsHumanReadable(t *testing.T) {
	t.Parallel()
	stdout := runOK(t, "account", "get")
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "Storage:") {
			if !strings.Contains(line, " of ") || !strings.Contains(line, "%") {
				t.Errorf("storage should read as a share of a total: %q", line)
			}
			return
		}
	}
	t.Error("no Storage line")
}

// Profiles are read off the filesystem, so this works without contacting Proton -
// which is the point: you can see what is configured before signing in.
func TestAccountProfilesList(t *testing.T) {
	t.Parallel()
	profiles := runJSONArray(t, "account", "profiles", "list")
	if len(profiles) == 0 {
		t.Skip("no saved session, so nothing to list")
	}
	found := false
	for _, p := range profiles {
		row, ok := p.(map[string]interface{})
		if !ok {
			t.Fatalf("unexpected row shape: %T", p)
		}
		for _, key := range []string{"name", "unlocked"} {
			if _, ok := row[key]; !ok {
				t.Errorf("missing %q in %v", key, keysOf(row))
			}
		}
		if row["name"] == "default" {
			found = true
		}
	}
	if !found {
		t.Error("the default profile should be listed")
	}
}

func TestAccountSessionsListMarksTheCurrentOne(t *testing.T) {
	t.Parallel()
	sessions := runJSONArray(t, "account", "sessions", "list")
	if len(sessions) == 0 {
		t.Fatal("a signed-in account has at least this session")
	}
	current := 0
	for _, s := range sessions {
		row := s.(map[string]interface{})
		for _, key := range []string{"uid", "client_id", "create_time", "current"} {
			if _, ok := row[key]; !ok {
				t.Errorf("missing %q in %v", key, keysOf(row))
			}
		}
		if row["current"] == true {
			current++
		}
	}
	if current != 1 {
		t.Errorf("exactly one session is the current one, got %d", current)
	}
}

// ── settings ──

func TestAccountSettingsGetAndList(t *testing.T) {
	t.Parallel()
	stdout := runOK(t, "account", "settings", "get")
	for _, want := range []string{"Locale:", "Date Format:", "Telemetry:"} {
		assertContains(t, stdout, want)
	}

	// The listing is a collection, so it comes out of --output json as one.
	keys := runJSONArray(t, "account", "settings", "list")
	if len(keys) == 0 {
		t.Fatal("there are writable account settings")
	}
	seen := map[string]bool{}
	for _, k := range keys {
		row := k.(map[string]interface{})
		for _, field := range []string{"key", "values", "page", "description"} {
			if _, ok := row[field]; !ok {
				t.Errorf("missing %q in %v", field, keysOf(row))
			}
		}
		seen[row["key"].(string)] = true
	}
	for _, want := range []string{"locale", "date-format", "week-start", "telemetry"} {
		if !seen[want] {
			t.Errorf("expected %q among the writable keys", want)
		}
	}
}

// The two accounts are in the two password modes, and `get` says which.
//
// This is the suite's only check on two-password mode, and it is a check on the
// accounts as much as on the command: the secondary is signed in through the
// second password every run, so an account switched back to one password would
// take that coverage away without anything failing. This is what fails.
func TestAccountSettingsReportTwoPasswordMode(t *testing.T) {
	t.Parallel()
	if mode := runJSON(t, "account", "settings", "get")["two_password_mode"]; mode != "off" {
		t.Errorf("the primary account's two_password_mode = %v, want off", mode)
	}
	if mode := runJSONSecondary(t, "account", "settings", "get")["two_password_mode"]; mode != "on" {
		t.Errorf("the secondary account's two_password_mode = %v, want on - the suite covers"+
			" two-password mode by signing that account in, so it has to stay in it", mode)
	}
}

// Reads and writes speak the same vocabulary: what `get` shows is what `set` takes.
func TestAccountSettingsRoundTripsNames(t *testing.T) {
	t.Parallel()
	lease(t, accountSettings)
	before := runJSON(t, "account", "settings", "get")
	original, ok := before["week_start"].(string)
	if !ok {
		t.Fatalf("week_start should be a name, got %T", before["week_start"])
	}
	target := "monday"
	if original == "monday" {
		target = "saturday"
	}
	cleanupRun(t, "Restore week-start", "account", "settings", "set", "week-start", original)

	runOK(t, "account", "settings", "set", "week-start", target)
	after := runJSON(t, "account", "settings", "get")
	if after["week_start"] != target {
		t.Errorf("week_start = %v, want %q", after["week_start"], target)
	}
}
