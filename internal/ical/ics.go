package ical

import (
	"strconv"

	"github.com/roman-16/proton-cli/internal/contentline"
)

// A self-contained iCalendar document is what goes out by email: an invitation,
// an update to one, or a reply. Unlike a stored card it carries a METHOD and every
// property in one component, because the recipient's client has nothing else to
// join it to.

// Document renders the event as a complete VCALENDAR with the given METHOD,
// suitable for attaching as text/calendar.
//
// REQUEST covers both a first invitation and an update to one; which it is, the
// recipient works out from the sequence.
func (v VEvent) Document(method string) string {
	lines := []contentline.Line{
		{Name: "BEGIN", Value: "VCALENDAR"},
		{Name: "VERSION", Value: "2.0"},
		{Name: "PRODID", Value: prodID},
		{Name: "METHOD", Value: method},
		{Name: "CALSCALE", Value: "GREGORIAN"},
		{Name: "BEGIN", Value: "VEVENT"},
		{Name: "UID", Value: v.UID},
		{Name: "DTSTAMP", Value: stamp(v.DTStamp)},
		v.Start.line("DTSTART"),
	}
	if !v.End.IsZero() {
		lines = append(lines, v.End.line("DTEND"))
	}
	if v.RecurrenceID != nil {
		lines = append(lines, v.RecurrenceID.line("RECURRENCE-ID"))
	}
	if v.RRule != "" {
		lines = append(lines, contentline.Line{Name: "RRULE", Value: v.RRule})
	}
	for _, d := range v.ExDates {
		lines = append(lines, d.line("EXDATE"))
	}
	if v.Summary != "" {
		lines = append(lines, contentline.Line{Name: "SUMMARY", Value: contentline.EscapeText(v.Summary)})
	}
	if v.Location != "" {
		lines = append(lines, contentline.Line{Name: "LOCATION", Value: contentline.EscapeText(v.Location)})
	}
	if v.Description != "" {
		lines = append(lines, contentline.Line{Name: "DESCRIPTION", Value: contentline.EscapeText(v.Description)})
	}
	if v.Organizer != "" {
		lines = append(lines, contentline.Line{
			Name:   "ORGANIZER",
			Params: contentline.Params{{Name: "CN", Value: v.Organizer}},
			Value:  "mailto:" + v.Organizer,
		})
	}
	for _, a := range v.Attendees {
		lines = append(lines, attendeeLine(a))
	}
	lines = append(lines,
		contentline.Line{Name: "SEQUENCE", Value: strconv.Itoa(v.Sequence)},
		contentline.Line{Name: "END", Value: "VEVENT"},
		contentline.Line{Name: "END", Value: "VCALENDAR"},
	)
	return contentline.Render(lines)
}

// ReplyDocument renders the METHOD:REPLY document that answers an invitation. It
// carries one ATTENDEE line, the responder's, tagged with the answer.
//
// protonReply adds the marker Proton sets on Proton-to-Proton replies.
func (v VEvent) ReplyDocument(attendeeEmail, partstat string, protonReply bool) string {
	reply := v
	reply.Description = ""
	reply.Attendees = []Attendee{{Email: attendeeEmail, PartStat: partstat}}
	doc := reply.Document("REPLY")
	if !protonReply {
		return doc
	}
	// The marker sits with the other event properties, so it goes in before the
	// attendee line that closes the component's meaningful content.
	return insertBefore(doc, "ATTENDEE;", "X-PM-PROTON-REPLY;TYPE=boolean:true")
}

// insertBefore puts a line ahead of the first line starting with prefix.
func insertBefore(doc, prefix, line string) string {
	lines := contentline.Unfold(doc)
	out := make([]string, 0, len(lines)+1)
	inserted := false
	for _, l := range lines {
		if !inserted && len(l) >= len(prefix) && l[:len(prefix)] == prefix {
			out = append(out, line)
			inserted = true
		}
		out = append(out, l)
	}
	if !inserted {
		out = append(out, line)
	}
	return joinCRLF(out)
}

func joinCRLF(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\r\n"
		}
		out += l
	}
	return out
}
