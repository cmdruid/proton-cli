package cmd

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/roman-16/proton-cli/internal/api"
	"github.com/roman-16/proton-cli/internal/app"
	"github.com/roman-16/proton-cli/internal/hv"
)

// cliHVResolver returns the HVResolver installed on every Client. It
// inspects which HV methods Proton offered on a 9001 response and:
//
//   - "captcha" — extracts the embedded proton-cli-hv helper and runs
//     it. The helper opens a webview at verify.proton.me, captures the
//     postMessage solution token, and returns it. On systems with no
//     GUI / missing libwebkit2gtk, the helper returns
//     *hv.UnavailableError, which we map to api.ErrHVUnavailable so
//     the api layer surfaces the original 9001 with the honest error
//     formatted in cmd/root.go's exit path.
//   - "email" / "sms" / "ownership-email" / "ownership-sms" — NOT YET
//     IMPLEMENTED. Returns api.ErrHVUnavailable. Future work: prompt
//     the user for the destination + code via stdin; for email/sms
//     POST /core/v4/users/code first, then send <dest>:<code> as the
//     HV header.
//
// The `a *app.App` parameter is currently unused but threaded so future
// email/SMS prompts can use a.R for prompts and stay consistent with
// other CLI interactive flows.
func cliHVResolver(_ context.Context, a *app.App) api.HVResolver {
	return func(hvErr *api.HumanVerificationError) (string, string, error) {
		if hvErr == nil {
			return "", "", api.ErrHVUnavailable
		}
		methods := hvErr.Methods

		// Captcha path: spawn the helper.
		if slices.Contains(methods, "captcha") {
			// Use a fresh context for the helper run. The user may take
			// minutes to solve the CAPTCHA; we don't want a stale request
			// context killing the webview mid-solve.
			//
			// internal/hv.Resolve runs the helper synchronously; the
			// helper itself enforces a 5-minute capture timeout.
			token, err := hv.Resolve(context.Background(), hvErr.Token)
			if err == nil {
				return token, "captcha", nil
			}
			// Map helper-specific errors to the resolver contract.
			var unavail *hv.UnavailableError
			var cancelled *hv.CancelledError
			switch {
			case errors.As(err, &unavail):
				// Helper couldn't run on this machine. Stash a hint on
				// the app so the cmd-layer error formatter can show
				// it. (See cmd/root.go's exit-formatting path.)
				if a != nil {
					a.HVUnavailableDetail = unavail.Detail
				}
				return "", "", api.ErrHVUnavailable
			case errors.As(err, &cancelled):
				// User explicitly closed the window. Don't pretend the
				// resolver isn't available; surface the cancellation
				// as a regular error so the user sees it clearly.
				return "", "", fmt.Errorf("captcha cancelled: %s", cancelled.Detail)
			case errors.Is(err, hv.ErrHelperMissing):
				// Build of the CLI doesn't include the helper. This
				// happens for `go install` / `go build` outside of
				// goreleaser. Treat as unavailable.
				if a != nil {
					a.HVUnavailableDetail = "this build of proton-cli has no embedded captcha helper " +
						"(produced by `go install` or a non-release build); install via the official " +
						"release tarball"
				}
				return "", "", api.ErrHVUnavailable
			default:
				return "", "", err
			}
		}

		// TODO: email/sms/ownership-email/ownership-sms via stdin
		// prompts. See tasks/11-human-verification-honest-error.md.
		// Until those land, surface a clear unavailable detail.
		if a != nil {
			a.HVUnavailableDetail = fmt.Sprintf(
				"Proton offered methods %v but only `captcha` is implemented in this build",
				methods)
		}
		return "", "", api.ErrHVUnavailable
	}
}
