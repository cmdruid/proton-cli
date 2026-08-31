//go:build !windows

package fido

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/telesma-app/ctap/authenticator"
	"github.com/telesma-app/ctap/backend"
	hidkeys "github.com/telesma-app/ctap/backend/hid"
	"github.com/telesma-app/ctap/credential"
	"github.com/telesma-app/ctap/protocol"
	"github.com/telesma-app/ctap/transport"
)

// assert talks CTAP2 to a key over USB, which is the transport every roaming
// security key answers on. A key reached over Bluetooth or lying in a phone is
// not one of these; nothing on this side of the wire can reach those.
func assert(ctx context.Context, o options, clientData []byte, p Prompts) (Assertion, error) {
	return assertVia(ctx, hidkeys.Enumerate, o, clientData, p)
}

// assertVia is the ceremony against whichever keys enumerate finds. Production
// finds them on the USB bus; a test can hand it one that answers in-process,
// which is the only way this path runs without hardware in the room.
func assertVia(
	ctx context.Context, enumerate backend.Enumerator,
	o options, clientData []byte, p Prompts,
) (Assertion, error) {
	// Announced before the search, not after: choosing between two connected keys
	// is itself done by touching one, so the first thing that can block on a
	// person is already behind this line.
	p.touch("Touch your security key.")
	device, err := authenticator.Select(ctx, enumerate)
	if err != nil {
		return Assertion{}, found(ctx, err)
	}
	defer func() { _ = device.Close() }()

	token, err := verification(ctx, device, o, p)
	if err != nil {
		return Assertion{}, err
	}
	a, err := assertion(ctx, device, token, o, clientData)
	// A key may hold its own opinion about being verified - one configured to
	// always ask says so here rather than in its capabilities, and the only
	// answer is its PIN.
	if errors.Is(err, errNeedsPIN) && token == nil {
		if token, err = pinToken(ctx, device, o, p); err != nil {
			return Assertion{}, err
		}
		a, err = assertion(ctx, device, token, o, clientData)
	}
	return a, err
}

// errNeedsPIN is the key saying it will not sign until the person is verified.
// It never reaches a caller: it is what turns into asking for the PIN.
var errNeedsPIN = errors.New("security key requires verification")

// verification obtains what the key needs to be satisfied the person is there,
// before anything is asked of it - but only when the relying party or the key
// itself insists. Asking for a PIN that nothing wanted spends the person's
// attention and, on a key that counts attempts, some of its patience.
func verification(ctx context.Context, device *authenticator.Device, o options, p Prompts) ([]byte, error) {
	info, err := device.GetInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the security key: %w", err)
	}
	if !o.needsVerification() && !info.Options[protocol.OptionAlwaysUv] {
		return nil, nil
	}
	// A key with no PIN set verifies by fingerprint or not at all, and either way
	// there is nothing to ask for.
	if set, supported := info.Options[protocol.OptionClientPIN]; !supported || !set {
		return nil, nil
	}
	return pinToken(ctx, device, o, p)
}

func pinToken(ctx context.Context, device *authenticator.Device, o options, p Prompts) ([]byte, error) {
	pin, err := p.pin()
	if err != nil {
		return nil, err
	}
	// The token is asked for with the one permission it is about to be used with,
	// so a key that supports scoped tokens hands over nothing wider.
	token, err := device.GetPinUvAuthTokenUsingPIN(ctx, pin, protocol.PermissionGetAssertion, o.rpID)
	if err != nil {
		return nil, answered(err)
	}
	return token, nil
}

func assertion(
	ctx context.Context, device *authenticator.Device, token []byte,
	o options, clientData []byte,
) (Assertion, error) {
	allow := make([]credential.PublicKeyCredentialDescriptor, 0, len(o.allowCredentials))
	for _, id := range o.allowCredentials {
		allow = append(allow, credential.PublicKeyCredentialDescriptor{
			Type: credential.PublicKeyCredentialTypePublicKey,
			ID:   id,
		})
	}
	for resp, err := range device.GetAssertion(ctx, token, o.rpID, clientData, allow,
		nil, map[protocol.Option]bool{protocol.OptionUserPresence: true}) {
		if err != nil {
			return Assertion{}, answered(err)
		}
		// The first assertion is the answer: Proton names the credentials it will
		// accept, so any of them proves the same thing, and asking the key for the
		// rest would be another touch for nothing.
		return Assertion{
			CredentialID:      resp.Credential.ID,
			AuthenticatorData: resp.AuthDataRaw,
			Signature:         resp.Signature,
		}, nil
	}
	return Assertion{}, ErrNoCredential
}

// found says why no key answered. A key that is present but unopenable is the
// common Linux case and a different sentence from no key at all.
func found(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, fs.ErrPermission) {
		return ErrPermission
	}
	return ErrNoDevice
}

// answered turns what a key said into what this package promises. A status this
// does not name is passed through: an unrecognised refusal is better read in the
// key's own words than flattened into somebody else's.
func answered(err error) error {
	var status *transport.CTAPError
	if !errors.As(err, &status) {
		return err
	}
	switch status.StatusCode {
	case transport.CTAP2_ERR_NO_CREDENTIALS:
		return ErrNoCredential
	case transport.CTAP2_ERR_OPERATION_DENIED, transport.CTAP2_ERR_ACTION_TIMEOUT, transport.CTAP1_ERR_TIMEOUT:
		return ErrDenied
	case transport.CTAP2_ERR_PUAT_REQUIRED:
		return errNeedsPIN
	case transport.CTAP2_ERR_PIN_INVALID:
		return ErrPINWrong
	case transport.CTAP2_ERR_PIN_BLOCKED, transport.CTAP2_ERR_PIN_AUTH_BLOCKED, transport.CTAP2_ERR_UV_BLOCKED:
		return ErrPINBlocked
	}
	return err
}
