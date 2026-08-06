package tests

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// ── calendars ──

func TestCalendarCalendarsList(t *testing.T) {
	stdout := runOK(t, "calendar", "settings", "calendars", "list")
	assertContains(t, stdout, "NAME")
	assertContains(t, stdout, "COLOR")
}

func TestCalendarCalendarsListColorPopulated(t *testing.T) {
	cals := runJSONArray(t, "calendar", "settings", "calendars", "list")
	if len(cals) == 0 {
		t.Skip("no calendars on account")
	}
	// Color lives on Members[0] in the API, service should surface it.
	gotAny := false
	for _, c := range cals {
		color, _ := c.(map[string]interface{})["color"].(string)
		if strings.HasPrefix(color, "#") {
			gotAny = true
			break
		}
	}
	if !gotAny {
		t.Error("expected at least one calendar with a populated #hex color")
	}
}

func TestCalendarCalendarsCreateAndDelete(t *testing.T) {
	name := testID() + "-cal"
	stdout := runOK(t, "calendar", "settings", "calendars", "create", "--name", name, "--color", "#8080FF")
	id := strings.TrimSpace(stdout)
	if !looksLikeID(id) {
		t.Fatalf("expected bare ID on stdout, got %q", stdout)
	}
	// Delete exercises the password-scope unlock path.
	cleanupRun(t, fmt.Sprintf("Delete calendar: proton-cli calendar settings calendars delete -- %s", id),
		"calendar", "settings", "calendars", "delete", "--", id)

	list := runOK(t, "calendar", "settings", "calendars", "list")
	assertContains(t, list, name)
}

// ── events ──

func TestCalendarEventsList(t *testing.T) {
	stdout := runOK(t, "calendar", "events", "list", "--calendar", "Default")
	_ = stdout // may be empty; only assert the command runs
}

func TestCalendarEventsCRUDByIDs(t *testing.T) {
	title := testID() + "-event"
	start := time.Now().Add(48 * time.Hour).Format("2006-01-02T15:04")

	idOut := runOK(t, "calendar", "events", "create",
		"--calendar", "Default",
		"--job-title", title,
		"--start", start,
		"--duration", "1h")
	eventID := strings.TrimSpace(idOut)
	if !looksLikeID(eventID) {
		t.Fatalf("expected bare event ID on stdout, got %q", idOut)
	}

	// Need both calendar ID + event ID for explicit ops and cleanup.
	cals := runJSONArray(t, "calendar", "settings", "calendars", "list")
	var calID string
	for _, c := range cals {
		m := c.(map[string]interface{})
		if n, _ := m["name"].(string); n == "Default" {
			calID, _ = m["id"].(string)
		}
	}
	if calID == "" {
		t.Fatal("could not find Default calendar")
	}
	cleanupRun(t, fmt.Sprintf("Delete event: proton-cli calendar events delete -- %s %s", calID, eventID),
		"calendar", "events", "delete", "--", calID, eventID)

	// Get by IDs
	got := runOK(t, "calendar", "events", "get", "--", calID, eventID)
	assertContains(t, got, title)
	// Signature: an event we just created is signed with our own address key.
	assertField(t, got, "Signature:", "verified")

	// Update title + location
	runOK(t, "calendar", "events", "update", "--job-title", title+"-updated", "--location", "Vienna",
		"--", calID, eventID)
	got2 := runOK(t, "calendar", "events", "get", "--", calID, eventID)
	assertContains(t, got2, title+"-updated")
	assertContains(t, got2, "Vienna")
}

func TestCalendarEventsGetByTitleRef(t *testing.T) {
	title := testID() + "-ref"
	start := time.Now().Add(48 * time.Hour).Format("2006-01-02T15:04")
	idOut := runOK(t, "calendar", "events", "create",
		"--calendar", "Default",
		"--job-title", title,
		"--start", start,
		"--duration", "30m")
	eventID := strings.TrimSpace(idOut)

	cals := runJSONArray(t, "calendar", "settings", "calendars", "list")
	var calID string
	for _, c := range cals {
		m := c.(map[string]interface{})
		if n, _ := m["name"].(string); n == "Default" {
			calID, _ = m["id"].(string)
		}
	}
	cleanupRun(t, fmt.Sprintf("Delete event: proton-cli calendar events delete -- %s %s", calID, eventID),
		"calendar", "events", "delete", "--", calID, eventID)

	// REF = title substring
	stdout := runOK(t, "calendar", "events", "get", title)
	assertContains(t, stdout, title)
}

func TestCalendarEventsDeleteByTitleRef(t *testing.T) {
	title := testID() + "-refdel"
	start := time.Now().Add(48 * time.Hour).Format("2006-01-02T15:04")
	runOK(t, "calendar", "events", "create",
		"--calendar", "Default",
		"--job-title", title,
		"--start", start,
		"--duration", "15m")

	runOK(t, "calendar", "events", "delete", title)

	_, _, code := run(t, "calendar", "events", "get", title)
	if code != 3 {
		t.Errorf("expected exit 3 after delete, got %d", code)
	}
}

func TestCalendarEventsNotFound(t *testing.T) {
	_, _, code := run(t, "calendar", "events", "get", "no-such-event-"+testID())
	if code != 3 {
		t.Errorf("expected exit 3 for unknown event, got %d", code)
	}
}

func firstCalendarID(t *testing.T) string {
	t.Helper()
	cals := runJSONArray(t, "calendar", "settings", "calendars", "list")
	if len(cals) == 0 {
		t.Skip("no calendars on this account")
	}
	return cals[0].(map[string]interface{})["id"].(string)
}

func TestCalendarEventRecurrenceAndDescription(t *testing.T) {
	calID := firstCalendarID(t)

	title := testID() + "-evt"
	eventID := strings.TrimSpace(runOK(t, "calendar", "events", "create",
		"--calendar", calID, "--job-title", title,
		"--description", "quarterly sync", "--start", "2026-08-16T14:00", "--duration", "1h",
		"--rrule", "FREQ=WEEKLY;COUNT=5", "--remind", "15m"))
	cleanupRun(t, fmt.Sprintf("Delete event: proton-cli calendar events delete %s %s", calID, eventID),
		"calendar", "events", "delete", calID, eventID)

	got := runOK(t, "calendar", "events", "get", calID, eventID)
	assertContains(t, got, "quarterly sync")
	assertContains(t, got, "FREQ=WEEKLY")
}

func TestCalendarEventReminderNotification(t *testing.T) {
	calID := firstCalendarID(t)

	title := testID() + "-remind"
	eventID := strings.TrimSpace(runOK(t, "calendar", "events", "create",
		"--calendar", calID, "--job-title", title,
		"--start", "2026-09-01T09:00", "--duration", "30m", "--remind", "15m"))
	cleanupRun(t, fmt.Sprintf("Delete event: proton-cli calendar events delete %s %s", calID, eventID),
		"calendar", "events", "delete", calID, eventID)

	data := runJSON(t, "api", "GET", "/calendar/v1/"+calID+"/events/"+eventID)
	ev, _ := data["Event"].(map[string]interface{})
	notifs, _ := ev["Notifications"].([]interface{})
	found := false
	for _, n := range notifs {
		if m, ok := n.(map[string]interface{}); ok {
			if trig, _ := m["Trigger"].(string); trig == "-PT15M" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected a -PT15M notification on the event, got: %v", notifs)
	}
}

func TestCalendarEventWithProtonAttendee(t *testing.T) {
	calID := firstCalendarID(t)

	title := testID() + "-attendee"
	eventID := strings.TrimSpace(runOK(t, "calendar", "events", "create",
		"--calendar", calID, "--job-title", title,
		"--start", "2026-08-17T10:00", "--duration", "30m",
		"--attendee", selfEmail()))
	cleanupRun(t, fmt.Sprintf("Delete event: proton-cli calendar events delete %s %s", calID, eventID),
		"calendar", "events", "delete", calID, eventID)

	if !looksLikeID(eventID) {
		t.Errorf("expected an event ID on stdout, got %q", eventID)
	}
	// The server accepted the encrypted attendee parts; the event must read back.
	runOK(t, "calendar", "events", "get", calID, eventID)
}

func TestCalendarCalendarsRename(t *testing.T) {
	name := testID() + "-cal"
	calID := strings.TrimSpace(runOK(t, "calendar", "settings", "calendars", "create", "--name", name, "--color", "#8080FF"))
	cleanupRun(t, fmt.Sprintf("Delete calendar: proton-cli calendar settings calendars delete %s", calID),
		"calendar", "settings", "calendars", "delete", calID)

	newName := name + "-renamed"
	runOK(t, "calendar", "settings", "calendars", "update", "--name", newName, "--color", "#DB60D6", calID)
	assertContains(t, runOK(t, "calendar", "settings", "calendars", "list"), newName)
}

// TestCalendarCreateUsable proves a freshly created calendar is provisioned
// with keys (setupCalendar) by creating an event in it.
func TestCalendarCreateUsable(t *testing.T) {
	name := testID() + "-usable"
	calID := strings.TrimSpace(runOK(t, "calendar", "settings", "calendars", "create", "--name", name, "--color", "#8080FF"))
	cleanupRun(t, fmt.Sprintf("Delete calendar: proton-cli calendar settings calendars delete %s", calID),
		"calendar", "settings", "calendars", "delete", calID)

	eventID := strings.TrimSpace(runOK(t, "calendar", "events", "create",
		"--calendar", calID, "--job-title", name+"-evt",
		"--start", "2026-08-20T10:00", "--duration", "1h"))
	if !looksLikeID(eventID) {
		t.Errorf("expected event ID on stdout, got %q", eventID)
	}
	assertContains(t, runOK(t, "calendar", "events", "get", calID, eventID), name+"-evt")
}

// ── events respond (RSVP) ──
//
// A full accept/decline round-trip needs a *second* Proton account to invite
// this one (the harness can't act as the alt), so it's manual - same as
// `drive invitations` accept/reject. These cover the branches reachable with
// a single account: flag validation, dry-run, and the organizer rejection.

func TestCalendarEventsRespondBadStatus(t *testing.T) {
	// --status is validated before auth, so an invalid value is a clean exit 1.
	_, stderr, code := run(t, "calendar", "events", "respond", "--status", "maybe", "some-event-ref")
	if code != 1 {
		t.Errorf("expected exit 1 for invalid --status, got %d", code)
	}
	assertContains(t, stderr, "--status accepts")
}

func TestCalendarEventsRespondDryRun(t *testing.T) {
	calID := firstCalendarID(t)
	title := testID() + "-rsvp-dry"
	start := time.Now().Add(48 * time.Hour).Format("2006-01-02T15:04")
	eventID := strings.TrimSpace(runOK(t, "calendar", "events", "create",
		"--calendar", calID, "--job-title", title, "--start", start, "--duration", "30m",
		"--attendee", selfEmail()))
	cleanupRun(t, fmt.Sprintf("Delete event: proton-cli calendar events delete %s %s", calID, eventID),
		"calendar", "events", "delete", calID, eventID)

	_, stderr := runOKStderr(t, "--dry-run", "calendar", "events", "respond",
		"--status", "accept", calID, eventID)
	assertContains(t, stderr, "Dry run")
	// The event still reads back (no mutation happened).
	assertContains(t, runOK(t, "calendar", "events", "get", calID, eventID), title)
}

func TestCalendarEventsRespondRejectsOrganizer(t *testing.T) {
	calID := firstCalendarID(t)
	title := testID() + "-rsvp-org"
	start := time.Now().Add(48 * time.Hour).Format("2006-01-02T15:04")
	// We create the event, so we are its organizer; RSVP must be refused.
	eventID := strings.TrimSpace(runOK(t, "calendar", "events", "create",
		"--calendar", calID, "--job-title", title, "--start", start, "--duration", "30m",
		"--attendee", selfEmail()))
	cleanupRun(t, fmt.Sprintf("Delete event: proton-cli calendar events delete %s %s", calID, eventID),
		"calendar", "events", "delete", calID, eventID)

	_, stderr, code := run(t, "calendar", "events", "respond", "--status", "accept", calID, eventID)
	if code != 1 {
		t.Errorf("expected exit 1 responding to your own event, got %d (stderr: %s)", code, stderr)
	}
	assertContains(t, stderr, "organizer")
}

// ── events respond: two-account RSVP round-trip ──
//
// Needs the "Proton Alt" second account (the `alt` profile): the alt organizes
// an event and invites the primary, the primary RSVPs, then we verify the
// primary's partstat flipped and the organizer got the METHOD:REPLY email.

func eventField(ev map[string]interface{}, key string) interface{} {
	e, _ := ev["Event"].(map[string]interface{})
	if e == nil {
		return nil
	}
	return e[key]
}

func firstAttendeeStatus(ev map[string]interface{}) (int, bool) {
	e, _ := ev["Event"].(map[string]interface{})
	if e == nil {
		return 0, false
	}
	ai, _ := e["AttendeesInfo"].(map[string]interface{})
	if ai == nil {
		return 0, false
	}
	atts, _ := ai["Attendees"].([]interface{})
	if len(atts) == 0 {
		return 0, false
	}
	a, _ := atts[0].(map[string]interface{})
	s, ok := a["Status"].(float64)
	return int(s), ok
}

func TestCalendarEventsRespondRoundTrip(t *testing.T) {
	altCals := runJSONArray(t, alt("calendar", "settings", "calendars", "list")...)
	if len(altCals) == 0 {
		t.Skip("alt account has no calendars")
	}
	altCal := altCals[0].(map[string]interface{})["id"].(string)

	title := testID() + "-rsvp-rt"
	start := time.Now().Add(72 * time.Hour).Format("2006-01-02T15:04")
	altEventID := strings.TrimSpace(runOK(t, alt("calendar", "events", "create",
		"--calendar", altCal, "--job-title", title, "--start", start, "--duration", "30m",
		"--attendee", selfEmail())...))
	cleanupRun(t, fmt.Sprintf("Delete alt event: proton-cli --profile alt calendar events delete %s %s", altCal, altEventID),
		alt("calendar", "events", "delete", altCal, altEventID)...)

	uid, _ := eventField(runJSON(t, alt("api", "GET", "/calendar/v1/"+altCal+"/events/"+altEventID)...), "UID").(string)
	if uid == "" {
		t.Fatal("could not read the alt event UID")
	}

	// The Proton-to-Proton invite lands on the primary's calendar as a shared
	// event (IsOrganizer=0). Find our copy by matching the UID.
	primaryCal := firstCalendarID(t)
	var primaryEventID string
	waitFor(45*time.Second, 3*time.Second, func() bool {
		evs := runJSONArray(t, "calendar", "events", "list", "--calendar", primaryCal,
			"--start", time.Now().Format("2006-01-02"),
			"--end", time.Now().Add(120*time.Hour).Format("2006-01-02"))
		for _, e := range evs {
			id := e.(map[string]interface{})["id"].(string)
			ev := runJSON(t, "api", "GET", "/calendar/v1/"+primaryCal+"/events/"+id)
			if u, _ := eventField(ev, "UID").(string); u == uid {
				if org, _ := eventField(ev, "IsOrganizer").(float64); int(org) == 0 {
					primaryEventID = id
					return true
				}
			}
		}
		return false
	})
	if primaryEventID == "" {
		t.Fatal("invitation did not appear on the primary's calendar")
	}
	cleanupRun(t, fmt.Sprintf("Delete primary event copy: proton-cli calendar events delete %s %s", primaryCal, primaryEventID),
		"calendar", "events", "delete", primaryCal, primaryEventID)

	runOK(t, "calendar", "events", "respond", primaryCal, primaryEventID, "--status", "accept")

	// The primary's own attendee record now shows ACCEPTED (ATTENDEE_STATUS_API 3).
	got := runJSON(t, "api", "GET", "/calendar/v1/"+primaryCal+"/events/"+primaryEventID)
	if status, ok := firstAttendeeStatus(got); !ok || status != 3 {
		t.Errorf("primary attendee Status = %d (ok=%v), want 3 (accepted)", status, ok)
	}

	// The organizer (alt) receives the METHOD:REPLY email naming the title.
	var replyID string
	waitFor(45*time.Second, 3*time.Second, func() bool {
		replyID = altMailContaining(t, selfEmail(), title)
		return replyID != ""
	})
	if replyID == "" {
		t.Error("alt did not receive the RSVP reply email")
	} else {
		cleanupRun(t, "Delete reply mail (alt): proton-cli --profile alt mail messages delete "+replyID,
			alt("mail", "messages", "delete", replyID)...)
	}
}
