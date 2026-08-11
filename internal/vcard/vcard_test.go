package vcard

import (
	"strings"
	"testing"
)

const signedCard = "BEGIN:VCARD\r\nVERSION:4.0\r\n" +
	"FN:Jane Doe\r\n" +
	"UID:proton-cli-1\r\n" +
	"item1.EMAIL;PREF=1:jane@example.test\r\n" +
	"item1.KEY;PREF=2:data:application/pgp-keys;base64,SECOND\r\n" +
	"item1.KEY;PREF=1:data:application/pgp-keys;base64,FIRST\r\n" +
	"item1.X-PM-ENCRYPT:true\r\n" +
	"item1.X-PM-SIGN:false\r\n" +
	"item1.X-PM-SCHEME:pgp-mime\r\n" +
	"item2.EMAIL;PREF=2:JANE@work.test\r\n" +
	"END:VCARD"

func TestParseSignedKeepsEachAddressSettings(t *testing.T) {
	c := ParseSigned(signedCard)
	if c.Name != "Jane Doe" || c.UID != "proton-cli-1" {
		t.Errorf("ParseSigned = %+v", c)
	}
	if len(c.Emails) != 2 {
		t.Fatalf("got %d addresses, want 2", len(c.Emails))
	}
	first := c.FindEmail("jane@example.test")
	if first == nil {
		t.Fatal("FindEmail did not find the address")
	}
	if len(first.KeyValues) != 2 || !strings.HasSuffix(first.KeyValues[0], "FIRST") {
		t.Errorf("pinned keys are not in preference order: %+v", first.KeyValues)
	}
	if first.Encrypt == nil || !*first.Encrypt || first.Sign == nil || *first.Sign {
		t.Errorf("crypto flags = %+v %+v", first.Encrypt, first.Sign)
	}
	if first.Scheme != "pgp-mime" {
		t.Errorf("scheme = %q", first.Scheme)
	}
}

func TestFindEmailIgnoresCaseAndSurroundingSpace(t *testing.T) {
	c := ParseSigned(signedCard)
	if c.FindEmail(" jane@WORK.test ") == nil {
		t.Error("FindEmail is case- or space-sensitive")
	}
}

// Rebuilding a contact from its addresses alone would silently unpin its keys, so
// a round trip has to carry them.
func TestBuildSignedRoundTripsPinnedKeysAndFlags(t *testing.T) {
	c := ParseSigned(signedCard)
	back := ParseSigned(BuildSigned(c))
	if len(back.Emails) != len(c.Emails) {
		t.Fatalf("round trip lost addresses: %+v", back.Emails)
	}
	got := back.FindEmail("jane@example.test")
	if got == nil || len(got.KeyValues) != 2 {
		t.Fatalf("round trip lost pinned keys: %+v", got)
	}
	if got.Scheme != "pgp-mime" || got.Encrypt == nil || !*got.Encrypt {
		t.Errorf("round trip lost the crypto settings: %+v", got)
	}
}

func TestEmailGroupFindsTheGroupAnAddressSettingsHangOff(t *testing.T) {
	if g := EmailGroup(signedCard, "JANE@example.test"); g != "item1" {
		t.Errorf("EmailGroup = %q, want item1", g)
	}
	if g := EmailGroup(signedCard, "nobody@example.test"); g != "" {
		t.Errorf("EmailGroup = %q, want empty", g)
	}
}

func TestValuesReturnsEveryValueInDocumentOrder(t *testing.T) {
	card := "BEGIN:VCARD\r\nTEL;PREF=1:+431\r\nTEL;PREF=2:+432\r\nEND:VCARD"
	got := Values(card, "TEL")
	if len(got) != 2 || got[0] != "+431" || got[1] != "+432" {
		t.Errorf("Values = %q", got)
	}
}

func TestBuildEncryptedEscapesTextAndOmitsEmptyProperties(t *testing.T) {
	out := BuildEncrypted(Encrypted{Note: "line one\nline two, with a comma", Org: "Acme"})
	if !strings.Contains(out, `NOTE:line one\nline two\, with a comma`) {
		t.Errorf("note was not escaped:\n%s", out)
	}
	if strings.Contains(out, "BDAY") || strings.Contains(out, "URL") {
		t.Errorf("empty properties were written:\n%s", out)
	}
	if got := Field(out, "NOTE"); got != "line one\nline two, with a comma" {
		t.Errorf("note did not survive a round trip: %q", got)
	}
}

func TestUIDDoesNotRepeat(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		uid := UID()
		if seen[uid] {
			t.Fatal("UID repeated itself")
		}
		seen[uid] = true
		if !strings.HasPrefix(uid, "proton-cli-") {
			t.Errorf("UID = %q, want the proton-cli prefix", uid)
		}
	}
}
