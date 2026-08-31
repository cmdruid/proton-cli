package kit

import (
	"github.com/spf13/cobra"
)

// SecondPassword declares where the second password of an account in Proton's
// two-password mode may be read from.
//
// It is not the account password: that one proves who the account is, over SRP,
// and this one opens its keys. Signing in is the only command that wants it -
// everything else either holds the key password already, sealed into the
// session, or is being asked to prove itself, which the password answers.
//
// Like a password, it is read from a pipe, a file or a prompt and never from a
// flag value: argv is readable by every user on the machine through ps, and it
// survives in shell history and in unit files.
type SecondPassword struct {
	file  string
	stdin bool
}

// The one thing each of these says, wherever it appears.
const (
	SecondPasswordFileUsage  = "Read the second password (two-password mode) from a file"
	SecondPasswordStdinUsage = "Read the second password (two-password mode) from stdin"
)

// Declare adds the flags to a command. Call Supply from its body.
func (p *SecondPassword) Declare(c *cobra.Command) {
	f := c.Flags()
	f.StringVar(&p.file, "second-password-file", "", SecondPasswordFileUsage)
	f.BoolVar(&p.stdin, "second-password-stdin", false, SecondPasswordStdinUsage)
}

// Supply hands what was given to the invocation, before anything that might ask
// for it runs.
func (p *SecondPassword) Supply(c *Invocation) error {
	return c.App.Creds.SupplySecondPassword(p.file, p.stdin)
}
