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
	// passphrase locks an exported file rather than the account, so it has a
	// source and a memory of its own.
	passphrase struct {
		source passwordSource
		value  string
		have   bool
	}
	// stdinOwner is set once the App exists, so Supply can claim standard input.
	stdinOwner func(claim string) (io.Reader, error)

	// prompter is built once and reused, so the three questions share one reader
	// and one label column: a value typed ahead of its question survives, and the
	// answers line up under each other.
	prompter *ui.Prompter

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
	// labelPassphrase is the secret that locks a file rather than the account.
	// It is never the account password, and calling it something else is what
	// keeps somebody from typing one where the other was meant.
	labelPassphrase = "Passphrase"
)

func newCredentials(u *ui.UI, signedInAs string) *Credentials {
	return &Credentials{ui: u, signedInAs: signedInAs}
}

// SupplyPassphrase records where the passphrase that locks an exported file may
// be read from.
//
// It is kept apart from the account password because it is a different secret
// with a different life: one unlocks the account, the other unlocks one file,
// and a person who exports a backup chooses it themselves.
func (c *Credentials) SupplyPassphrase(file string, stdin bool) error {
	c.passphrase.source.file = file
	if !stdin {
		return nil
	}
	r, err := c.stdinOwner("--passphrase-stdin")
	if err != nil {
		return err
	}
	c.passphrase.source.stdin = r
	return nil
}

// Passphrase returns the passphrase for a file, asking for it if there is
// somebody to ask. reason completes "A passphrase is required to <reason>".
func (c *Credentials) Passphrase(reason string) (string, error) {
	if c.passphrase.have {
		return c.passphrase.value, nil
	}
	v, err := c.read(c.passphrase.source, labelPassphrase,
		errs.Problemf("A passphrase is required to %s.", reason).
			Hint("pass --passphrase-file, or run this in a terminal"))
	if err != nil {
		return "", err
	}
	c.passphrase.value, c.passphrase.have = v, true
	return v, nil
}

// Supply records the credentials a command was given. Only the commands that can
// be asked to re-authenticate declare them, so this is the one place standard
// input is claimed for a password.
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
			Hint("proton account login"))
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
	return c.read(c.source, labelPassword,
		errs.Problemf("Your password is required to %s.", reason).
			Hint("pass --password-file, or run this in a terminal"))
}

// read takes a secret from wherever it was told to look, and asks for it only
// when it was told nothing. A file that exists but holds nothing is an error
// rather than an empty secret: somebody meant to put one there.
func (c *Credentials) read(src passwordSource, label string, missing error) (string, error) {
	if src.file != "" {
		b, err := os.ReadFile(src.file)
		if err != nil {
			return "", errs.Problemf("Could not read %s: %v", src.file, err)
		}
		if v := strings.TrimSpace(string(b)); v != "" {
			return v, nil
		}
		return "", errs.Problemf("%s is empty.", src.file)
	}
	if src.stdin != nil {
		b, err := io.ReadAll(src.stdin)
		if err != nil {
			return "", errs.Problemf("Could not read from stdin: %v", err)
		}
		if v := strings.TrimSpace(string(b)); v != "" {
			return v, nil
		}
		return "", errs.Problemf("Nothing arrived on stdin.")
	}
	return c.ask(label, true, missing)
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
	if c.prompter == nil {
		c.prompter = c.ui.Ask(labelEmail, labelPassword, labelTOTP, labelPassphrase)
	}
	p := c.prompter
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
