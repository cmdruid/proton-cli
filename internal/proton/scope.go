package proton

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Scope elevation, mirroring how the web clients do it.
//
// Proton guards its most destructive endpoints behind an elevated session scope.
// A request that needs one and does not have it comes back as HTTP 403 with
// Proton code 9100, and the client is expected to prove the user is present by
// re-running SRP against a scope-granting endpoint, retry, and then drop the
// scope again.
//
// WebClients handles this in the API layer rather than in each feature: the
// api helper raises an "unlock" event, a password dialog appears, srpAuth calls
// queryUnlock(), and the original request is retried
// (packages/shared/lib/api/helpers/apiErrorHelper.ts,
// packages/components/containers/api/ApiModals.tsx).
//
// This client does the same, which is why no command in the CLI knows that any
// particular operation is password-guarded. The alternative - a per-command
// check - can only ever cover the operations someone remembered to annotate,
// and gets the scope wrong as easily as right.

// Scope names an elevated capability a session can hold.
type Scope string

const (
	// ScopeLocked guards destructive data operations: deleting a calendar,
	// exporting a private key, reading a secret back out.
	ScopeLocked Scope = "locked"
	// ScopePassword guards changes to the credentials themselves: the password,
	// two-factor settings, the recovery phrase.
	ScopePassword Scope = "password"
)

// endpoint is the SRP-guarded call that grants the scope. Mirrors queryUnlock
// and unlockPasswordChanges in WebClients (packages/shared/lib/api/user.ts).
func (s Scope) endpoint() string {
	if s == ScopePassword {
		return "/core/v4/users/password"
	}
	return "/core/v4/users/unlock"
}

// lockPath drops every elevated scope again. WebClients calls this after a
// sensitive operation finishes (lockSensitiveSettings); leaving a session
// elevated for the rest of its life would make the elevation pointless.
const lockPath = "/core/v4/users/lock"

// scopesPath reports the scopes a session currently holds.
const scopesPath = "/core/v4/auth/scopes"

// ScopeCredentials is what an elevation needs. TOTP is only consulted when the
// account has two-factor enabled and the server asks for it.
type ScopeCredentials struct {
	Username string
	Password []byte
	TOTP     string
}

// ScopeResolver supplies credentials when the server reports a missing scope.
// Returning an error aborts the operation with that error, so a resolver that
// cannot ask (no terminal, --no-input) simply explains why.
type ScopeResolver func(ctx context.Context, s Scope) (ScopeCredentials, error)

// SetScopeResolver installs the callback used to elevate a session on demand.
func (c *Client) SetScopeResolver(r ScopeResolver) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scopeResolver = r
}

func (c *Client) getScopeResolver() ScopeResolver {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.scopeResolver
}

// missingScopeCode is Proton's SCOPE_MISSING_UNEXPECTED
// (packages/shared/lib/errors.ts).
const missingScopeCode = 9100

// isMissingScope reports whether a response is the server asking for an elevated
// session.
//
// WebClients matches on 9100 alone, which is not enough here: deleting a
// calendar answers 403 with code 9101 and no name for it anywhere in their
// source. What both answers share is Details.MissingScopes - the server saying
// which scope it wants - so that is the surer signal, and the code is only a
// fallback for an answer that names nothing.
func isMissingScope(status int, body []byte) bool {
	if status != http.StatusForbidden {
		return false
	}
	var env struct {
		Code    int
		Details struct{ MissingScopes []string }
	}
	if json.Unmarshal(body, &env) != nil {
		return false
	}
	return len(env.Details.MissingScopes) > 0 || env.Code == missingScopeCode
}

// Elevate re-authenticates within the current session to obtain s.
//
// The server's proof is verified here as it is at login. SRP authenticates both
// directions, and discarding the server's half would throw away the guarantee
// that we are talking to something which knows the verifier.
func (c *Client) Elevate(ctx context.Context, s Scope, cr ScopeCredentials) error {
	info, err := c.getAuthInfo(ctx, cr.Username, s)
	if err != nil {
		return fmt.Errorf("elevate to %s scope: %w", s, err)
	}
	extra := map[string]any{}
	if cr.TOTP != "" {
		extra["TwoFactorCode"] = cr.TOTP
	}
	if _, err := c.srpCall(ctx, "PUT", s.endpoint(), cr.Username, cr.Password, info, extra, "", ""); err != nil {
		return fmt.Errorf("elevate to %s scope: %w", s, err)
	}
	return nil
}

// Relock drops every elevated scope. Best-effort: the operation it protects has
// already finished, so a failure here is worth a log line and nothing more.
func (c *Client) Relock(ctx context.Context) {
	if err := c.Decode(ctx, Request{Method: "PUT", Path: lockPath}, nil); err != nil {
		c.log.Debug("relock failed; the session stays elevated until it expires", "err", err)
	}
}

// Scopes returns the scopes the session currently holds, for reporting what a
// saved session can actually do.
func (c *Client) Scopes(ctx context.Context) ([]string, error) {
	var r struct{ Scopes []string }
	if err := c.Decode(ctx, Request{Method: "GET", Path: scopesPath}, &r); err != nil {
		return nil, err
	}
	return r.Scopes, nil
}

// elevateAndRetry handles a missing-scope response: ask for credentials, elevate,
// run the request again, then drop the scope.
//
// The retry is marked so a second 403 cannot start the cycle again - an account
// whose password no longer works would otherwise loop.
func (c *Client) elevateAndRetry(ctx context.Context, req Request, resp *Response) (*Response, error) {
	resolver := c.getScopeResolver()
	if resolver == nil || req.elevated {
		_, apiErr := classifyErrorBody(resp.Status, resp.Body)
		return resp, apiErr
	}

	// The body names the scope when it can; ScopeLocked is the one every data
	// operation needs, so it is the safe assumption.
	scope := scopeFromBody(resp.Body)

	cr, err := resolver(ctx, scope)
	if err != nil {
		return resp, err
	}
	if err := c.Elevate(ctx, scope, cr); err != nil {
		return resp, err
	}
	defer c.Relock(ctx)

	retry := req
	retry.elevated = true
	return c.Do(ctx, retry)
}

// scopeFromBody reads the scope the server said was missing. Proton reports it
// inconsistently across endpoints, so anything unrecognised falls back to the
// weaker of the two: elevating further than necessary is worse than being asked
// again.
func scopeFromBody(body []byte) Scope {
	var env struct {
		Details struct {
			MissingScopes []string
		}
	}
	if err := json.Unmarshal(body, &env); err == nil {
		for _, s := range env.Details.MissingScopes {
			if Scope(s) == ScopePassword {
				return ScopePassword
			}
		}
	}
	return ScopeLocked
}

// ErrScopeUnavailable reports that an elevation was needed but could not be
// performed. Resolvers wrap their own reason; this exists so callers can detect
// the class.
var ErrScopeUnavailable = errors.New("session cannot be elevated")
