//go:build windows && !webauthn

package fido

import "context"

// A Windows build made without the webauthn tag cannot reach a security key.
//
// Every authenticator on Windows belongs to the operating system - it hands out
// assertions through webauthn.dll and reserves the USB devices themselves to
// administrators - and reaching that API means a dependency the released
// binaries carry and a `go install` build does not. It is the same bargain the
// CAPTCHA helper makes, and the same answer: say so plainly, so the sign-in ends
// in a sentence rather than in a wait for a key nothing will ever ask for.
func assert(context.Context, options, []byte, Prompts) (Assertion, error) {
	return Assertion{}, ErrUnsupported
}
