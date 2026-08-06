// Package hv handles Proton's Human Verification (HV) flow from the
// CLI side. It exposes a single entry point - Resolve - that:
//
//  1. Extracts the embedded proton-cli-hv helper binary to the user
//     cache directory (once, then reuses it).
//  2. Exec's the helper with the CAPTCHA page URL as argv[1].
//  3. Translates the helper's exit code + stderr into typed errors.
//
// The main binary stays CGO-free and statically linked. The helper
// links against libwebkit2gtk-4.x and libgtk-3 dynamically, so its
// exec failure on minimal Linux systems is reported back as a clean
// *UnavailableError that the cli layer can format for the user.
//
// Contract with cmd/proton-cli-hv (helper exit codes):
//
//	0   success; stdout = the solved token, verbatim
//	2   bad usage
//	3   webview unavailable (no display, no webkit, init failed, ...)
//	4   user cancelled (window closed before solving)
//	5   network/server error
//	126/127 dynamic-linker failures (libwebkit2gtk missing on Linux);
//	         emitted by glibc, treated identically to 3
//
// Stderr first line is always "hv: <category>: <detail>".
package hv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/roman-16/proton-cli/internal/hv/hvexit"
)

// Helper exit codes. Sourced from internal/hv/hvexit so the helper binary and
// this interpreter share one definition.
const (
	exitSuccess     = hvexit.Success
	exitUsage       = hvexit.Usage
	exitUnavailable = hvexit.Unavailable
	exitCancelled   = hvexit.Cancelled
	exitNetwork     = hvexit.Network
)

// UnavailableError signals the helper could not run on this machine
// (no GUI, no webkit, init failed, missing libs). The cli layer
// converts this into a user-facing message that suggests workarounds
// (Proton web/mobile login, install libwebkit2gtk, etc.).
type UnavailableError struct {
	Detail string
}

func (e *UnavailableError) Error() string {
	if e.Detail == "" {
		return "human verification cannot run on this machine"
	}
	return "human verification cannot run on this machine: " + e.Detail
}

// CancelledError signals the user closed the window without solving
// the CAPTCHA, or the helper hit its capture timeout.
type CancelledError struct {
	Detail string
}

func (e *CancelledError) Error() string {
	if e.Detail == "" {
		return "human verification was cancelled"
	}
	return "human verification was cancelled: " + e.Detail
}

// ErrHelperMissing signals the helper binary could not be located or
// extracted. This is a packaging bug (the embed didn't include a real
// binary for this OS/arch) and never normal operation.
var ErrHelperMissing = errors.New("hv helper binary not embedded for this OS/arch")

// Resolve runs the embedded helper against a CAPTCHA page URL, as built by
// proton.Client.CaptchaURL, and returns the solved token verbatim, ready for the
// x-pm-human-verification-token header.
//
// Which URL to open is the caller's decision: it depends on the API being talked
// to, and getting it wrong is invisible from here. The helper only opens what it
// is given and waits for the page to report a result.
//
// Returns *UnavailableError if the helper signals or appears to signal
// "can't run a webview here". Returns *CancelledError if the user
// cancelled. Returns plain error for other failures.
func Resolve(ctx context.Context, captchaURL string) (string, error) {
	if captchaURL == "" {
		return "", errors.New("hv: empty captcha url")
	}
	bin, err := extractHelper()
	if err != nil {
		return "", err
	}
	return resolveWithBinary(ctx, bin, captchaURL)
}

// resolveWithBinary is the post-extract translation: exec the helper
// at binPath and map its exit code + stderr into typed errors per the
// contract documented at the top of this file. Exposed (un-exported)
// as a seam for tests; production callers go through Resolve.
func resolveWithBinary(ctx context.Context, binPath, captchaURL string) (string, error) {
	cmd := exec.CommandContext(ctx, binPath, captchaURL)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	code := 0
	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
		code = exitSuccess
	case errors.As(runErr, &exitErr):
		code = exitErr.ExitCode()
	default:
		// Process failed to start (e.g. ENOENT, EACCES). Treat as
		// unavailable; the embedded helper exists on disk so this is
		// almost certainly an environment problem.
		return "", &UnavailableError{Detail: runErr.Error()}
	}

	detail := firstLine(stderr.String())
	switch code {
	case exitSuccess:
		token := strings.TrimSpace(stdout.String())
		if token == "" {
			return "", &UnavailableError{Detail: "helper exited 0 but printed no token"}
		}
		return token, nil
	case exitUnavailable, 126, 127:
		return "", &UnavailableError{Detail: detail}
	case exitCancelled:
		return "", &CancelledError{Detail: detail}
	case exitNetwork:
		return "", fmt.Errorf("hv: server error: %s", detail)
	case exitUsage:
		// Programming bug on our side - we passed the helper bad args.
		return "", fmt.Errorf("hv: helper rejected arguments: %s", detail)
	default:
		return "", fmt.Errorf("hv: helper exit %d: %s", code, detail)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
