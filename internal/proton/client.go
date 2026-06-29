// Package proton is the low-level Proton HTTP client: request building,
// response decoding, typed errors, auth, token refresh, rate-limit and
// human-verification retries. No domain logic lives here.
package proton

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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/roman-16/proton-cli/internal/session"
)

const (
	DefaultBaseURL    = "https://mail.proton.me/api"
	DefaultAppVersion = "Other"

	// maxRateLimitWait caps how long a single 429 retry will sleep before
	// giving up and surfacing the error.
	maxRateLimitWait = 30 * time.Second
)

// Doer is the seam domain services depend on. *Client satisfies it; tests
// supply fakes.
type Doer interface {
	Do(ctx context.Context, r Request) (*Response, error)
	Decode(ctx context.Context, r Request, out any) error
}

type Client struct {
	hc   *http.Client
	base string
	app  string
	log  *slog.Logger

	mu            sync.RWMutex
	uid           string
	acc           string
	ref           string
	saltedKeyPass string // in-memory cleartext only; never persisted as cleartext
	encKeyBlob    string // persisted (salted key password encrypted with the server-held client key)
	profile       string
	hvResolver    HVResolver
}

type Options struct {
	BaseURL    string
	AppVersion string
	Profile    string
	HTTPClient *http.Client
	Logger     *slog.Logger
}

// New fills empty Options fields with defaults.
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

func (c *Client) Profile() string { return c.profile }

func (c *Client) SetTokens(uid, acc, ref string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.uid, c.acc, c.ref = uid, acc, ref
}

func (c *Client) SaltedKeyPass() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.saltedKeyPass
}

func (c *Client) SetSaltedKeyPass(skp string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.saltedKeyPass = skp
}

func (c *Client) EncKeyBlob() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.encKeyBlob
}

func (c *Client) SetEncKeyBlob(blob string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.encKeyBlob = blob
}

func (c *Client) Session() *session.Session {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s := &session.Session{
		UID:          c.uid,
		AccessToken:  c.acc,
		RefreshToken: c.ref,
		AppVersion:   c.app,
		BaseURL:      c.base,
	}
	// Prefer the encrypted blob. Preserve a legacy cleartext key password only
	// until it has been migrated to a blob; never write new cleartext.
	switch {
	case c.encKeyBlob != "":
		s.EncKeyBlob = c.encKeyBlob
	case c.saltedKeyPass != "":
		s.SaltedKeyPass = c.saltedKeyPass
	}
	return s
}

// Request is a typed API request. Body is JSON-encoded when it is not nil and
// not already a []byte / string / io.Reader.
type Request struct {
	Method string
	Path   string
	Query  url.Values
	Body   any

	// ContentType overrides the request Content-Type. Empty means
	// application/json. Set it (e.g. multipart/form-data with a boundary) when
	// Body is a pre-encoded []byte/io.Reader.
	ContentType string

	// Human-verification state (set by retry logic, not by most callers).
	HVToken string
	HVType  string
}

type Response struct {
	Status int
	Body   []byte

	// retryHeader carries the raw Retry-After header value for 429 handling.
	retryHeader string
}

// Do sends a request and returns the response. Non-2xx responses return a
// typed error. It transparently handles 401 (refresh + retry), 429
// (Retry-After + retry) and 9001 (human verification + retry).
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

	if resp.Status == http.StatusTooManyRequests {
		if d, ok := retryAfter(resp); ok {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(d):
			}
			resp, err = c.doOnce(ctx, req)
			if err != nil {
				return nil, err
			}
		}
	}

	if resp.Status >= 200 && resp.Status < 300 {
		return resp, nil
	}

	hvErr, apiErr := classifyErrorBody(resp.Status, resp.Body)
	if hvErr != nil {
		// Avoid re-resolving if this request already carried an HV header
		// (resolver loop guard).
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

// Decode is Do + JSON unmarshal into out (out may be nil for discard). A 2xx
// with a Proton Code that isn't 1000 (OK) or 1001 (multi-response OK) is
// treated as an API error.
func (c *Client) Decode(ctx context.Context, req Request, out any) error {
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

	body, err := encodeBody(req.Body)
	if err != nil {
		return nil, err
	}

	r, err := http.NewRequestWithContext(ctx, strings.ToUpper(req.Method), u, body)
	if err != nil {
		return nil, err
	}
	if req.ContentType != "" {
		r.Header.Set("Content-Type", req.ContentType)
	} else {
		r.Header.Set("Content-Type", "application/json")
	}
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
			"method", req.Method, "path", req.Path, "err", err,
			"duration_ms", time.Since(start).Milliseconds())
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	c.log.Debug("api request",
		"method", req.Method, "path", req.Path, "status", resp.StatusCode,
		"bytes", len(buf), "duration_ms", time.Since(start).Milliseconds())
	return &Response{Status: resp.StatusCode, Body: buf, retryHeader: resp.Header.Get("Retry-After")}, nil
}

func encodeBody(b any) (io.Reader, error) {
	switch v := b.(type) {
	case nil:
		return nil, nil
	case []byte:
		return bytes.NewReader(v), nil
	case string:
		if v == "" {
			return nil, nil
		}
		return strings.NewReader(v), nil
	case io.Reader:
		return v, nil
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		return bytes.NewReader(raw), nil
	}
}

// retryAfter parses the Retry-After header (seconds form) into a bounded delay.
func retryAfter(resp *Response) (time.Duration, bool) {
	if resp.retryHeader == "" {
		return 0, false
	}
	secs, err := strconv.Atoi(strings.TrimSpace(resp.retryHeader))
	if err != nil || secs < 0 {
		return 0, false
	}
	d := time.Duration(secs) * time.Second
	if d > maxRateLimitWait {
		d = maxRateLimitWait
	}
	return d, true
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

func httpStatusText(status int) string {
	if s := http.StatusText(status); s != "" {
		return s
	}
	return fmt.Sprintf("HTTP %d", status)
}
