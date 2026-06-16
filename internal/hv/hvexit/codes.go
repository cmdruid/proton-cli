// Package hvexit defines the process-exit-code contract shared between the
// proton-cli-hv helper binary (which produces these codes) and the internal/hv
// package (which interprets them). Both sides reference these constants, so the
// contract can never silently drift.
package hvexit

const (
	// Success: stdout carries "<challenge>:<recaptcha_response>".
	Success = 0
	// Usage: wrong or missing argv.
	Usage = 2
	// Unavailable: a webview cannot run here (no display, no webkit, init
	// failed). On Linux, glibc's 126/127 dynamic-linker failures are treated
	// the same way by the parent.
	Unavailable = 3
	// Cancelled: the window closed before the captcha was solved, or the
	// capture timed out.
	Cancelled = 4
	// Network: error reaching verify.proton.me.
	Network = 5
)
