package proton

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const hvProtonBody = `{"Code":9001,"Error":"Human verification required","Details":{"HumanVerificationToken":"chal-xyz","HumanVerificationMethods":["captcha"],"WebUrl":"https://verify.proton.me/?methods=captcha&token=chal-xyz"}}`

// newHVTestServer returns a server that 422+9001s the first request (no HV
// header) and 200s the second (HV header present).
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
	if seenHV == nil || seenHV.Token != "chal-xyz" {
		t.Fatalf("resolver received %+v", seenHV)
	}
	if len(*received) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(*received))
	}
	if (*received)[0].Get("x-pm-human-verification-token") != "" {
		t.Error("first request should NOT carry HV header")
	}
	if (*received)[1].Get("x-pm-human-verification-token") != "solved-token" {
		t.Errorf("retry HV token = %q, want solved-token", (*received)[1].Get("x-pm-human-verification-token"))
	}
	if (*received)[1].Get("x-pm-human-verification-token-type") != "captcha" {
		t.Errorf("retry HV type = %q, want captcha", (*received)[1].Get("x-pm-human-verification-token-type"))
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
		t.Fatalf("err = %v, want *HumanVerificationError", err)
	}
	if len(*received) != 1 {
		t.Errorf("expected 1 request, got %d", len(*received))
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
		t.Fatalf("err = %v, want resolverErr", err)
	}
}

func TestDoResolver_NoLoopOnSecondHV(t *testing.T) {
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
		t.Errorf("resolver called %d times, want 1", resolverCalls)
	}
	if calls != 2 {
		t.Errorf("server saw %d requests, want 2", calls)
	}
	// The two failures want different words. Somebody who has just solved a
	// CAPTCHA has to be told it was not accepted, not sent to solve another - and
	// the challenge they answered is spent either way.
	if !hv.Refused {
		t.Error("a challenge raised against a verified request is a refusal, not a fresh ask")
	}
}

func TestDo_FirstChallengeIsNotARefusal(t *testing.T) {
	srv, _ := newHVTestServer(t)
	c := New(Options{BaseURL: srv.URL})
	_, err := c.Do(context.Background(), Request{Method: "POST", Path: "/test"})
	var hv *HumanVerificationError
	if !errors.As(err, &hv) {
		t.Fatalf("err = %v, want *HumanVerificationError", err)
	}
	if hv.Refused {
		t.Error("nothing was verified, so nothing was refused")
	}
}

func TestDoResolver_NotCalledFor5xx(t *testing.T) {
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
		t.Error("resolver should not be called on non-9001 errors")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("err = %v, want *APIError with HTTP 500", err)
	}
}

func TestClassifyErrorBody_NonHV(t *testing.T) {
	hv, apiErr := classifyErrorBody(http.StatusForbidden, []byte(`{"Code":2050,"Error":"forbidden"}`))
	if hv != nil {
		t.Error("hv should be nil for non-9001")
	}
	if apiErr == nil || apiErr.Code != 2050 {
		t.Errorf("apiErr = %+v", apiErr)
	}
}

func TestClassifyErrorBody_HV(t *testing.T) {
	hv, apiErr := classifyErrorBody(http.StatusUnprocessableEntity, []byte(hvProtonBody))
	if hv == nil {
		t.Fatal("expected hv error, got nil")
		return
	}
	if apiErr != nil {
		t.Error("apiErr should be nil when hv is non-nil")
	}
	if !strings.Contains(hv.WebURL, "verify.proton.me") {
		t.Errorf("hv.WebURL = %q", hv.WebURL)
	}
}
