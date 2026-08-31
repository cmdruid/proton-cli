package proton

import "errors"

// MethodCaptcha is the one human-verification method a client can carry through
// on its own. A code sent by email or SMS is checked by Proton's page and
// reported to nobody, so a client that offered to complete one would be
// promising something it cannot deliver.
const MethodCaptcha = "captcha"

// HVResolver is invoked when a Proton API request returns code 9001 (human
// verification required). It obtains a token and the method that produced it;
// the client then retries the original request once with the HV headers set.
//
// The token is not always the answer to the challenge. A CAPTCHA solved in a
// browser is submitted by Proton's own page, which leaves the proof standing on
// the challenge token itself - so what comes back from a resolver that sent
// somebody to that page is the challenge it was given. Both are carried in
// x-pm-human-verification-token, and the server tells them apart.
//
// Returning ErrHVUnavailable signals "nothing here can complete the offered
// methods"; the original *HumanVerificationError bubbles up. Any other error
// aborts the retry and replaces the HV error.
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
