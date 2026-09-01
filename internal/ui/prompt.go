package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/roman-16/proton-cli/internal/errs"
	"golang.org/x/term"
)

// This file is the only place in the CLI that reads from standard input for a
// value the user is being asked for. Keeping it to one file is what makes the
// scripting contract checkable: a command can only ever block on input if it
// went through here.

// ErrNoInput reports that a value was needed but could not be asked for.
var ErrNoInput = errs.Problemf("input is required but the terminal is not interactive")

// CanPrompt reports whether asking a question is possible: input has to be a
// terminal, and prompting must not have been forbidden.
//
// Everything else in the CLI stays non-interactive, so a cron job fails with a
// message instead of hanging on a question nobody will answer.
//
// An In that is not a file is a test's buffer, which can always be read and
// never blocks; that is what lets the confirmations be pinned by golden files
// rather than trusted.
func (u *UI) CanPrompt() bool {
	if u.NoInput {
		return false
	}
	if f, ok := u.In.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return u.In != nil
}

// Confirm asks a yes/no question and reports whether the answer was yes.
//
// The default is no. A bare newline, an unreadable stdin or anything that is not
// a plain yes all mean no, so the dangerous path is the one that has to be typed
// out. Like every other prompt, the question goes to Err: it is not an answer.
func (u *UI) Confirm(question string) (bool, error) {
	if !u.CanPrompt() {
		return false, ErrNoInput
	}
	_, _ = fmt.Fprintf(u.Err, "%s %s ", question, u.errStyle.Paint(Muted, "[y/N]"))
	line, err := bufio.NewReader(u.In).ReadString('\n')
	if err != nil && line == "" {
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	}
	return false, nil
}

// Await blocks until the person says that something which happened elsewhere is
// done, and reports whether they said so.
//
// It is the one thing asked for here that is not a value. Nothing is read but
// the fact that a line arrived, because the work was done in another window on
// another machine and there is nothing to type about it - which is also why an
// end of input is a no rather than an empty answer.
func (u *UI) Await(question string) error {
	if !u.CanPrompt() {
		return ErrNoInput
	}
	_, _ = fmt.Fprintf(u.Err, "%s ", u.errStyle.Paint(Muted, question))
	line, err := bufio.NewReader(u.In).ReadString('\n')
	if err != nil && line == "" {
		_, _ = fmt.Fprintln(u.Err)
		return ErrNoInput
	}
	return nil
}

// Prompter asks a sequence of questions over one reader, so a value typed ahead
// of the question that wants it is not lost between prompts.
//
// The questions form one block, so it is measured like one: the labels are
// declared up front and padded to a common width, and the answers line up in a
// column the way a record's values do.
type Prompter struct {
	u     *UI
	in    *bufio.Reader
	width int
}

// Ask returns a Prompter reading from this UI's input. The labels are every
// question the caller may go on to ask; naming them here is what lets the block
// be aligned before the first one is written.
func (u *UI) Ask(labels ...string) *Prompter {
	width := 0
	for _, l := range labels {
		if n := Cells(l + ":"); n > width {
			width = n
		}
	}
	return &Prompter{u: u, in: bufio.NewReader(u.In), width: width}
}

// Line asks for a visible value.
func (p *Prompter) Line(label string) (string, error) {
	if !p.u.CanPrompt() {
		return "", ErrNoInput
	}
	p.write(label)
	s, err := p.in.ReadString('\n')
	if err != nil && s == "" {
		return "", err
	}
	return strings.TrimSpace(s), nil
}

// Secret asks for a value without echoing it. The bytes are never written
// anywhere but the caller's variable.
//
// Turning the echo off needs a terminal rather than a stream, so this reads the
// descriptor directly when there is one and falls back to an ordinary line
// otherwise - the fallback only being reachable from a test, since CanPrompt
// has already refused a non-terminal file.
func (p *Prompter) Secret(label string) (string, error) {
	if !p.u.CanPrompt() {
		return "", ErrNoInput
	}
	p.write(label)
	f, isFile := p.u.In.(*os.File)
	if !isFile {
		s, err := p.in.ReadString('\n')
		if err != nil && s == "" {
			return "", err
		}
		return strings.TrimSpace(s), nil
	}
	b, err := term.ReadPassword(int(f.Fd()))
	// ReadPassword swallows the newline the user typed; put it back so the next
	// line of output does not land on the prompt.
	_, _ = fmt.Fprintln(p.u.Err)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// write emits the prompt on stderr, never stdout: a question is not an answer,
// so redirecting the answer must not capture it.
// The two spaces after the label are the gap a record block puts between a
// label and its value, so a sign-in and a record read as the same shape.
func (p *Prompter) write(label string) {
	_, _ = fmt.Fprintf(p.u.Err, "%s  ", p.u.errStyle.Paint(Muted, padCells(label+":", p.width, false)))
}
