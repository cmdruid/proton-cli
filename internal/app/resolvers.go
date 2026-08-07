package app

import (
	"context"

	"github.com/roman-16/proton-cli/internal/proton"
)

// The transport has exactly two reasons to need something from a person: Proton
// asked for human verification, or Proton asked the session to be elevated. Both
// arrive as callbacks installed here, so the request path stays free of any
// notion of a user interface and no command has to anticipate either event.

type scopeReasonKey struct{}

// WithScopeReason records what the command is about to do, so a password prompt
// can say why it appeared. Commands that knowingly touch a guarded endpoint set
// it; anything else falls back to a generic phrasing.
//
// It travels in the context rather than on the App because it describes one
// operation, not the invocation.
func WithScopeReason(ctx context.Context, reason string) context.Context {
	return context.WithValue(ctx, scopeReasonKey{}, reason)
}

func scopeReason(ctx context.Context, s proton.Scope) string {
	if r, ok := ctx.Value(scopeReasonKey{}).(string); ok && r != "" {
		return r
	}
	if s == proton.ScopePassword {
		return "change your account credentials"
	}
	return "confirm this operation"
}

// installScopeResolver teaches the client how to elevate a session when the
// server refuses a request for want of a scope.
func (a *App) installScopeResolver() {
	a.API.SetScopeResolver(func(ctx context.Context, s proton.Scope) (proton.ScopeCredentials, error) {
		user, err := a.Creds.User()
		if err != nil {
			return proton.ScopeCredentials{}, err
		}
		password, err := a.Creds.Password(scopeReason(ctx, s))
		if err != nil {
			return proton.ScopeCredentials{}, err
		}
		return proton.ScopeCredentials{
			Username: user,
			Password: []byte(password),
			// A code is supplied only if one is already to hand. Prompting for a
			// TOTP that may not be wanted would burn a thirty-second window on a
			// guess; if the server does want one, it says so and the user can
			// pass --totp.
			TOTP: a.Creds.TOTPIfSet(),
		}, nil
	})
}
