package kit

import (
	"github.com/spf13/cobra"
)

// Passphrase declares where the secret that locks an exported file may be read
// from.
//
// It is not the account password: it locks one file, a person choosing a backup
// chooses it themselves, and typing one where the other was meant should not be
// possible - so it has flags of its own rather than sharing the ones a
// re-authentication uses.
//
// Like a password, it is read from a pipe, a file or a prompt and never from a
// flag value: argv is readable by every user on the machine through ps, and it
// survives in shell history and in unit files.
type Passphrase struct {
	file  string
	stdin bool
}

// The one thing each of these says, wherever it appears.
const (
	PassphraseFileUsage  = "Read the passphrase that locks the file from a file"
	PassphraseStdinUsage = "Read the passphrase that locks the file from stdin"
)

// Declare adds the flags to a command. Call Supply from its steps.
func (p *Passphrase) Declare(c *cobra.Command) {
	f := c.Flags()
	f.StringVar(&p.file, "passphrase-file", "", PassphraseFileUsage)
	f.BoolVar(&p.stdin, "passphrase-stdin", false, PassphraseStdinUsage)
}

// Supply hands what was given to the invocation, before anything that might ask
// for it runs.
func (p *Passphrase) Supply(c *Invocation) error {
	return c.App.Creds.SupplyPassphrase(p.file, p.stdin)
}

// Wanted reports whether a passphrase was named at all, which is what decides
// whether an export is encrypted.
func (p *Passphrase) Wanted() bool { return p.file != "" || p.stdin }
