package proton

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/roman-16/proton-cli/internal/errs"
)

func TestNetworkErrorExitCode(t *testing.T) {
	inner := errors.New("dial tcp: connection refused")
	var ne error = &NetworkError{Err: inner}

	// Carries exit code 5 via the errs.ExitCoder interface (how the cli maps it).
	var coder errs.ExitCoder
	if !errors.As(ne, &coder) {
		t.Fatalf("NetworkError should satisfy errs.ExitCoder")
	}
	if coder.ExitCode() != 5 {
		t.Errorf("NetworkError exit = %d, want 5", coder.ExitCode())
	}
	// Unwraps to the transport cause.
	if !errors.Is(ne, inner) {
		t.Error("NetworkError should unwrap to its cause")
	}
}

// TestDoTransportFailureIsNetworkError proves the wiring end to end: a request
// that never reaches an HTTP response (here, a closed server = connection
// refused) surfaces from Client.Do as *NetworkError, so the cli maps it to 5.
func TestDoTransportFailureIsNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // connections to url are now refused

	c := New(Options{BaseURL: url})
	_, err := c.Do(context.Background(), Request{Method: "GET", Path: "/tests/ping"})
	var ne *NetworkError
	if !errors.As(err, &ne) {
		t.Fatalf("err = %v, want *NetworkError", err)
	}
	if ne.ExitCode() != 5 {
		t.Errorf("exit = %d, want 5", ne.ExitCode())
	}
}
