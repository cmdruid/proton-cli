//go:build webview

// proton-cli-hv is the human-verification helper binary for proton-cli.
//
// The main proton-cli binary is CGO-free and statically linked. When a
// command hits Proton's CAPTCHA-only HV state (API code 9001), the main
// binary extracts this helper from an embedded resource, exec's it with
// the challenge token, and reads the captured solution from stdout.
//
// This helper is the SOURCE OF TRUTH for whether HV-via-webview can run
// on the machine. The main binary does not probe the environment; it
// just reads our exit code + stderr.
//
// Contract:
//
//	proton-cli-hv <challenge-token>
//
//	Exit codes
//	  0   success; stdout = "<challenge>:<recaptcha_response>"
//	  2   bad usage (wrong/missing argv)
//	  3   webview unavailable on this system
//	  4   user cancelled (window closed before solving)
//	  5   network/server error reaching verify.proton.me
//	  126/127 dynamic-linker errors (libwebkit2gtk missing); emitted by
//	          glibc, not us — main binary handles same as 3
//
//	Stderr (always emits one machine-parseable first line):
//	  hv: webview-unavailable: <detail>
//	  hv: user-cancelled: <detail>
//	  hv: network: <detail>
//	  hv: timeout: <detail>
//	  hv: usage: <detail>
//
// Built only with `-tags webview` (CGO + webkit2gtk).
package main

import (
	"fmt"
	"net/url"
	"os"
	"runtime"
	"time"

	webview "github.com/webview/webview_go"

	"github.com/roman-16/proton-cli/internal/hv/hvexit"
)

const (
	verifyOrigin = "https://verify.proton.me"
	verifyPath   = "/"

	// Window times out after this long with no postMessage. Keeps
	// helper from hanging forever in agent-driven contexts.
	captureTimeout = 5 * time.Minute

	// Init script: runs at the start of every page load. Listens for
	// the HUMAN_VERIFICATION_SUCCESS postMessage that Verify.tsx
	// broadcasts after the user solves the CAPTCHA. broadcast.ts uses
	// `window.parent.postMessage(serialized, '*')`; for a top-level
	// page in a webview, window.parent === window.self, so the message
	// is dispatched on this same window where our listener is registered.
	//
	// Payload shape (from packages/components/.../HumanVerificationModal.tsx
	// and applications/verify/src/app/broadcast.ts):
	//
	//   JSON.stringify({
	//     type: 'HUMAN_VERIFICATION_SUCCESS',
	//     payload: { token: '<challenge>:<recaptcha_response>', type: 'captcha' }
	//   })
	initScript = `
(() => {
  window.addEventListener('message', (e) => {
    let d = e.data;
    if (typeof d === 'string') { try { d = JSON.parse(d); } catch (_) { return; } }
    if (!d || d.type !== 'HUMAN_VERIFICATION_SUCCESS') return;
    if (!d.payload || !d.payload.token) return;
    try { window.captureToken(d.payload.token, d.payload.type || 'captcha'); }
    catch (err) { console.error('captureToken bind failed:', err); }
  }, false);
})();
`
)

// Exit codes match the contract documented above. Sourced from
// internal/hv/hvexit so the helper and its parent interpreter share one
// definition.
const (
	exitSuccess     = hvexit.Success
	exitUsage       = hvexit.Usage
	exitUnavailable = hvexit.Unavailable
	exitCancelled   = hvexit.Cancelled
	exitNetwork     = hvexit.Network
)

// fail emits a one-line machine-parseable diagnostic to stderr and exits
// with the given code.
func fail(code int, category, detail string) {
	fmt.Fprintf(os.Stderr, "hv: %s: %s\n", category, detail)
	os.Exit(code)
}

func main() {
	if len(os.Args) != 2 {
		fail(exitUsage, "usage", "expected exactly one argument: <challenge-token>")
	}
	challenge := os.Args[1]
	if challenge == "" {
		fail(exitUsage, "usage", "challenge token is empty")
	}

	// Linux-specific guard: bail before webkit even tries to init,
	// with a more specific stderr line than webview's own. Wayland and
	// X11 are the only supported display servers; if neither variable
	// is set we definitely can't open a window.
	if runtime.GOOS == "linux" {
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			fail(exitUnavailable, "webview-unavailable",
				"$DISPLAY and $WAYLAND_DISPLAY both empty (no display server)")
		}
		setLinuxBackendShims()
	}

	w := webview.New(false)
	if w == nil {
		fail(exitUnavailable, "webview-unavailable", "webview_new returned nil")
	}
	defer w.Destroy()

	w.SetTitle("proton-cli — Human Verification")
	w.SetSize(540, 720, webview.HintNone)

	var (
		captured     string
		capturedType string
	)

	// Bind the Go-side capture callback BEFORE Init() so the bound
	// `window.captureToken` exists by the time the init script runs.
	if err := w.Bind("captureToken", func(token, kind string) {
		captured = token
		capturedType = kind
		w.Dispatch(func() { w.Terminate() })
	}); err != nil {
		fail(exitUnavailable, "webview-unavailable", "Bind(captureToken) failed: "+err.Error())
	}
	w.Init(initScript)

	// 5-minute capture timeout. Posts Terminate() on the main thread
	// via Dispatch so it runs in webview's event loop.
	timeoutDone := make(chan struct{})
	go func() {
		select {
		case <-time.After(captureTimeout):
			w.Dispatch(func() { w.Terminate() })
		case <-timeoutDone:
		}
	}()

	// Build URL: https://verify.proton.me/?methods=captcha&token=<challenge>
	u, err := url.Parse(verifyOrigin + verifyPath)
	if err != nil {
		fail(exitNetwork, "network", "URL parse failed: "+err.Error())
	}
	q := u.Query()
	q.Set("methods", "captcha")
	q.Set("token", challenge)
	u.RawQuery = q.Encode()

	w.Navigate(u.String())
	w.Run()
	close(timeoutDone)

	if captured == "" {
		// Distinguish timeout vs user-cancelled by elapsed time. We
		// don't track elapsed precisely; if Run returned without a
		// capture we assume user closed the window. A separate
		// dedicated timeout channel could refine this later.
		fail(exitCancelled, "user-cancelled",
			"window closed before captcha was solved (or capture timed out after "+
				captureTimeout.String()+")")
	}
	// capturedType is informational; the wire format the API expects
	// is just the token string. We log type to stderr for diagnostics.
	fmt.Fprintf(os.Stderr, "hv: captured: type=%s\n", capturedType)
	fmt.Println(captured)
	os.Exit(exitSuccess)
}

// setLinuxBackendShims installs the WebKit/GTK environment variables
// that work around hardware-acceleration quirks on common Linux setups.
//
//   - GDK_BACKEND=x11 forces Xwayland. webkit2gtk-4.1 hits "Error 71
//     (Protocol error)" on many Wayland sessions; Xwayland is reliable.
//   - WEBKIT_DISABLE_COMPOSITING_MODE=1 turns off WebKit's GL/GBM
//     compositor. NVIDIA drivers + Xwayland frequently fail GBM
//     buffer allocation, leaving the page blank.
//   - WEBKIT_DISABLE_DMABUF_RENDERER=1 forces a non-DMA-BUF render
//     path that doesn't need a working GBM device.
//
// All can be opted out via PROTON_HV_BACKEND or by exporting the
// WEBKIT_* variable yourself before launching.
func setLinuxBackendShims() {
	if b := os.Getenv("PROTON_HV_BACKEND"); b != "" {
		_ = os.Setenv("GDK_BACKEND", b)
	} else {
		_ = os.Setenv("GDK_BACKEND", "x11")
	}
	if os.Getenv("WEBKIT_DISABLE_COMPOSITING_MODE") == "" {
		_ = os.Setenv("WEBKIT_DISABLE_COMPOSITING_MODE", "1")
	}
	if os.Getenv("WEBKIT_DISABLE_DMABUF_RENDERER") == "" {
		_ = os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
	}
}
