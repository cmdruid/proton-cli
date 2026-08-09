package kit

import (
	"github.com/spf13/cobra"
)

// Reauth declares the credentials a command may be asked for beyond the session
// it already holds.
//
// `account login` performs the SRP exchange that attaches an account to a
// profile. The others reach an endpoint Proton guards behind an elevated
// session, which it grants only for another SRP exchange. Which endpoints those
// are is Proton's to decide, so the set is written down and pinned by
// conformance rather than inferred.
//
// A password is read from a pipe or a file, never from a flag value: argv is
// readable by every user on the machine through ps, and it survives in shell
// history and in unit files.
type Reauth struct {
	passwordFile  string
	passwordStdin bool
	totp          string
}

// Declare adds the flags to a command. Call Supply from its body.
func (r *Reauth) Declare(c *cobra.Command) {
	f := c.Flags()
	f.StringVar(&r.passwordFile, "password-file", "", "Read the account password from a file")
	f.BoolVar(&r.passwordStdin, "password-stdin", false, "Read the account password from stdin")
	f.StringVar(&r.totp, "totp", "", "Two-factor code")
}

// Supply hands what was given to the invocation, before anything that might ask
// for it runs.
func (r *Reauth) Supply(c *Invocation) error {
	return c.App.Creds.Supply(r.passwordFile, r.passwordStdin, r.totp)
}
