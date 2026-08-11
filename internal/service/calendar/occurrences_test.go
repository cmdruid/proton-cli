package calendar

import (
	"testing"
	"time"

	"github.com/roman-16/proton-cli/internal/ical"
)

func atVienna(t *testing.T, month, day, hour int) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Vienna")
	if err != nil {
		t.Skipf("Europe/Vienna is not available: %v", err)
	}
	return time.Date(2026, time.Month(month), day, hour, 0, 0, 0, loc)
}

// readingIn fixes the zone a test reads occurrence references in.
func readingIn(t *testing.T, loc *time.Location) {
	t.Helper()
	saved := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = saved })
}

// seriesStored builds the weekly series the window tests expand, read from the
// zone it is anchored to.
//
// The reading matters as much as the series. An occurrence reference is the local
// reading of an instant, so a test that writes one as a literal is writing it as
// read from somewhere; left to the machine, that somewhere is whoever runs the
// test.
func seriesStored(t *testing.T, rule string) stored {
	t.Helper()
	start := atVienna(t, 4, 6, 9)
	readingIn(t, start.Location())
	return stored{
		raw: rawEvent{ID: "master", CalendarID: "cal1", UID: "uid-1"},
		model: ical.VEvent{
			UID:     "uid-1",
			Summary: "Standup",
			Start:   ical.Timed(start, "Europe/Vienna"),
			End:     ical.Timed(start.Add(15*time.Minute), "Europe/Vienna"),
			RRule:   rule,
		},
	}
}

func overrideStored(t *testing.T, at time.Time, title string) stored {
	t.Helper()
	id := ical.Timed(at, "Europe/Vienna")
	return stored{
		raw: rawEvent{ID: "override", CalendarID: "cal1", UID: "uid-1"},
		model: ical.VEvent{
			UID:          "uid-1",
			Summary:      title,
			Start:        ical.Timed(at.Add(90*time.Minute), "Europe/Vienna"),
			End:          ical.Timed(at.Add(120*time.Minute), "Europe/Vienna"),
			RecurrenceID: &id,
		},
	}
}

// A series is stored once and happens many times, so a window has to report the
// occurrences rather than the record.
func TestExpandTurnsASeriesIntoItsOccurrences(t *testing.T) {
	rows := expand([]stored{seriesStored(t, "FREQ=WEEKLY;COUNT=4")},
		atVienna(t, 4, 1, 0), atVienna(t, 5, 1, 0))
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want one per occurrence", len(rows))
	}
	for i, r := range rows {
		if r.ID != "master" {
			t.Errorf("row %d is addressed by %q, want the series", i, r.ID)
		}
		if r.Occurrence == "" || r.Number != i+1 {
			t.Errorf("row %d = %+v", i, r)
		}
	}
	if rows[0].Occurrence != "2026-04-06T09:00" {
		t.Errorf("first occurrence = %q", rows[0].Occurrence)
	}
}

func TestExpandReportsOnlyTheOccurrencesInTheWindow(t *testing.T) {
	// The record itself is dated in April; asking about one week in May must answer
	// with that week's occurrence and nothing else.
	rows := expand([]stored{seriesStored(t, "FREQ=WEEKLY;COUNT=20")},
		atVienna(t, 5, 4, 0), atVienna(t, 5, 11, 0))
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want one", len(rows))
	}
	if rows[0].Occurrence != "2026-05-04T09:00" {
		t.Errorf("occurrence = %q", rows[0].Occurrence)
	}
}

// An occurrence edited on its own is stored separately. It replaces the one the
// rule would have generated, and keeps the reference it had before it was edited,
// so a reference does not change the first time somebody edits an occurrence.
func TestExpandLetsAnEditedOccurrenceReplaceTheGeneratedOne(t *testing.T) {
	at := atVienna(t, 4, 13, 9)
	rows := expand([]stored{
		seriesStored(t, "FREQ=WEEKLY;COUNT=3"),
		overrideStored(t, at, "Standup (long)"),
	}, atVienna(t, 4, 1, 0), atVienna(t, 5, 1, 0))

	if len(rows) != 3 {
		t.Fatalf("got %d rows, want three", len(rows))
	}
	var edited *Event
	for i := range rows {
		if rows[i].Title == "Standup (long)" {
			edited = &rows[i]
		}
	}
	if edited == nil {
		t.Fatal("the edited occurrence is missing from the window")
	}
	if edited.ID != "master" || edited.StoredID != "override" {
		t.Errorf("the edited occurrence is addressed by %q/%q, want the series and its own record", edited.ID, edited.StoredID)
	}
	if edited.Occurrence != "2026-04-13T09:00" {
		t.Errorf("the edited occurrence is named %q, want its original start", edited.Occurrence)
	}
	if edited.RRule == "" {
		t.Error("the edited occurrence does not report that it belongs to a series")
	}
	for _, r := range rows {
		if r.Occurrence == "2026-04-13T09:00" && r.Title == "Standup" {
			t.Error("the generated occurrence was reported alongside the edited one")
		}
	}
}

func TestExpandStillReportsAnEventItCannotRead(t *testing.T) {
	// A row you cannot read is worth seeing; it just cannot be expanded or written.
	e := stored{
		raw:     rawEvent{ID: "broken", CalendarID: "cal1", StartTime: 1000, EndTime: 2000},
		readErr: errRead,
	}
	rows := expand([]stored{e}, time.Unix(0, 0), time.Unix(5000, 0))
	if len(rows) != 1 || rows[0].ID != "broken" || rows[0].Title != "" {
		t.Errorf("expand = %+v", rows)
	}
}

var errRead = &readError{}

type readError struct{}

func (*readError) Error() string { return "cannot read" }

// ── the patch ──

func TestPatchLeavesUnmentionedFieldsAlone(t *testing.T) {
	start := atVienna(t, 4, 6, 9)
	v := ical.VEvent{
		UID: "u", Summary: "Standup", Location: "Meet", Description: "notes",
		Start: ical.Timed(start, "Europe/Vienna"),
		End:   ical.Timed(start.Add(15*time.Minute), "Europe/Vienna"),
		RRule: "FREQ=WEEKLY;COUNT=4",
		ExDates: []ical.DateTime{
			ical.Timed(start.AddDate(0, 0, 7), "Europe/Vienna"),
		},
	}
	title := "Renamed"
	out := EventPatch{Title: &title}.apply(v, v.Start.TZID)

	if out.Summary != "Renamed" {
		t.Errorf("title = %q", out.Summary)
	}
	if out.Location != "Meet" || out.Description != "notes" {
		t.Errorf("a rename dropped other content: %+v", out)
	}
	if out.RRule != v.RRule {
		t.Errorf("a rename dropped the recurrence: %q", out.RRule)
	}
	if len(out.ExDates) != 1 {
		t.Errorf("a rename dropped the exclusions: %+v", out.ExDates)
	}
	if !out.Start.Equal(v.Start) || out.Start.TZID != v.Start.TZID {
		t.Errorf("a rename moved the event: %+v", out.Start)
	}
}

func TestPatchKeepsTheLengthWhenOnlyTheStartMoves(t *testing.T) {
	start := atVienna(t, 4, 6, 9)
	v := ical.VEvent{
		UID:   "u",
		Start: ical.Timed(start, "Europe/Vienna"),
		End:   ical.Timed(start.Add(30*time.Minute), "Europe/Vienna"),
	}
	moved := start.Add(3 * time.Hour)
	out := EventPatch{Start: &moved}.apply(v, v.Start.TZID)
	if out.Duration() != 30*time.Minute {
		t.Errorf("duration = %v, want the length it had", out.Duration())
	}
	if !out.Start.Time.Equal(moved) {
		t.Errorf("start = %v", out.Start.Time)
	}
}

func TestPatchTurningAnEventAllDayGivesItAWholeDay(t *testing.T) {
	start := atVienna(t, 4, 6, 9)
	v := ical.VEvent{
		UID:   "u",
		Start: ical.Timed(start, "Europe/Vienna"),
		End:   ical.Timed(start.Add(30*time.Minute), "Europe/Vienna"),
	}
	yes := true
	out := EventPatch{AllDay: &yes}.apply(v, "Europe/Vienna")
	if !out.Start.AllDay || !out.End.AllDay {
		t.Fatalf("out = %+v", out)
	}
	if out.Start.String() != "2026-04-06" || out.End.String() != "2026-04-07" {
		t.Errorf("an all-day event runs %s to %s", out.Start.String(), out.End.String())
	}
}

func TestPatchBreaksOnlyWhenItMovesOrRepatterns(t *testing.T) {
	title, zone := "x", "Europe/Vienna"
	if (EventPatch{Title: &title}).breaks() {
		t.Error("a rename counts as a breaking change")
	}
	if !(EventPatch{Zone: &zone}).breaks() {
		t.Error("re-anchoring does not count as a breaking change")
	}
	rule := "FREQ=DAILY"
	if !(EventPatch{RRule: &rule}).breaks() {
		t.Error("changing the pattern does not count as a breaking change")
	}
}

func TestPatchRefusesContradictoryReminderFlags(t *testing.T) {
	raw := rawEvent{Notifications: []rawNotification{{Type: 1, Trigger: "-PT42M"}}}
	kept, err := EventPatch{}.reminders(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 1 || kept[0]["Trigger"] != "-PT42M" {
		t.Errorf("an unmentioned reminder list was not preserved: %v", kept)
	}
	none := []string{}
	cleared, err := EventPatch{Reminders: &none}.reminders(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cleared == nil || len(cleared) != 0 {
		t.Errorf("clearing the reminders produced %v, want an empty list", cleared)
	}
}

// ── the chain ──

func TestSeriesFindsAnOverrideAndTheOnesFromAnInstantOn(t *testing.T) {
	second := atVienna(t, 4, 13, 9)
	third := atVienna(t, 4, 20, 9)
	chain := series{
		master: seriesStored(t, "FREQ=WEEKLY;COUNT=4"),
		overrides: []stored{
			overrideStored(t, second, "second"),
			overrideStored(t, third, "third"),
		},
	}
	chain.overrides[1].raw.ID = "override-3"

	if got := chain.overrideAt(ical.Timed(second, "Europe/Vienna")); got == nil || got.model.Summary != "second" {
		t.Errorf("overrideAt = %+v", got)
	}
	if got := chain.overrideAt(ical.Timed(atVienna(t, 4, 27, 9), "Europe/Vienna")); got != nil {
		t.Errorf("overrideAt found an override that does not exist: %+v", got)
	}
	from := chain.idsFrom(ical.Timed(third, "Europe/Vienna"))
	if len(from) != 1 || from[0] != "override-3" {
		t.Errorf("idsFrom = %v, want only the one at or after the instant", from)
	}
	if len(chain.allOverrideIDs()) != 2 {
		t.Errorf("allOverrideIDs = %v", chain.allOverrideIDs())
	}
}
