package tests

import (
	"fmt"
	"strings"
	"testing"
)

// Settings are scoped per product, mirroring Proton's own settings app: bare
// `settings` is the account, and each product carries its own tree.

func mailViewMode(t *testing.T) int {
	t.Helper()
	data := runJSON(t, "api", "GET", "/mail/v4/settings")
	ms, ok := data["MailSettings"].(map[string]interface{})
	if !ok {
		t.Fatalf("no MailSettings in response: %v", data)
	}
	vm, ok := ms["ViewMode"].(float64)
	if !ok {
		t.Fatalf("no ViewMode in MailSettings: %v", ms)
	}
	return int(vm)
}

// Named values are the point of the typed key table: nobody should have to
// remember that "conversations" is zero.
func TestMailSettingsSetByName(t *testing.T) {
	orig := mailViewMode(t)
	origName, targetName, targetValue := "conversations", "messages", 1
	if orig == 1 {
		origName, targetName, targetValue = "messages", "conversations", 0
	}
	cleanup(t, fmt.Sprintf("Restore mail view mode: proton-cli mail settings set view-mode %s", origName),
		func() error {
			if _, _, code := run(t, "mail", "account", "settings", "set", "view-mode", origName); code != 0 {
				return fmt.Errorf("restore exit %d", code)
			}
			return nil
		})

	runOK(t, "mail", "account", "settings", "set", "view-mode", targetName)
	if got := mailViewMode(t); got != targetValue {
		t.Errorf("ViewMode after setting %q: got %d want %d", targetName, got, targetValue)
	}
	// The numeric form Proton itself uses stays valid.
	runOK(t, "mail", "account", "settings", "set", "view-mode", fmt.Sprintf("%d", targetValue))
}

func TestMailSettingsSetRejectsValuesOutsideTheDomain(t *testing.T) {
	tests := []struct{ key, value, want string }{
		{"view-mode", "threads", "conversations, messages"},
		{"view-mode", "7", "conversations, messages"},
		{"delay-send", "999", "0-20 (seconds)"},
		{"page-size", "3", "50, 100, 200"},
		{"draft-type", "text/markdown", "text/html, text/plain"},
	}
	for _, tt := range tests {
		_, stderr, code := run(t, "mail", "account", "settings", "set", tt.key, tt.value)
		if code == 0 {
			t.Errorf("%s %s should have been rejected", tt.key, tt.value)
			continue
		}
		if !strings.Contains(stderr, tt.want) {
			t.Errorf("%s %s: stderr = %q, want it to list %q", tt.key, tt.value, stderr, tt.want)
		}
	}
}

func TestMailSettingsSetUnknownKey(t *testing.T) {
	_, stderr, code := run(t, "mail", "account", "settings", "set", "no-such-key", "1")
	if code == 0 {
		t.Error("an unknown key should be rejected")
	}
	assertContains(t, stderr, "unknown mail setting")
	assertContains(t, stderr, "mail settings list")
}

// With no arguments, `set` lists the writable keys grouped by the settings page
// they come from.
func TestMailSettingsSetListsKeysByPage(t *testing.T) {
	stdout := runOK(t, "mail", "settings", "list")
	for _, want := range []string{"General", "Email privacy", "view-mode", "hide-remote-images"} {
		assertContains(t, stdout, want)
	}
}

func TestMailSettingsSetDryRun(t *testing.T) {
	orig := mailViewMode(t)
	_, stderr := runOKStderr(t, "--dry-run", "mail", "account", "settings", "set", "view-mode", "messages")
	assertContains(t, stderr, "Dry run")
	if got := mailViewMode(t); got != orig {
		t.Error("--dry-run changed the setting")
	}
}

func TestAccountSettings(t *testing.T) {
	stdout := runOK(t, "account", "settings", "get")
	for _, want := range []string{"Locale", "Date Format", "Time Format", "Week Start"} {
		assertContains(t, stdout, want)
	}
}

func TestAccountSettingsJSON(t *testing.T) {
	data := runJSON(t, "account", "settings", "get")
	if _, ok := data["UserSettings"]; !ok {
		t.Error("expected UserSettings key in JSON output")
	}
}

func TestAccountSettingsSetListsKeys(t *testing.T) {
	stdout := runOK(t, "account", "settings", "set")
	for _, want := range []string{"Language and time", "locale", "week-start"} {
		assertContains(t, stdout, want)
	}
}

func TestMailSettings(t *testing.T) {
	stdout := runOK(t, "mail", "settings", "get")
	for _, want := range []string{"Display Name", "Page Size", "View Mode", "Auto-reply"} {
		assertContains(t, stdout, want)
	}
}

func TestCalendarSettings(t *testing.T) {
	stdout := runOK(t, "calendar", "settings", "get")
	assertContains(t, stdout, "Primary Time Zone")
}

func TestCalendarSettingsSetListsKeys(t *testing.T) {
	stdout := runOK(t, "calendar", "settings", "list")
	assertContains(t, stdout, "primary-timezone")
	assertContains(t, stdout, "week-numbers")
}

func TestDriveSettings(t *testing.T) {
	stdout := runOK(t, "drive", "settings", "get")
	assertContains(t, stdout, "Version History")
}

func TestDriveSettingsSetListsKeys(t *testing.T) {
	stdout := runOK(t, "drive", "settings", "list")
	assertContains(t, stdout, "version-history")
}
