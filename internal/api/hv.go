package api

import "errors"

// HVResolver is invoked by the api layer when a Proton API request
// returns code 9001 (human verification required). The resolver must
// obtain a verification token and type — typically by prompting the
// user via stdin (email/SMS/ownership) or by spawning a webview
// (captcha) — and return them. The api layer then retries the
// original request once with the HV headers.
//
// Returning ErrHVUnavailable signals "this resolver can't help with
// the offered methods" (e.g. captcha-only on a server with no GUI).
// The original *HumanVerificationError bubbles up to the caller.
//
// Returning any other error aborts the retry; the error replaces the
// HV error on the way up.
type HVResolver func(*HumanVerificationError) (token string, tokenType string, err error)

// ErrHVUnavailable is the sentinel a resolver returns when none of the
// offered HV methods can be completed in this environment. The api
// layer treats it as "give up, surface the original HV error".
var ErrHVUnavailable = errors.New("human verification cannot be completed in this context")

// SetHVResolver installs an HVResolver on the client. Pass nil to
// disable resolver-driven retries (back to surfacing 9001 directly).
//
// Safe to call before any request; not safe concurrently with one.
func (c *Client) SetHVResolver(r HVResolver) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hvResolver = r
}

// hvResolver returns the currently-installed resolver, or nil. Used by
// Do and the auth path to decide whether to attempt a retry.
func (c *Client) getHVResolver() HVResolver {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hvResolver
}
