// Package calendar provides Proton Calendar operations.
package calendar

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/crypto/ical"
	"github.com/roman-16/proton-cli/internal/crypto/keys"
	pgphelper "github.com/roman-16/proton-cli/internal/crypto/pgp"
	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/proton"
	"github.com/roman-16/proton-cli/internal/render"
)

type Service struct{ C proton.Doer }

func New(c proton.Doer) *Service { return &Service{C: c} }

type Calendar struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description,omitempty"`
	MemberCount int    `json:"member_count"`
}

type Event struct {
	ID          string                 `json:"id"`
	CalendarID  string                 `json:"calendar_id"`
	Title       string                 `json:"title"`
	Location    string                 `json:"location,omitempty"`
	Description string                 `json:"description,omitempty"`
	RRule       string                 `json:"rrule,omitempty"`
	Start       time.Time              `json:"start"`
	End         time.Time              `json:"end"`
	AllDay      bool                   `json:"all_day"`
	UID         string                 `json:"uid,omitempty"`
	Signature   pgphelper.VerifyResult `json:"signature,omitempty"`
}

type calKeys struct {
	calKR    *pgp.KeyRing
	addrKR   *pgp.KeyRing
	memberID string
	email    string
}

// CalendarsList reads per-user prefs (Name/Color/Description) from Members[0].
func (s *Service) CalendarsList(ctx context.Context) ([]Calendar, error) {
	var r struct {
		Calendars []struct {
			ID      string
			Members []struct {
				Name        string
				Color       string
				Description string
				Email       string
			}
		}
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/calendar/v1"}, &r); err != nil {
		return nil, err
	}
	out := make([]Calendar, 0, len(r.Calendars))
	for _, c := range r.Calendars {
		var name, color, desc string
		if len(c.Members) > 0 {
			name = c.Members[0].Name
			color = c.Members[0].Color
			desc = c.Members[0].Description
		}
		out = append(out, Calendar{ID: c.ID, Name: name, Color: color, Description: desc, MemberCount: len(c.Members)})
	}
	return out, nil
}

func (s *Service) CalendarCreate(ctx context.Context, u *keys.Unlocked, name, color string) (string, error) {
	addrKR, addrID, _, err := u.PrimaryAddrKR()
	if err != nil {
		return "", err
	}
	var r struct{ Calendar struct{ ID string } }
	if err := s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: "/calendar/v1",
		Body: map[string]any{"Name": name, "Color": color, "Display": 1, "AddressID": addrID},
	}, &r); err != nil {
		return "", err
	}

	// A freshly created calendar has no keys; provision them (setupCalendar)
	// so the calendar can hold events and accept member updates.
	payload, err := pgphelper.GenerateCalendarKey(addrKR)
	if err != nil {
		return "", fmt.Errorf("generate calendar key: %w", err)
	}
	if err := s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: "/calendar/v1/" + r.Calendar.ID + "/keys",
		Body: map[string]any{
			"AddressID":  addrID,
			"PrivateKey": payload.PrivateKey,
			"Passphrase": map[string]any{"DataPacket": payload.DataPacket, "KeyPacket": payload.KeyPacket},
			"Signature":  payload.Signature,
		},
	}, nil); err != nil {
		return "", fmt.Errorf("set up calendar keys: %w", err)
	}
	return r.Calendar.ID, nil
}

// CalendarDelete requires the caller to have unlocked the password scope first.
func (s *Service) CalendarDelete(ctx context.Context, id string) error {
	return s.C.Decode(ctx, proton.Request{Method: "DELETE", Path: "/calendar/v1/" + id}, nil)
}

func (s *Service) calendarMemberID(ctx context.Context, u *keys.Unlocked, calendarID string) (string, error) {
	var mem struct {
		Members []struct {
			ID, AddressID string
		}
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/calendar/v1/" + calendarID + "/members"}, &mem); err != nil {
		return "", err
	}
	for _, m := range mem.Members {
		if _, ok := u.AddrKR(m.AddressID); ok {
			return m.ID, nil
		}
	}
	return "", fmt.Errorf("no matching member for calendar %s", calendarID)
}

// CalendarRename updates a calendar's display name and/or color (stored as
// per-member settings). Empty fields are left unchanged.
func (s *Service) CalendarRename(ctx context.Context, u *keys.Unlocked, calendarID, name, color string) error {
	memberID, err := s.calendarMemberID(ctx, u, calendarID)
	if err != nil {
		return err
	}
	body := map[string]any{}
	if name != "" {
		body["Name"] = name
	}
	if color != "" {
		body["Color"] = color
	}
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: fmt.Sprintf("/calendar/v1/%s/members/%s", calendarID, memberID), Body: body,
	}, nil)
}

func (s *Service) ResolveCalendarID(ctx context.Context, nameOrID string) (string, error) {
	cals, err := s.CalendarsList(ctx)
	if err != nil {
		return "", err
	}
	if nameOrID == "" {
		if len(cals) == 0 {
			return "", &errs.NotFound{Kind: "calendar"}
		}
		return cals[0].ID, nil
	}
	for _, c := range cals {
		if c.ID == nameOrID {
			return c.ID, nil
		}
	}
	for _, c := range cals {
		if strings.EqualFold(c.Name, nameOrID) {
			return c.ID, nil
		}
	}
	return "", &errs.NotFound{Kind: "calendar", Ref: nameOrID}
}

func (s *Service) EventsList(ctx context.Context, u *keys.Unlocked, calendarID string, start, end time.Time) ([]Event, error) {
	ck, err := s.unlockCalendar(ctx, u, calendarID)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("Start", fmt.Sprintf("%d", start.Unix()))
	q.Set("End", fmt.Sprintf("%d", end.Unix()))
	q.Set("Timezone", "UTC")
	q.Set("Type", "0")

	var r struct {
		Events []rawEvent
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/calendar/v1/" + calendarID + "/events", Query: q}, &r); err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(r.Events))
	for _, e := range r.Events {
		out = append(out, e.toEvent(ck))
	}
	return out, nil
}

type rawEvent struct {
	ID              string
	CalendarID      string
	StartTime       int64
	EndTime         int64
	FullDay         int
	UID             string
	SharedKeyPacket string
	SharedEvents    []map[string]any
}

func (e rawEvent) toEvent(ck *calKeys) Event {
	title, location, description, rrule, _, sig := decryptEventCard(e.SharedEvents, e.SharedKeyPacket, ck)
	return Event{
		ID: e.ID, CalendarID: e.CalendarID, Title: title, Location: location, Description: description, RRule: rrule,
		Start: time.Unix(e.StartTime, 0), End: time.Unix(e.EndTime, 0),
		AllDay: e.FullDay == 1, UID: e.UID, Signature: sig,
	}
}

func (s *Service) EventGet(ctx context.Context, u *keys.Unlocked, calendarID, eventID string) (*Event, error) {
	ck, err := s.unlockCalendar(ctx, u, calendarID)
	if err != nil {
		return nil, err
	}
	var r struct{ Event rawEvent }
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/calendar/v1/" + calendarID + "/events/" + eventID}, &r); err != nil {
		return nil, err
	}
	ev := r.Event.toEvent(ck)
	return &ev, nil
}

// EventInput is the full set of fields for creating an event.
type EventInput struct {
	Title       string
	Location    string
	Description string
	Start, End  time.Time
	AllDay      bool
	RRule       string   // iCal RRULE value, e.g. "FREQ=WEEKLY;COUNT=10"
	Reminders   []string // durations before start, e.g. "15m", "1h", "1d"
	Attendees   []string // participant emails
}

// EventResult is the outcome of EventCreate. Invite is non-nil only when the
// event has external (non-Proton) attendees that need an emailed ICS.
type EventResult struct {
	ID     string
	Invite *Invite
}

// Invite is a ready-to-send ICS invitation for external attendees.
type Invite struct {
	ICS        string
	Recipients []string
	Subject    string
}

func canonicalEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// attendeeToken is the Proton attendee token: hex SHA-1 of UID+canonicalEmail.
func attendeeToken(uid, email string) string {
	sum := sha1.Sum([]byte(uid + canonicalEmail(email)))
	return hex.EncodeToString(sum[:])
}

// buildAttendees computes per-attendee tokens, the clear-text Attendees list,
// AddedProtonAttendees (the shared session key wrapped to each Proton
// attendee's key) and the list of external (non-Proton) attendee emails.
func (s *Service) buildAttendees(ctx context.Context, uid string, emails []string, sk *pgp.SessionKey) (atts []ical.Attendee, clear, added []map[string]any, external []string, err error) {
	seen := map[string]bool{}
	for _, raw := range emails {
		email := strings.TrimSpace(raw)
		if email == "" || seen[canonicalEmail(email)] {
			continue
		}
		seen[canonicalEmail(email)] = true
		token := attendeeToken(uid, email)
		atts = append(atts, ical.Attendee{Email: email, Token: token})
		clear = append(clear, map[string]any{"Token": token, "Status": 0})

		var keysRes struct {
			Address struct {
				Keys []struct{ PublicKey string }
			}
		}
		if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/core/v4/keys/all", Query: keys.Query("Email", email)}, &keysRes); err != nil {
			return nil, nil, nil, nil, err
		}
		if len(keysRes.Address.Keys) > 0 {
			recKey, err := pgp.NewKeyFromArmored(keysRes.Address.Keys[0].PublicKey)
			if err != nil {
				return nil, nil, nil, nil, fmt.Errorf("parse attendee key for %s: %w", email, err)
			}
			recKR, err := pgp.NewKeyRing(recKey)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			kp, err := recKR.EncryptSessionKey(sk)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			added = append(added, map[string]any{"Email": email, "AddressKeyPacket": base64.StdEncoding.EncodeToString(kp)})
		} else {
			external = append(external, email)
		}
	}
	return atts, clear, added, external, nil
}

func (s *Service) EventCreate(ctx context.Context, u *keys.Unlocked, calendarID string, in EventInput) (*EventResult, error) {
	ck, err := s.unlockCalendar(ctx, u, calendarID)
	if err != nil {
		return nil, err
	}
	notifs, err := buildReminders(in.Reminders)
	if err != nil {
		return nil, err
	}

	organizer := ""
	if len(in.Attendees) > 0 {
		organizer = ck.email
	}
	uid := ical.EventUID()
	signed := ical.SignedVEVENT(uid, in.Start, in.End, in.AllDay, 0, in.RRule, organizer)
	encrypted := ical.EncryptedVEVENT(in.Title, in.Location, in.Description)
	signedCard, encCard, keyPacket, sk, err := pgphelper.EncryptAndSignCardSplit(signed, encrypted, ck.calKR, ck.addrKR, "")
	if err != nil {
		return nil, err
	}

	event := map[string]any{
		"Permissions":        63,
		"IsOrganizer":        1,
		"SharedKeyPacket":    keyPacket,
		"SharedEventContent": []any{signedCard, encCard},
		"Notifications":      notifs,
		"Color":              nil,
	}

	var invite *Invite
	if len(in.Attendees) > 0 {
		atts, clear, added, external, err := s.buildAttendees(ctx, uid, in.Attendees, sk)
		if err != nil {
			return nil, err
		}
		attCard, err := pgphelper.EncryptPartWithSessionKey(ical.AttendeesVEVENT(uid, atts), sk, ck.addrKR)
		if err != nil {
			return nil, err
		}
		event["AttendeesEventContent"] = []any{attCard}
		event["Attendees"] = clear
		if len(added) > 0 {
			event["AddedProtonAttendees"] = added
		}
		if len(external) > 0 {
			invite = &Invite{
				ICS:        ical.InviteICS(uid, in.Title, in.Location, in.Description, in.Start, in.End, in.AllDay, organizer, atts),
				Recipients: external,
				Subject:    "Invitation: " + in.Title,
			}
		}
	}

	body := map[string]any{
		"MemberID": ck.memberID,
		"Events":   []map[string]any{{"Overwrite": 0, "Event": event}},
	}
	var r struct {
		Responses []struct {
			Response struct {
				Event struct{ ID string }
			}
		}
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/calendar/v1/" + calendarID + "/events/sync", Body: body}, &r); err != nil {
		return nil, err
	}
	res := &EventResult{Invite: invite}
	if len(r.Responses) > 0 {
		res.ID = r.Responses[0].Response.Event.ID
	}
	return res, nil
}

// EventUpdate leaves empty fields unchanged.
func (s *Service) EventUpdate(ctx context.Context, u *keys.Unlocked, calendarID, eventID, title, location, description string, start, end time.Time) error {
	ck, err := s.unlockCalendar(ctx, u, calendarID)
	if err != nil {
		return err
	}
	var r struct {
		Event struct {
			UID             string
			StartTime       int64
			EndTime         int64
			FullDay         int
			SharedEvents    []map[string]any
			SharedKeyPacket string
		}
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/calendar/v1/" + calendarID + "/events/" + eventID}, &r); err != nil {
		return err
	}

	curTitle, curLoc, curDesc, curRRule, curOrganizer, _ := decryptEventCard(r.Event.SharedEvents, r.Event.SharedKeyPacket, ck)
	if title == "" {
		title = curTitle
	}
	if location == "" {
		location = curLoc
	}
	if description == "" {
		description = curDesc
	}
	if start.IsZero() {
		start = time.Unix(r.Event.StartTime, 0)
	}
	if end.IsZero() {
		end = time.Unix(r.Event.EndTime, 0)
	}

	// Preserve recurrence and organizer across updates (both live in the signed part).
	signed := ical.SignedVEVENT(r.Event.UID, start, end, r.Event.FullDay == 1, 1, curRRule, curOrganizer)
	encrypted := ical.EncryptedVEVENT(title, location, description)
	signedCard, encCard, _, _, err := pgphelper.EncryptAndSignCardSplit(signed, encrypted, ck.calKR, ck.addrKR, r.Event.SharedKeyPacket)
	if err != nil {
		return err
	}
	body := map[string]any{
		"MemberID": ck.memberID,
		"Events": []map[string]any{{
			"ID": eventID,
			"Event": map[string]any{
				"Permissions":        63,
				"IsOrganizer":        1,
				"SharedEventContent": []any{signedCard, encCard},
				"Notifications":      nil,
				"Color":              nil,
			},
		}},
	}
	return s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/calendar/v1/" + calendarID + "/events/sync", Body: body}, nil)
}

func (s *Service) EventDelete(ctx context.Context, u *keys.Unlocked, calendarID, eventID string) error {
	ck, err := s.unlockCalendar(ctx, u, calendarID)
	if err != nil {
		return err
	}
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/calendar/v1/" + calendarID + "/events/sync",
		Body: map[string]any{"MemberID": ck.memberID, "Events": []map[string]any{{"ID": eventID}}},
	}, nil)
}

// ResolveEvent takes (calendarID, eventID), or a single title which it searches
// across all calendars over the next 30 days.
func (s *Service) ResolveEvent(ctx context.Context, u *keys.Unlocked, args []string) (string, string, error) {
	if len(args) == 2 {
		return args[0], args[1], nil
	}
	needle := args[0]
	cals, err := s.CalendarsList(ctx)
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 0, 30)

	type match struct {
		cal, ev, title string
		when           time.Time
	}
	var matches []match
	for _, c := range cals {
		events, err := s.EventsList(ctx, u, c.ID, start, end)
		if err != nil {
			continue
		}
		for _, e := range events {
			if e.Title != "" && strings.Contains(strings.ToLower(e.Title), strings.ToLower(needle)) {
				matches = append(matches, match{cal: c.ID, ev: e.ID, title: e.Title, when: e.Start})
			}
		}
	}
	switch len(matches) {
	case 0:
		return "", "", &errs.NotFound{Kind: "event", Ref: needle}
	case 1:
		return matches[0].cal, matches[0].ev, nil
	}
	cands := make([]errs.Candidate, 0, len(matches))
	for _, m := range matches {
		cands = append(cands, errs.Candidate{
			ID:    m.ev,
			Label: fmt.Sprintf("%s  %s  (calendar %s)", m.when.Local().Format("2006-01-02 15:04"), m.title, m.cal),
		})
	}
	return "", "", &errs.Ambiguous{Kind: "event", Ref: needle, Candidates: cands}
}

func (s *Service) unlockCalendar(ctx context.Context, u *keys.Unlocked, calendarID string) (*calKeys, error) {
	var mem struct {
		Members []struct {
			ID, CalendarID, Email, AddressID string
		}
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/calendar/v1/" + calendarID + "/members"}, &mem); err != nil {
		return nil, err
	}
	var addrKR *pgp.KeyRing
	var memberID, email string
	for _, m := range mem.Members {
		if kr, ok := u.AddrKR(m.AddressID); ok {
			addrKR = kr
			memberID = m.ID
			email = m.Email
			break
		}
	}
	if addrKR == nil {
		return nil, fmt.Errorf("no matching address key for calendar %s", calendarID)
	}

	var pass struct {
		Passphrase struct {
			MemberPassphrases []struct {
				MemberID, Passphrase, Signature string
			}
		}
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/calendar/v1/" + calendarID + "/passphrase"}, &pass); err != nil {
		return nil, err
	}
	var calPass []byte
	for _, mp := range pass.Passphrase.MemberPassphrases {
		if mp.MemberID != memberID {
			continue
		}
		msg, err := pgp.NewPGPMessageFromArmored(mp.Passphrase)
		if err != nil {
			return nil, err
		}
		sig, err := pgp.NewPGPSignatureFromArmored(mp.Signature)
		if err != nil {
			return nil, err
		}
		dec, err := addrKR.Decrypt(msg, nil, pgp.GetUnixTime())
		if err != nil {
			return nil, fmt.Errorf("decrypt calendar passphrase: %w", err)
		}
		if err := addrKR.VerifyDetached(dec, sig, pgp.GetUnixTime()); err != nil {
			return nil, err
		}
		calPass = dec.GetBinary()
		break
	}
	if calPass == nil {
		return nil, fmt.Errorf("no passphrase found for member %s", memberID)
	}

	var keyRes struct {
		Keys []struct{ PrivateKey string }
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/calendar/v1/" + calendarID + "/keys"}, &keyRes); err != nil {
		return nil, err
	}
	calKR, err := pgp.NewKeyRing(nil)
	if err != nil {
		return nil, err
	}
	for _, k := range keyRes.Keys {
		locked, err := pgp.NewKeyFromArmored(k.PrivateKey)
		if err != nil {
			continue
		}
		unlocked, err := locked.Unlock(calPass)
		if err != nil {
			continue
		}
		_ = calKR.AddKey(unlocked)
	}
	if calKR.CountEntities() == 0 {
		return nil, fmt.Errorf("failed to unlock calendar keys")
	}
	return &calKeys{calKR: calKR, addrKR: addrKR, memberID: memberID, email: email}, nil
}

func decryptEventCard(cards []map[string]any, keyPacket string, ck *calKeys) (title, location, description, rrule, organizer string, sig pgphelper.VerifyResult) {
	kp, _ := base64.StdEncoding.DecodeString(keyPacket)
	decrypted, verdicts, err := pgphelper.DecryptCardsRaw(cards, ck.calKR, ck.addrKR, kp)
	if err != nil {
		return "", "", "", "", "", pgphelper.Unverified
	}
	joined := strings.Join(decrypted, "\n")
	organizer = strings.TrimPrefix(ical.Field(joined, "ORGANIZER"), "mailto:")
	return ical.Field(joined, "SUMMARY"), ical.Field(joined, "LOCATION"), ical.Field(joined, "DESCRIPTION"), ical.Field(joined, "RRULE"), organizer, pgphelper.Aggregate(verdicts...)
}

func buildReminders(reminders []string) ([]map[string]any, error) {
	if len(reminders) == 0 {
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
	d, err := render.ParseDuration(dur)
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

func DefaultRange() (time.Time, time.Time) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return start, start.AddDate(0, 0, 30)
}
