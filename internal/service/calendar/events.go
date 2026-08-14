package calendar

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	pgphelper "github.com/roman-16/proton-cli/internal/crypto/pgp"
	"github.com/roman-16/proton-cli/internal/fetch"
	"github.com/roman-16/proton-cli/internal/ical"
	"github.com/roman-16/proton-cli/internal/proton"
	"github.com/roman-16/proton-cli/internal/ref"
	"github.com/roman-16/proton-cli/internal/units"
)

// Event is one thing on a calendar: a one-off event, or one occurrence of a
// recurring series.
//
// A series is stored once and shown many times, so ID names the event a
// reference addresses - the series itself for every occurrence of it - while
// StoredID names the record actually holding this row's content. They differ
// exactly when an occurrence has been edited on its own.
type Event struct {
	ID          string    `json:"id"`
	StoredID    string    `json:"stored_id,omitempty"`
	CalendarID  string    `json:"calendar_id"`
	Title       string    `json:"title"`
	Location    string    `json:"location,omitempty"`
	Description string    `json:"description,omitempty"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	AllDay      bool      `json:"all_day"`
	Zone        string    `json:"zone,omitempty"`
	UID         string    `json:"uid,omitempty"`
	RRule       string    `json:"rrule,omitempty"`
	// Occurrence is this instance's original start in the series' own frame, and
	// the half of a reference that names it. It is empty for a one-off event and
	// for a series addressed as a whole.
	Occurrence string `json:"occurrence,omitempty"`
	// Number is this instance's position in the series.
	Number int `json:"occurrence_number,omitempty"`
	// Count is how many occurrences a series has, set only when the series itself
	// is being reported rather than one of its instances.
	Count int `json:"occurrence_count,omitempty"`
	// Reminders are the triggers before the start, e.g. "-PT15M". Absent means the
	// event takes the calendar's defaults.
	Reminders []string               `json:"reminders,omitempty"`
	Signature pgphelper.VerifyResult `json:"signature,omitempty"`
}

// Recurring reports whether this row belongs to a series.
func (e Event) Recurring() bool { return e.RRule != "" || e.Occurrence != "" }

// ── reading ──

// eventsPageSize is the largest page the events endpoint serves.
const eventsPageSize = 100

// queryTypes are the four windows the events endpoint can be asked for.
//
// Type is not a kind of event, it is a two-by-two selector: part-day or full-day,
// crossed with starting inside the window or having started before it and
// reaching in. Asking only for the first hides every all-day event, and hides
// every recurring series whose first occurrence is in the past - which is how a
// series reaches a later window at all.
var queryTypes = []string{"0", "1", "2", "3"}

type rawNotification struct {
	Type    int
	Trigger string
}

type rawEvent struct {
	ID         string
	CalendarID string
	// StartTime and EndTime are the cleartext times Proton keeps beside the
	// encrypted content. For a full-day event they are the dates it names, held as
	// UTC midnights; for any other event they are instants anchored to the zones
	// below. They are what places an event nobody can decrypt.
	StartTime            int64
	EndTime              int64
	StartTimezone        string
	EndTimezone          string
	FullDay              int
	UID                  string
	IsOrganizer          int
	IsProtonProtonInvite int
	AddressID            string
	Color                *string
	Notifications        []rawNotification
	AttendeesInfo        struct {
		Attendees     []rawAttendee
		MoreAttendees int
	}
	SharedKeyPacket  string
	AddressKeyPacket string
	SharedEvents     []map[string]any
	AttendeesEvents  []map[string]any
}

// rawAttendee is the cleartext per-attendee record from AttendeesInfo: it maps
// the deterministic X-PM token to the server attendee ID and current status.
type rawAttendee struct {
	ID     string
	Token  string
	Status int
}

// notifications re-renders the reminder list for a write, preserving the
// difference between "none" and "the calendar's defaults".
func (e rawEvent) notifications() []map[string]any {
	if e.Notifications == nil {
		return nil
	}
	out := make([]map[string]any, 0, len(e.Notifications))
	for _, n := range e.Notifications {
		out = append(out, map[string]any{"Type": n.Type, "Trigger": n.Trigger})
	}
	return out
}

func (e rawEvent) triggers() []string {
	out := make([]string, 0, len(e.Notifications))
	for _, n := range e.Notifications {
		out = append(out, n.Trigger)
	}
	return out
}

// stored is an event as Proton holds it, together with what its cards said.
type stored struct {
	raw   rawEvent
	model ical.VEvent
	sig   pgphelper.VerifyResult
	// readErr records that the content could not be read. Such an event is still
	// reported, from its cleartext times, because a row you cannot read is worth
	// seeing; it is never expanded or written back.
	readErr error
}

// decrypt resolves which key the event's content is wrapped to and reads it.
//
// The attendee list is read separately and best-effort. It lives in its own card,
// it is not part of what the event says, and an event that arrived without one this
// key can open is still an event: folding it into the same read would make a
// missing participant list look like an unreadable event.
func (s *Service) decrypt(ctx context.Context, ck *calKeys, raw rawEvent) stored {
	// An invitation you received wraps its content to the invited address rather
	// than to the calendar key.
	packet, decKR := raw.SharedKeyPacket, ck.calKR
	if packet == "" && raw.AddressKeyPacket != "" {
		if kr, ok := s.addressKeyRing(ctx, raw.AddressID); ok {
			packet, decKR = raw.AddressKeyPacket, kr
		}
	}
	model, sig, err := decryptEvent(raw.SharedEvents, packet, decKR, ck.addrKR)
	if err == nil && len(raw.AttendeesEvents) > 0 {
		if attendees, _, aerr := decryptEvent(raw.AttendeesEvents, packet, decKR, ck.addrKR); aerr == nil {
			model.Attendees = attendees.Attendees
		}
	}
	return stored{raw: raw, model: model, sig: sig, readErr: err}
}

// EventsList returns everything the window covers on the given calendars,
// expanding each series into the occurrences that fall in it.
//
// A calendar that cannot be read is left out rather than allowed to empty the
// answer: the list of calendars is eventually consistent, so one that was deleted
// a moment ago can still be named, and "what is on my calendars" is worth
// answering from the ones that are there. Only when nothing could be read at all
// is that reported - which is also what makes a single named calendar strict,
// since then the one failure is the only one.
func (s *Service) EventsList(ctx context.Context, calendarIDs []string, w ical.Window) ([]Event, error) {
	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		out   []Event
		first error
		read  int
	)
	for _, calID := range calendarIDs {
		wg.Add(1)
		go func(calID string) {
			defer wg.Done()
			events, err := s.calendarEvents(ctx, calID, w)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if first == nil {
					first = err
				}
				slog.Debug("calendar: skipped a calendar that could not be read",
					"calendar", calID, "error", err)
				return
			}
			read++
			out = append(out, events...)
		}(calID)
	}
	wg.Wait()
	if read == 0 && first != nil {
		return nil, first
	}
	slices.SortStableFunc(out, func(a, b Event) int {
		if c := a.Start.Compare(b.Start); c != 0 {
			return c
		}
		return strings.Compare(a.Title, b.Title)
	})
	return out, nil
}

func (s *Service) calendarEvents(ctx context.Context, calendarID string, w ical.Window) ([]Event, error) {
	ck, err := s.unlockCalendar(ctx, calendarID)
	if err != nil {
		return nil, err
	}
	raws, err := s.rawEventsBetween(ctx, calendarID, w)
	if err != nil {
		return nil, err
	}
	events := make([]stored, 0, len(raws))
	for _, raw := range raws {
		events = append(events, s.decrypt(ctx, ck, raw))
	}
	return expand(events, w), nil
}

// rawEventsBetween asks for all four windows and pages each one.
//
// The four are independent queries over the same range, so they run together:
// serialising them would quadruple the wall-clock of a call that is already
// waiting on the network. An event can legitimately answer more than one of them,
// so the union is deduplicated.
func (s *Service) rawEventsBetween(ctx context.Context, calendarID string, w ical.Window) ([]rawEvent, error) {
	from, to := fetchBounds(w)
	byType := make([][]rawEvent, len(queryTypes))
	queries := make([]func(context.Context) error, len(queryTypes))
	for i, typ := range queryTypes {
		queries[i] = func(ctx context.Context) error {
			page, err := s.rawEventsOfType(ctx, calendarID, from, to, typ)
			byType[i] = page
			return err
		}
	}
	if err := fetch.Together(ctx, queries...); err != nil {
		return nil, err
	}

	var out []rawEvent
	seen := map[string]bool{}
	for _, page := range byType {
		for _, e := range page {
			if seen[e.ID] {
				continue
			}
			seen[e.ID] = true
			out = append(out, e)
		}
	}
	return out, nil
}

// fetchBounds are the instants the events endpoint is asked for: the window, a day
// wider at each end.
//
// Wider on purpose. An all-day event names a date rather than an instant, so Proton
// holds it at an instant up to a day from the day it belongs to here, and the
// endpoint's own idea of which events touch the edge of a range is not this CLI's.
// What is fetched only has to contain the answer; the window decides it.
func fetchBounds(w ical.Window) (from, to time.Time) {
	first, until := w.Bounds()
	return first.AddDate(0, 0, -1), until.AddDate(0, 0, 1)
}

func (s *Service) rawEventsOfType(ctx context.Context, calendarID string, from, to time.Time, typ string) ([]rawEvent, error) {
	var out []rawEvent
	for page := 0; ; page++ {
		q := url.Values{}
		q.Set("Start", fmt.Sprintf("%d", max(from.Unix(), 0)))
		q.Set("End", fmt.Sprintf("%d", max(to.Unix(), 0)))
		// UTC, because that is the frame in which a full-day event's cleartext times
		// are the dates it names.
		q.Set("Timezone", "UTC")
		q.Set("Type", typ)
		q.Set("Page", fmt.Sprintf("%d", page))
		q.Set("PageSize", fmt.Sprintf("%d", eventsPageSize))

		var r struct {
			Events []rawEvent
			More   int
		}
		if err := s.C.Decode(ctx, proton.Request{
			Method: "GET", Path: "/calendar/v1/" + calendarID + "/events", Query: q,
		}, &r); err != nil {
			return nil, err
		}
		out = append(out, r.Events...)
		if r.More != 1 || len(r.Events) == 0 {
			return out, nil
		}
	}
}

// expand turns stored events into the rows a person sees: a one-off is itself, a
// series becomes its occurrences in the window, and an occurrence that has been
// edited on its own replaces the one the rule would have generated.
//
// Every row is put to the window by the same rule, whether it came from a stored
// event or from a rule this CLI expanded. The endpoint is asked for more than the
// window holds, so the window is what makes the answer the one that was asked for
// rather than the one the server happened to return.
func expand(events []stored, w ical.Window) []Event {
	masters := map[string]stored{}
	overrides := map[string][]stored{}
	var plain []stored
	for _, e := range events {
		switch {
		case e.readErr != nil:
			plain = append(plain, e)
		case e.model.IsOverride():
			overrides[e.raw.UID] = append(overrides[e.raw.UID], e)
		case e.model.Recurring():
			masters[e.raw.UID] = e
		default:
			plain = append(plain, e)
		}
	}

	var out []Event
	for _, e := range plain {
		if w.Covers(e.when()) {
			out = append(out, e.row())
		}
	}

	for uid, master := range masters {
		replaced := make([]ical.DateTime, 0, len(overrides[uid]))
		for _, o := range overrides[uid] {
			replaced = append(replaced, *o.model.RecurrenceID)
		}
		occurrences, err := master.model.Occurrences(w)
		if err != nil {
			// A rule this build cannot read still describes a real event, so the
			// series is reported at its own start rather than dropped.
			out = append(out, master.row())
			continue
		}
		for _, occ := range occurrences {
			if slices.ContainsFunc(replaced, occ.Start.Equal) {
				continue
			}
			out = append(out, master.occurrenceRow(occ))
		}
	}

	for uid, list := range overrides {
		master, hasMaster := masters[uid]
		for _, o := range list {
			if !w.Covers(o.when()) {
				continue
			}
			row := o.row()
			if hasMaster {
				// An occurrence is addressed by where it sits in its series, not by
				// which record happens to hold it, so that the reference does not
				// change the first time somebody edits it.
				row.ID = master.raw.ID
				row.StoredID = o.raw.ID
				row.Occurrence = master.model.Start.At(o.model.RecurrenceID.Time).String()
				row.RRule = master.model.RRule
			} else {
				row.Occurrence = o.model.RecurrenceID.String()
			}
			out = append(out, row)
		}
	}
	return out
}

// when is the pair of values the event occupies, whether or not its content could
// be read.
//
// An event nobody can decrypt is still placed on the right day, because the times
// Proton keeps in the clear beside it are the same values its content carries.
func (e stored) when() (start, end ical.DateTime) {
	if e.readErr == nil {
		return e.model.Span()
	}
	if e.raw.FullDay == 1 {
		return ical.Span(
			ical.Day(time.Unix(e.raw.StartTime, 0).UTC()),
			ical.Day(time.Unix(e.raw.EndTime, 0).UTC()))
	}
	return ical.Span(
		ical.Timed(time.Unix(e.raw.StartTime, 0), e.raw.StartTimezone),
		ical.Timed(time.Unix(e.raw.EndTime, 0), e.raw.EndTimezone))
}

// row reports the stored event as itself.
//
// The times are read in the zone the reader is in, which for an all-day event is
// the only way to name the day it is on: it carries a date and no instant, so
// placing it anywhere else moves it to the day before or after.
func (e stored) row() Event {
	start, end := e.when()
	ev := Event{
		ID:         e.raw.ID,
		CalendarID: e.raw.CalendarID,
		UID:        e.raw.UID,
		Start:      start.In(time.Local),
		End:        end.In(time.Local),
		AllDay:     start.AllDay,
		Zone:       start.TZID,
		Signature:  e.sig,
		Reminders:  e.raw.triggers(),
	}
	if e.readErr != nil {
		return ev
	}
	ev.Title = e.model.Summary
	ev.Location = e.model.Location
	ev.Description = e.model.Description
	ev.RRule = e.model.RRule
	return ev
}

// occurrenceRow reports one instance of a series.
func (e stored) occurrenceRow(occ ical.Occurrence) Event {
	ev := e.row()
	ev.Start = occ.Start.In(time.Local)
	ev.End = occ.End.In(time.Local)
	ev.Occurrence = occ.Start.String()
	ev.Number = occ.Number
	return ev
}

// EventGet reports one event. An empty occurrence names the stored event, which
// for a series is the series itself; otherwise it names one instance.
//
// The event and the keys that read it are asked for at the same time. The
// reference names the event, so neither request needs the other's answer - only
// the decryption between them does.
func (s *Service) EventGet(ctx context.Context, calendarID, eventID, occurrence string) (*Event, error) {
	var (
		ck  *calKeys
		raw rawEvent
	)
	if err := fetch.Together(ctx,
		func(ctx context.Context) error {
			var err error
			ck, err = s.unlockCalendar(ctx, calendarID)
			return err
		},
		func(ctx context.Context) error {
			var err error
			raw, err = s.rawEvent(ctx, calendarID, eventID)
			return err
		},
	); err != nil {
		return nil, err
	}
	e := s.decrypt(ctx, ck, raw)
	if occurrence == "" {
		ev := e.row()
		if e.readErr == nil && e.model.Recurring() {
			if n, err := e.model.CountOccurrences(maxSeriesReport); err == nil {
				ev.Count = n
			}
		}
		return &ev, nil
	}
	if e.readErr != nil {
		return nil, e.readErr
	}
	occ, err := s.resolveOccurrence(ctx, ck, calendarID, e, occurrence)
	if err != nil {
		return nil, err
	}
	ev := occ.row()
	return &ev, nil
}

// maxSeriesReport caps the occurrence count reported for an unbounded series, so
// `get` on "every weekday forever" answers rather than counting forever.
const maxSeriesReport = 1000

// EventOccurrences lists a series' occurrences from its start, up to limit.
//
// It is what a confirmation shows before removing a whole series: a count is not
// enough to check, and the occurrences are the things that would go.
func (s *Service) EventOccurrences(ctx context.Context, calendarID, eventID string, limit int) ([]Event, error) {
	ck, err := s.unlockCalendar(ctx, calendarID)
	if err != nil {
		return nil, err
	}
	master, err := s.readMaster(ctx, ck, calendarID, eventID)
	if err != nil {
		return nil, err
	}
	if !master.model.Recurring() {
		return []Event{master.row()}, nil
	}
	chain, err := s.loadSeries(ctx, ck, calendarID, master)
	if err != nil {
		return nil, err
	}
	var out []Event
	if err := master.model.Walk(func(occ ical.Occurrence) bool {
		if override := chain.overrideAt(occ.Start); override != nil {
			row := override.row()
			row.ID = master.raw.ID
			row.StoredID = override.raw.ID
			row.Occurrence = occ.Start.String()
			row.Number = occ.Number
			row.RRule = master.model.RRule
			out = append(out, row)
		} else {
			out = append(out, master.occurrenceRow(occ))
		}
		return len(out) < limit
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) rawEvent(ctx context.Context, calendarID, eventID string) (rawEvent, error) {
	var r struct{ Event rawEvent }
	if err := s.C.Decode(ctx, proton.Request{
		Method: "GET", Path: "/calendar/v1/" + calendarID + "/events/" + eventID,
	}, &r); err != nil {
		return rawEvent{}, err
	}
	return r.Event, nil
}

func (s *Service) storedEvent(ctx context.Context, ck *calKeys, calendarID, eventID string) (stored, error) {
	raw, err := s.rawEvent(ctx, calendarID, eventID)
	if err != nil {
		return stored{}, err
	}
	return s.decrypt(ctx, ck, raw), nil
}

// ── creating ──

// EventInput is the full set of fields for a new event.
type EventInput struct {
	Title       string
	Location    string
	Description string
	Start, End  time.Time
	AllDay      bool
	// Zone anchors the event, so a recurring series keeps its wall-clock time
	// across a daylight-saving change instead of drifting by an hour.
	Zone      string
	RRule     string
	Reminders []string
	Attendees []string
}

// EventResult is the outcome of a write. Mail is non-nil when participants have
// to be told by email, which is the case for attendees Proton cannot reach
// through their own calendar.
type EventResult struct {
	ID   string
	Ref  string
	Mail *Mail
}

// Mail is a ready-to-send iCalendar message.
type Mail struct {
	ICS        string
	Recipients []string
	Subject    string
	Body       string
	Method     string
}

func (s *Service) EventCreate(ctx context.Context, calendarID string, in EventInput) (*EventResult, error) {
	ck, err := s.unlockCalendar(ctx, calendarID)
	if err != nil {
		return nil, err
	}
	notifs, err := buildReminders(in.Reminders)
	if err != nil {
		return nil, err
	}
	v := ical.VEvent{
		UID:         ical.EventUID(),
		Summary:     in.Title,
		Location:    in.Location,
		Description: in.Description,
		RRule:       in.RRule,
	}
	loc, err := zoneOf(in.Zone)
	if err != nil {
		return nil, err
	}
	v = withTimes(v, in.Start, in.End, in.AllDay, loc.String())
	if len(in.Attendees) > 0 {
		v.Organizer = ck.email
	}

	body := eventBody{model: v, notifications: notifs, isOrganizer: 1}
	external, err := s.attachAttendees(ctx, &body, in.Attendees)
	if err != nil {
		return nil, err
	}
	op, _, err := ck.createOp(body)
	if err != nil {
		return nil, err
	}
	created, err := s.sync(ctx, calendarID, ck.memberID, []syncOp{op})
	if err != nil {
		return nil, err
	}
	res := &EventResult{}
	if len(created) > 0 {
		res.ID = created[0]
	}
	if len(external) > 0 {
		res.Mail = inviteMail(body.model, external)
	}
	return res, nil
}

// attachAttendees resolves the participants onto an event: their tokens, the
// cleartext record, the keys their copies have to be readable with, and the
// addresses Proton cannot deliver to, which have to be emailed instead.
func (s *Service) attachAttendees(ctx context.Context, body *eventBody, emails []string) (external []string, err error) {
	if len(emails) == 0 {
		return nil, nil
	}
	atts, clear, keys, external, err := s.resolveAttendees(ctx, body.model.UID, emails)
	if err != nil {
		return nil, err
	}
	body.model.Attendees = atts
	body.attendeeList = clear
	body.attendeeKeys = keys
	return external, nil
}

func inviteMail(v ical.VEvent, recipients []string) *Mail {
	return &Mail{
		ICS:        v.Document("REQUEST"),
		Recipients: recipients,
		Subject:    "Invitation: " + v.Summary,
		Body: fmt.Sprintf("You have been invited to %q.\n\nThe calendar invitation is attached.",
			v.Summary),
		Method: "REQUEST",
	}
}

func updateMail(v ical.VEvent, recipients []string) *Mail {
	return &Mail{
		ICS:        v.Document("REQUEST"),
		Recipients: recipients,
		Subject:    "Updated invitation: " + v.Summary,
		Body: fmt.Sprintf("%q has been updated.\n\nThe updated calendar invitation is attached.",
			v.Summary),
		Method: "REQUEST",
	}
}

// resolveAttendees computes per-attendee tokens, the cleartext Attendees list, the
// keys of the participants with Proton accounts, and the addresses without one,
// which have to be emailed.
//
// None of it needs the event's session key, which is what lets the key be made
// once, when the cards are built.
func (s *Service) resolveAttendees(ctx context.Context, uid string, emails []string) (atts []ical.Attendee, clear []map[string]any, keys []protonAttendee, external []string, err error) {
	written := make([]string, 0, len(emails))
	for _, raw := range emails {
		if e := strings.TrimSpace(raw); e != "" {
			written = append(written, e)
		}
	}
	canonical, err := s.canonicalEmails(ctx, written)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	seen := map[canonicalAddr]bool{}
	for _, email := range written {
		// Two addresses Proton reduces to the same one are the same person, so
		// the canonical form is what decides a duplicate.
		if seen[canonical[email]] {
			continue
		}
		seen[canonical[email]] = true
		token := attendeeToken(uid, canonical[email])
		atts = append(atts, ical.Attendee{Email: email, Token: token})
		clear = append(clear, map[string]any{"Token": token, "Status": 0})

		kr, err := s.attendeeKeyRing(ctx, email)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if kr == nil {
			external = append(external, email)
			continue
		}
		keys = append(keys, protonAttendee{email: email, kr: kr})
	}
	return atts, clear, keys, external, nil
}

// attendeeKeyRing is an address's public key, or nil when the address has no
// Proton account and therefore no calendar to deliver to.
func (s *Service) attendeeKeyRing(ctx context.Context, email string) (*pgp.KeyRing, error) {
	var r struct {
		Address struct {
			Keys []struct{ PublicKey string }
		}
	}
	if err := s.C.Decode(ctx, proton.Request{
		Method: "GET", Path: "/core/v4/keys/all", Query: proton.Query("Email", email),
	}, &r); err != nil {
		return nil, err
	}
	if len(r.Address.Keys) == 0 {
		return nil, nil
	}
	key, err := pgp.NewKeyFromArmored(r.Address.Keys[0].PublicKey)
	if err != nil {
		return nil, fmt.Errorf("parse the key for %s: %w", email, err)
	}
	return pgp.NewKeyRing(key)
}

// ── resolving ──

// ResolveEvent finds an event by title across every calendar over the days the
// default window covers.
func (s *Service) ResolveEvent(ctx context.Context, needle string) (calendarID, eventID, occurrence string, err error) {
	cals, err := s.CalendarsList(ctx)
	if err != nil {
		return "", "", "", err
	}
	ids := make([]string, 0, len(cals))
	for _, c := range cals {
		ids = append(ids, c.ID)
	}
	events, err := s.EventsList(ctx, ids, ical.Days(DefaultDays()))
	if err != nil {
		return "", "", "", err
	}
	var matches []Event
	for _, e := range events {
		if e.Title != "" && strings.Contains(strings.ToLower(e.Title), strings.ToLower(needle)) {
			matches = append(matches, e)
		}
	}
	m, err := ref.Pick("event", needle, matches,
		func(e Event) string { return e.ID },
		func(e Event) string {
			return fmt.Sprintf("%s  %s  (calendar %s)", e.Start.Local().Format("2006-01-02 15:04"), e.Title, e.CalendarID)
		})
	if err != nil {
		return "", "", "", err
	}
	return m.CalendarID, m.ID, m.Occurrence, nil
}

// ── reminders ──

func buildReminders(reminders []string) ([]map[string]any, error) {
	if reminders == nil {
		return nil, nil
	}
	out := make([]map[string]any, 0, len(reminders))
	for _, r := range reminders {
		trig, err := icalTrigger(r)
		if err != nil {
			return nil, fmt.Errorf("invalid reminder %q: %w", r, err)
		}
		// Type 1 = device notification.
		out = append(out, map[string]any{"Type": 1, "Trigger": trig})
	}
	return out, nil
}

// icalTrigger converts a duration-before-start ("15m", "1h", "1d") into an iCal
// negative trigger ("-PT15M", "-PT60M", "-P1D").
func icalTrigger(dur string) (string, error) {
	d, err := units.ParseDuration(dur)
	if err != nil {
		return "", err
	}
	if d <= 0 {
		return "", fmt.Errorf("must be positive")
	}
	if d%(24*time.Hour) == 0 {
		return fmt.Sprintf("-P%dD", int(d/(24*time.Hour))), nil
	}
	return fmt.Sprintf("-PT%dM", int(d/time.Minute)), nil
}
