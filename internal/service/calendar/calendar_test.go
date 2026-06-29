package calendar

import (
	"crypto/sha1" //nolint:gosec // matching Proton's attendee-token algorithm
	"encoding/hex"
	"testing"
)

func TestAttendeeTokenMatchesSHA1OfUIDAndCanonicalEmail(t *testing.T) {
	// Casing/whitespace are canonicalised away, so these inputs hash identically.
	got := attendeeToken("event-uid-42", "  Alice@Proton.me ")

	sum := sha1.Sum([]byte("event-uid-42" + "alice@proton.me")) //nolint:gosec
	want := hex.EncodeToString(sum[:])

	if got != want {
		t.Errorf("attendeeToken = %q, want %q", got, want)
	}
	if len(got) != 40 {
		t.Errorf("expected a 40-char hex SHA-1, got %d chars", len(got))
	}
}

func TestICalTriggerConvertsDurationsToNegativeTriggers(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"15m", "-PT15M"},
		{"1h", "-PT60M"},
		{"90m", "-PT90M"},
		{"1d", "-P1D"},
		{"2d", "-P2D"},
	}
	for _, c := range cases {
		got, err := icalTrigger(c.in)
		if err != nil {
			t.Errorf("icalTrigger(%q) returned error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("icalTrigger(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if _, err := icalTrigger("0m"); err == nil {
		t.Error("expected an error for a zero-length reminder")
	}
	if _, err := icalTrigger("not-a-duration"); err == nil {
		t.Error("expected an error for an unparseable reminder")
	}
}

func TestBuildRemindersMapsToDeviceNotifications(t *testing.T) {
	out, err := buildReminders([]string{"15m", "1d"})
	if err != nil {
		t.Fatalf("buildReminders error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(out))
	}
	if out[0]["Type"] != 1 || out[0]["Trigger"] != "-PT15M" {
		t.Errorf("first reminder = %v, want Type 1 / -PT15M", out[0])
	}
	if out[1]["Trigger"] != "-P1D" {
		t.Errorf("second reminder trigger = %v, want -P1D", out[1]["Trigger"])
	}

	if out, _ := buildReminders(nil); out != nil {
		t.Errorf("expected nil for no reminders, got %v", out)
	}
}
