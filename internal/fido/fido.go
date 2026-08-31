// Package fido answers Proton's security-key challenge from a terminal.
//
// Signing in with a security key is a WebAuthn ceremony, and a browser is only
// one way to hold it. What the key signs is its own authenticator data followed
// by the hash of a clientDataJSON the client writes itself, so nothing in the
// ceremony needs a page, a window or a JavaScript engine - which is why Proton's
// own Go client performs it the same way.
//
// What a browser does that matters is refuse to ask a key about a relying party
// the visited site has no claim to. That refusal is the whole of WebAuthn's
// phishing resistance, and with no browser in the picture it has to live here:
// the rpId Proton names is checked against the host that named it before any key
// is asked anything. A client that signed whatever rpId an answer carried would
// be a relay for whoever could forge that answer.
package fido

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Request is a security-key challenge as Proton stated it.
type Request struct {
	// Options is Proton's AuthenticationOptions exactly as it arrived. It is
	// echoed back alongside the answer, so it is carried rather than rebuilt:
	// re-encoding a structure the server wrote is a way of disagreeing with it
	// about what it said.
	Options json.RawMessage
	// Host is the API host that sent the challenge. The relying party the key is
	// asked about must belong to it.
	Host string
}

// Assertion is what the key answered, in the shape Proton asks for it back.
type Assertion struct {
	ClientData        []byte
	AuthenticatorData []byte
	Signature         []byte
	CredentialID      []byte
}

// Prompts is how the ceremony reaches the person holding the key.
//
// Touching a key is not something a program can do on somebody's behalf, so the
// wait has to be announced or it reads as a hang. What to announce comes from
// here rather than from the caller, because what happens next differs by
// platform: a key waiting for a finger, or a dialog Windows is about to draw.
// PIN is consulted only when the key insists on one; a key that verifies by
// fingerprint, or not at all, never asks.
type Prompts struct {
	Touch func(instruction string)
	PIN   func() (string, error)
}

func (p Prompts) touch(instruction string) {
	if p.Touch != nil {
		p.Touch(instruction)
	}
}

func (p Prompts) pin() (string, error) {
	if p.PIN == nil {
		return "", ErrPINRequired
	}
	return p.PIN()
}

// What can go wrong on the way to an assertion. Each is a different sentence to
// the person in front of the terminal, so they are distinguishable here rather
// than being one opaque failure.
var (
	// ErrNoDevice means nothing that could answer is plugged in.
	ErrNoDevice = errors.New("no security key is connected")
	// ErrPermission means a key is there and this user may not open it, which on
	// Linux is a missing udev rule rather than anything the person did.
	ErrPermission = errors.New("a security key is connected but cannot be opened")
	// ErrNoCredential means the key works and holds nothing for this account.
	ErrNoCredential = errors.New("this security key holds no credential for the account")
	// ErrDenied covers a key that was never touched and a ceremony called off.
	ErrDenied = errors.New("the security key was not touched")
	// ErrPINRequired means the key will not answer without its PIN and there was
	// nobody to ask for one.
	ErrPINRequired = errors.New("this security key needs its PIN")
	// ErrPINWrong is a PIN the key rejected. The key counts these and locks
	// itself after enough of them, which is why nothing here tries twice.
	ErrPINWrong = errors.New("that is not the PIN of this security key")
	// ErrPINBlocked means the key has locked itself and only a reset will do.
	ErrPINBlocked = errors.New("this security key has locked itself after too many wrong PINs")
	// ErrUnsupported means this build on this platform cannot talk to a key.
	ErrUnsupported = errors.New("security keys are not available on this platform")
)

// Assert asks a key to answer req, and returns the answer Proton wants.
func Assert(ctx context.Context, req Request, p Prompts) (Assertion, error) {
	o, err := parse(req.Options)
	if err != nil {
		return Assertion{}, err
	}
	if err := o.belongsTo(req.Host); err != nil {
		return Assertion{}, err
	}
	return o.ceremony(ctx, assert, p)
}

// signer is a key being asked to sign, once the challenge has been understood
// and found to be about Proton. There is one of these per platform.
type signer func(context.Context, options, []byte, Prompts) (Assertion, error)

// ceremony is everything around the signature: what the key is given to sign,
// and what has to be true of what comes back.
func (o options) ceremony(ctx context.Context, sign signer, p Prompts) (Assertion, error) {
	clientData, err := o.clientData()
	if err != nil {
		return Assertion{}, err
	}
	a, err := sign(ctx, o, clientData, p)
	if err != nil {
		return Assertion{}, err
	}
	if a.CredentialID, err = o.credential(a.CredentialID); err != nil {
		return Assertion{}, err
	}
	a.ClientData = clientData
	return a, nil
}

// credential names the credential that answered.
//
// A key given exactly one to choose from is allowed to answer without saying
// which it used, and most sign-ins are that case - Proton names one registered
// key per challenge. Proton still wants to be told, so the one it offered is the
// answer.
func (o options) credential(reported []byte) ([]byte, error) {
	switch {
	case len(reported) > 0:
		return reported, nil
	case len(o.allowCredentials) == 1:
		return o.allowCredentials[0], nil
	}
	return nil, errors.New("the security key did not say which credential answered")
}

// options is the part of Proton's AuthenticationOptions this ceremony uses.
type options struct {
	rpID             string
	challenge        []byte
	allowCredentials [][]byte
	userVerification string
	milliseconds     uint32
}

// octets is a byte string written as an array of numbers, which is how Proton
// serialises the binary parts of a WebAuthn challenge.
type octets []byte

func (o *octets) UnmarshalJSON(b []byte) error {
	var numbers []uint8
	if err := json.Unmarshal(b, &numbers); err != nil {
		return err
	}
	*o = numbers
	return nil
}

func parse(raw json.RawMessage) (options, error) {
	var doc struct {
		PublicKey struct {
			RPID             string `json:"rpId"`
			Challenge        octets `json:"challenge"`
			AllowCredentials []struct {
				ID octets `json:"id"`
			} `json:"allowCredentials"`
			UserVerification string `json:"userVerification"`
			Timeout          uint32 `json:"timeout"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return options{}, fmt.Errorf("unreadable security-key challenge: %w", err)
	}
	o := options{
		rpID:             doc.PublicKey.RPID,
		challenge:        doc.PublicKey.Challenge,
		userVerification: doc.PublicKey.UserVerification,
		milliseconds:     doc.PublicKey.Timeout,
	}
	for _, c := range doc.PublicKey.AllowCredentials {
		if len(c.ID) > 0 {
			o.allowCredentials = append(o.allowCredentials, c.ID)
		}
	}
	switch {
	case o.rpID == "":
		return options{}, errors.New("the security-key challenge names no relying party")
	case len(o.challenge) == 0:
		return options{}, errors.New("the security-key challenge carries nothing to sign")
	case len(o.allowCredentials) == 0:
		return options{}, ErrNoCredential
	}
	return o, nil
}

// belongsTo refuses a challenge that asks about somebody else's relying party.
//
// This is the check a browser makes on every ceremony, and the reason a key
// cannot be phished through one. Proton names the rpId in its own answer, so
// taking that name on trust would mean a forged or altered answer could have
// this program collect an assertion for any site the key holds a credential for.
// The rule is WebAuthn's: the relying party is the host that sent the challenge,
// or a domain that host sits under.
func (o options) belongsTo(host string) error {
	name := strings.ToLower(strings.TrimSuffix(o.rpID, "."))
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if h != "" && strings.Contains(name, ".") &&
		(h == name || strings.HasSuffix(h, "."+name)) {
		return nil
	}
	return fmt.Errorf("refusing to ask the security key about %q, which %q has no claim to", o.rpID, host)
}

// clientData is what the key signs the hash of, and what Proton checks the
// signature against. Its shape is WebAuthn's and its origin is the one Proton's
// own clients present for this relying party.
func (o options) clientData() ([]byte, error) {
	return json.Marshal(struct {
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
		Origin    string `json:"origin"`
	}{
		Type:      "webauthn.get",
		Challenge: base64.RawURLEncoding.EncodeToString(o.challenge),
		Origin:    "https://" + o.rpID,
	})
}

// needsVerification reports whether the relying party asked for the person to be
// verified, rather than merely present.
func (o options) needsVerification() bool {
	return strings.EqualFold(o.userVerification, "required")
}

// timeout is how long the relying party is willing to wait, for the one platform
// that runs the ceremony on a clock of its own. A challenge that names no
// deadline gets the two minutes a person needs to find a key in a drawer.
func (o options) timeout() time.Duration {
	if o.milliseconds == 0 {
		return 2 * time.Minute
	}
	return time.Duration(o.milliseconds) * time.Millisecond
}
