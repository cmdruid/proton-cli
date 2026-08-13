package calendar

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/account/keys"
	pgphelper "github.com/roman-16/proton-cli/internal/crypto/pgp"
	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/fetch"
	"github.com/roman-16/proton-cli/internal/ical"
	"github.com/roman-16/proton-cli/internal/proton"
)

// zoneOf resolves an IANA zone name, defaulting to the host's.
func zoneOf(name string) (*time.Location, error) {
	if name == "" {
		return time.Local, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("unknown time zone %q", name)
	}
	return loc, nil
}

type Service struct {
	C         proton.Doer
	canonical map[string]canonicalAddr
	zoneCache

	// What one invocation asks Proton for, asked for once. A calendar's own record,
	// its unlocked keys and the list of calendars are each wanted by several steps
	// of a single command.
	bootstraps fetch.Memo[*bootstrap]
	unlocked   fetch.Memo[*calKeys]
	calendars  fetch.Memo[[]Calendar]
}

func New(c proton.Doer) *Service { return &Service{C: c} }

// member is one account's membership of one calendar. Proton keeps the display
// name, colour and description here rather than on the calendar, because they are
// each member's own.
type member struct {
	ID          string
	CalendarID  string
	Email       string
	AddressID   string
	Name        string
	Color       string
	Description string
}

// bootstrap is everything needed to open a calendar: the membership that names
// it, the passphrase that unlocks its keys, and the keys.
//
// One request answers all of it, which is how the web client opens a calendar
// (getFullCalendar, packages/shared/lib/api/calendars.ts).
type bootstrap struct {
	Keys       []struct{ PrivateKey string }
	Passphrase struct {
		MemberPassphrases []struct {
			MemberID, Passphrase, Signature string
		}
	}
	Members []member
}

func (s *Service) calendarBootstrap(ctx context.Context, calendarID string) (*bootstrap, error) {
	return s.bootstraps.Do(calendarID, func() (*bootstrap, error) {
		var b bootstrap
		if err := s.C.Decode(ctx, proton.Request{
			Method: "GET", Path: "/calendar/v2/" + calendarID + "/bootstrap",
		}, &b); err != nil {
			// The only thing the request named was the calendar, so a server that
			// does not recognise it is answering about the reference, and the answer
			// reads the same whether the reference was a name or an ID.
			if proton.DoesNotExist(err) {
				return nil, &errs.NotFound{Kind: "calendar", Ref: calendarID}
			}
			return nil, err
		}
		return &b, nil
	})
}

// ourMember is the membership this account holds, which is the one whose address
// key we can open.
//
// Proton reports a calendar's members as the ones belonging to whoever asked, so
// there is normally one; matching it to an address of ours is what the web client
// does too (getMemberAndAddress, packages/shared/lib/calendar/members.ts).
func ourMember(members []member, u *keys.Unlocked) (member, *pgp.KeyRing, bool) {
	for _, m := range members {
		if kr, ok := u.AddrKR(m.AddressID); ok {
			return m, kr, true
		}
	}
	return member{}, nil, false
}

type calKeys struct {
	calKR    *pgp.KeyRing
	addrKR   *pgp.KeyRing
	memberID string
	email    string
}

// unlockCalendar opens a calendar's keys.
func (s *Service) unlockCalendar(ctx context.Context, u *keys.Unlocked, calendarID string) (*calKeys, error) {
	return s.unlocked.Do(calendarID, func() (*calKeys, error) {
		b, err := s.calendarBootstrap(ctx, calendarID)
		if err != nil {
			return nil, err
		}

		me, addrKR, ok := ourMember(b.Members, u)
		if !ok {
			return nil, fmt.Errorf("no matching address key for calendar %s", calendarID)
		}

		var calPass []byte
		for _, mp := range b.Passphrase.MemberPassphrases {
			if mp.MemberID != me.ID {
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
			return nil, fmt.Errorf("no passphrase found for member %s", me.ID)
		}

		calKR, err := pgp.NewKeyRing(nil)
		if err != nil {
			return nil, err
		}
		for _, k := range b.Keys {
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
		return &calKeys{calKR: calKR, addrKR: addrKR, memberID: me.ID, email: me.Email}, nil
	})
}

// decryptEvent decrypts an event's cards and parses them into one model.
//
// The session key is wrapped to decryptionKR - the calendar key for an event you
// own, the invited address key for an invitation you received - via keyPacket;
// signatures are checked against verificationKR.
//
// A failure to decrypt is returned rather than folded into empty fields. Reading
// an event that cannot be read should say so, and writing one back must be
// impossible: an update built from blanks would sign those blanks over the real
// title, location, description, rule and exclusions.
func decryptEvent(cards []map[string]any, keyPacket string, decryptionKR, verificationKR *pgp.KeyRing) (ical.VEvent, pgphelper.VerifyResult, error) {
	kp, err := base64.StdEncoding.DecodeString(keyPacket)
	if err != nil {
		return ical.VEvent{}, pgphelper.Unverified, fmt.Errorf("decode event key packet: %w", err)
	}
	decrypted, verdicts, err := pgphelper.DecryptCardsRaw(cards, decryptionKR, verificationKR, kp)
	if err != nil {
		return ical.VEvent{}, pgphelper.Unverified, fmt.Errorf("decrypt event content: %w", err)
	}
	v, err := ical.Parse(strings.Join(decrypted, "\r\n"))
	if err != nil {
		return ical.VEvent{}, pgphelper.Unverified, fmt.Errorf("read event content: %w", err)
	}
	return v, pgphelper.Aggregate(verdicts...), nil
}

// defaultDays is how many days a listing covers when it is not told which.
const defaultDays = 30

// DefaultDays are the first and last day a listing covers when it is not told
// which: today, and the rest of the month ahead.
func DefaultDays() (first, last time.Time) {
	now := time.Now()
	first = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return first, first.AddDate(0, 0, defaultDays-1)
}
