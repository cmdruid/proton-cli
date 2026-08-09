package calendar

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/url"
	"slices"

	"github.com/roman-16/proton-cli/internal/proton"
)

// canonicalAddr is an address in the form Proton derives it to. Only
// canonicalEmails produces one, which is the point: an attendee token computed
// from anything else is wrong in a way nothing local can detect.
type canonicalAddr string

// attendeeToken is the Proton attendee token: hex SHA-1 of UID+canonical address.
func attendeeToken(uid string, addr canonicalAddr) string {
	sum := sha1.Sum([]byte(uid + string(addr)))
	return hex.EncodeToString(sum[:])
}

// canonicalEmails resolves addresses to the form Proton derives them to.
//
// The rule is the server's and differs by domain: gmail.com drops dots and
// +tags, proton.me drops dots, most domains only lowercase. So it is asked for
// rather than guessed, and an address the server declines to answer for is an
// error rather than a fallback to the address as written.
//
// The answers are memoised for the life of the Service, and every address a
// caller needs should be asked for in one call.
func (s *Service) canonicalEmails(ctx context.Context, emails []string) (map[string]canonicalAddr, error) {
	out := make(map[string]canonicalAddr, len(emails))
	var missing []string
	for _, e := range emails {
		if c, ok := s.canonical[e]; ok {
			out[e] = c
			continue
		}
		if !slices.Contains(missing, e) {
			missing = append(missing, e)
		}
	}
	if len(missing) == 0 {
		return out, nil
	}

	q := url.Values{}
	for _, e := range missing {
		q.Add("Emails[]", e)
	}
	var r struct {
		Responses []struct {
			Email    string
			Response struct{ CanonicalEmail string }
		}
	}
	if err := s.C.Decode(ctx, proton.Request{
		Method: "GET", Path: "/core/v4/addresses/canonical", Query: q,
	}, &r); err != nil {
		return nil, err
	}
	if s.canonical == nil {
		s.canonical = map[string]canonicalAddr{}
	}
	for _, resp := range r.Responses {
		if resp.Response.CanonicalEmail == "" {
			continue
		}
		c := canonicalAddr(resp.Response.CanonicalEmail)
		s.canonical[resp.Email] = c
		out[resp.Email] = c
	}
	for _, e := range missing {
		if _, ok := out[e]; !ok {
			return nil, fmt.Errorf("%s is not an address Proton can address an attendee by", e)
		}
	}
	return out, nil
}
