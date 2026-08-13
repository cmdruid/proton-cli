package tests

import (
	"fmt"
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
	t.Parallel()
	lease(t, mailSettings)
	orig := mailViewMode(t)
	origName, targetName, targetValue := "conversations", "messages", 1
	if orig == 1 {
		origName, targetName, targetValue = "messages", "conversations", 0
	}
	cleanup(t, fmt.Sprintf("Restore mail view mode: proton-cli mail settings set view-mode %s", origName),
		func() error {
			if _, _, code := run(t, "mail", "settings", "set", "view-mode", origName); code != 0 {
				return fmt.Errorf("restore exit %d", code)
			}
			return nil
		})

	runOK(t, "mail", "settings", "set", "view-mode", targetName)
	if got := mailViewMode(t); got != targetValue {
		t.Errorf("ViewMode after setting %q: got %d want %d", targetName, got, targetValue)
	}
	// The numeric form Proton itself uses stays valid.
	runOK(t, "mail", "settings", "set", "view-mode", fmt.Sprintf("%d", targetValue))
}

// With no arguments, `set` lists the writable keys grouped by the settings page
// they come from.
func TestMailSettingsSetListsKeysByPage(t *testing.T) {
	t.Parallel()
	stdout := runOK(t, "mail", "settings", "list")
	for _, want := range []string{"General", "Email privacy", "view-mode", "hide-remote-images"} {
		assertContains(t, stdout, want)
	}
}

func TestMailSettingsSetDryRun(t *testing.T) {
	t.Parallel()
	lease(t, mailSettings)
	orig := mailViewMode(t)
	_, stderr := runOKStderr(t, "--dry-run", "mail", "settings", "set", "view-mode", "messages")
	assertContains(t, stderr, "Dry run")
	if got := mailViewMode(t); got != orig {
		t.Error("--dry-run changed the setting")
	}
}

func TestAccountSettings(t *testing.T) {
	t.Parallel()
	stdout := runOK(t, "account", "settings", "get")
	for _, want := range []string{"Locale", "Date Format", "Time Format", "Week Start"} {
		assertContains(t, stdout, want)
	}
}

// The JSON is the view the command renders, in the CLI's own snake_case names,
// not Proton's envelope passed through.
func TestAccountSettingsJSON(t *testing.T) {
	t.Parallel()
	data := runJSON(t, "account", "settings", "get")
	for _, want := range []string{"locale", "date_format", "week_start", "two_factor"} {
		if _, ok := data[want]; !ok {
			t.Errorf("expected %q in JSON output, got keys %v", want, keysOf(data))
		}
	}
}

// `set` writes one key; `list` is what shows which keys there are.
func TestAccountSettingsListsKeys(t *testing.T) {
	t.Parallel()
	stdout := runOK(t, "account", "settings", "list")
	for _, want := range []string{"Language and time", "locale", "week-start"} {
		assertContains(t, stdout, want)
	}
}

// A display name belongs to an address, not to the mail settings page.
func TestMailSettings(t *testing.T) {
	t.Parallel()
	stdout := runOK(t, "mail", "settings", "get")
	for _, want := range []string{"Page Size", "View Mode", "Draft Format", "Auto-reply"} {
		assertContains(t, stdout, want)
	}
}

func TestCalendarSettings(t *testing.T) {
	t.Parallel()
	stdout := runOK(t, "calendar", "settings", "get")
	assertContains(t, stdout, "Primary Time Zone")
}

func TestCalendarSettingsSetListsKeys(t *testing.T) {
	t.Parallel()
	stdout := runOK(t, "calendar", "settings", "list")
	assertContains(t, stdout, "primary-timezone")
	assertContains(t, stdout, "week-numbers")
}

func TestDriveSettings(t *testing.T) {
	t.Parallel()
	stdout := runOK(t, "drive", "settings", "get")
	assertContains(t, stdout, "Version History")
}

func TestDriveSettingsSetListsKeys(t *testing.T) {
	t.Parallel()
	stdout := runOK(t, "drive", "settings", "list")
	assertContains(t, stdout, "version-history")
}
