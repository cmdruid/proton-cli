// Package errs defines the cross-cutting typed errors that drive the CLI's
// exit-code scheme. Domain services return these instead of formatting
// strings; the cli layer classifies them via errors.As against the ExitCoder
// interface rather than by matching message text.
package errs

import (
	"fmt"
	"strings"
)

// ExitCoder is implemented by errors that carry a specific process exit code.
type ExitCoder interface {
	ExitCode() int
}

// NotFound means a REF matched no resource. Exit 3.
type NotFound struct {
	Kind string // "message", "contact", "event", "item", "vault", "calendar", "path"
	Ref  string
}

func (e *NotFound) Error() string {
	if e.Ref == "" {
		return fmt.Sprintf("no %s found", e.Kind)
	}
	return fmt.Sprintf("no %s matching %q", e.Kind, e.Ref)
}
func (e *NotFound) ExitCode() int { return 3 }

// Candidate is one entry in an Ambiguous error's disambiguation list.
type Candidate struct {
	ID    string
	Label string // human-readable hint (sender, subject, email, …)
}

// Ambiguous means a REF matched more than one resource. Exit 4.
type Ambiguous struct {
	Kind       string
	Ref        string
	Candidates []Candidate
}

func (e *Ambiguous) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "ambiguous: %d %ss match %q:", len(e.Candidates), e.Kind, e.Ref)
	for _, c := range e.Candidates {
		b.WriteString("\n  ")
		b.WriteString(c.ID)
		if c.Label != "" {
			b.WriteString("  ")
			b.WriteString(c.Label)
		}
	}
	return b.String()
}
func (e *Ambiguous) ExitCode() int { return 4 }

// Exit wraps an error with an explicit process exit code.
type Exit struct {
	Code int
	Err  error
}

func (e *Exit) Error() string { return e.Err.Error() }
func (e *Exit) Unwrap() error { return e.Err }
func (e *Exit) ExitCode() int { return e.Code }

// WithExit wraps err so it exits with the given code. Returns nil for nil err.
func WithExit(code int, err error) error {
	if err == nil {
		return nil
	}
	return &Exit{Code: code, Err: err}
}
