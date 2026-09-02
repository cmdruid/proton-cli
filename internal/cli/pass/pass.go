// Package pass is the `proton pass` tree: vaults, the items in them, aliases,
// and the trash.
//
// An item lives inside a share, so it takes two IDs to address. They are written
// as one slash-separated token, keeping every command to a single REF - and a name
// or URL still works, which is how anyone actually reaches an item.
package pass

import (
	"github.com/cmdruid/proton-cli/internal/cli/kit"
	"github.com/cmdruid/proton-cli/internal/secret"
	passsvc "github.com/cmdruid/proton-cli/internal/service/pass"
	"github.com/cmdruid/proton-cli/internal/ui"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	c := &cobra.Command{
		Use:   "pass",
		Short: "Vaults, logins and secrets",
	}
	c.AddCommand(itemsCmd(), vaultsCmd(), aliasesCmd(), trashCmd(), generateCmd(), breachesCmd(), linksCmd(),
		invitationsCmd(), settingsCmd(),
		exportCmd(), importCmd())
	return c
}

// itemRef is the single token that addresses an item.
func itemRef(it passsvc.Item) string { return kit.JoinPair(it.ShareID, it.ItemID) }

// resolveItem turns a reference into the share and item IDs the service needs.
func resolveItem(c *kit.Invocation, ref string) (shareID, itemID string, err error) {
	if first, second, err := kit.ExpandPair(c.App, ref); err != nil || first != "" {
		return first, second, err
	}
	return c.App.Pass.ResolveItem(c.Ctx, []string{ref})
}

// resolveVault accepts a vault name or ID, defaulting to the first vault.
func resolveVault(c *kit.Invocation, ref string) (string, error) {
	expanded, err := kit.Expand(c.App, ref)
	if err != nil {
		return "", err
	}
	return c.App.Pass.ResolveVault(c.Ctx, expanded)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// ── making a password ──

// Generate makes a password without storing it anywhere.
//
// It reaches no account and needs no session: a password is made on this machine
// and may never leave it. That is also the point - a generator you already have
// beats reaching for whatever is on the path.
//
// The alphabet is Proton's own, which leaves out the characters people misread
// unless letters are all there is.
func generateCmd() *cobra.Command {
	var o secret.Options
	var noDigits, noSymbols, noUpper bool
	c := &cobra.Command{
		Use:   "generate",
		Short: "Make a password",
		Long: "Make a password, without storing it anywhere.\n\n" +
			"It reaches no account and needs no session. The alphabet is Proton's own,\n" +
			"which leaves out i, o, l and their capitals - the characters people misread -\n" +
			"unless letters are all the password has.\n\n" +
			"Every kind asked for is guaranteed to appear, so a password that has to\n" +
			"contain a digit does.",
		Args: cobra.NoArgs,
		RunE: kit.Run(nil, func(c *kit.Invocation) error {
			o.Digits, o.Symbols, o.Upper = !noDigits, !noSymbols, !noUpper
			pw, err := secret.Password(o)
			if err != nil {
				return kit.Fail("%v", err)
			}
			return kit.Show(c, ui.RecordSpec{
				Object: struct {
					Password string `json:"password"`
				}{pw},
				Fields: []ui.Field{{Label: "Password", Value: pw}},
			})
		}),
	}
	c.Flags().IntVar(&o.Length, "length", secret.DefaultLength, "How many characters")
	c.Flags().BoolVar(&noDigits, "no-digits", false, "Leave the digits out")
	c.Flags().BoolVar(&noSymbols, "no-symbols", false, "Leave the symbols out")
	c.Flags().BoolVar(&noUpper, "no-uppercase", false, "Leave the capitals out")
	return c
}
