package ical

import (
	"strings"
	"testing"
	"time"
)

func TestAttendeesVEVENTEmitsTokenedAttendeeLines(t *testing.T) {
	out := AttendeesVEVENT("uid-123", []Attendee{
		{Email: "alice@proton.me", Token: "tok-alice"},
		{Email: "bob@example.com", Token: "tok-bob"},
	})
	for _, want := range []string{
		"BEGIN:VEVENT",
		"UID:uid-123",
		"ATTENDEE;CN=alice@proton.me;ROLE=REQ-PARTICIPANT;RSVP=TRUE;PARTSTAT=NEEDS-ACTION;X-PM-TOKEN=tok-alice:mailto:alice@proton.me",
		"ATTENDEE;CN=bob@example.com;ROLE=REQ-PARTICIPANT;RSVP=TRUE;PARTSTAT=NEEDS-ACTION;X-PM-TOKEN=tok-bob:mailto:bob@example.com",
		"END:VEVENT",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("AttendeesVEVENT missing %q in:\n%s", want, out)
		}
	}
}

func TestInviteICSIsAMethodRequestInvitation(t *testing.T) {
	start := time.Date(2026, 8, 16, 14, 0, 0, 0, time.UTC)
	out := InviteICS("uid-9", "Quarterly Sync", "Vienna", "agenda items", start, start.Add(time.Hour), false,
		"organizer@proton.me", []Attendee{{Email: "alice@proton.me", Token: "tok-1"}})
	for _, want := range []string{
		"BEGIN:VCALENDAR", "METHOD:REQUEST",
		"BEGIN:VEVENT", "UID:uid-9",
		"DTSTART:20260816T140000Z", "DTEND:20260816T150000Z",
		"SUMMARY:Quarterly Sync", "LOCATION:Vienna", "DESCRIPTION:agenda items",
		"ORGANIZER;CN=organizer@proton.me:mailto:organizer@proton.me",
		"X-PM-TOKEN=tok-1:mailto:alice@proton.me",
		"END:VEVENT", "END:VCALENDAR",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("InviteICS missing %q in:\n%s", want, out)
		}
	}
}

func TestReplyICSEmitsMethodReplyAndPartstat(t *testing.T) {
	start := time.Date(2026, 8, 16, 14, 0, 0, 0, time.UTC)
	out := ReplyICS("uid-42", "Quarterly Sync", "Vienna", "organizer@proton.me",
		"me@proton.me", "ACCEPTED", start, start.Add(time.Hour), false, false)
	for _, want := range []string{
		"BEGIN:VCALENDAR", "METHOD:REPLY",
		"BEGIN:VEVENT", "UID:uid-42",
		"DTSTART:20260816T140000Z", "DTEND:20260816T150000Z",
		"SUMMARY:Quarterly Sync", "LOCATION:Vienna",
		"ORGANIZER;CN=organizer@proton.me:mailto:organizer@proton.me",
		"ATTENDEE;PARTSTAT=ACCEPTED:mailto:me@proton.me",
		"SEQUENCE:0", "END:VEVENT", "END:VCALENDAR",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ReplyICS missing %q in:\n%s", want, out)
		}
	}
	// A non-token ATTENDEE line: the reply must not leak an X-PM-TOKEN.
	if strings.Contains(out, "X-PM-TOKEN") {
		t.Errorf("reply ATTENDEE should carry no X-PM-TOKEN, got:\n%s", out)
	}
}

func TestReplyICSProtonReplyMarker(t *testing.T) {
	start := time.Date(2026, 8, 16, 14, 0, 0, 0, time.UTC)
	marker := "X-PM-PROTON-REPLY;TYPE=boolean:true"

	with := ReplyICS("uid", "S", "", "o@proton.me", "me@proton.me", "DECLINED", start, start.Add(time.Hour), false, true)
	if !strings.Contains(with, marker) {
		t.Errorf("expected %q when protonReply=true, got:\n%s", marker, with)
	}
	if !strings.Contains(with, "ATTENDEE;PARTSTAT=DECLINED:mailto:me@proton.me") {
		t.Errorf("expected declined PARTSTAT, got:\n%s", with)
	}

	without := ReplyICS("uid", "S", "", "o@proton.me", "me@proton.me", "TENTATIVE", start, start.Add(time.Hour), false, false)
	if strings.Contains(without, marker) {
		t.Errorf("expected no %q when protonReply=false, got:\n%s", marker, without)
	}
}

func TestReplyICSAllDayUsesDateValue(t *testing.T) {
	start := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	out := ReplyICS("uid", "S", "", "o@proton.me", "me@proton.me", "ACCEPTED", start, start.AddDate(0, 0, 1), true, false)
	if !strings.Contains(out, "DTSTART;VALUE=DATE:20260816") {
		t.Errorf("all-day reply should use VALUE=DATE DTSTART, got:\n%s", out)
	}
}

func TestSignedVEVENTOrganizerLine(t *testing.T) {
	start := time.Date(2026, 8, 16, 14, 0, 0, 0, time.UTC)
	withOrg := SignedVEVENT("uid", start, start.Add(time.Hour), false, 0, "", "me@proton.me")
	if !strings.Contains(withOrg, "ORGANIZER;CN=me@proton.me:mailto:me@proton.me") {
		t.Errorf("expected ORGANIZER line, got:\n%s", withOrg)
	}
	noOrg := SignedVEVENT("uid", start, start.Add(time.Hour), false, 0, "", "")
	if strings.Contains(noOrg, "ORGANIZER") {
		t.Errorf("expected no ORGANIZER line when organizer empty, got:\n%s", noOrg)
	}
}

func TestEmailGroupAndGroupValues(t *testing.T) {
	text := strings.Join([]string{
		"BEGIN:VCARD",
		"VERSION:4.0",
		"FN:Bob",
		"item1.EMAIL;PREF=1:bob@example.com",
		"item1.KEY;PREF=2:data:application/pgp-keys;base64,SECOND",
		"item1.KEY;PREF=1:data:application/pgp-keys;base64,FIRST",
		"item1.X-PM-ENCRYPT:true",
		"item2.EMAIL;PREF=1:other@example.com",
		"item2.KEY;PREF=1:data:application/pgp-keys;base64,OTHER",
		"END:VCARD",
	}, "\r\n")

	t.Run("EmailGroup matches case-insensitively", func(t *testing.T) {
		if g := EmailGroup(text, "BOB@Example.com"); g != "item1" {
			t.Errorf("EmailGroup(bob) = %q, want item1", g)
		}
		if g := EmailGroup(text, "other@example.com"); g != "item2" {
			t.Errorf("EmailGroup(other) = %q, want item2", g)
		}
		if g := EmailGroup(text, "nobody@example.com"); g != "" {
			t.Errorf("EmailGroup(nobody) = %q, want empty", g)
		}
	})

	t.Run("GroupValues are PREF-ordered and group-scoped", func(t *testing.T) {
		keys := GroupValues(text, "item1", "KEY")
		if len(keys) != 2 {
			t.Fatalf("item1 KEY count = %d, want 2", len(keys))
		}
		if !strings.HasSuffix(keys[0], "FIRST") || !strings.HasSuffix(keys[1], "SECOND") {
			t.Errorf("KEY values not PREF-ordered: %v", keys)
		}
		if other := GroupValues(text, "item2", "KEY"); len(other) != 1 || !strings.HasSuffix(other[0], "OTHER") {
			t.Errorf("item2 KEY = %v, want one OTHER", other)
		}
	})

	t.Run("GroupValue returns first / empty", func(t *testing.T) {
		if v := GroupValue(text, "item1", "X-PM-ENCRYPT"); v != "true" {
			t.Errorf("X-PM-ENCRYPT = %q, want true", v)
		}
		if v := GroupValue(text, "item2", "X-PM-ENCRYPT"); v != "" {
			t.Errorf("item2 X-PM-ENCRYPT = %q, want empty", v)
		}
	})
}

func TestSignedVCardModelRoundTrip(t *testing.T) {
	enc := true
	in := SignedContact{
		Name: "Bob", UID: "uid-1",
		Emails: []SignedEmail{
			{
				Address:   "bob@example.com",
				KeyValues: []string{"data:application/pgp-keys;base64,AAAA", "data:application/pgp-keys;base64,BBBB"},
				Encrypt:   &enc,
				Scheme:    "pgp-mime",
			},
			{Address: "b2@example.com"},
		},
	}
	text := BuildSignedVCard(in)

	if !strings.Contains(text, "item1.KEY;PREF=1:data:application/pgp-keys;base64,AAAA") {
		t.Errorf("first key not emitted with PREF=1:\n%s", text)
	}
	if !strings.Contains(text, "item1.X-PM-ENCRYPT:true") || !strings.Contains(text, "item1.X-PM-SCHEME:pgp-mime") {
		t.Errorf("crypto flags not emitted:\n%s", text)
	}

	got := ParseSignedVCard(text)
	if got.Name != "Bob" || got.UID != "uid-1" {
		t.Errorf("name/uid round-trip: got %q / %q", got.Name, got.UID)
	}
	if len(got.Emails) != 2 {
		t.Fatalf("emails round-trip: got %d, want 2", len(got.Emails))
	}
	first := got.FindEmail("BOB@example.com")
	if first == nil {
		t.Fatal("FindEmail did not match case-insensitively")
	}
	if len(first.KeyValues) != 2 || !strings.HasSuffix(first.KeyValues[0], "AAAA") {
		t.Errorf("key round-trip: %v", first.KeyValues)
	}
	if first.Encrypt == nil || !*first.Encrypt {
		t.Errorf("encrypt flag lost: %v", first.Encrypt)
	}
	if second := got.FindEmail("b2@example.com"); second == nil || len(second.KeyValues) != 0 {
		t.Errorf("second email should have no keys: %+v", second)
	}
}

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
	out := SignedVEVENT("uid-123", start, end, false, 0, "", "")
	for _, want := range []string{"BEGIN:VEVENT", "UID:uid-123", "DTSTART:20260416T140000Z", "DTEND:20260416T150000Z", "SEQUENCE:0", "END:VEVENT"} {
		if !strings.Contains(out, want) {
			t.Errorf("SignedVEVENT missing %q in:\n%s", want, out)
		}
	}
}

func TestSignedVEVENTAllDay(t *testing.T) {
	start := time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC)
	out := SignedVEVENT("uid", start, start.AddDate(0, 0, 1), true, 1, "FREQ=DAILY", "")
	if !strings.Contains(out, "DTSTART;VALUE=DATE:20260416") {
		t.Errorf("all-day event should use VALUE=DATE DTSTART, got:\n%s", out)
	}
	if !strings.Contains(out, "SEQUENCE:1") {
		t.Errorf("expected SEQUENCE:1, got:\n%s", out)
	}
	if !strings.Contains(out, "RRULE:FREQ=DAILY") {
		t.Errorf("expected RRULE line, got:\n%s", out)
	}
}

func TestEncryptedVEVENT(t *testing.T) {
	full := EncryptedVEVENT("Meeting", "Vienna", "Quarterly sync")
	for _, want := range []string{"SUMMARY:Meeting", "LOCATION:Vienna", "DESCRIPTION:Quarterly sync"} {
		if !strings.Contains(full, want) {
			t.Errorf("expected %q, got:\n%s", want, full)
		}
	}
	bare := EncryptedVEVENT("Solo", "", "")
	if strings.Contains(bare, "LOCATION:") {
		t.Errorf("empty location should be omitted, got:\n%s", bare)
	}
	if strings.Contains(bare, "DESCRIPTION:") {
		t.Errorf("empty description should be omitted, got:\n%s", bare)
	}
}

func TestSignedVCard(t *testing.T) {
	withEmails := SignedVCard("John", []string{"john@x.com", "j2@x.com"}, "uid-1")
	for _, want := range []string{"FN:John", "UID:uid-1", "item1.EMAIL;PREF=1:john@x.com", "item2.EMAIL;PREF=2:j2@x.com"} {
		if !strings.Contains(withEmails, want) {
			t.Errorf("SignedVCard missing %q in:\n%s", want, withEmails)
		}
	}
	noEmail := SignedVCard("John", nil, "uid-1")
	if strings.Contains(noEmail, "EMAIL") {
		t.Errorf("empty email should be omitted, got:\n%s", noEmail)
	}
}

func TestEncryptedVCard(t *testing.T) {
	full := EncryptedVCard(VCardFields{Phones: []string{"+123", "+456"}, Note: "a note", Org: "ACME", Title: "CTO", Birthday: "1990-01-02", Address: "1 St", URL: "https://x"})
	for _, want := range []string{"TEL;PREF=1:+123", "TEL;PREF=2:+456", "NOTE:a note", "ORG:ACME", "TITLE:CTO", "BDAY:1990-01-02", "ADR:1 St", "URL:https://x"} {
		if !strings.Contains(full, want) {
			t.Errorf("EncryptedVCard missing %q in:\n%s", want, full)
		}
	}
	empty := EncryptedVCard(VCardFields{})
	for _, bad := range []string{"TEL", "NOTE", "ORG", "TITLE", "BDAY", "ADR", "URL"} {
		if strings.Contains(empty, bad) {
			t.Errorf("empty fields should be omitted, got %q in:\n%s", bad, empty)
		}
	}
}

func TestFields(t *testing.T) {
	text := strings.Join([]string{
		"item1.EMAIL;PREF=1:a@x.com",
		"item2.EMAIL;PREF=2:b@x.com",
		"TEL;PREF=1:+1",
		"TEL;PREF=2:+2",
	}, "\n")
	emails := Fields(text, "EMAIL")
	if len(emails) != 2 || emails[0] != "a@x.com" || emails[1] != "b@x.com" {
		t.Errorf("Fields(EMAIL) = %v", emails)
	}
	phones := Fields(text, "TEL")
	if len(phones) != 2 || phones[0] != "+1" || phones[1] != "+2" {
		t.Errorf("Fields(TEL) = %v", phones)
	}
	if got := Fields(text, "ORG"); len(got) != 0 {
		t.Errorf("Fields(ORG) = %v, want empty", got)
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
