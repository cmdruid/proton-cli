//go:build windows && webauthn

package fido

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-ctap/ctaphid/pkg/webauthntypes"
	"github.com/go-ctap/winhello"
	"github.com/go-ctap/winhello/window"
	"golang.org/x/sys/windows"
)

// assert hands the ceremony to Windows, which owns every authenticator on the
// machine: a USB key plugged into it, and the fingerprint or PIN that Windows
// Hello itself is. Reaching the USB key directly is not an option here - Windows
// reserves that to processes running as administrator, which no sign-in should
// need to be.
func assert(ctx context.Context, o options, clientData []byte, p Prompts) (Assertion, error) {
	if winhello.InitError != nil {
		return Assertion{}, ErrUnsupported
	}
	// Windows draws its own dialog and puts it in front of whatever window asked.
	// A terminal has the foreground when somebody is typing in it, which is the
	// only moment this runs.
	hwnd, err := window.GetForegroundWindow()
	if err != nil {
		return Assertion{}, fmt.Errorf("asking Windows for a security key: %w", err)
	}

	allow := make([]webauthntypes.PublicKeyCredentialDescriptor, 0, len(o.allowCredentials))
	for _, id := range o.allowCredentials {
		allow = append(allow, webauthntypes.PublicKeyCredentialDescriptor{
			Type: webauthntypes.PublicKeyCredentialTypePublicKey,
			ID:   id,
		})
	}

	p.touch("Follow the prompt from Windows to use your security key.")
	assertion, err := winhello.GetAssertion(hwnd, o.rpID, clientData, allow, nil,
		&winhello.AuthenticatorGetAssertionOptions{
			Timeout: o.timeout(),
			// Neither attachment is ruled out: Proton registers a key plugged into
			// the machine and a Windows Hello credential living in it under the same
			// setting, and either can be the one this account has.
			AuthenticatorAttachment:     winhello.WinHelloAuthenticatorAttachmentAny,
			UserVerificationRequirement: verificationRequirement(o),
		})
	if err != nil {
		return Assertion{}, answered(err)
	}
	return Assertion{
		CredentialID:      assertion.Credential.ID,
		AuthenticatorData: assertion.AuthDataRaw,
		Signature:         assertion.Signature,
	}, nil
}

func verificationRequirement(o options) winhello.WinHelloUserVerificationRequirement {
	if o.needsVerification() {
		return winhello.WinHelloUserVerificationRequirementRequired
	}
	return winhello.WinHelloUserVerificationRequirementDiscouraged
}

// errTimeout is what the WebAuthn API returns when its dialog waited long
// enough, as the Win32 timeout wrapped into an HRESULT.
const errTimeout = windows.Errno(0x80070000 | uint32(windows.ERROR_TIMEOUT))

// answered turns what Windows said into what this package promises. Anything
// else is passed through: an unrecognised HRESULT is better read as itself than
// flattened into a sentence that may be wrong.
func answered(err error) error {
	switch {
	case errors.Is(err, windows.Errno(windows.NTE_USER_CANCELLED)), errors.Is(err, errTimeout):
		return ErrDenied
	case errors.Is(err, windows.Errno(windows.NTE_DEVICE_NOT_FOUND)):
		return ErrNoDevice
	case errors.Is(err, windows.Errno(windows.NTE_NOT_FOUND)):
		return ErrNoCredential
	case errors.Is(err, windows.Errno(windows.NTE_NOT_SUPPORTED)):
		return ErrUnsupported
	}
	return err
}
