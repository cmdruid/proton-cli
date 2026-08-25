package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/roman-16/proton-cli/internal/app"
	"github.com/roman-16/proton-cli/internal/hv"
	"github.com/roman-16/proton-cli/internal/proton"
)

// cliHVResolver returns the HVResolver installed on the client. On a captcha
// challenge it extracts and runs the embedded proton-hv webview helper;
// other methods are not yet implemented and map to proton.ErrHVUnavailable so
// the original 9001 surfaces with an honest message.
func cliHVResolver(_ context.Context, a *app.App) proton.HVResolver {
	return func(hvErr *proton.HumanVerificationError) (string, string, error) {
		if hvErr == nil {
			return "", "", proton.ErrHVUnavailable
		}
		methods := hvErr.Methods

		if slices.Contains(methods, "captcha") {
			if a == nil {
				return "", "", proton.ErrHVUnavailable
			}
			// The page must come from the API that issued the challenge, so the
			// client builds the URL rather than the helper guessing.
			captchaURL, err := a.API.CaptchaURL(hvErr.Token)
			if err != nil {
				return "", "", err
			}

			// Fresh context: the user may take minutes to solve; the helper
			// enforces its own 5-minute capture timeout.
			token, err := hv.Resolve(context.Background(), captchaURL)
			if err == nil {
				return token, "captcha", nil
			}
			var unavail *hv.UnavailableError
			var cancelled *hv.CancelledError
			switch {
			case errors.As(err, &unavail):
				if a != nil {
					a.HVUnavailableDetail = unavail.Detail
				}
				return "", "", proton.ErrHVUnavailable
			case errors.As(err, &cancelled):
				return "", "", fmt.Errorf("captcha cancelled: %s", cancelled.Detail)
			case errors.Is(err, hv.ErrHelperMissing):
				if a != nil {
					a.HVUnavailableDetail = "this build of proton has no embedded captcha helper " +
						"(produced by `go install` or a non-release build); install via the official " +
						"release tarball, or set " + hv.EnvHelper + " to a helper you built"
				}
				return "", "", proton.ErrHVUnavailable
			default:
				return "", "", err
			}
		}

		if a != nil {
			a.HVUnavailableDetail = fmt.Sprintf(
				"Proton offered methods %v but only `captcha` is implemented in this build",
				methods)
		}
		return "", "", proton.ErrHVUnavailable
	}
}
