package proton

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ProtonMail/go-srp"
	"github.com/roman-16/proton-cli/internal/errs"
)

type authInfo struct {
	Code            int
	Modulus         string
	ServerEphemeral string
	Version         int
	Salt            string
	SRPSession      string
}

type authResp struct {
	Code         int
	UID          string
	AccessToken  string
	RefreshToken string
	ServerProof  string
	TwoFA        struct {
		Enabled int
	} `json:"2FA"`
}

// Two-factor methods, as the bitfield Proton returns in 2FA.Enabled.
// Mirrors SETTINGS_2FA_ENABLED in WebClients
// (packages/shared/lib/interfaces/UserSettings.ts).
const (
	twoFAOTP   = 1
	twoFAFIDO2 = 2
)

// TOTPFunc supplies a two-factor code. It is called only when the server says
// the account has TOTP enabled, so an account without two-factor never triggers
// a prompt and a code is never asked for speculatively - which matters, because
// a TOTP is only valid for thirty seconds.
type TOTPFunc func() (string, error)

// Login performs the full web-client auth flow (unauth session → SRP → 2FA).
// On a Proton 9001 (human-verification) response from the SRP step, Login
// consults the installed HVResolver (if any) and retries the auth POST once
// with HV headers, redoing getAuthInfo to obtain a fresh SRPSession.
func (c *Client) Login(ctx context.Context, username string, password []byte, totp TOTPFunc) error {
	sess, err := c.createSession(ctx)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	c.mu.Lock()
	c.uid, c.acc, c.ref = sess.UID, sess.AccessToken, sess.RefreshToken
	c.mu.Unlock()

	auth, err := c.loginSRP(ctx, username, password, "", "")
	var hvErr *HumanVerificationError
	if errors.As(err, &hvErr) {
		if resolver := c.getHVResolver(); resolver != nil {
			token, kind, rerr := resolver(hvErr)
			switch {
			case rerr == nil && token != "":
				auth, err = c.loginSRP(ctx, username, password, token, kind)
			case errors.Is(rerr, ErrHVUnavailable):
				// keep err = hvErr, surfaced below
			case rerr != nil:
				return fmt.Errorf("login: hv resolver: %w", rerr)
			}
		}
	}
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	c.mu.Lock()
	c.uid, c.acc, c.ref = auth.UID, auth.AccessToken, auth.RefreshToken
	// A sealed key password belongs to the session it was wrapped for, and this is
	// a different session: Proton holds no client key for it, so the blob a
	// previous sign-in left behind can never be opened again. Dropping it here is
	// what keeps `account get` from reporting a profile as unlocked on the strength
	// of a blob nothing can read.
	c.encKeyBlob = ""
	c.mu.Unlock()

	switch {
	case auth.TwoFA.Enabled&twoFAOTP != 0:
		if totp == nil {
			return fmt.Errorf("login: account requires a two-factor code")
		}
		code, err := totp()
		if err != nil {
			return err
		}
		if err := c.auth2FA(ctx, code); err != nil {
			return fmt.Errorf("login: %w", err)
		}
	case auth.TwoFA.Enabled&twoFAFIDO2 != 0:
		// A security key needs a browser to talk WebAuthn, which a terminal has no
		// way to do. Saying so is better than the stream of 401s that would follow
		// a silently half-finished sign-in.
		return errs.Problemf("This account signs in with a security key, which proton cannot use.").
			Hint("add a TOTP authenticator app at https://account.proton.me/mail/account-password,",
				"then sign in with --totp or let proton ask for the code.").Exit(2)
	case auth.TwoFA.Enabled != 0:
		return errs.Problemf("This account uses a two-factor method proton does not support (0x%x).",
			auth.TwoFA.Enabled).Exit(2)
	}
	return nil
}

// srpExchange is one SRP exchange: the endpoint that answers it, who is proving
// what, and what the proof carries alongside itself.
type srpExchange struct {
	method string
	path   string

	username string
	password []byte

	// scope is the elevation the parameters are asked for, empty when signing in.
	scope Scope
	// extra is what the endpoint wants alongside the proof.
	extra map[string]any

	hvToken string
	hvType  string
}

// repeatable reports whether the whole exchange may be run a second time.
//
// One carrying a two-factor code may not: a code is single-use, so a second run
// submits one Proton has already taken and is told it is wrong - which would put
// the blame on the person for a bad moment upstream.
func (x srpExchange) repeatable() bool {
	_, twoFactor := x.extra["TwoFactorCode"]
	return !twoFactor
}

// exchange runs an SRP exchange: fresh parameters, then the proof.
//
// The pair is the unit that gets another go, because the proof on its own cannot
// be sent twice - the SRPSession the parameters carried is spent by the first
// attempt, so a second attempt needs a second set. That is exactly what a person
// does on seeing the error, and it has the same consequence.
func (c *Client) exchange(ctx context.Context, x srpExchange) (*Response, error) {
	return c.retrying(ctx,
		Request{Method: x.method, Path: x.path, Repeatable: x.repeatable()},
		func() (*Response, error) {
			info, err := c.getAuthInfo(ctx, x.username, x.scope)
			if err != nil {
				return nil, err
			}
			return c.srpCall(ctx, x, info)
		})
}

// srpCall proves the password against info and validates the server's proof.
//
// It is the single SRP code path: signing in and elevating a scope differ only
// in which endpoint they call and what they add to the body. Sharing it is what
// guarantees both verify the server's proof, which is the half of SRP that
// authenticates Proton to us.
//
// Mirrors srpAuth in WebClients (packages/shared/lib/srp.ts), whose
// callAndValidate rejects an unexpected server proof for every caller.
func (c *Client) srpCall(ctx context.Context, x srpExchange, info *authInfo) (*Response, error) {
	auth, err := srp.NewAuth(info.Version, x.username, x.password, info.Salt, info.Modulus, info.ServerEphemeral)
	if err != nil {
		return nil, fmt.Errorf("SRP setup: %w", err)
	}
	proofs, err := auth.GenerateProofs(2048)
	if err != nil {
		return nil, fmt.Errorf("SRP proofs: %w", err)
	}

	payload := map[string]any{
		"ClientProof":     base64.StdEncoding.EncodeToString(proofs.ClientProof),
		"ClientEphemeral": base64.StdEncoding.EncodeToString(proofs.ClientEphemeral),
		"SRPSession":      info.SRPSession,
	}
	for k, v := range x.extra {
		payload[k] = v
	}

	resp, err := c.authCall(ctx, Request{
		Method: x.method, Path: x.path, Body: payload,
		HVToken: x.hvToken, HVType: x.hvType,
	})
	if err != nil {
		return nil, err
	}
	var r struct{ ServerProof string }
	if err := readAnswer(resp.Body, &r); err != nil {
		return nil, err
	}
	serverProof, err := base64.StdEncoding.DecodeString(r.ServerProof)
	if err != nil {
		return nil, fmt.Errorf("server proof decode: %w", err)
	}
	if !bytes.Equal(serverProof, proofs.ExpectedServerProof) {
		return nil, fmt.Errorf("server proof verification failed")
	}
	return resp, nil
}

func (c *Client) createSession(ctx context.Context) (*authResp, error) {
	resp, err := c.authCall(ctx, Request{
		Method: "POST", Path: "/auth/v4/sessions", Body: []byte("{}"),
		headers: map[string]string{"x-enforce-unauthsession": "true"},
		// An unauthenticated session is scratch: one created twice leaves behind a
		// session nothing holds the tokens to use, which Proton reaps, and failing
		// the whole sign-in instead is the worse of the two outcomes.
		Repeatable: true,
	})
	if err != nil {
		return nil, err
	}
	var r authResp
	if err := readAnswer(resp.Body, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// getAuthInfo fetches the SRP parameters for username.
//
// Intent identifies the sign-in flow, and ReauthScope tells the server which
// elevation the following exchange is for; both are what the web clients send
// (packages/shared/lib/api/auth.ts getInfo). reauthScope is empty for a fresh
// sign-in.
//
// It is not marked repeatable, though asking twice spends nothing: it is only
// ever one half of an exchange, and the exchange is what gets another go. Two
// budgets over the same failure would multiply into minutes of waiting.
func (c *Client) getAuthInfo(ctx context.Context, username string, reauthScope Scope) (*authInfo, error) {
	payload := map[string]any{"Intent": "Proton"}
	if username != "" {
		payload["Username"] = username
	}
	if reauthScope != "" {
		payload["ReauthScope"] = string(reauthScope)
	}
	resp, err := c.authCall(ctx, Request{Method: "POST", Path: "/core/v4/auth/info", Body: payload})
	if err != nil {
		return nil, err
	}
	var r authInfo
	if err := readAnswer(resp.Body, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// loginSRP proves the password to the endpoint that hands out a session.
// When hvToken/hvType are non-empty they're attached as HV headers on the proof.
func (c *Client) loginSRP(ctx context.Context, username string, password []byte, hvToken, hvType string) (*authResp, error) {
	resp, err := c.exchange(ctx, srpExchange{
		method: "POST", path: "/core/v4/auth",
		username: username, password: password,
		extra:   map[string]any{"Username": username},
		hvToken: hvToken, hvType: hvType,
	})
	if err != nil {
		return nil, err
	}
	var r authResp
	if err := readAnswer(resp.Body, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) auth2FA(ctx context.Context, totp string) error {
	// Not repeatable, whatever goes wrong: a code is single-use and lives for
	// thirty seconds, so by the time a wait is over it is either spent or expired,
	// and submitting it again is answered with "incorrect code" - a sentence that
	// blames the person for something Proton did.
	_, err := c.authCall(ctx, Request{
		Method: "POST", Path: "/core/v4/auth/2fa",
		Body: map[string]string{"TwoFactorCode": totp},
	})
	return err
}

// authCall sends one request of the auth flow and returns the answer to it.
//
// The flow stands here rather than on Do, one layer up: refreshing a session,
// elevating a scope and refusing a dry run are all answers about a session, and
// during a sign-in there is not one yet. What it does share with every other
// request is the part that matters - the status is read before the body, and a
// bad moment upstream is waited out rather than parsed.
func (c *Client) authCall(ctx context.Context, req Request) (*Response, error) {
	resp, err := c.attempt(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := responseError(resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// readAnswer reads an auth-flow answer into out.
//
// A body that is not the JSON it should be, with a status that said the request
// succeeded, is Proton answering something nobody can act on. Saying that beats
// quoting the parser at somebody who cannot fix it.
func readAnswer(raw []byte, out any) error {
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("unreadable answer from Proton: %w", err)
	}
	return nil
}
