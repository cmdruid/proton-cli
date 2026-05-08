// Package api is the low-level Proton HTTP client: request building, response
// decoding, typed errors, and token refresh. No domain logic lives here.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/roman-16/proton-cli/internal/session"
)

const (
	DefaultBaseURL    = "https://mail.proton.me/api"
	DefaultAppVersion = "Other"
)

// Client is a Proton API client for a single profile/session.
type Client struct {
	hc   *http.Client
	base string
	app  string
	log  *slog.Logger

	mu            sync.RWMutex
	uid           string
	acc           string
	ref           string
	saltedKeyPass string
	profile       string
	hvResolver    HVResolver
}

// Options configures a new Client.
type Options struct {
	BaseURL    string
	AppVersion string
	Profile    string
	HTTPClient *http.Client
	Logger     *slog.Logger
}

// New constructs a Client. Empty fields fall back to defaults.
func New(opts Options) *Client {
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultBaseURL
	}
	if opts.AppVersion == "" {
		opts.AppVersion = DefaultAppVersion
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Minute}
	}
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Client{hc: hc, base: opts.BaseURL, app: opts.AppVersion, profile: opts.Profile, log: log}
}

// SetLogger swaps the logger used for request tracing.
func (c *Client) SetLogger(l *slog.Logger) {
	if l != nil {
		c.log = l
	}
}

// BaseURL returns the base URL the client is configured with.
func (c *Client) BaseURL() string { return c.base }

// AppVersion returns the configured application-version header value.
func (c *Client) AppVersion() string { return c.app }

// Profile returns the session profile the client is bound to.
func (c *Client) Profile() string { return c.profile }

// SetTokens installs auth state (from a previously saved session).
func (c *Client) SetTokens(uid, acc, ref, saltedKeyPass string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.uid = uid
	c.acc = acc
	c.ref = ref
	c.saltedKeyPass = saltedKeyPass
}

// SaltedKeyPass returns the cached salted key password.
func (c *Client) SaltedKeyPass() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.saltedKeyPass
}

// SetSaltedKeyPass stores the salted key password.
func (c *Client) SetSaltedKeyPass(skp string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.saltedKeyPass = skp
}

// Session returns a snapshot of the current auth state for persistence.
func (c *Client) Session() *session.Session {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return &session.Session{
		UID:           c.uid,
		AccessToken:   c.acc,
		RefreshToken:  c.ref,
		SaltedKeyPass: c.saltedKeyPass,
		AppVersion:    c.app,
		BaseURL:       c.base,
	}
}

// Request is a typed API request. Body is JSON-encoded when it is not nil
// and not already a []byte / io.Reader.
type Request struct {
	Method string
	Path   string
	Query  url.Values
	Body   any

	// Human-verification state (set by retry logic, not by most callers).
	HVToken string
	HVType  string
}

// Response is the raw response returned by Do.
type Response struct {
	Status int
	Body   []byte
}

// Do sends a request and returns the response. Non-2xx responses return a
// typed error; connection errors are returned as-is.
//
// HV retry: if the response is a 9001 (human-verification required) AND
// an HVResolver is installed (see SetHVResolver), Do invokes the
// resolver, then retries the request once with the resulting
// x-pm-human-verification-token and -type headers set. Resolver returns
// ErrHVUnavailable => original HV error surfaces unchanged.
func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	resp, err := c.doOnce(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.Status == http.StatusUnauthorized {
		if rerr := c.refreshAuth(ctx); rerr != nil {
			return resp, ErrUnauthorized
		}
		_ = session.Save(c.profile, c.Session())
		resp, err = c.doOnce(ctx, req)
		if err != nil {
			return nil, err
		}
	}

	if resp.Status >= 200 && resp.Status < 300 {
		return resp, nil
	}

	hvErr, apiErr := classifyErrorBody(resp.Status, resp.Body)
	if hvErr != nil {
		// Try the resolver path. Avoid re-resolving if this request
		// already carried an HV header (resolver loop guard).
		if req.HVToken == "" {
			if resolver := c.getHVResolver(); resolver != nil {
				token, kind, rerr := resolver(hvErr)
				switch {
				case rerr == nil && token != "":
					retry := req
					retry.HVToken = token
					retry.HVType = kind
					return c.Do(ctx, retry)
				case errors.Is(rerr, ErrHVUnavailable):
					// fall through and return the original HV error
				case rerr != nil:
					return resp, rerr
				}
			}
		}
		return resp, hvErr
	}
	return resp, apiErr
}

// classifyErrorBody decodes a non-2xx Proton response body into either
// a *HumanVerificationError (Code 9001) or a generic *APIError.
func classifyErrorBody(status int, body []byte) (*HumanVerificationError, *APIError) {
	var env struct {
		Code    int
		Error   string
		Details *struct {
			HumanVerificationToken   string
			HumanVerificationMethods []string
			WebUrl                   string
		}
	}
	_ = json.Unmarshal(body, &env)
	if env.Code == 9001 && env.Details != nil {
		return &HumanVerificationError{
			Token:   env.Details.HumanVerificationToken,
			Methods: env.Details.HumanVerificationMethods,
			WebURL:  env.Details.WebUrl,
		}, nil
	}
	msg := env.Error
	if msg == "" {
		msg = http.StatusText(status)
	}
	return nil, &APIError{HTTPStatus: status, Code: env.Code, Message: msg, RawBody: body}
}

// Send is Do + JSON unmarshal into out (out may be nil for discard).
// 2xx with a Proton Code that isn't 1000 (OK) or 1001 (multi-response OK)
// is treated as an API error.
func (c *Client) Send(ctx context.Context, req Request, out any) error {
	resp, err := c.Do(ctx, req)
	if err != nil {
		return err
	}
	var env struct {
		Code  int
		Error string
	}
	if json.Unmarshal(resp.Body, &env) == nil && env.Code != 0 && env.Code != 1000 && env.Code != 1001 {
		return &APIError{HTTPStatus: resp.Status, Code: env.Code, Message: env.Error, RawBody: resp.Body}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(resp.Body, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) doOnce(ctx context.Context, req Request) (*Response, error) {
	c.mu.RLock()
	uid, acc := c.uid, c.acc
	c.mu.RUnlock()

	u := c.base + req.Path
	if len(req.Query) > 0 {
		u += "?" + req.Query.Encode()
	}

	var body io.Reader
	switch b := req.Body.(type) {
	case nil:
	case []byte:
		body = bytes.NewReader(b)
	case string:
		if b != "" {
			body = strings.NewReader(b)
		}
	case io.Reader:
		body = b
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		body = bytes.NewReader(raw)
	}

	r, err := http.NewRequestWithContext(ctx, strings.ToUpper(req.Method), u, body)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("x-pm-appversion", c.app)
	if uid != "" {
		r.Header.Set("x-pm-uid", uid)
	}
	if acc != "" {
		r.Header.Set("Authorization", "Bearer "+acc)
	}
	if req.HVToken != "" && req.HVType != "" {
		r.Header.Set("x-pm-human-verification-token", req.HVToken)
		r.Header.Set("x-pm-human-verification-token-type", req.HVType)
	}

	start := time.Now()
	resp, err := c.hc.Do(r)
	if err != nil {
		c.log.Debug("api request failed",
			"method", req.Method, "path", req.Path,
			"err", err,
			"duration_ms", time.Since(start).Milliseconds())
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	c.log.Debug("api request",
		"method", req.Method, "path", req.Path,
		"status", resp.StatusCode,
		"bytes", len(buf),
		"duration_ms", time.Since(start).Milliseconds())
	return &Response{Status: resp.StatusCode, Body: buf}, nil
}

func (c *Client) refreshAuth(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	reqBody, _ := json.Marshal(map[string]string{
		"UID":          c.uid,
		"RefreshToken": c.ref,
		"ResponseType": "token",
		"GrantType":    "refresh_token",
		"RedirectURI":  "https://protonmail.ch",
		"State":        fmt.Sprintf("%d", time.Now().UnixNano()),
	})

	r, err := http.NewRequestWithContext(ctx, "POST", c.base+"/auth/v4/refresh", bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("x-pm-uid", c.uid)
	r.Header.Set("x-pm-appversion", c.app)

	resp, err := c.hc.Do(r)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("refresh returned %d: %s", resp.StatusCode, string(b))
	}
	var result struct {
		AccessToken  string
		RefreshToken string
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	c.acc = result.AccessToken
	c.ref = result.RefreshToken
	return nil
}
