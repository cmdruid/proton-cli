package calendar

import (
	"context"
	"fmt"
	"strings"

	"github.com/roman-16/proton-cli/internal/account/keys"
	pgphelper "github.com/roman-16/proton-cli/internal/crypto/pgp"
	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/idcache"
	"github.com/roman-16/proton-cli/internal/proton"
)

type Calendar struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description,omitempty"`
	MemberCount int    `json:"member_count"`
}

// CalendarsList reads per-user prefs (Name/Color/Description) from Members[0].
func (s *Service) CalendarsList(ctx context.Context) ([]Calendar, error) {
	return s.calendars.Do("", func() ([]Calendar, error) {
		var r struct {
			Calendars []struct {
				ID      string
				Members []member
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
	})
}

// CalendarName is the name this account gave a calendar.
//
// It reads the membership, which is where Proton keeps the name and which every
// command that touches a calendar has already fetched. Asking for the whole list
// of calendars to turn one ID into one string is a request for something already
// in hand. A calendar this account is not a member of is reported by its ID,
// which is the only name it has here.
func (s *Service) CalendarName(ctx context.Context, u *keys.Unlocked, calendarID string) (string, error) {
	b, err := s.calendarBootstrap(ctx, calendarID)
	if err != nil {
		return "", err
	}
	if me, _, ok := ourMember(b.Members, u); ok && me.Name != "" {
		return me.Name, nil
	}
	return calendarID, nil
}

func (s *Service) CalendarCreate(ctx context.Context, u *keys.Unlocked, name, color string) (string, error) {
	addrKR, addr, err := u.PrimaryAddr()
	if err != nil {
		return "", err
	}
	var r struct{ Calendar struct{ ID string } }
	if err := s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: "/calendar/v1",
		Body: map[string]any{"Name": name, "Color": color, "Display": 1, "AddressID": addr.ID},
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
			"AddressID":  addr.ID,
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
	b, err := s.calendarBootstrap(ctx, calendarID)
	if err != nil {
		return "", err
	}
	if me, _, ok := ourMember(b.Members, u); ok {
		return me.ID, nil
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

// ResolveCalendarID turns a name or an ID into an ID.
//
// An ID is already the answer, so it is returned without asking: a reference that
// names the calendar outright should not cost the list of all of them. A name has
// to be looked up, and nothing named at all means the first one.
func (s *Service) ResolveCalendarID(ctx context.Context, nameOrID string) (string, error) {
	if idcache.IsFullID(nameOrID) {
		return nameOrID, nil
	}
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
