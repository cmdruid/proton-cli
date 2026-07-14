package proton

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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
