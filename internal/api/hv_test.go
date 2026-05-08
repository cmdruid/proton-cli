package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// hvProtonBody is the canonical 9001 response body Proton sends.
const hvProtonBody = `{"Code":9001,"Error":"Human verification required","Details":{"HumanVerificationToken":"chal-xyz","HumanVerificationMethods":["captcha"],"WebUrl":"https://verify.proton.me/?methods=captcha&token=chal-xyz"}}`

// newHVTestServer returns a server that:
//  1. Returns 422 + 9001 on the first request (no HV header).
//  2. Returns 200 + {"Code":1000} on the second (HV header present).
//
// Captures all received requests in `received` for assertions.
func newHVTestServer(t *testing.T) (*httptest.Server, *[]http.Header) {
	t.Helper()
	var received []http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = append(received, r.Header.Clone())
		_, _ = io.Copy(io.Discard, r.Body)

		if r.Header.Get("x-pm-human-verification-token") == "" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(hvProtonBody))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"Code":1000}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &received
}

func TestDoNoResolver_SurfacesHVError(t *testing.T) {
	srv, _ := newHVTestServer(t)
	c := New(Options{BaseURL: srv.URL})
	_, err := c.Do(context.Background(), Request{Method: "POST", Path: "/test"})
	var hv *HumanVerificationError
	if !errors.As(err, &hv) {
		t.Fatalf("err = %v, want *HumanVerificationError", err)
	}
	if hv.Token != "chal-xyz" {
		t.Errorf("hv.Token = %q, want chal-xyz", hv.Token)
	}
	if len(hv.Methods) != 1 || hv.Methods[0] != "captcha" {
		t.Errorf("hv.Methods = %v, want [captcha]", hv.Methods)
	}
}

func TestDoResolverSuccess_RetriesWithHVHeaders(t *testing.T) {
	srv, received := newHVTestServer(t)
	c := New(Options{BaseURL: srv.URL})

	var seenHV *HumanVerificationError
	c.SetHVResolver(func(hv *HumanVerificationError) (string, string, error) {
		seenHV = hv
		return "solved-token", "captcha", nil
	})

	resp, err := c.Do(context.Background(), Request{Method: "POST", Path: "/test"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Errorf("resp.Status = %d, want 200", resp.Status)
	}
	if seenHV == nil {
		t.Fatal("resolver was not called")
	}
	if seenHV.Token != "chal-xyz" {
		t.Errorf("resolver received hv.Token = %q, want chal-xyz", seenHV.Token)
	}

	if len(*received) != 2 {
		t.Fatalf("expected 2 requests (initial + retry), got %d", len(*received))
	}
	first := (*received)[0]
	retry := (*received)[1]
	if first.Get("x-pm-human-verification-token") != "" {
		t.Errorf("first request should NOT carry HV header")
	}
	if retry.Get("x-pm-human-verification-token") != "solved-token" {
		t.Errorf("retry HV token = %q, want solved-token", retry.Get("x-pm-human-verification-token"))
	}
	if retry.Get("x-pm-human-verification-token-type") != "captcha" {
		t.Errorf("retry HV type = %q, want captcha", retry.Get("x-pm-human-verification-token-type"))
	}
}

func TestDoResolverUnavailable_SurfacesOriginalHV(t *testing.T) {
	srv, received := newHVTestServer(t)
	c := New(Options{BaseURL: srv.URL})
	c.SetHVResolver(func(hv *HumanVerificationError) (string, string, error) {
		return "", "", ErrHVUnavailable
	})
	_, err := c.Do(context.Background(), Request{Method: "POST", Path: "/test"})
	var hv *HumanVerificationError
	if !errors.As(err, &hv) {
		t.Fatalf("err = %v, want *HumanVerificationError when resolver is unavailable", err)
	}
	if len(*received) != 1 {
		t.Errorf("expected 1 request when resolver unavailable, got %d", len(*received))
	}
}

func TestDoResolverError_PropagatesError(t *testing.T) {
	srv, _ := newHVTestServer(t)
	c := New(Options{BaseURL: srv.URL})
	resolverErr := errors.New("user pressed Ctrl-C")
	c.SetHVResolver(func(hv *HumanVerificationError) (string, string, error) {
		return "", "", resolverErr
	})
	_, err := c.Do(context.Background(), Request{Method: "POST", Path: "/test"})
	if !errors.Is(err, resolverErr) {
		t.Fatalf("err = %v, want resolverErr to be wrapped", err)
	}
}

func TestDoResolver_NoLoopOnSecondHV(t *testing.T) {
	// Server that ALWAYS returns HV — even with HV headers set.
	// Verifies the loop guard: we don't call the resolver twice.
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(hvProtonBody))
	}))
	defer srv.Close()
	c := New(Options{BaseURL: srv.URL})

	resolverCalls := 0
	c.SetHVResolver(func(hv *HumanVerificationError) (string, string, error) {
		resolverCalls++
		return "solved", "captcha", nil
	})
	_, err := c.Do(context.Background(), Request{Method: "POST", Path: "/test"})
	var hv *HumanVerificationError
	if !errors.As(err, &hv) {
		t.Fatalf("err = %v, want *HumanVerificationError", err)
	}
	if resolverCalls != 1 {
		t.Errorf("resolver called %d times, want 1 (loop guard)", resolverCalls)
	}
	// Initial + one retry = 2 server requests, no third.
	if calls != 2 {
		t.Errorf("server saw %d requests, want 2", calls)
	}
}

func TestDoResolver_NotCalledFor5xx(t *testing.T) {
	// Resolver should only fire on 9001. Other errors pass through.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"Code":2000,"Error":"server boom"}`))
	}))
	defer srv.Close()
	c := New(Options{BaseURL: srv.URL})

	called := false
	c.SetHVResolver(func(hv *HumanVerificationError) (string, string, error) {
		called = true
		return "", "", nil
	})
	_, err := c.Do(context.Background(), Request{Method: "POST", Path: "/test"})
	if called {
		t.Errorf("resolver should not be called on non-9001 errors")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("err = %v, want *APIError with HTTP 500", err)
	}
}

func TestClassifyErrorBody_NonHV(t *testing.T) {
	// Direct test of classify helper for safety.
	body := []byte(`{"Code":2050,"Error":"forbidden"}`)
	hv, apiErr := classifyErrorBody(http.StatusForbidden, body)
	if hv != nil {
		t.Errorf("hv should be nil for non-9001")
	}
	if apiErr == nil || apiErr.Code != 2050 {
		t.Errorf("apiErr = %+v", apiErr)
	}
}

func TestClassifyErrorBody_HV(t *testing.T) {
	hv, apiErr := classifyErrorBody(http.StatusUnprocessableEntity, []byte(hvProtonBody))
	if hv == nil {
		t.Fatal("expected hv error, got nil")
	}
	if apiErr != nil {
		t.Errorf("apiErr should be nil when hv is non-nil")
	}
	if !strings.Contains(hv.WebURL, "verify.proton.me") {
		t.Errorf("hv.WebURL = %q", hv.WebURL)
	}
}

// Helper to avoid `unused import` if the test file shrinks later.
var _ = bytes.NewReader
