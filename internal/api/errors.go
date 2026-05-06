package api

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrUnauthorized signals that auth failed and token refresh did not recover.
var ErrUnauthorized = errors.New("unauthorized: session expired")

// APIError is returned for non-2xx responses that carry a Proton error code.
type APIError struct {
	HTTPStatus int
	Code       int
	Message    string
	RawBody    []byte
}

func (e *APIError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("[HTTP %d] %d: %s", e.HTTPStatus, e.Code, e.Message)
	}
	return fmt.Sprintf("[HTTP %d] %s", e.HTTPStatus, e.Message)
}

// HumanVerificationError is returned when the API requires human verification
// (Proton code 9001). Callers typically display WebURL, wait for the user,
// and then retry with Token + Methods[0] set on the next Request.
type HumanVerificationError struct {
	Token   string
	Methods []string
	WebURL  string
}

func (e *HumanVerificationError) Error() string {
	return fmt.Sprintf("human verification required: %s", e.WebURL)
}

// parseAuthError converts a Proton auth response body into a typed error.
// Returns *HumanVerificationError for code 9001 (CAPTCHA required), otherwise
// an *APIError carrying the server-provided message and raw body. The HTTP
// request itself succeeded — the failure is application-level — so HTTPStatus
// is left at zero.
func parseAuthError(raw []byte, code int) error {
	var env struct {
		Error   string
		Details *struct {
			HumanVerificationToken   string
			HumanVerificationMethods []string
			WebUrl                   string
		}
	}
	_ = json.Unmarshal(raw, &env)
	if code == 9001 && env.Details != nil {
		return &HumanVerificationError{
			Token:   env.Details.HumanVerificationToken,
			Methods: env.Details.HumanVerificationMethods,
			WebURL:  env.Details.WebUrl,
		}
	}
	msg := env.Error
	if msg == "" {
		msg = fmt.Sprintf("auth code %d", code)
	}
	return &APIError{Code: code, Message: msg, RawBody: raw}
}
