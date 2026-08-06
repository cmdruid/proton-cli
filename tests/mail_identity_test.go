package tests

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Identity and auto-reply are the two structured pages under `mail settings`.
// Both tests capture the account's current state first and restore it in
// cleanup, since they change real settings.

// ── mail settings addresses ──

func TestMailSettingsAddressesList(t *testing.T) {
	stdout := runOK(t, "mail", "settings", "addresses", "list")
	assertContains(t, stdout, "EMAIL")
	assertContains(t, stdout, selfEmail())
}

func TestMailSettingsAddressesGetByEmail(t *testing.T) {
	stdout := runOK(t, "mail", "settings", "addresses", "get", selfEmail())
	assertField(t, stdout, "Email:", selfEmail())
	assertContains(t, stdout, "Can Send:")
}

func TestMailSettingsAddressesUpdateSignature(t *testing.T) {
	addrID := primaryAddressID(t)
	original := addressSignature(t, addrID)
	restoreSignature(t, addrID, original)

	marker := testID() + "-signature"
	runOK(t, "mail", "settings", "addresses", "update", "--signature", marker, "--", addrID)
	assertContains(t, runOK(t, "mail", "settings", "addresses", "get", "--", addrID), marker)

	// Plain text is stored as HTML, so newlines survive as line breaks.
	runOK(t, "mail", "settings", "addresses", "update", "--signature", "line one\nline two", "--", addrID)
	if got := addressSignature(t, addrID); got != "line one<br>line two" {
		t.Errorf("stored signature = %q, want the newline turned into a line break", got)
	}

	runOK(t, "mail", "settings", "addresses", "update", "--clear-signature", "--", addrID)
	assertContains(t, runOK(t, "mail", "settings", "addresses", "get", "--", addrID), "(none)")
}

func TestMailSettingsAddressesUpdateRequiresAField(t *testing.T) {
	_, stderr, code := run(t, "mail", "settings", "addresses", "update", selfEmail())
	if code == 0 {
		t.Error("an update with nothing to change should fail")
	}
	assertContains(t, stderr, "Nothing to change")
}

func restoreSignature(t *testing.T, addrID, original string) {
	t.Helper()
	cleanup(t, "Restore the address signature", func() error {
		args := []string{"mail", "settings", "addresses", "update", "--html", "--signature", original, "--", addrID}
		if strings.TrimSpace(original) == "" {
			args = []string{"mail", "settings", "addresses", "update", "--clear-signature", "--", addrID}
		}
		if _, stderr, code := run(t, args...); code != 0 {
			return fmt.Errorf("exit %d: %s", code, strings.TrimSpace(stderr))
		}
		return nil
	})
}

// ── mail settings autoreply ──

// TestMailSettingsAutoreply drives the whole auto-reply lifecycle. The schedule
// is deliberately set years into the future so the account can never actually
// auto-reply to anyone while the suite runs.
func TestMailSettingsAutoreply(t *testing.T) {
	before := autoReplyState(t)

	_, stderr, code := run(t, "mail", "settings", "autoreply", "set",
		"--repeat", "fixed",
		"--start", "2099-07-01T09:00", "--end", "2099-07-14T18:00",
		"--zone", "Europe/Vienna",
		"--message", "Out of office (integration test).")
	if code != 0 {
		if strings.Contains(stderr, "upgrade") || strings.Contains(stderr, "paid") {
			t.Skip("auto-reply is a paid feature and this account does not have it")
		}
		t.Fatalf("autoreply set failed (exit %d): %s", code, stderr)
	}
	restoreAutoReply(t, before)

	got := autoReplyState(t)
	if !got.Enabled {
		t.Error("setting a schedule should enable the auto-reply")
	}
	if got.Repeat != "fixed" {
		t.Errorf("repeat = %q, want fixed", got.Repeat)
	}
	// What `get` prints is what `set` accepts.
	if got.Start != "2099-07-01T09:00" || got.End != "2099-07-14T18:00" {
		t.Errorf("schedule = %s to %s, want the times we set back verbatim", got.Start, got.End)
	}
	if got.Zone != "Europe/Vienna" {
		t.Errorf("zone = %q", got.Zone)
	}

	runOK(t, "mail", "settings", "autoreply", "disable")
	if autoReplyState(t).Enabled {
		t.Error("disable did not turn the auto-reply off")
	}

	runOK(t, "mail", "settings", "autoreply", "enable")
	after := autoReplyState(t)
	if !after.Enabled {
		t.Error("enable did not turn the auto-reply back on")
	}
	if after.Start != got.Start {
		t.Errorf("toggling must preserve the schedule; start = %q, want %q", after.Start, got.Start)
	}

	// The status shows up on the settings overview too.
	assertContains(t, runOK(t, "mail", "settings", "get"), "Auto-reply:")
}

func TestMailSettingsAutoreplyRejectsMismatchedSchedules(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--repeat", "permanent", "--start", "09:00", "--message", "x"}, "takes no --start"},
		{[]string{"--repeat", "daily", "--start", "09:00", "--end", "17:00", "--message", "x"}, "needs --days"},
		{[]string{"--repeat", "weekly", "--start", "mon:09:00", "--end", "fri:17:00",
			"--days", "mon", "--message", "x"}, "--days applies to --repeat daily"},
		{[]string{"--repeat", "hourly", "--message", "x"}, "unknown repeat"},
		{[]string{"--repeat", "permanent"}, "--message is required"},
	}
	for _, tt := range tests {
		_, stderr, code := run(t, append([]string{"mail", "settings", "autoreply", "set"}, tt.args...)...)
		if code == 0 {
			t.Errorf("%v should have been rejected", tt.args)
			continue
		}
		if !strings.Contains(stderr, tt.want) {
			t.Errorf("%v: stderr = %q, want it to mention %q", tt.args, stderr, tt.want)
		}
	}
}

type autoReply struct {
	Enabled bool     `json:"enabled"`
	Repeat  string   `json:"repeat"`
	Start   string   `json:"start"`
	End     string   `json:"end"`
	Days    []string `json:"days"`
	Zone    string   `json:"zone"`
	Message string   `json:"message"`
}

func autoReplyState(t *testing.T) autoReply {
	t.Helper()
	raw := runOK(t, "mail", "settings", "autoreply", "--output", "json")
	var out autoReply
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("could not read the auto-reply: %v\n%s", err, truncateOutput(raw))
	}
	return out
}

// restoreAutoReply puts the account's original auto-reply back, so a test run
// never leaves one armed.
func restoreAutoReply(t *testing.T, before autoReply) {
	t.Helper()
	cleanup(t, "Restore the auto-reply", func() error {
		if !before.Enabled {
			if _, stderr, code := run(t, "mail", "settings", "autoreply", "disable"); code != 0 {
				return fmt.Errorf("exit %d: %s", code, strings.TrimSpace(stderr))
			}
			return nil
		}
		args := []string{"mail", "settings", "autoreply", "set",
			"--repeat", before.Repeat, "--html", "--message", before.Message}
		if before.Zone != "" {
			args = append(args, "--zone", before.Zone)
		}
		if before.Repeat != "permanent" {
			args = append(args, "--start", before.Start, "--end", before.End)
		}
		if len(before.Days) > 0 {
			args = append(args, "--days", strings.Join(before.Days, ","))
		}
		if _, stderr, code := run(t, args...); code != 0 {
			return fmt.Errorf("exit %d: %s", code, strings.TrimSpace(stderr))
		}
		return nil
	})
}
