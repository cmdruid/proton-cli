package calendar

import (
	"testing"
	"time"
)

func TestSaysRendersProtonWording(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 4, 20, 10, 0, 0, 0, loc)

	cases := []struct {
		name string
		e    Event
		want string
	}{
		{
			name: "a timed event a while out",
			e:    Event{Title: "Standup", Start: now.Add(1 * time.Hour)},
			want: "Standup starts at 11:00",
		},
		{
			name: "an event at this very minute",
			e:    Event{Title: "Standup", Start: now.Add(10 * time.Second)},
			want: "Standup starts now",
		},
		{
			name: "a timed event already begun",
			e:    Event{Title: "Standup", Start: now.Add(-2 * time.Hour)},
			want: "Standup started at 08:00",
		},
		{
			name: "an all-day event starting tomorrow",
			e:    Event{Title: "Holiday", Start: now.Add(24 * time.Hour), AllDay: true},
			want: "Holiday starts tomorrow",
		},
		{
			name: "an all-day event that began yesterday",
			e:    Event{Title: "Holiday", Start: now.Add(-24 * time.Hour), AllDay: true},
			want: "Holiday started yesterday",
		},
		{
			name: "an all-day event further out",
			e:    Event{Title: "Trip", Start: now.Add(3 * 24 * time.Hour), AllDay: true},
			want: "Trip starts on 2026-04-23",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := says(c.e, now); got != c.want {
				t.Errorf("says() = %q, want %q", got, c.want)
			}
		})
	}
}

// The occurrence start comes from Proton's own word for it; when an answer
// leaves that field out, the trigger is read back from the fire time honouring
// its sign - before the start puts the start after the alarm, after puts it
// before.
func TestOccurrenceStart(t *testing.T) {
	fire := time.Date(2026, 3, 29, 6, 45, 0, 0, time.UTC)
	if got := occurrenceStart(calendarAlarm{Occurrence: fire.Unix(), EventOccurrence: fire.Add(15 * time.Minute).Unix()}); !got.Equal(fire.Add(15 * time.Minute)) {
		t.Errorf("a served start was second-guessed: %v", got)
	}
	if got := occurrenceStart(calendarAlarm{Occurrence: fire.Unix(), Trigger: "-PT15M"}); !got.Equal(fire.Add(15 * time.Minute)) {
		t.Errorf("a trigger before the start gave %v, want %v", got, fire.Add(15*time.Minute))
	}
	if got := occurrenceStart(calendarAlarm{Occurrence: fire.Unix(), Trigger: "PT9H"}); !got.Equal(fire.Add(-9 * time.Hour)) {
		t.Errorf("an all-day morning trigger gave %v, want %v", got, fire.Add(-9*time.Hour))
	}
}

// A cancelled event, or an email action, produces no device reminder.
func TestReminderFromAlarmFilters(t *testing.T) {
	now := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	al := calendarAlarm{CalendarID: "c", EventID: "e", Occurrence: now.Add(15 * time.Minute).Unix(), EventOccurrence: now.Add(30 * time.Minute).Unix(), Trigger: "-PT15M"}
	cancelled := &Event{ID: "e", CalendarID: "c", Title: "Nope", Status: "cancelled", Zone: "UTC"}
	if r := reminderFromAlarm(al, cancelled, now); r != nil {
		t.Errorf("a cancelled event produced a reminder: %+v", r)
	}

	good := &Event{ID: "e", CalendarID: "c", Title: "Standup", Status: "confirmed", Zone: "UTC"}
	r := reminderFromAlarm(al, good, now)
	if r == nil {
		t.Fatal("a confirmed event produced no reminder")
	}
	if r.Remind != "15m" {
		t.Errorf("remind = %q, want 15m", r.Remind)
	}
	if r.Fires.Unix() != al.Occurrence {
		t.Errorf("fires = %v, want the alarm's own time", r.Fires)
	}
}

// The two readings of an event disagree but the fire time is fixed by the
// alarm, so a reminder is identified by both rather than by either alone.
func TestRemindKeyDistinguishesFirings(t *testing.T) {
	a := Reminder{Event: Event{CalendarID: "c", ID: "e"}, Fires: time.Date(2026, 4, 20, 8, 45, 0, 0, time.UTC), Remind: "15m"}
	b := a
	b.Remind = "1h"
	if remindKey(a) == remindKey(b) {
		t.Errorf("two triggers on the same start shared a key: %q", remindKey(a))
	}
}
