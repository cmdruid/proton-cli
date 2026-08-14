package proton

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrUnauthorized signals that auth failed and token refresh did not recover.
var ErrUnauthorized = errors.New("unauthorized: session expired")

// NetworkError wraps a transport-level failure (DNS, connection refused, TLS,
// timeout) where the request never received an HTTP response. It carries exit
// code 5, matching the documented "network/server" contract - distinct from an
// APIError (a response with a non-2xx status).
type NetworkError struct{ Err error }

func (e *NetworkError) Error() string { return "request failed: " + e.Err.Error() }
func (e *NetworkError) Unwrap() error { return e.Err }
func (e *NetworkError) ExitCode() int { return 5 }

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

// The codes Proton answers with when the thing named does not exist: an ID it
// does not recognise, a resource that is gone, and a conversation in particular.
// The web clients treat all three as the same answer
// (isNotExistError, packages/shared/lib/api/helpers/apiErrorHelper.ts).
//
// They arrive with HTTP 400 or 422, which on their own read as a bad request or a
// conflict, so the Proton code has the final say. Anything that matched no
// resource has to exit 3, because that is the code scripts branch on.
const (
	invalidIDCode            = 2061
	notFoundCode             = 2501
	conversationNotFoundCode = 20052
)

// succeeded reports whether a Proton code is one of the ways of saying it worked:
// done, one answer per item, or accepted and being carried out in the background.
func succeeded(code int) bool {
	switch code {
	case okCode, multiCode, acceptedCode:
		return true
	}
	return false
}

const (
	okCode       = 1000
	multiCode    = 1001
	acceptedCode = 1002
)

// The code Proton answers with when the name is already taken where it was to
// be written. The web clients read the same code to know a duplicate was found
// and ask what to do about it (useUploadFile.ts).
const alreadyExistsCode = 2500

// DoesNotExist reports whether err is Proton saying that what was named is not
// there, so a caller holding the reference can say so in its own words.
func DoesNotExist(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.Code {
	case invalidIDCode, notFoundCode, conversationNotFoundCode:
		return true
	}
	return apiErr.HTTPStatus == 404
}

// AlreadyExists reports whether err is Proton refusing to write a second thing
// under a name that is taken, so a caller that knows which name and which folder
// can say so instead of passing the code on.
func AlreadyExists(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Code == alreadyExistsCode
}

// ExitCode maps an HTTP failure to the CLI's exit-code scheme.
func (e *APIError) ExitCode() int {
	switch e.Code {
	case invalidIDCode, notFoundCode, conversationNotFoundCode:
		return 3
	}
	switch e.HTTPStatus {
	case 401, 403:
		return 2
	case 404:
		return 3
	case 409, 422:
		return 4
	}
	if e.HTTPStatus >= 500 {
		return 5
	}
	return 1
}

// HumanVerificationError is returned when the API requires human verification
// (Proton code 9001). Callers typically display WebURL, run the resolver, and
// retry with Token + Methods[0] set on the next Request.
type HumanVerificationError struct {
	Token   string
	Methods []string
	WebURL  string
}

func (e *HumanVerificationError) Error() string {
	return fmt.Sprintf("human verification required: %s", e.WebURL)
}

// hvDetails is the shared shape of the Details object Proton returns on a
// 9001 (human-verification) response.
type hvDetails struct {
	HumanVerificationToken   string
	HumanVerificationMethods []string
	WebUrl                   string
}

// classifyErrorBody decodes a non-2xx Proton response body into either a
// *HumanVerificationError (Code 9001) or a generic *APIError.
func classifyErrorBody(status int, body []byte) (*HumanVerificationError, *APIError) {
	var env struct {
		Code    int
		Error   string
		Details *hvDetails
	}
	_ = json.Unmarshal(body, &env)
	if env.Code == 9001 && env.Details != nil {
		return &HumanVerificationError{
			Token:   env.Details.HumanVerificationToken,
			Methods: env.Details.HumanVerificationMethods,
			WebURL:  env.Details.WebUrl,
		}, nil
	}
	msg := env.Error
	if msg == "" {
		msg = httpStatusText(status)
	}
	return nil, &APIError{HTTPStatus: status, Code: env.Code, Message: msg, RawBody: body}
}

// parseAuthError converts a Proton auth-flow response body into a typed error.
// The HTTP request itself succeeded - the failure is application-level - so
// HTTPStatus is left at zero.
func parseAuthError(raw []byte, code int) error {
	var env struct {
		Error   string
		Details *hvDetails
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
