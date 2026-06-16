// Package calendar provides Proton Calendar operations.
package calendar

import (
	"context"
	"encoding/base64"
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
)

// Service is the Calendar domain service.
type Service struct{ C proton.Doer }

// New constructs a calendar service.
func New(c proton.Doer) *Service { return &Service{C: c} }

// Calendar is a Proton calendar entry.
type Calendar struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description,omitempty"`
	MemberCount int    `json:"member_count"`
}

// Event is a decrypted calendar event.
type Event struct {
	ID         string    `json:"id"`
	CalendarID string    `json:"calendar_id"`
	Title      string    `json:"title"`
	Location   string    `json:"location,omitempty"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	AllDay     bool      `json:"all_day"`
	UID        string    `json:"uid,omitempty"`
}

type calKeys struct {
	calKR    *pgp.KeyRing
	addrKR   *pgp.KeyRing
	memberID string
}

// CalendarsList returns all calendars on the account. Per-user prefs
// (Name/Color/Description) live under Members[].
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

// CalendarCreate creates a new calendar on the primary address and returns its ID.
func (s *Service) CalendarCreate(ctx context.Context, u *keys.Unlocked, name, color string) (string, error) {
	_, addrID, _, err := u.PrimaryAddrKR()
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
	return r.Calendar.ID, nil
}

// CalendarDelete deletes a calendar. Requires scope unlock by the caller.
func (s *Service) CalendarDelete(ctx context.Context, id string) error {
	return s.C.Decode(ctx, proton.Request{Method: "DELETE", Path: "/calendar/v1/" + id}, nil)
}

// ResolveCalendarID accepts a literal ID or a name.
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

// EventsList returns decrypted events in the given time range.
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
	title, location := decryptTitleLocation(e.SharedEvents, e.SharedKeyPacket, ck)
	return Event{
		ID: e.ID, CalendarID: e.CalendarID, Title: title, Location: location,
		Start: time.Unix(e.StartTime, 0), End: time.Unix(e.EndTime, 0),
		AllDay: e.FullDay == 1, UID: e.UID,
	}
}

// EventGet returns a single decrypted event.
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

// EventCreate creates a new event and returns its ID.
func (s *Service) EventCreate(ctx context.Context, u *keys.Unlocked, calendarID, title, location string, start, end time.Time, allDay bool) (string, error) {
	ck, err := s.unlockCalendar(ctx, u, calendarID)
	if err != nil {
		return "", err
	}
	signed := ical.SignedVEVENT(ical.EventUID(), start, end, allDay, 0)
	encrypted := ical.EncryptedVEVENT(title, location)
	signedCard, encCard, keyPacket, err := pgphelper.EncryptAndSignCardSplit(signed, encrypted, ck.calKR, ck.addrKR, "")
	if err != nil {
		return "", err
	}
	body := map[string]any{
		"MemberID": ck.memberID,
		"Events": []map[string]any{{
			"Overwrite": 0,
			"Event": map[string]any{
				"Permissions":        63,
				"IsOrganizer":        1,
				"SharedKeyPacket":    keyPacket,
				"SharedEventContent": []any{signedCard, encCard},
				"Notifications":      nil,
				"Color":              nil,
			},
		}},
	}
	var r struct {
		Responses []struct {
			Response struct {
				Event struct{ ID string }
			}
		}
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/calendar/v1/" + calendarID + "/events/sync", Body: body}, &r); err != nil {
		return "", err
	}
	if len(r.Responses) > 0 {
		return r.Responses[0].Response.Event.ID, nil
	}
	return "", nil
}

// EventUpdate updates an existing event. Empty fields are left unchanged.
func (s *Service) EventUpdate(ctx context.Context, u *keys.Unlocked, calendarID, eventID, title, location string, start, end time.Time) error {
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

	curTitle, curLoc := decryptTitleLocation(r.Event.SharedEvents, r.Event.SharedKeyPacket, ck)
	if title == "" {
		title = curTitle
	}
	if location == "" {
		location = curLoc
	}
	if start.IsZero() {
		start = time.Unix(r.Event.StartTime, 0)
	}
	if end.IsZero() {
		end = time.Unix(r.Event.EndTime, 0)
	}

	signed := ical.SignedVEVENT(r.Event.UID, start, end, r.Event.FullDay == 1, 1)
	encrypted := ical.EncryptedVEVENT(title, location)
	signedCard, encCard, _, err := pgphelper.EncryptAndSignCardSplit(signed, encrypted, ck.calKR, ck.addrKR, r.Event.SharedKeyPacket)
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

// EventDelete deletes an event.
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

// ResolveEvent accepts (calendarID eventID) or a single title searched across
// calendars in the next 30 days.
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
	var memberID string
	for _, m := range mem.Members {
		if kr, ok := u.AddrKR(m.AddressID); ok {
			addrKR = kr
			memberID = m.ID
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
	return &calKeys{calKR: calKR, addrKR: addrKR, memberID: memberID}, nil
}

func decryptTitleLocation(cards []map[string]any, keyPacket string, ck *calKeys) (string, string) {
	kp, _ := base64.StdEncoding.DecodeString(keyPacket)
	decrypted, err := pgphelper.DecryptCardsRaw(cards, ck.calKR, ck.addrKR, kp)
	if err != nil {
		return "", ""
	}
	joined := strings.Join(decrypted, "\n")
	return ical.Field(joined, "SUMMARY"), ical.Field(joined, "LOCATION")
}

// DefaultRange returns start/end of a default 30-day window.
func DefaultRange() (time.Time, time.Time) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return start, start.AddDate(0, 0, 30)
}
