package ical

import "time"

// Window is the range of days a listing asks about, and the zone those days are
// read in.
//
// Days rather than instants, because a range of days is the question every caller
// asks and the only one a date on the command line can express. Carrying it as a
// pair of instants is what lets one end of a call treat the last one as inside the
// range and the other as outside; carrying the days and deriving the instants once,
// here, means nothing can disagree.
type Window struct {
	// from is the instant the first day begins, until the instant after the last
	// day ends. Unexported, because the only useful questions are the two below.
	from, until time.Time
	zone        *time.Location
}

// Days is the window covering first to last inclusive, read in first's own zone.
func Days(first, last time.Time) Window {
	loc := first.Location()
	y, m, d := first.Date()
	from := time.Date(y, m, d, 0, 0, 0, 0, loc)
	y, m, d = last.In(loc).Date()
	return Window{from: from, until: time.Date(y, m, d, 0, 0, 0, 0, loc).AddDate(0, 0, 1), zone: loc}
}

// DaysFrom is the window of n days beginning with the day first falls on.
func DaysFrom(first time.Time, n int) Window {
	return Days(first, first.AddDate(0, 0, n-1))
}

// Covers reports whether an event occupying start to end touches any day in the
// window.
//
// Overlap rather than containment: an event that began before the window and
// reaches into it is on its days. An event that merely ends as the window opens is
// not, and neither is one that starts as the window closes - which is what an
// all-day event on the day after the last one does. An event with no length at all
// is the instant it names.
func (w Window) Covers(start, end DateTime) bool {
	start, end = Span(start, end)
	if w.Ended(start) {
		return false
	}
	s, e := start.In(w.zone), end.In(w.zone)
	return e.After(w.from) || (e.Equal(s) && !s.Before(w.from))
}

// Ended reports whether an occurrence starting here is past the window, which is
// what stops the walk down a series that never ends.
func (w Window) Ended(start DateTime) bool {
	return !start.In(w.zone).Before(w.until)
}

// Bounds are the instants the window spans: the first day's start, and the instant
// the last day ends.
func (w Window) Bounds() (from, until time.Time) { return w.from, w.until }
