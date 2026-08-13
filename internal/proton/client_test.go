package proton

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// newHeaderCaptureServer records the headers of every request and replies with
// a minimal Proton-OK body.
func newHeaderCaptureServer(t *testing.T) (*httptest.Server, *[]http.Header) {
	t.Helper()
	var received []http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = append(received, r.Header.Clone())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"Code":1000}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &received
}

func TestDoSendsHonestIdentityHeaders(t *testing.T) {
	srv, received := newHeaderCaptureServer(t)
	c := New(Options{BaseURL: srv.URL})
	if _, err := c.Do(context.Background(), Request{Method: "GET", Path: "/test"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(*received) != 1 {
		t.Fatalf("expected 1 request, got %d", len(*received))
	}
	h := (*received)[0]
	if got := h.Get("User-Agent"); got != DefaultUserAgent {
		t.Errorf("User-Agent = %q, want %q", got, DefaultUserAgent)
	}
	if got := h.Get("x-pm-appversion"); got != DefaultAppVersion {
		t.Errorf("x-pm-appversion = %q, want %q", got, DefaultAppVersion)
	}
}

func TestDoIdentityHeadersHonorOverrides(t *testing.T) {
	srv, received := newHeaderCaptureServer(t)
	c := New(Options{
		BaseURL:    srv.URL,
		AppVersion: "external-proton_cli@9.9.9-beta",
		UserAgent:  "proton-cli/9.9.9",
	})
	if _, err := c.Do(context.Background(), Request{Method: "GET", Path: "/test"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(*received) != 1 {
		t.Fatalf("expected 1 request, got %d", len(*received))
	}
	h := (*received)[0]
	if got := h.Get("User-Agent"); got != "proton-cli/9.9.9" {
		t.Errorf("User-Agent = %q, want proton-cli/9.9.9", got)
	}
	if got := h.Get("x-pm-appversion"); got != "external-proton_cli@9.9.9-beta" {
		t.Errorf("x-pm-appversion = %q, want external-proton_cli@9.9.9-beta", got)
	}
}

func TestRateLimitDelayPrefersTheServersAnswer(t *testing.T) {
	got := rateLimitDelay(&Response{retryHeader: "3"}, 1)
	if got != 3*time.Second {
		t.Errorf("delay = %v, want the Retry-After the server named", got)
	}
}

func TestRateLimitDelayBacksOffWithJitterWhenTheServerNamesNothing(t *testing.T) {
	var last time.Duration
	for attempt := 1; attempt <= 6; attempt++ {
		floor := min(backoffFloor<<(attempt-1), backoffCeiling)
		for range 20 {
			got := rateLimitDelay(&Response{}, attempt)
			if got < floor/2 || got > floor {
				t.Fatalf("attempt %d delay = %v, want between %v and %v", attempt, got, floor/2, floor)
			}
		}
		last = floor
	}
	if last != backoffCeiling {
		t.Errorf("the wait reached %v, want it capped at %v", last, backoffCeiling)
	}
}

func TestRateLimitedRequestIsWaitedOutRatherThanFailed(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"Code":1000}`))
	}))
	defer srv.Close()

	c := New(Options{BaseURL: srv.URL, Logger: slog.New(slog.DiscardHandler)})
	resp, err := c.Do(context.Background(), Request{Method: "GET", Path: "/x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Errorf("status = %d, want the answer after the wait", resp.Status)
	}
	if calls != 2 {
		t.Errorf("the server was asked %d times, want a second attempt", calls)
	}
}

func TestNoMoreThanMaxInFlightRequestsAtOnce(t *testing.T) {
	var mu sync.Mutex
	var inFlight, peak int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		inFlight++
		if inFlight > peak {
			peak = inFlight
		}
		mu.Unlock()
		time.Sleep(30 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
		_, _ = w.Write([]byte(`{"Code":1000}`))
	}))
	defer srv.Close()

	c := New(Options{BaseURL: srv.URL, Logger: slog.New(slog.DiscardHandler)})
	var wg sync.WaitGroup
	for range maxInFlight * 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Do(context.Background(), Request{Method: "GET", Path: "/x"})
		}()
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if peak > maxInFlight {
		t.Errorf("%d requests were in flight at once, want at most %d", peak, maxInFlight)
	}
	if peak < 2 {
		t.Errorf("peak was %d; the requests did not overlap at all, so this proves nothing", peak)
	}
}

func TestUnauthorizedAdoptsASessionRefreshedElsewhere(t *testing.T) {
	var asked int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		if r.Header.Get("Authorization") != "Bearer fresh" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"Code":1000}`))
	}))
	defer srv.Close()

	c := New(Options{BaseURL: srv.URL, Logger: slog.New(slog.DiscardHandler)})
	c.SetTokens("uid", "stale", "stale-refresh")
	c.SetReloadHook(func() (string, string, bool) { return "fresh", "fresh-refresh", true })

	resp, err := c.Do(context.Background(), Request{Method: "GET", Path: "/x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Errorf("status = %d, want the request retried with the stored tokens", resp.Status)
	}
	if asked != 2 {
		t.Errorf("the server was asked %d times, want exactly one retry", asked)
	}
	if _, acc, ref := c.Tokens(); acc != "fresh" || ref != "fresh-refresh" {
		t.Errorf("tokens = %q/%q, want the stored pair adopted", acc, ref)
	}
}

// Concurrent requests discover an expired session at the same moment. A refresh
// token is single-use, so the second and third refresh would spend one the first
// had already replaced: the web clients guard this with a once-handler, and so
// does this.
func TestConcurrentUnauthorizedRequestsRefreshOnce(t *testing.T) {
	var mu sync.Mutex
	refreshes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/v4/refresh" {
			mu.Lock()
			refreshes++
			mu.Unlock()
			time.Sleep(20 * time.Millisecond)
			_, _ = w.Write([]byte(`{"AccessToken":"fresh","RefreshToken":"fresh-refresh"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer fresh" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"Code":1000}`))
	}))
	defer srv.Close()

	c := New(Options{BaseURL: srv.URL, Logger: slog.New(slog.DiscardHandler)})
	c.SetTokens("uid", "stale", "stale-refresh")

	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := c.Do(context.Background(), Request{Method: "GET", Path: "/x"})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if resp.Status != http.StatusOK {
				t.Errorf("status = %d, want the request to succeed after one refresh", resp.Status)
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if refreshes != 1 {
		t.Errorf("the session was refreshed %d times, want once", refreshes)
	}
}

// A rate-limited refresh means the server is busy, not that the session is gone.
func TestRateLimitedRefreshIsWaitedOutRatherThanReportedAsSignedOut(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/v4/refresh" {
			attempts++
			if attempts == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			_, _ = w.Write([]byte(`{"AccessToken":"fresh","RefreshToken":"fresh-refresh"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer fresh" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"Code":1000}`))
	}))
	defer srv.Close()

	c := New(Options{BaseURL: srv.URL, Logger: slog.New(slog.DiscardHandler)})
	c.SetTokens("uid", "stale", "stale-refresh")

	resp, err := c.Do(context.Background(), Request{Method: "GET", Path: "/x"})
	if err != nil {
		t.Fatalf("err = %v, want the refresh retried rather than the session declared dead", err)
	}
	if resp.Status != http.StatusOK {
		t.Errorf("status = %d, want success", resp.Status)
	}
	if attempts != 2 {
		t.Errorf("the refresh was attempted %d times, want a second try", attempts)
	}
}

// Several commands can share one session, so by the time this one has taken up
// the tokens another process wrote, a third may have replaced them again. One
// renewal is not always enough.
func TestASessionReplacedTwiceUnderARequestStillSucceeds(t *testing.T) {
	stored := []string{"second", "third"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer third" {
			_, _ = w.Write([]byte(`{"Code":1000}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New(Options{BaseURL: srv.URL, Logger: slog.New(slog.DiscardHandler)})
	c.SetTokens("uid", "first", "first-refresh")
	// Each look finds what somebody else has since written.
	c.SetReloadHook(func() (string, string, bool) {
		if len(stored) == 0 {
			return "", "", false
		}
		next := stored[0]
		stored = stored[1:]
		return next, next + "-refresh", true
	})

	resp, err := c.Do(context.Background(), Request{Method: "GET", Path: "/x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Errorf("status = %d, want the request to succeed once the session settled", resp.Status)
	}
}

// A session that is genuinely gone is reported rather than renewed forever.
func TestAnUnrecoverableSessionIsReportedRatherThanRetriedForever(t *testing.T) {
	var asked int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/v4/refresh" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		asked++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New(Options{BaseURL: srv.URL, Logger: slog.New(slog.DiscardHandler)})
	c.SetTokens("uid", "stale", "stale-refresh")
	if _, err := c.Do(context.Background(), Request{Method: "GET", Path: "/x"}); err == nil {
		t.Fatal("a dead session should be reported")
	}
	if asked > sessionRenewals+1 {
		t.Errorf("the request was made %d times, want at most %d", asked, sessionRenewals+1)
	}
}

// Proton answers some refusals with 401 and a code of its own - a feature the
// plan does not include, for one. Refreshing the session cannot change that
// answer, and doing so spends a refresh token and loses Proton's reason.
func TestA401WithProtonsOwnCodeIsAnAnswerRatherThanAStaleSession(t *testing.T) {
	var refreshes, asked int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/v4/refresh" {
			refreshes++
			_, _ = w.Write([]byte(`{"AccessToken":"fresh","RefreshToken":"fresh-refresh"}`))
			return
		}
		asked++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"Code":2027,"Error":"Cannot edit contact groups"}`))
	}))
	defer srv.Close()

	c := New(Options{BaseURL: srv.URL, Logger: slog.New(slog.DiscardHandler)})
	c.SetTokens("uid", "good", "good-refresh")

	_, err := c.Do(context.Background(), Request{Method: "POST", Path: "/core/v4/labels"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want Proton's own refusal", err)
	}
	if apiErr.Code != 2027 {
		t.Errorf("code = %d, want 2027 kept intact", apiErr.Code)
	}
	if refreshes != 0 {
		t.Errorf("the session was refreshed %d times for an answer a refresh cannot change", refreshes)
	}
	if asked != 1 {
		t.Errorf("the request was made %d times, want once", asked)
	}
}
