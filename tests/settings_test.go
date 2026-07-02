package tests

import (
	"fmt"
	"testing"
)

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

func TestSettingsSet(t *testing.T) {
	orig := mailViewMode(t)
	target := 0
	if orig == 0 {
		target = 1
	}
	cleanup(t, fmt.Sprintf("Restore mail ViewMode: proton-cli settings set view-mode %d", orig), func() error {
		if _, _, code := run(t, "settings", "set", "view-mode", fmt.Sprintf("%d", orig)); code != 0 {
			return fmt.Errorf("restore exit %d", code)
		}
		return nil
	})

	runOK(t, "settings", "set", "view-mode", fmt.Sprintf("%d", target))
	if got := mailViewMode(t); got != target {
		t.Errorf("ViewMode after set: got %d want %d", got, target)
	}
}

func TestSettingsGet(t *testing.T) {
	stdout := runOK(t, "settings", "get")
	assertContains(t, stdout, "Locale")
}

func TestSettingsMail(t *testing.T) {
	stdout := runOK(t, "settings", "mail")
	assertContains(t, stdout, "Display Name")
	assertContains(t, stdout, "Page Size")
}

func TestSettingsGetJSON(t *testing.T) {
	data := runJSON(t, "settings", "get")
	if _, ok := data["UserSettings"]; !ok {
		t.Error("expected UserSettings key in JSON output")
	}
}
