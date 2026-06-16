package hv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeHelper builds a tiny shell-script "helper" that prints the
// requested stdout, the requested stderr, and exits with the given
// code. The hv package uses exec.Command, so anything that's an
// executable file works - we don't actually need a Go binary here.
//
// Skips on Windows (POSIX shebangs).
func fakeHelper(t *testing.T, exitCode int, stdout, stderr string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fakeHelper uses a POSIX shell; skipped on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-hv")
	script := "#!/bin/sh\n"
	if stdout != "" {
		// Use printf to avoid trailing newline ambiguity.
		script += "printf '%s\\n' " + shellQuote(stdout) + "\n"
	}
	if stderr != "" {
		script += "printf '%s\\n' " + shellQuote(stderr) + " 1>&2\n"
	}
	script += "exit " + itoa(exitCode) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake helper: %v", err)
	}
	return path
}

func shellQuote(s string) string {
	// Single-quote and escape any single-quotes inside.
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// runWith calls Resolve but with the helper path injected by replacing
// extractHelper for the duration of the test via a tiny fake. We can't
// override extractHelper directly (function value, not a variable), so
// we run the helper inline via exec.CommandContext logic mirrored here.
//
// Simpler alternative: test the translation logic by exposing a
// run-helper-then-translate seam. We do that: mirror Resolve's switch
// against a fake stdout/stderr/exitCode.
func runHelperViaExec(t *testing.T, helperPath, challenge string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return resolveWithHelper(ctx, helperPath, challenge)
}

// resolveWithHelper is what Resolve does after extractHelper succeeds.
// Exposed here as a seam so tests don't need to populate the embed.
func resolveWithHelper(ctx context.Context, helperPath, challenge string) (string, error) {
	return resolveWithBinary(ctx, helperPath, challenge)
}

func TestResolveSuccess(t *testing.T) {
	helper := fakeHelper(t, 0, "abc:def\n", "hv: captured: type=captcha\n")
	tok, err := runHelperViaExec(t, helper, "abc")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if tok != "abc:def" {
		t.Errorf("token = %q, want %q", tok, "abc:def")
	}
}

func TestResolveUnavailable(t *testing.T) {
	helper := fakeHelper(t, exitUnavailable, "", "hv: webview-unavailable: webview_new returned nil\n")
	_, err := runHelperViaExec(t, helper, "abc")
	var unavail *UnavailableError
	if !errors.As(err, &unavail) {
		t.Fatalf("err = %v, want *UnavailableError", err)
	}
	if !strings.Contains(unavail.Detail, "webview_new") {
		t.Errorf("detail = %q, want it to contain 'webview_new'", unavail.Detail)
	}
}

func TestResolveCancelled(t *testing.T) {
	helper := fakeHelper(t, exitCancelled, "", "hv: user-cancelled: window closed\n")
	_, err := runHelperViaExec(t, helper, "abc")
	var cancel *CancelledError
	if !errors.As(err, &cancel) {
		t.Fatalf("err = %v, want *CancelledError", err)
	}
}

func TestResolveLdSoFailureTreatedAsUnavailable(t *testing.T) {
	// Exit 127 with the canonical glibc dynamic-linker error.
	helper := fakeHelper(t, 127, "", "/some/binary: error while loading shared libraries: libwebkit2gtk-4.1.so.0: cannot open shared object file\n")
	_, err := runHelperViaExec(t, helper, "abc")
	var unavail *UnavailableError
	if !errors.As(err, &unavail) {
		t.Fatalf("err = %v, want *UnavailableError (exit 127 maps to unavailable)", err)
	}
	if !strings.Contains(unavail.Detail, "libwebkit2gtk") {
		t.Errorf("detail = %q, want libwebkit2gtk message preserved", unavail.Detail)
	}
}

func TestResolveExit126AlsoUnavailable(t *testing.T) {
	helper := fakeHelper(t, 126, "", "/some/binary: Permission denied\n")
	_, err := runHelperViaExec(t, helper, "abc")
	var unavail *UnavailableError
	if !errors.As(err, &unavail) {
		t.Fatalf("err = %v, want *UnavailableError", err)
	}
}

func TestResolveNetworkError(t *testing.T) {
	helper := fakeHelper(t, exitNetwork, "", "hv: network: failed to load verify.proton.me: connection refused\n")
	_, err := runHelperViaExec(t, helper, "abc")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "server error") {
		t.Errorf("err = %v, want it to mention 'server error'", err)
	}
}

func TestResolveSuccessButEmptyStdoutTreatedAsUnavailable(t *testing.T) {
	// Helper exited 0 but produced no token - defensive guard.
	helper := fakeHelper(t, 0, "", "")
	_, err := runHelperViaExec(t, helper, "abc")
	var unavail *UnavailableError
	if !errors.As(err, &unavail) {
		t.Fatalf("err = %v, want *UnavailableError when stdout empty", err)
	}
}

func TestResolveProcessFailedToStart(t *testing.T) {
	// Path that doesn't exist => exec returns non-ExitError.
	_, err := runHelperViaExec(t, "/nonexistent/path/to/hv", "abc")
	var unavail *UnavailableError
	if !errors.As(err, &unavail) {
		t.Fatalf("err = %v, want *UnavailableError on exec failure", err)
	}
}

func TestResolveEmptyChallengeRejected(t *testing.T) {
	_, err := Resolve(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty challenge, got nil")
	}
	if !strings.Contains(err.Error(), "empty challenge") {
		t.Errorf("err = %v, want 'empty challenge' message", err)
	}
}

func TestResolveStubHelperMissing(t *testing.T) {
	// In default (non-embed_hv) builds, helperBinary is empty so
	// extractHelper returns ErrHelperMissing.
	if len(helperBinary) != 0 {
		t.Skip("helper binary is embedded in this build; stub test irrelevant")
	}
	_, err := Resolve(context.Background(), "valid-challenge")
	if !errors.Is(err, ErrHelperMissing) {
		t.Errorf("err = %v, want ErrHelperMissing", err)
	}
}

func TestFirstLine(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"", ""},
		{"single", "single"},
		{"first\nsecond", "first"},
		{"trailing\n", "trailing"},
		{"  with whitespace  \nrest", "with whitespace"},
		{"\n", ""},
	} {
		got := firstLine(tc.in)
		if got != tc.want {
			t.Errorf("firstLine(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
