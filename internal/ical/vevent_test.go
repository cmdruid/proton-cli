package ical

import (
	"strings"
	"testing"
	"time"
)

// A stored event arrives as several cards; the model is their union.
const storedCards = "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\n" +
	"UID:uid-1\r\n" +
	"DTSTAMP:20260101T000000Z\r\n" +
	"DTSTART;TZID=Europe/Vienna:20260416T090000\r\n" +
	"DTEND;TZID=Europe/Vienna:20260416T091500\r\n" +
	"RRULE:FREQ=WEEKLY;COUNT=10\r\n" +
	"EXDATE;TZID=Europe/Vienna:20260430T090000\r\n" +
	"EXDATE;TZID=Europe/Vienna:20260423T090000\r\n" +
	"ORGANIZER;CN=me@proton.me:mailto:me@proton.me\r\n" +
	"SEQUENCE:3\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR\r\n" +
	"BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\n" +
	"UID:uid-1\r\n" +
	"SUMMARY:Standup\r\n" +
	"LOCATION:Meet\r\n" +
	"DESCRIPTION:first line\\nsecond line\\, with a comma\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR"

func TestParseMergesEveryCardIntoOneEvent(t *testing.T) {
	v, err := Parse(storedCards)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if v.UID != "uid-1" || v.Summary != "Standup" || v.Location != "Meet" {
		t.Errorf("Parse = %+v", v)
	}
	if v.Description != "first line\nsecond line, with a comma" {
		t.Errorf("description did not survive escaping: %q", v.Description)
	}
	if v.RRule != "FREQ=WEEKLY;COUNT=10" || v.Sequence != 3 || v.Organizer != "me@proton.me" {
		t.Errorf("Parse = %+v", v)
	}
	if v.Start.TZID != "Europe/Vienna" || v.Start.AllDay {
		t.Errorf("start lost its anchor: %+v", v.Start)
	}
	if want := time.Date(2026, 4, 16, 9, 0, 0, 0, v.Start.Location()); !v.Start.Time.Equal(want) {
		t.Errorf("start = %v, want %v", v.Start.Time, want)
	}
	if v.Duration() != 15*time.Minute {
		t.Errorf("duration = %v", v.Duration())
	}
}

func TestParseSortsAndDeduplicatesExclusions(t *testing.T) {
	v, err := Parse(storedCards + "\r\nEXDATE;TZID=Europe/Vienna:20260423T090000")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(v.ExDates) != 2 {
		t.Fatalf("got %d exclusions, want 2 after deduplication", len(v.ExDates))
	}
	if !v.ExDates[0].Time.Before(v.ExDates[1].Time) {
		t.Errorf("exclusions are not sorted: %v", v.ExDates)
	}
}

func TestParseRefusesContentWithNoUID(t *testing.T) {
	// A card that decrypted into something without an identity is not an event,
	// and writing it back would sign nonsense over a real one.
	if _, err := Parse("BEGIN:VEVENT\r\nSUMMARY:x\r\nEND:VEVENT"); err == nil {
		t.Fatal("Parse accepted content with no UID")
	}
}

func TestParseReadsAllDayAndUTCForms(t *testing.T) {
	v, err := Parse("UID:u\r\nDTSTART;VALUE=DATE:20260416\r\nDTEND;VALUE=DATE:20260417")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !v.Start.AllDay || v.Start.String() != "2026-04-16" {
		t.Errorf("all-day start = %+v", v.Start)
	}

	v, err = Parse("UID:u\r\nDTSTART:20260416T070000Z")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if v.Start.TZID != "" || !v.Start.Time.Equal(time.Date(2026, 4, 16, 7, 0, 0, 0, time.UTC)) {
		t.Errorf("UTC start = %+v", v.Start)
	}
}

// Clients write an all-day event's end three ways: the exclusive next day, no end
// at all, and an end equal to the start. All three mean the same days, so an event
// measures the same however it arrived.
func TestSpanReadsEveryShapeOfAllDayEnd(t *testing.T) {
	day := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	for name, end := range map[string]DateTime{
		"an exclusive end": Day(day.AddDate(0, 0, 1)),
		"no end":           {},
		"an end at start":  Day(day),
	} {
		v := VEvent{UID: "u", Start: Day(day), End: end}
		_, got := v.Span()
		if got.String() != "2026-08-15" {
			t.Errorf("an all-day event with %s ends %s, want the midnight after its day", name, got.String())
		}
		if v.Duration() != 24*time.Hour {
			t.Errorf("an all-day event with %s lasts %v, want a day", name, v.Duration())
		}
	}
}

func TestSpanOfAThreeDayAllDayEventKeepsItsLength(t *testing.T) {
	day := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	v := VEvent{UID: "u", Start: Day(day), End: Day(day.AddDate(0, 0, 3))}
	if _, end := v.Span(); end.String() != "2026-08-17" {
		t.Errorf("end = %s, want the exclusive end it was written with", end.String())
	}
	if v.Duration() != 72*time.Hour {
		t.Errorf("duration = %v, want three days", v.Duration())
	}
}

// An event written with no end is an instant, and stays one: only a whole day has a
// length nobody wrote down.
func TestSpanOfATimedEventWithoutAnEndIsAnInstant(t *testing.T) {
	loc := vienna(t)
	at := time.Date(2026, 8, 14, 9, 0, 0, 0, loc)
	v := VEvent{UID: "u", Start: Timed(at, "Europe/Vienna")}
	start, end := v.Span()
	if !end.Equal(start) {
		t.Errorf("end = %+v, want the start", end)
	}
	if v.Duration() != 0 {
		t.Errorf("duration = %v, want none", v.Duration())
	}
}

// Reading is not rewriting: an event that arrived without an end is stored again
// without one, because normalising what another client wrote would change what the
// event says.
func TestSpanLeavesTheStoredValuesAlone(t *testing.T) {
	v, err := Parse("UID:u\r\nDTSTART;VALUE=DATE:20260814")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, end := v.Span(); end.IsZero() {
		t.Fatal("Span reported no end")
	}
	if !v.End.IsZero() {
		t.Errorf("reading the event gave it a stored end of %+v", v.End)
	}
	if got := v.SharedSigned(); strings.Contains(got, "DTEND") {
		t.Errorf("an event that arrived without an end was stored with one:\n%s", got)
	}
}

// The property split is Proton's: recurrence lives in the signed card, the words
// people read live in the encrypted one. An event rebuilt without the recurrence
// properties is an event silently turned back into a one-off.
func TestSharedSignedCarriesEveryRecurrenceProperty(t *testing.T) {
	v, err := Parse(storedCards)
	if err != nil {
		t.Fatal(err)
	}
	card := v.SharedSigned()
	for _, want := range []string{
		"UID:uid-1",
		"DTSTART;TZID=Europe/Vienna:20260416T090000",
		"DTEND;TZID=Europe/Vienna:20260416T091500",
		"RRULE:FREQ=WEEKLY;COUNT=10",
		"EXDATE;TZID=Europe/Vienna:20260423T090000",
		"EXDATE;TZID=Europe/Vienna:20260430T090000",
		"ORGANIZER;CN=me@proton.me:mailto:me@proton.me",
		"SEQUENCE:3",
	} {
		if !strings.Contains(card, want) {
			t.Errorf("signed card is missing %q:\n%s", want, card)
		}
	}
	for _, unwanted := range []string{"SUMMARY", "LOCATION", "DESCRIPTION"} {
		if strings.Contains(card, unwanted) {
			t.Errorf("signed card leaks %s, which belongs in the encrypted card", unwanted)
		}
	}
}

func TestSharedEncryptedCarriesTheWordsAndTheIdentity(t *testing.T) {
	v, err := Parse(storedCards)
	if err != nil {
		t.Fatal(err)
	}
	card := v.SharedEncrypted()
	// Proton lists uid and dtstamp in both parts, so a card without them is not
	// the shape its own clients write.
	for _, want := range []string{"UID:uid-1", "DTSTAMP:", "SUMMARY:Standup", "LOCATION:Meet"} {
		if !strings.Contains(card, want) {
			t.Errorf("encrypted card is missing %q:\n%s", want, card)
		}
	}
	if strings.Contains(card, "RRULE") {
		t.Error("encrypted card carries the recurrence rule, which belongs in the signed card")
	}
	if !strings.Contains(card, `DESCRIPTION:first line\nsecond line\, with a comma`) {
		t.Errorf("description was not escaped on the way out:\n%s", card)
	}
}

func TestRoundTripPreservesEverythingTheModelHolds(t *testing.T) {
	v, err := Parse(storedCards)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Parse(v.SharedSigned() + "\r\n" + v.SharedEncrypted())
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if back.RRule != v.RRule || len(back.ExDates) != len(v.ExDates) || back.Sequence != v.Sequence {
		t.Errorf("round trip lost recurrence: %+v", back)
	}
	if !back.Start.Equal(v.Start) || back.Start.TZID != v.Start.TZID {
		t.Errorf("round trip moved the start: %+v", back.Start)
	}
	if back.Summary != v.Summary || back.Description != v.Description || back.Location != v.Location {
		t.Errorf("round trip lost the content: %+v", back)
	}
}

func TestAttendeesCardCarriesTokensAndIsEmptyWithoutAttendees(t *testing.T) {
	v := VEvent{UID: "u"}
	if v.AttendeesEncrypted() != "" {
		t.Error("an event with no attendees produced an attendees card")
	}
	v.Attendees = []Attendee{{Email: "alice@proton.me", Token: "tok-a"}}
	card := v.AttendeesEncrypted()
	if !strings.Contains(card, "X-PM-TOKEN=tok-a") || !strings.Contains(card, "mailto:alice@proton.me") {
		t.Errorf("attendees card = %s", card)
	}
	back, err := Parse(card)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Attendees) != 1 || back.Attendees[0].Token != "tok-a" {
		t.Errorf("attendees did not round trip: %+v", back.Attendees)
	}
}

func TestEventUIDDoesNotRepeat(t *testing.T) {
	// Two events created in the same moment must not share an identity, which a
	// nanosecond clock cannot promise.
	seen := map[string]bool{}
	for range 100 {
		uid := EventUID()
		if seen[uid] {
			t.Fatal("EventUID repeated itself")
		}
		seen[uid] = true
		if !strings.HasSuffix(uid, "@proton-cli") {
			t.Errorf("EventUID = %q, want the proton suffix", uid)
		}
	}
}
