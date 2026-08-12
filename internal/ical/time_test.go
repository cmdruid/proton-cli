package ical

import (
	"testing"
	"time"
)

func TestDateTimeRendersItsOwnAnchor(t *testing.T) {
	loc := vienna(t)
	at := time.Date(2026, 4, 16, 9, 0, 0, 0, loc)

	zoned := Timed(at, "Europe/Vienna").line("DTSTART")
	if got := zoned.String(); got != "DTSTART;TZID=Europe/Vienna:20260416T090000" {
		t.Errorf("zoned = %q", got)
	}

	utc := Timed(at, "").line("DTSTART")
	if got := utc.String(); got != "DTSTART:20260416T070000Z" {
		t.Errorf("UTC = %q", got)
	}

	allDay := Day(at).line("DTSTART")
	if got := allDay.String(); got != "DTSTART;VALUE=DATE:20260416" {
		t.Errorf("all-day = %q", got)
	}
}

// The reference form is what a list prints beside its local columns and what a
// person types back off that row, so it is the local reading rather than the
// value's own anchor. Here the two agree because the reading is declared to be
// from Vienna, which is where the series is anchored.
func TestStringIsTheLocalReadingOfTheInstant(t *testing.T) {
	loc := vienna(t)
	readingIn(t, loc)
	at := time.Date(2026, 4, 16, 9, 0, 0, 0, loc)
	if got := Timed(at, "Europe/Vienna").String(); got != "2026-04-16T09:00" {
		t.Errorf("String = %q", got)
	}
	if got := Day(at).String(); got != "2026-04-16" {
		t.Errorf("all-day String = %q", got)
	}
}

// A date-time names one instant, whoever reads it. An all-day date names a day, and
// a day begins when the reader's day begins: read as an instant it slips to the day
// before in every zone behind UTC.
func TestInReadsAnAllDayDateAsTheReadersOwnDay(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("America/New_York is not available: %v", err)
	}
	day := Day(time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC))
	if got := day.In(newYork); got.Format("2006-01-02 15:04") != "2026-08-14 00:00" {
		t.Errorf("the day read in New York = %s", got)
	}

	loc := vienna(t)
	at := time.Date(2026, 8, 14, 9, 0, 0, 0, loc)
	timed := Timed(at, "Europe/Vienna")
	if got := timed.In(newYork); !got.Equal(at) {
		t.Errorf("reading a date-time in another zone moved it: %s", got)
	}
	if got := timed.In(newYork).Format("15:04"); got != "03:00" {
		t.Errorf("09:00 in Vienna reads as %s in New York, want 03:00", got)
	}
}

func TestUntilValueIsTheLastSecondOfThePreviousDay(t *testing.T) {
	loc := vienna(t)
	// 26 October 2026 is after the clocks go back, so the previous day ends at
	// 22:59:59 UTC rather than 21:59:59.
	occ := Timed(time.Date(2026, 10, 26, 9, 0, 0, 0, loc), "Europe/Vienna")
	if got := UntilValue(occ); got != "20261025T225959Z" {
		t.Errorf("UntilValue = %q", got)
	}
}

func TestUntilValueOfAnAllDaySeriesIsAFloatingDate(t *testing.T) {
	occ := Day(time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC))
	if got := UntilValue(occ); got != "20260415" {
		t.Errorf("UntilValue = %q", got)
	}
}

func TestLocationFallsBackToUTCForAZoneThisHostDoesNotKnow(t *testing.T) {
	// An event anchored to a zone this build has never heard of is still readable;
	// refusing it outright would hide a real event over a name.
	d := DateTime{Time: time.Now(), TZID: "Mars/Olympus_Mons"}
	if d.Location() != time.UTC {
		t.Errorf("Location = %v, want UTC", d.Location())
	}
}

func TestParseTimeReadsBareValuesInTheGivenZone(t *testing.T) {
	loc := vienna(t)
	got, err := ParseTime("2026-04-16T09:00", loc)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 4, 16, 9, 0, 0, 0, loc); !got.Equal(want) {
		t.Errorf("ParseTime = %v, want %v", got, want)
	}
	if _, err := ParseTime("not a time", loc); err == nil {
		t.Error("ParseTime accepted a value that is not a time")
	}
}

// A reference is printed beside local columns and typed back by a person reading
// them, so it is rendered and read local whatever the series is anchored to.
// Matching is by the instant, so the reference still names the right occurrence.
func TestOccurrenceReferencesRoundTripAcrossAnchors(t *testing.T) {
	loc := vienna(t)
	at := time.Date(2026, 8, 31, 10, 0, 0, 0, loc)

	utcAnchored := VEvent{UID: "u", Start: Timed(at, ""), End: Timed(at.Add(time.Hour), "")}
	zoneAnchored := VEvent{UID: "u", Start: Timed(at, "Europe/Vienna"), End: Timed(at.Add(time.Hour), "Europe/Vienna")}

	for _, v := range []VEvent{utcAnchored, zoneAnchored} {
		printed := v.Start.String()
		if printed != at.Local().Format(refTimeLayout) {
			t.Errorf("a reference reads %q, want the local reading %q", printed, at.Local().Format(refTimeLayout))
		}
		back, err := v.ParseOccurrence(printed)
		if err != nil {
			t.Fatalf("ParseOccurrence(%q): %v", printed, err)
		}
		if !back.Equal(v.Start) {
			t.Errorf("a reference did not name the instant it was printed from: %v vs %v", back.Time, v.Start.Time)
		}
	}
}
