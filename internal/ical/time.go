package ical

import (
	"fmt"
	"strings"
	"time"

	"github.com/cmdruid/proton-cli/internal/contentline"
)

// Layouts for the three shapes an iCalendar date value takes.
const (
	dateLayout      = "20060102"
	dateTimeLayout  = "20060102T150405"
	utcDateTime     = "20060102T150405Z"
	stampLayout     = "20060102T150405Z"
	refTimeLayout   = "2006-01-02T15:04"
	refDateLayout   = "2006-01-02"
	untilDayEndHour = 23
)

// DateTime is an iCalendar date or date-time value together with the anchor it
// was written against.
//
// The anchor is not decoration. A weekly event at 09:00 anchored to
// Europe/Vienna stays at 09:00 across a daylight-saving change; the same event
// stored as a UTC instant moves to 08:00. Proton's clients always write an
// anchor, so the CLI has to carry one.
type DateTime struct {
	// Time is the instant. For an all-day value it is midnight UTC on that day,
	// because an all-day date has no time of day and no zone to have one in.
	Time time.Time
	// TZID is the IANA zone the value is anchored to. Empty means the value is
	// written as a UTC instant.
	TZID string
	// AllDay marks a VALUE=DATE value: a whole day, with no time of day.
	AllDay bool
}

// Timed builds a date-time anchored to an IANA zone. An empty zone anchors to
// UTC.
func Timed(t time.Time, tzid string) DateTime {
	return DateTime{Time: t, TZID: tzid}
}

// Day builds an all-day value for the calendar day t falls on in its own
// location.
func Day(t time.Time) DateTime {
	y, m, d := t.Date()
	return DateTime{Time: time.Date(y, m, d, 0, 0, 0, 0, time.UTC), AllDay: true}
}

// IsZero reports whether the value is absent.
func (d DateTime) IsZero() bool { return d.Time.IsZero() }

// Equal compares two values by the instant they name.
func (d DateTime) Equal(o DateTime) bool { return d.Time.Equal(o.Time) }

// Location resolves the anchor, falling back to UTC when the zone is empty or
// unknown to this system.
func (d DateTime) Location() *time.Location {
	if d.TZID == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(d.TZID)
	if err != nil {
		return time.UTC
	}
	return loc
}

// Wall is the value's wall-clock reading in its own anchor, which is what the
// serialized form carries and what recurrence arithmetic has to advance.
func (d DateTime) Wall() time.Time {
	if d.AllDay {
		return d.Time
	}
	return d.Time.In(d.Location())
}

// In returns the instant the value names when it is read in loc.
//
// A date-time names one instant whatever zone reads it, so it is only
// re-expressed. An all-day date names no instant at all: it is a day, and a day
// begins when the reader's own day begins. Reading it anywhere else is what puts a
// whole-day event on the wrong date.
func (d DateTime) In(loc *time.Location) time.Time {
	if !d.AllDay {
		return d.Time.In(loc)
	}
	y, m, day := d.Time.Date()
	return time.Date(y, m, day, 0, 0, 0, 0, loc)
}

// At returns the same anchor and all-day-ness carrying a different instant, so
// a derived value cannot accidentally lose the series' zone.
func (d DateTime) At(t time.Time) DateTime {
	if d.AllDay {
		return Day(t)
	}
	return DateTime{Time: t, TZID: d.TZID}
}

// line renders the value as a content line under the given property name.
func (d DateTime) line(name string) contentline.Line {
	switch {
	case d.AllDay:
		return contentline.Line{
			Name:   name,
			Params: contentline.Params{{Name: "VALUE", Value: "DATE"}},
			Value:  d.Time.Format(dateLayout),
		}
	case d.TZID != "":
		return contentline.Line{
			Name:   name,
			Params: contentline.Params{{Name: "TZID", Value: d.TZID}},
			Value:  d.Time.In(d.Location()).Format(dateTimeLayout),
		}
	default:
		return contentline.Line{Name: name, Value: d.Time.UTC().Format(utcDateTime)}
	}
}

// String renders the value the way a reference names it: the local reading, to
// the minute, or a bare date when it is all-day.
//
// Local rather than the value's own anchor, because this is the string a list
// prints inside an occurrence reference and a person types back, and the columns
// beside it are local too. A reference that read 08:00 next to a row that read
// 10:00 would be one nobody could copy.
func (d DateTime) String() string {
	if d.AllDay {
		return d.Time.Format(refDateLayout)
	}
	return d.Time.Local().Format(refTimeLayout)
}

// parseValues reads every date value on a line. EXDATE carries a comma-separated
// list; DTSTART and RECURRENCE-ID carry one.
func parseValues(l contentline.Line) ([]DateTime, error) {
	tzid := l.Params.Get("TZID")
	allDay := strings.EqualFold(l.Params.Get("VALUE"), "DATE")
	out := make([]DateTime, 0, 1)
	for _, raw := range contentline.SplitList(l.Value) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		d, err := parseValue(raw, tzid, allDay)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", l.Name, err)
		}
		out = append(out, d)
	}
	return out, nil
}

func parseValue(raw, tzid string, allDay bool) (DateTime, error) {
	if allDay || len(raw) == len(dateLayout) {
		t, err := time.ParseInLocation(dateLayout, raw, time.UTC)
		if err != nil {
			return DateTime{}, err
		}
		return DateTime{Time: t, AllDay: true}, nil
	}
	if strings.HasSuffix(raw, "Z") {
		t, err := time.ParseInLocation(utcDateTime, raw, time.UTC)
		if err != nil {
			return DateTime{}, err
		}
		return DateTime{Time: t}, nil
	}
	loc := time.UTC
	if tzid != "" {
		l, err := time.LoadLocation(tzid)
		if err != nil {
			// An unknown zone is a value we can still place, just not anchor.
			// Refusing the whole event over a zone this host has never heard of
			// would make the event unreadable rather than merely unanchored.
			tzid = ""
		} else {
			loc = l
		}
	}
	t, err := time.ParseInLocation(dateTimeLayout, raw, loc)
	if err != nil {
		return DateTime{}, err
	}
	return DateTime{Time: t, TZID: tzid}, nil
}

// ParseTime accepts the date and time formats a person types, reading bare
// dates and times in loc.
func ParseTime(s string, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.Local
	}
	for _, f := range []string{
		time.RFC3339,
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	} {
		if t, err := time.ParseInLocation(f, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized time format: %s", s)
}

// UntilValue is the UNTIL that ends a series just before the given occurrence.
//
// The rule Proton follows: an all-day series gets the floating date of the
// previous day, and a timed series gets the last second of the previous day in
// the series' own zone, expressed as the UTC instant the RFC requires UNTIL to
// be.
func UntilValue(before DateTime) string {
	wall := before.Wall().AddDate(0, 0, -1)
	if before.AllDay {
		return wall.Format(dateLayout)
	}
	loc := before.Location()
	endOfDay := time.Date(wall.Year(), wall.Month(), wall.Day(), untilDayEndHour, 59, 59, 0, loc)
	return endOfDay.UTC().Format(utcDateTime)
}

// stamp renders a DTSTAMP, which is always a UTC instant.
func stamp(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return t.UTC().Format(stampLayout)
}
