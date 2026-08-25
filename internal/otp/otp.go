// Package otp turns a stored TOTP secret into the code it is standing in for.
//
// Pass stores the secret, not the code, so every client works the code out for
// itself. The web client shows it beside the item with a countdown; a terminal
// wants it on stdout, where a script can use it.
//
// The arithmetic is RFC 6238, which is HMAC over the number of periods since the
// epoch, truncated. Nothing here talks to Proton.
package otp

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"hash"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Proton's defaults, from OTP_DEFAULTS in its own client. A URI that states none
// of these means all of them.
const (
	defaultAlgorithm = "SHA1"
	defaultDigits    = 6
	defaultPeriod    = 30
)

// Secret is a parsed TOTP configuration.
type Secret struct {
	Key       []byte
	Algorithm string
	Digits    int
	Period    int
	Issuer    string
	Label     string
}

// Parse reads either spelling of a stored secret.
//
// Pass accepts a whole otpauth:// URI and also a bare base32 secret, because
// people paste both, so this takes both: anything that does not look like a URI
// is read as the secret itself with the defaults around it.
func Parse(raw string) (*Secret, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("no TOTP secret")
	}
	if !strings.HasPrefix(strings.ToLower(raw), "otpauth:") {
		key, err := decodeSecret(raw)
		if err != nil {
			return nil, err
		}
		return &Secret{
			Key: key, Algorithm: defaultAlgorithm,
			Digits: defaultDigits, Period: defaultPeriod,
		}, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("not a TOTP URI: %w", err)
	}
	q := u.Query()
	key, err := decodeSecret(q.Get("secret"))
	if err != nil {
		return nil, err
	}
	s := &Secret{
		Key:       key,
		Algorithm: defaultAlgorithm,
		Digits:    defaultDigits,
		Period:    defaultPeriod,
		Issuer:    q.Get("issuer"),
	}
	if a := strings.ToUpper(strings.TrimSpace(q.Get("algorithm"))); a != "" {
		s.Algorithm = a
	}
	// A digits or period that is not a number is the URI being wrong about
	// itself, so the default stands rather than the code being refused.
	if n, err := strconv.Atoi(q.Get("digits")); err == nil && n > 0 {
		s.Digits = n
	}
	if n, err := strconv.Atoi(q.Get("period")); err == nil && n > 0 {
		s.Period = n
	}
	// The path is "issuer:label" or just "label".
	path := strings.TrimPrefix(u.Path, "/")
	if decoded, err := url.PathUnescape(path); err == nil {
		path = decoded
	}
	if issuer, label, found := strings.Cut(path, ":"); found {
		s.Label = strings.TrimSpace(label)
		if s.Issuer == "" {
			s.Issuer = strings.TrimSpace(issuer)
		}
	} else {
		s.Label = strings.TrimSpace(path)
	}
	return s, nil
}

// decodeSecret reads a base32 secret the way every authenticator writes one:
// case-insensitively, without padding, and ignoring the spaces and dashes people
// use to make it readable.
func decodeSecret(raw string) ([]byte, error) {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '_', '\t', '\n':
			return -1
		}
		return r
	}, raw)
	if cleaned == "" {
		return nil, fmt.Errorf("no TOTP secret")
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).
		DecodeString(strings.ToUpper(cleaned))
	if err != nil {
		return nil, fmt.Errorf("the TOTP secret is not valid base32")
	}
	return key, nil
}

// Code is the code standing at a moment, and how long it has left.
type Code struct {
	Code string `json:"code"`
	// Expires is how many seconds the code is still good for. A code with two
	// seconds left is a code worth waiting out, which is why this is reported
	// rather than left to be guessed.
	Expires int `json:"expires_in_seconds"`
}

// At works out the code standing at a moment.
func (s *Secret) At(t time.Time) (Code, error) {
	counter := uint64(t.Unix()) / uint64(s.Period)
	mac, err := s.mac()
	if err != nil {
		return Code{}, err
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	// RFC 6238 dynamic truncation: the low nibble of the last byte says where to
	// read the four bytes from.
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	mod := uint32(math.Pow10(s.Digits))
	remaining := s.Period - int(uint64(t.Unix())%uint64(s.Period))
	return Code{
		Code:    fmt.Sprintf("%0*d", s.Digits, value%mod),
		Expires: remaining,
	}, nil
}

// Now works out the code standing now.
func (s *Secret) Now() (Code, error) { return s.At(time.Now()) }

func (s *Secret) mac() (hash.Hash, error) {
	switch s.Algorithm {
	case "SHA1", "":
		return hmac.New(sha1.New, s.Key), nil
	case "SHA256":
		return hmac.New(sha256.New, s.Key), nil
	case "SHA512":
		return hmac.New(sha512.New, s.Key), nil
	}
	return nil, fmt.Errorf("unsupported TOTP algorithm %q", s.Algorithm)
}
