package tests

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// ── calendars ──

func TestCalendarCalendarsList(t *testing.T) {
	skipIfNoCredentials(t)
	stdout := runOK(t, "calendar", "calendars", "list")
	assertContains(t, stdout, "NAME")
	assertContains(t, stdout, "COLOR")
}

func TestCalendarCalendarsListColorPopulated(t *testing.T) {
	skipIfNoCredentials(t)
	cals := runJSONArray(t, "calendar", "calendars", "list")
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
	skipIfNoCredentials(t)
	name := testID() + "-cal"
	stdout := runOK(t, "calendar", "calendars", "create", "--name", name, "--color", "#8080FF")
	id := strings.TrimSpace(stdout)
	if !looksLikeID(id) {
		t.Fatalf("expected bare ID on stdout, got %q", stdout)
	}
	// Delete exercises the password-scope unlock path.
	cleanupRun(t, fmt.Sprintf("Delete calendar: proton-cli calendar calendars delete -- %s", id),
		"calendar", "calendars", "delete", "--", id)

	list := runOK(t, "calendar", "calendars", "list")
	assertContains(t, list, name)
}

// ── events ──

func TestCalendarEventsList(t *testing.T) {
	skipIfNoCredentials(t)
	stdout := runOK(t, "calendar", "events", "list", "--calendar", "Default")
	_ = stdout // may be empty; only assert the command runs
}

func TestCalendarEventsCRUDByIDs(t *testing.T) {
	skipIfNoCredentials(t)
	title := testID() + "-event"
	start := time.Now().Add(48 * time.Hour).Format("2006-01-02T15:04")

	idOut := runOK(t, "calendar", "events", "create",
		"--calendar", "Default",
		"--title", title,
		"--start", start,
		"--duration", "1h")
	eventID := strings.TrimSpace(idOut)
	if !looksLikeID(eventID) {
		t.Fatalf("expected bare event ID on stdout, got %q", idOut)
	}

	// Need both calendar ID + event ID for explicit ops and cleanup.
	cals := runJSONArray(t, "calendar", "calendars", "list")
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
	runOK(t, "calendar", "events", "update", "--title", title+"-updated", "--location", "Vienna",
		"--", calID, eventID)
	got2 := runOK(t, "calendar", "events", "get", "--", calID, eventID)
	assertContains(t, got2, title+"-updated")
	assertContains(t, got2, "Vienna")
}

func TestCalendarEventsGetByTitleRef(t *testing.T) {
	skipIfNoCredentials(t)
	title := testID() + "-ref"
	start := time.Now().Add(48 * time.Hour).Format("2006-01-02T15:04")
	idOut := runOK(t, "calendar", "events", "create",
		"--calendar", "Default",
		"--title", title,
		"--start", start,
		"--duration", "30m")
	eventID := strings.TrimSpace(idOut)

	cals := runJSONArray(t, "calendar", "calendars", "list")
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
	skipIfNoCredentials(t)
	title := testID() + "-refdel"
	start := time.Now().Add(48 * time.Hour).Format("2006-01-02T15:04")
	runOK(t, "calendar", "events", "create",
		"--calendar", "Default",
		"--title", title,
		"--start", start,
		"--duration", "15m")

	runOK(t, "calendar", "events", "delete", title)

	_, _, code := run(t, "calendar", "events", "get", title)
	if code != 3 {
		t.Errorf("expected exit 3 after delete, got %d", code)
	}
}

func TestCalendarEventsNotFound(t *testing.T) {
	skipIfNoCredentials(t)
	_, _, code := run(t, "calendar", "events", "get", "no-such-event-"+testID())
	if code != 3 {
		t.Errorf("expected exit 3 for unknown event, got %d", code)
	}
}

func firstCalendarID(t *testing.T) string {
	t.Helper()
	cals := runJSONArray(t, "calendar", "calendars", "list")
	if len(cals) == 0 {
		t.Skip("no calendars on this account")
	}
	return cals[0].(map[string]interface{})["id"].(string)
}

func TestCalendarEventRecurrenceAndDescription(t *testing.T) {
	skipIfNoCredentials(t)
	calID := firstCalendarID(t)

	title := testID() + "-evt"
	eventID := strings.TrimSpace(runOK(t, "calendar", "events", "create",
		"--calendar", calID, "--title", title,
		"--description", "quarterly sync", "--start", "2026-08-16T14:00", "--duration", "1h",
		"--rrule", "FREQ=WEEKLY;COUNT=5", "--remind", "15m"))
	cleanupRun(t, fmt.Sprintf("Delete event: proton-cli calendar events delete %s %s", calID, eventID),
		"calendar", "events", "delete", calID, eventID)

	got := runOK(t, "calendar", "events", "get", calID, eventID)
	assertContains(t, got, "quarterly sync")
	assertContains(t, got, "FREQ=WEEKLY")
}

func TestCalendarEventReminderNotification(t *testing.T) {
	skipIfNoCredentials(t)
	calID := firstCalendarID(t)

	title := testID() + "-remind"
	eventID := strings.TrimSpace(runOK(t, "calendar", "events", "create",
		"--calendar", calID, "--title", title,
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
	skipIfNoCredentials(t)
	calID := firstCalendarID(t)

	title := testID() + "-attendee"
	eventID := strings.TrimSpace(runOK(t, "calendar", "events", "create",
		"--calendar", calID, "--title", title,
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
	skipIfNoCredentials(t)

	name := testID() + "-cal"
	calID := strings.TrimSpace(runOK(t, "calendar", "calendars", "create", "--name", name, "--color", "#8080FF"))
	cleanupRun(t, fmt.Sprintf("Delete calendar: proton-cli calendar calendars delete %s", calID),
		"calendar", "calendars", "delete", calID)

	newName := name + "-renamed"
	runOK(t, "calendar", "calendars", "rename", "--name", newName, "--color", "#DB60D6", calID)
	assertContains(t, runOK(t, "calendar", "calendars", "list"), newName)
}

// TestCalendarCreateUsable proves a freshly created calendar is provisioned
// with keys (setupCalendar) by creating an event in it.
func TestCalendarCreateUsable(t *testing.T) {
	skipIfNoCredentials(t)

	name := testID() + "-usable"
	calID := strings.TrimSpace(runOK(t, "calendar", "calendars", "create", "--name", name, "--color", "#8080FF"))
	cleanupRun(t, fmt.Sprintf("Delete calendar: proton-cli calendar calendars delete %s", calID),
		"calendar", "calendars", "delete", calID)

	eventID := strings.TrimSpace(runOK(t, "calendar", "events", "create",
		"--calendar", calID, "--title", name+"-evt",
		"--start", "2026-08-20T10:00", "--duration", "1h"))
	if !looksLikeID(eventID) {
		t.Errorf("expected event ID on stdout, got %q", eventID)
	}
	assertContains(t, runOK(t, "calendar", "events", "get", calID, eventID), name+"-evt")
}
