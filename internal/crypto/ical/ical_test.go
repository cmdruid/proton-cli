package ical

import (
	"strings"
	"testing"
	"time"
)

func TestField(t *testing.T) {
	text := strings.Join([]string{
		"BEGIN:VCARD",
		"FN:John Doe",
		"item1.EMAIL;PREF=1:john@example.com",
		"TEL;TYPE=CELL:+1234567890",
		"NOTE:hello",
		"END:VCARD",
	}, "\n")

	tests := []struct {
		name, field, want string
	}{
		{"plain field", "FN", "John Doe"},
		{"param field", "TEL", "+1234567890"},
		{"itemN-prefixed param field", "EMAIL", "john@example.com"},
		{"plain note", "NOTE", "hello"},
		{"missing field", "ORG", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Field(text, tc.field); got != tc.want {
				t.Errorf("Field(%q) = %q, want %q", tc.field, got, tc.want)
			}
		})
	}
}

func TestParseTime(t *testing.T) {
	for _, in := range []string{
		"2026-04-16T14:00:00Z",
		"2026-04-16T14:00",
		"2026-04-16T14:00:00",
		"2026-04-16 14:00",
		"2026-04-16",
	} {
		if _, err := ParseTime(in); err != nil {
			t.Errorf("ParseTime(%q) unexpected error: %v", in, err)
		}
	}
	if _, err := ParseTime("not-a-time"); err == nil {
		t.Error("ParseTime(\"not-a-time\") should error")
	}
}

func TestParseTimeLocal(t *testing.T) {
	got, err := ParseTime("2026-04-16T14:30")
	if err != nil {
		t.Fatal(err)
	}
	if got.Location() != time.Local {
		t.Errorf("bare datetime should parse in local zone, got %v", got.Location())
	}
	if got.Hour() != 14 || got.Minute() != 30 {
		t.Errorf("parsed time = %v, want 14:30", got)
	}
}

func TestSignedVEVENT(t *testing.T) {
	start := time.Date(2026, 4, 16, 14, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	out := SignedVEVENT("uid-123", start, end, false, 0)
	for _, want := range []string{"BEGIN:VEVENT", "UID:uid-123", "DTSTART:20260416T140000Z", "DTEND:20260416T150000Z", "SEQUENCE:0", "END:VEVENT"} {
		if !strings.Contains(out, want) {
			t.Errorf("SignedVEVENT missing %q in:\n%s", want, out)
		}
	}
}

func TestSignedVEVENTAllDay(t *testing.T) {
	start := time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC)
	out := SignedVEVENT("uid", start, start.AddDate(0, 0, 1), true, 1)
	if !strings.Contains(out, "DTSTART;VALUE=DATE:20260416") {
		t.Errorf("all-day event should use VALUE=DATE DTSTART, got:\n%s", out)
	}
	if !strings.Contains(out, "SEQUENCE:1") {
		t.Errorf("expected SEQUENCE:1, got:\n%s", out)
	}
}

func TestEncryptedVEVENT(t *testing.T) {
	withLoc := EncryptedVEVENT("Meeting", "Vienna")
	if !strings.Contains(withLoc, "SUMMARY:Meeting") || !strings.Contains(withLoc, "LOCATION:Vienna") {
		t.Errorf("expected SUMMARY+LOCATION, got:\n%s", withLoc)
	}
	noLoc := EncryptedVEVENT("Solo", "")
	if strings.Contains(noLoc, "LOCATION:") {
		t.Errorf("empty location should be omitted, got:\n%s", noLoc)
	}
}

func TestSignedVCard(t *testing.T) {
	withEmail := SignedVCard("John", "john@x.com", "uid-1")
	for _, want := range []string{"FN:John", "UID:uid-1", "EMAIL;PREF=1:john@x.com"} {
		if !strings.Contains(withEmail, want) {
			t.Errorf("SignedVCard missing %q in:\n%s", want, withEmail)
		}
	}
	noEmail := SignedVCard("John", "", "uid-1")
	if strings.Contains(noEmail, "EMAIL") {
		t.Errorf("empty email should be omitted, got:\n%s", noEmail)
	}
}

func TestEncryptedVCard(t *testing.T) {
	full := EncryptedVCard("+123", "a note", "ACME")
	for _, want := range []string{"TEL;PREF=1:+123", "NOTE:a note", "ORG:ACME"} {
		if !strings.Contains(full, want) {
			t.Errorf("EncryptedVCard missing %q in:\n%s", want, full)
		}
	}
	empty := EncryptedVCard("", "", "")
	for _, bad := range []string{"TEL", "NOTE", "ORG"} {
		if strings.Contains(empty, bad) {
			t.Errorf("empty fields should be omitted, got %q in:\n%s", bad, empty)
		}
	}
}

func TestUIDsUnique(t *testing.T) {
	if a, b := EventUID(), EventUID(); a == b {
		t.Error("EventUID returned duplicate values")
	}
	if a, b := ContactUID(), ContactUID(); a == b {
		t.Error("ContactUID returned duplicate values")
	}
	if !strings.HasSuffix(EventUID(), "@proton-cli") {
		t.Error("EventUID should end with @proton-cli")
	}
	if !strings.HasPrefix(ContactUID(), "proton-cli-") {
		t.Error("ContactUID should start with proton-cli-")
	}
}
