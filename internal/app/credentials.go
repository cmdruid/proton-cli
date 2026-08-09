package app

import (
	"io"
	"os"
	"strings"

	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/ui"
)

// passwordSource says where the account password may be read from. A file path
// is the channel secret delivery already speaks - systemd's LoadCredential,
// Kubernetes secrets and Docker secrets all hand one over.
type passwordSource struct {
	// file is read whole, with surrounding whitespace stripped.
	file string
	// stdin is standard input, claimed the moment --password-stdin is seen rather
	// than when a password turns out to be wanted: a `-` argument would otherwise
	// read the stream first and quietly send the password wherever it pointed.
	stdin io.Reader
}

// Credentials resolves the values that identify and unlock an account.
//
// It is the only thing in the CLI that may ask a person for one. Every other
// command stays non-interactive and fails with a message, so a scheduled job can
// never hang waiting on a question nobody will answer.
//
// Resolution, most specific first:
//
//	email     the account this profile is signed in as, else a prompt
//	password  --password-file, else --password-stdin, else a prompt
//	code      --totp, else a prompt
//
// Only `account login` names an account, and it does so with its own --user.
//
// Each value is asked for at most once per invocation: a command that needs the
// password twice does not ask twice.
type Credentials struct {
	ui *ui.UI

	// signedInAs is the account this profile already holds a session for, so
	// elevating that session never asks for an address the CLI can see.
	signedInAs string
	flagTOTP   string
	source     passwordSource
	// stdinOwner is set once the App exists, so Supply can claim standard input.
	stdinOwner func(claim string) (io.Reader, error)

	user, password, totp string
	haveUser             bool
	havePassword         bool
	haveTOTP             bool
}

// The prompt labels.
const (
	labelEmail    = "Email"
	labelPassword = "Password"
	labelTOTP     = "Two-factor code"
)

func newCredentials(u *ui.UI, signedInAs string) *Credentials {
	return &Credentials{ui: u, signedInAs: signedInAs}
}

// Supply records the credentials a command was given. Only the two commands
// that can be asked to re-authenticate declare them, so this is the one place
// standard input is claimed for a password.
func (c *Credentials) Supply(passwordFile string, passwordStdin bool, totp string) error {
	c.source.file = passwordFile
	c.flagTOTP = totp
	if !passwordStdin {
		return nil
	}
	r, err := c.stdinOwner("--password-stdin")
	if err != nil {
		return err
	}
	c.source.stdin = r
	return nil
}

// User returns the account email.
//
// Everything but signing in reaches this with a session already in hand, so the
// address is known and nothing is asked. `account login` passes its own --user
// rather than going through here, which is what keeps a stray address from
// reaching the SRP exchange that elevates a session.
func (c *Credentials) User() (string, error) {
	if c.haveUser {
		return c.user, nil
	}
	v := c.signedInAs
	if v == "" {
		var err error
		v, err = c.ask(labelEmail, false, errs.Problemf("An account email is required.").
			Hint("proton-cli account login"))
		if err != nil {
			return "", err
		}
	}
	c.user, c.haveUser = v, true
	return v, nil
}

// Password returns the account password. reason completes the sentence "Your
// password is required to <reason>" reported when there is nobody to ask, so it
// reads as a clause: "sign in", "unlock your keys", "delete a calendar".
func (c *Credentials) Password(reason string) (string, error) {
	if c.havePassword {
		return c.password, nil
	}
	v, err := c.readPassword(reason)
	if err != nil {
		return "", err
	}
	c.password, c.havePassword = v, true
	return v, nil
}

func (c *Credentials) readPassword(reason string) (string, error) {
	if c.source.file != "" {
		b, err := os.ReadFile(c.source.file)
		if err != nil {
			return "", errs.Problemf("Could not read the password file: %v", err)
		}
		if v := strings.TrimSpace(string(b)); v != "" {
			return v, nil
		}
		return "", errs.Problemf("The password file %s is empty.", c.source.file)
	}
	if c.source.stdin != nil {
		b, err := io.ReadAll(c.source.stdin)
		if err != nil {
			return "", errs.Problemf("Could not read the password from stdin: %v", err)
		}
		if v := strings.TrimSpace(string(b)); v != "" {
			return v, nil
		}
		return "", errs.Problemf("No password arrived on stdin.")
	}
	return c.ask(labelPassword, true,
		errs.Problemf("Your password is required to %s.", reason).
			Hint("pass --password-file, or run this in a terminal"))
}

// TOTP returns the current two-factor code.
//
// A code is single-use and expires within thirty seconds, which is why it is the
// one credential a flag may carry and the value a prompt helps with most.
func (c *Credentials) TOTP() (string, error) {
	if c.haveTOTP {
		return c.totp, nil
	}
	v := c.flagTOTP
	if v == "" {
		var err error
		v, err = c.ask(labelTOTP, false,
			errs.Problemf("This account has two-factor authentication enabled, so a code is required.").
				Hint("pass --totp, or run this in a terminal").Exit(2))
		if err != nil {
			return "", err
		}
	}
	c.totp, c.haveTOTP = v, true
	return v, nil
}

// TOTPIfSet returns a two-factor code only if one was already supplied, for
// callers that would rather proceed and let the server say whether it wanted one.
func (c *Credentials) TOTPIfSet() string {
	if c.haveTOTP {
		return c.totp
	}
	return c.flagTOTP
}

// ask prompts, or reports missing when there is nobody to ask.
func (c *Credentials) ask(label string, secret bool, missing error) (string, error) {
	if !c.ui.CanPrompt() {
		return "", missing
	}
	p := c.ui.Ask()
	var (
		v   string
		err error
	)
	if secret {
		v, err = p.Secret(label)
	} else {
		v, err = p.Line(label)
	}
	if err != nil {
		return "", err
	}
	if v == "" {
		return "", missing
	}
	return v, nil
}
