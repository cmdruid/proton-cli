package otp

import (
	"testing"
	"time"
)

// The vectors from RFC 6238's own appendix, which is what says this arithmetic
// is the arithmetic every authenticator agrees on.
//
// The RFC's secret is the ASCII "12345678901234567890"; base32 of that is
// GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ.
func TestRFC6238Vectors(t *testing.T) {
	const secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	for _, c := range []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	} {
		s, err := Parse(secret)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		s.Digits = 6
		got, err := s.At(time.Unix(c.unix, 0))
		if err != nil {
			t.Fatalf("at %d: %v", c.unix, err)
		}
		if got.Code != c.want {
			t.Errorf("at %d: code = %s, want %s", c.unix, got.Code, c.want)
		}
	}
}

// A stored secret is written both ways, because people paste both.
func TestParseTakesAURIOrABareSecret(t *testing.T) {
	bare, err := Parse("GEZDGNBVGY3TQOJQ")
	if err != nil {
		t.Fatalf("bare secret: %v", err)
	}
	if bare.Digits != 6 || bare.Period != 30 || bare.Algorithm != "SHA1" {
		t.Errorf("a bare secret should carry Proton's defaults, got %+v", bare)
	}

	uri, err := Parse("otpauth://totp/Example:jane@example.com?secret=GEZDGNBVGY3TQOJQ&issuer=Example&digits=8&period=60&algorithm=SHA256")
	if err != nil {
		t.Fatalf("uri: %v", err)
	}
	if uri.Digits != 8 || uri.Period != 60 || uri.Algorithm != "SHA256" {
		t.Errorf("the URI's own options should win, got %+v", uri)
	}
	if uri.Issuer != "Example" || uri.Label != "jane@example.com" {
		t.Errorf("issuer/label = %q/%q, want Example/jane@example.com", uri.Issuer, uri.Label)
	}
}

// People write secrets with spaces and dashes so they can be read aloud, and
// every authenticator ignores them.
func TestParseIgnoresTheSpacingPeopleAdd(t *testing.T) {
	spaced, err := Parse("gezd gnbv gy3t qojq")
	if err != nil {
		t.Fatalf("spaced: %v", err)
	}
	plain, err := Parse("GEZDGNBVGY3TQOJQ")
	if err != nil {
		t.Fatalf("plain: %v", err)
	}
	if string(spaced.Key) != string(plain.Key) {
		t.Error("spacing and case should not change the secret")
	}
}

// A URI that is wrong about its own options falls back to the defaults rather
// than refusing to produce a code: the secret is the part that matters.
func TestParseFallsBackOnNonsenseOptions(t *testing.T) {
	s, err := Parse("otpauth://totp/X?secret=GEZDGNBVGY3TQOJQ&digits=abc&period=-1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if s.Digits != 6 || s.Period != 30 {
		t.Errorf("got %d digits and a %ds period, want the defaults", s.Digits, s.Period)
	}
}

func TestParseRefusesWhatIsNotASecret(t *testing.T) {
	for _, in := range []string{"", "   ", "otpauth://totp/X?secret=", "not-base32-!!"} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) should have failed", in)
		}
	}
}

// A code two seconds from expiring is one worth waiting out, so how long it has
// left is reported rather than left to be guessed.
func TestCodeReportsHowLongItHasLeft(t *testing.T) {
	s, err := Parse("GEZDGNBVGY3TQOJQ")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// One second into a 30-second window leaves 29.
	got, err := s.At(time.Unix(31, 0))
	if err != nil {
		t.Fatalf("at: %v", err)
	}
	if got.Expires != 29 {
		t.Errorf("expires in %d seconds, want 29", got.Expires)
	}
	// The code holds for the whole window and changes at its edge.
	a, _ := s.At(time.Unix(31, 0))
	b, _ := s.At(time.Unix(59, 0))
	c, _ := s.At(time.Unix(60, 0))
	if a.Code != b.Code {
		t.Error("the code should hold for the whole period")
	}
	if a.Code == c.Code {
		t.Error("the code should change when the period does")
	}
}
