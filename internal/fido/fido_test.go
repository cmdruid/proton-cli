package fido

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Proton writes the binary parts of a challenge as arrays of numbers rather than
// as strings, which is the one thing about its shape a client has to know.
func TestParseReadsProtonsChallenge(t *testing.T) {
	o, err := parse(json.RawMessage(`{"publicKey":{
		"challenge":[1,2,255],
		"rpId":"account.proton.me",
		"timeout":60000,
		"allowCredentials":[{"id":[9,8],"type":"public-key"},{"id":[7],"type":"public-key"}],
		"userVerification":"discouraged"}}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if o.rpID != "account.proton.me" {
		t.Errorf("rpId = %q", o.rpID)
	}
	if string(o.challenge) != string([]byte{1, 2, 255}) {
		t.Errorf("challenge = %v", o.challenge)
	}
	if len(o.allowCredentials) != 2 || string(o.allowCredentials[0]) != string([]byte{9, 8}) {
		t.Errorf("allowCredentials = %v", o.allowCredentials)
	}
	if o.timeout() != time.Minute {
		t.Errorf("timeout = %v, want 1m", o.timeout())
	}
	if o.needsVerification() {
		t.Error("a discouraged verification was read as a required one")
	}
}

func TestParseRefusesAChallengeNothingCanAnswer(t *testing.T) {
	for _, tc := range []struct{ name, raw string }{
		{"no relying party", `{"publicKey":{"challenge":[1],"allowCredentials":[{"id":[2]}]}}`},
		{"nothing to sign", `{"publicKey":{"rpId":"proton.me","allowCredentials":[{"id":[2]}]}}`},
		{"no registered key", `{"publicKey":{"rpId":"proton.me","challenge":[1],"allowCredentials":[]}}`},
		{"not an answer at all", `"nonsense"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parse(json.RawMessage(tc.raw)); err == nil {
				t.Error("a challenge nothing could answer was accepted")
			}
		})
	}
}

// The check a browser makes on every ceremony, and the reason a security key
// cannot be phished through one.
func TestChallengeMustComeFromTheHostItNames(t *testing.T) {
	for _, tc := range []struct {
		rpID, host string
		ok         bool
	}{
		{rpID: "account.proton.me", host: "account.proton.me", ok: true},
		{rpID: "proton.me", host: "account.proton.me", ok: true},
		{rpID: "proton.me", host: "mail.proton.me", ok: true},
		{rpID: "account.proton.me", host: "proton.me"},
		{rpID: "evil.example", host: "account.proton.me"},
		{rpID: "notproton.me", host: "account.proton.me"},
		{rpID: "me", host: "account.proton.me"},
		{rpID: "account.proton.me", host: ""},
		{rpID: "", host: "account.proton.me"},
	} {
		t.Run(tc.rpID+" from "+tc.host, func(t *testing.T) {
			err := options{rpID: tc.rpID}.belongsTo(tc.host)
			if tc.ok && err != nil {
				t.Errorf("refused a challenge from the host that sent it: %v", err)
			}
			if !tc.ok && err == nil {
				t.Error("answered a challenge about a relying party the host has no claim to")
			}
		})
	}
}

// What the key signs the hash of, and what Proton checks the signature against.
// The challenge is base64url without padding, as WebAuthn writes it.
func TestClientDataIsWhatWebAuthnAsksFor(t *testing.T) {
	raw, err := options{rpID: "account.proton.me", challenge: []byte{0xff, 0xef, 0xbf}}.clientData()
	if err != nil {
		t.Fatalf("client data: %v", err)
	}
	const want = `{"type":"webauthn.get","challenge":"_--_","origin":"https://account.proton.me"}`
	if string(raw) != want {
		t.Errorf("client data =\n\t%s\nwant\n\t%s", raw, want)
	}
	if strings.Contains(string(raw), "=") {
		t.Error("the challenge is padded, and WebAuthn's is not")
	}
}

func TestCredentialNamesWhoAnswered(t *testing.T) {
	one := options{allowCredentials: [][]byte{[]byte("only")}}
	two := options{allowCredentials: [][]byte{[]byte("first"), []byte("second")}}

	if got, err := two.credential([]byte("second")); err != nil || string(got) != "second" {
		t.Errorf("credential = %q, %v; want the one the key named", got, err)
	}
	if got, err := one.credential(nil); err != nil || string(got) != "only" {
		t.Errorf("credential = %q, %v; want the only one offered", got, err)
	}
	if _, err := two.credential(nil); err == nil {
		t.Error("a key that named nothing out of two credentials was believed")
	}
}

func TestTimeoutFallsBackToSomethingAPersonCanMeet(t *testing.T) {
	if got := (options{}).timeout(); got != 2*time.Minute {
		t.Errorf("timeout = %v, want 2m", got)
	}
}
