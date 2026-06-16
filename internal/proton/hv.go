package proton

import "errors"

// HVResolver is invoked when a Proton API request returns code 9001 (human
// verification required). It must obtain a verification token and type —
// typically by spawning a webview (captcha) — and return them; the client
// then retries the original request once with the HV headers set.
//
// Returning ErrHVUnavailable signals "this resolver can't help with the
// offered methods"; the original *HumanVerificationError bubbles up. Any
// other error aborts the retry and replaces the HV error.
type HVResolver func(*HumanVerificationError) (token string, tokenType string, err error)

// ErrHVUnavailable is the sentinel a resolver returns when none of the offered
// HV methods can be completed in this environment.
var ErrHVUnavailable = errors.New("human verification cannot be completed in this context")

// SetHVResolver installs an HVResolver. Pass nil to disable resolver-driven
// retries. Safe to call before any request; not safe concurrently with one.
func (c *Client) SetHVResolver(r HVResolver) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hvResolver = r
}

func (c *Client) getHVResolver() HVResolver {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hvResolver
}
