// Package pass is the `proton-cli pass` tree: vaults, the items in them, aliases,
// and the trash.
//
// An item lives inside a share, so it takes two IDs to address. They are written
// as one slash-separated token, keeping every command to a single REF - and a name
// or URL still works, which is how anyone actually reaches an item.
package pass

import (
	"github.com/roman-16/proton-cli/internal/cli/kit"
	passsvc "github.com/roman-16/proton-cli/internal/service/pass"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	c := &cobra.Command{
		Use:   "pass",
		Short: "Vaults, logins and secrets",
	}
	c.AddCommand(itemsCmd(), vaultsCmd(), aliasesCmd(), trashCmd())
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
