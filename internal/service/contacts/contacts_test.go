package contacts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	gopenpgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/account/keys"
	"github.com/roman-16/proton-cli/internal/crypto/pgp"
	"github.com/roman-16/proton-cli/internal/proton"
	"github.com/roman-16/proton-cli/internal/vcard"
)

// testKeyRing generates a throwaway keyring to sign/verify contact cards.
func testKeyRing(t *testing.T) *gopenpgp.KeyRing {
	t.Helper()
	key, err := gopenpgp.GenerateKey("test", "test@example.invalid", "x25519", 0)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	kr, err := gopenpgp.NewKeyRing(key)
	if err != nil {
		t.Fatalf("NewKeyRing: %v", err)
	}
	return kr
}

// armoredPubKey returns a fresh armored public key plus its raw KEY-property value.
func armoredPubKey(t *testing.T) (armored, keyValue string) {
	t.Helper()
	key, err := gopenpgp.GenerateKey("pin", "pin@example.invalid", "x25519", 0)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	armored, err = key.GetArmoredPublicKey()
	if err != nil {
		t.Fatalf("GetArmoredPublicKey: %v", err)
	}
	bin, err := key.GetPublicKey()
	if err != nil {
		t.Fatalf("GetPublicKey: %v", err)
	}
	return armored, "data:application/pgp-keys;base64," + base64.StdEncoding.EncodeToString(bin)
}

// signedCard builds a Type-2 signed card from vCard text using kr.
func signedCard(t *testing.T, kr *gopenpgp.KeyRing, data string) map[string]any {
	t.Helper()
	card, err := pgp.SignCard(data, kr)
	if err != nil {
		t.Fatalf("SignCard: %v", err)
	}
	return map[string]any{"Type": card.Type, "Data": card.Data, "Signature": card.Signature}
}

// contactDoer fakes the contacts API: it serves the emails lookup and a single
// contact's raw cards, and captures the PUT body.
type contactDoer struct {
	emails  []map[string]any
	cards   []map[string]any
	putBody map[string]any
}

func (d *contactDoer) Do(_ context.Context, _ proton.Request) (*proton.Response, error) {
	return &proton.Response{Status: 200, Body: []byte(`{"Code":1000}`)}, nil
}

func (d *contactDoer) Decode(_ context.Context, r proton.Request, out any) error {
	if r.Method == "PUT" {
		d.putBody, _ = r.Body.(map[string]any)
		return nil
	}
	var payload any
	switch {
	case strings.HasSuffix(r.Path, "/contacts/emails"):
		payload = map[string]any{"ContactEmails": d.emails}
	default: // GET a contact by id
		payload = map[string]any{"Contact": map[string]any{"Cards": d.cards}}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(b, out)
}

// putSignedCardText returns the signed (Type-2) card's Data from the captured PUT.
func putSignedCardText(t *testing.T, d *contactDoer) string {
	t.Helper()
	cards, ok := d.putBody["Cards"].([]any)
	if !ok || len(cards) == 0 {
		t.Fatalf("no Cards in PUT body: %#v", d.putBody)
	}
	// The signed card is a *pgp.Card (first element).
	if c, ok := cards[0].(*pgp.Card); ok && c.Type == pgp.CardSigned {
		return c.Data
	}
	t.Fatalf("first PUT card is not a signed *pgp.Card: %#v", cards[0])
	return ""
}

func TestPinnedKeysForReadsSignedCard(t *testing.T) {
	kr := testKeyRing(t)
	_, keyValue := armoredPubKey(t)
	card := vcard.BuildSigned(vcard.Signed{
		Name: "Bob", UID: "uid-1",
		Emails: []vcard.SignedEmail{{
			Address:   "bob@example.com",
			KeyValues: []string{keyValue},
			Encrypt:   ptr(true),
		}},
	})
	d := &contactDoer{
		emails: []map[string]any{{"ContactID": "c1", "Defaults": 0}},
		cards:  []map[string]any{signedCard(t, kr, card)},
	}
	u := &keys.Unlocked{UserKR: kr}

	cc, err := New(d).PinnedKeysFor(context.Background(), u, "bob@example.com")
	if err != nil {
		t.Fatalf("PinnedKeysFor: %v", err)
	}
	if cc == nil || len(cc.ArmoredKeys) != 1 {
		t.Fatalf("expected one pinned key, got %+v", cc)
	}
	if cc.Encrypt == nil || !*cc.Encrypt {
		t.Errorf("Encrypt = %v, want true", cc.Encrypt)
	}
	if !cc.SignatureVerified {
		t.Error("SignatureVerified should be true for a card signed by the user key")
	}
	if !strings.Contains(cc.ArmoredKeys[0], "PGP PUBLIC KEY BLOCK") {
		t.Errorf("pinned key is not armored: %q", cc.ArmoredKeys[0])
	}
}

func TestPinnedKeysForNoConfigIsMiss(t *testing.T) {
	d := &contactDoer{emails: []map[string]any{{"ContactID": "c1", "Defaults": 1}}}
	cc, err := New(d).PinnedKeysFor(context.Background(), &keys.Unlocked{UserKR: testKeyRing(t)}, "x@example.com")
	if err != nil || cc != nil {
		t.Errorf("Defaults==1 should be a clean miss; got %+v, %v", cc, err)
	}
}

func TestPinKeyAddsKeyAndPreservesOtherCards(t *testing.T) {
	kr := testKeyRing(t)
	base := vcard.BuildSigned(vcard.Signed{
		Name: "Bob", UID: "uid-1",
		Emails: []vcard.SignedEmail{{Address: "bob@example.com"}},
	})
	// A verbatim encrypted card that must survive the edit untouched.
	encrypted := map[string]any{"Type": float64(pgp.CardEncryptedSigned), "Data": "ENC", "Signature": "SIG"}
	d := &contactDoer{cards: []map[string]any{signedCard(t, kr, base), encrypted}}
	u := &keys.Unlocked{UserKR: kr}

	armored, keyValue := armoredPubKey(t)
	if err := New(d).PinKey(context.Background(), u, "c1", "bob@example.com", armored, nil, nil, ""); err != nil {
		t.Fatalf("PinKey: %v", err)
	}

	newSigned := putSignedCardText(t, d)
	model := vcard.ParseSigned(newSigned)
	e := model.FindEmail("bob@example.com")
	if e == nil || len(e.KeyValues) != 1 || e.KeyValues[0] != keyValue {
		t.Fatalf("pinned key not written: %+v", e)
	}
	if e.Encrypt == nil || !*e.Encrypt || e.Sign == nil || !*e.Sign {
		t.Errorf("encrypt/sign should default to true: enc=%v sign=%v", e.Encrypt, e.Sign)
	}
	// The encrypted card must be re-attached verbatim.
	cards := d.putBody["Cards"].([]any)
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards in PUT, got %d", len(cards))
	}
	if got := cards[1].(map[string]any); got["Data"] != "ENC" || got["Signature"] != "SIG" {
		t.Errorf("encrypted card not preserved verbatim: %#v", got)
	}
}

func TestPinKeyRejectsUnverifiedCard(t *testing.T) {
	kr := testKeyRing(t)
	other := testKeyRing(t) // signs with a different key than the verifier
	base := vcard.BuildSigned(vcard.Signed{Name: "Bob", UID: "u", Emails: []vcard.SignedEmail{{Address: "bob@example.com"}}})
	d := &contactDoer{cards: []map[string]any{signedCard(t, other, base)}}
	u := &keys.Unlocked{UserKR: kr}

	armored, _ := armoredPubKey(t)
	if err := New(d).PinKey(context.Background(), u, "c1", "bob@example.com", armored, nil, nil, ""); err == nil {
		t.Error("PinKey should refuse to edit a card it cannot verify")
	}
}

func TestUnpinKeyRemovesKeys(t *testing.T) {
	kr := testKeyRing(t)
	_, keyValue := armoredPubKey(t)
	base := vcard.BuildSigned(vcard.Signed{
		Name: "Bob", UID: "u",
		Emails: []vcard.SignedEmail{{Address: "bob@example.com", KeyValues: []string{keyValue}, Encrypt: ptr(true)}},
	})
	d := &contactDoer{cards: []map[string]any{signedCard(t, kr, base)}}
	u := &keys.Unlocked{UserKR: kr}

	if err := New(d).UnpinKey(context.Background(), u, "c1", "bob@example.com"); err != nil {
		t.Fatalf("UnpinKey: %v", err)
	}
	model := vcard.ParseSigned(putSignedCardText(t, d))
	if e := model.FindEmail("bob@example.com"); e == nil {
		t.Error("email should remain after unpin")
	} else if len(e.KeyValues) != 0 || e.Encrypt != nil {
		t.Errorf("keys/flags should be gone: %+v", e)
	}
}

func encodePinnedKeyValue(t *testing.T, armored string) string {
	t.Helper()
	v, err := encodePinnedKey(armored)
	if err != nil {
		t.Fatalf("encodePinnedKey: %v", err)
	}
	return v
}

func TestEncodePinnedKeyRoundTrip(t *testing.T) {
	armored, want := armoredPubKey(t)
	if got := encodePinnedKeyValue(t, armored); got != want {
		t.Errorf("encodePinnedKey mismatch:\n got %q\nwant %q", got, want)
	}
	if _, err := encodePinnedKey("not-a-key"); err == nil {
		t.Error("expected an error for a non-key input")
	}
}

func TestPrependUnique(t *testing.T) {
	got := prependUnique([]string{"b", "a"}, "a")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("prependUnique = %v, want [a b] (a moved to front, deduped)", got)
	}
}

func ptr[T any](v T) *T { return &v }
