package cli

import (
	"errors"

	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/proton"
)

// exitCode classifies an error into the CLI's exit-code scheme:
//
//	0 success · 1 user error · 2 auth · 3 not-found · 4 conflict/ambiguous ·
//	5 network/server · 6 refused by the confirmation policy.
//
// A refusal has a code of its own because a caller has to be able to tell it
// from a mistake: nothing about the command was wrong, and repeating it with
// different arguments will not help.
//
// Typed errors that implement errs.ExitCoder (NotFound, Ambiguous, WrongTable,
// explicit Exit wraps, APIError) carry their own code; everything else is a
// generic user error (1), except auth sentinels.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var coder errs.ExitCoder
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	if errors.Is(err, proton.ErrUnauthorized) {
		return 2
	}
	return 1
}
