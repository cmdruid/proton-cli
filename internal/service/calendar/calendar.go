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
}

func New(c proton.Doer) *Service { return &Service{C: c} }

type calKeys struct {
	calKR    *pgp.KeyRing
	addrKR   *pgp.KeyRing
	memberID string
	email    string
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
