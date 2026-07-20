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
)

const (
	DefaultBaseURL    = "https://mail.proton.me/api"
	DefaultAppVersion = "Other"
	DefaultUserAgent  = "proton-cli/dev"

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
	ua   string
	log  *slog.Logger

	mu         sync.RWMutex
	uid        string
	acc        string
	ref        string
	encKeyBlob string // persisted (salted key password encrypted with the server-held client key)
	profile    string
	hvResolver HVResolver
	persist    func()
}

type Options struct {
	BaseURL    string
	AppVersion string
	UserAgent  string
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
	if opts.UserAgent == "" {
		opts.UserAgent = DefaultUserAgent
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Minute}
	}
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Client{hc: hc, base: opts.BaseURL, app: opts.AppVersion, ua: opts.UserAgent, profile: opts.Profile, log: log}
}

func (c *Client) Profile() string { return c.profile }

func (c *Client) SetTokens(uid, acc, ref string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.uid, c.acc, c.ref = uid, acc, ref
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

// Tokens returns the current session auth tokens.
func (c *Client) Tokens() (uid, acc, ref string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.uid, c.acc, c.ref
}

// AppVersion returns the app-version header value (a fixed connection setting).
func (c *Client) AppVersion() string { return c.app }

// BaseURL returns the API base URL (a fixed connection setting).
func (c *Client) BaseURL() string { return c.base }

// SetPersistHook installs a callback invoked whenever the client's persistable
// state changes (e.g. a token refresh). The higher layer uses it to write the
// session file, keeping this transport package free of any persistence format.
func (c *Client) SetPersistHook(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.persist = fn
}

// Persist invokes the persist hook if one is installed (best-effort).
func (c *Client) Persist() {
	c.mu.RLock()
	fn := c.persist
	c.mu.RUnlock()
	if fn != nil {
		fn()
	}
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
		c.Persist()
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

// setClientHeaders applies the identity headers every Proton request carries:
// an honest User-Agent and x-pm-appversion. Per Proton's third-party rules
// these identify the app honestly and must never impersonate an official
// client.
func (c *Client) setClientHeaders(r *http.Request) {
	r.Header.Set("User-Agent", c.ua)
	r.Header.Set("x-pm-appversion", c.app)
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
	c.setClientHeaders(r)
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
		return nil, &NetworkError{Err: err}
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
	c.setClientHeaders(r)

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
