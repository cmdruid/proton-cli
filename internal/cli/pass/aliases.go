package pass

import (
	"strconv"

	"github.com/roman-16/proton-cli/internal/cli/kit"
	passsvc "github.com/roman-16/proton-cli/internal/service/pass"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/spf13/cobra"
)

// Hide-my-email aliases. An alias is a Pass item of type alias, so `items list`
// shows them too; this tree exists because creating one has its own vocabulary of
// prefixes, suffixes and mailboxes.

func aliasesCmd() *cobra.Command {
	c := &cobra.Command{Use: "aliases", Short: "Hide-my-email addresses that forward to you"}
	c.AddCommand(aliasesListCmd(), aliasesCreateCmd(), aliasesOptionsCmd())
	return c
}

func aliasesListCmd() *cobra.Command {
	var vault string
	c := &cobra.Command{
		Use:   "list",
		Short: "List your aliases",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepUnlock}, func(c *kit.Invocation) error {
			vaultRef, err := kit.Expand(c.App, vault)
			if err != nil {
				return err
			}
			items, err := c.App.Pass.ItemsList(c.Ctx, c.U, vaultRef)
			if err != nil {
				return err
			}
			aliases := keepType(items, "alias")
			return kit.List(c, ui.TableSpec[passsvc.Item]{
				Noun:  "aliases",
				Total: ui.Unknown, Page: ui.Unpaged,
				Columns: []ui.Column[passsvc.Item]{
					{Header: "ID", ID: true, Cell: itemRef},
					{Header: "ADDRESS", Flex: true, Cell: func(it passsvc.Item) string { return it.Alias }},
					{Header: "NAME", Flex: true, Cell: func(it passsvc.Item) string { return it.Name }},
				},
			}, aliases, func(it passsvc.Item) []string { return []string{it.ShareID, it.ItemID} })
		}),
	}
	c.Flags().StringVar(&vault, "vault", "", "Show only this vault, by name or ID")
	return c
}

func aliasesCreateCmd() *cobra.Command {
	var prefix, suffix, mailbox, name, vault string
	c := &cobra.Command{
		Use:   "create",
		Short: "Create an alias",
		Long: "Create an alias.\n\n" +
			"The address is a prefix you choose plus a suffix Proton offers; mail sent to it\n" +
			"arrives in the mailbox you name. `aliases options` lists both.",
		Args: cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepUnlock}, func(c *kit.Invocation) error {
			if prefix == "" {
				return kit.Fail("An alias needs a prefix.").
					Hint("--prefix shop", "proton-cli pass aliases options")
			}
			shareID, err := resolveVault(c, vault)
			if err != nil {
				return err
			}
			// The address is the answer, so it is worked out before the alias is
			// made: the confirmation, the machine output and a dry run then all name
			// the same address rather than the prefix it was asked for.
			plan, err := c.App.Pass.PlanAlias(c.Ctx, shareID, prefix, suffix, mailbox)
			if err != nil {
				return err
			}
			if name == "" {
				name = prefix
			}
			spec := ui.ResultSpec{Action: ui.Created, Kind: "aliases", Name: name}
			if !c.App.DryRun {
				// Proton invents a suffix each time it is asked for one, so the
				// address is only settled by using this one. A preview can name the
				// alias it would make but not the address it would get.
				spec.Detail = "as " + plan.Address
				spec.Extra = map[string]any{"alias": plan.Address}
			}
			return kit.Create(c, spec, func() (string, error) {
				itemID, err := c.App.Pass.AliasCreate(c.Ctx, c.U, shareID, plan, name)
				if err != nil {
					return "", err
				}
				return kit.JoinPair(shareID, itemID), nil
			})
		}),
	}
	c.Flags().StringVar(&prefix, "prefix", "", "The part before the @")
	c.Flags().StringVar(&suffix, "suffix", "", "The part from the @ onwards (default: the first Proton offers)")
	c.Flags().StringVar(&mailbox, "mailbox", "", "Where mail to the alias should arrive")
	c.Flags().StringVar(&name, "name", "", "Name for the alias item")
	c.Flags().StringVar(&vault, "vault", "", "Which vault to keep it in, by name or ID")
	return c
}

// option is one choice `aliases options` offers, in either category.
type option struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	ID    string `json:"id,omitempty"`
}

func aliasesOptionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "options",
		Short: "List the suffixes and mailboxes an alias can use",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepUnlock}, func(c *kit.Invocation) error {
			shareID, err := resolveVault(c, "")
			if err != nil {
				return err
			}
			suffixes, mailboxes, err := c.App.Pass.AliasOptions(c.Ctx, shareID)
			if err != nil {
				return err
			}
			rows := make([]option, 0, len(suffixes)+len(mailboxes))
			for _, s := range suffixes {
				rows = append(rows, option{Kind: "suffix", Value: s.Suffix})
			}
			for _, m := range mailboxes {
				rows = append(rows, option{Kind: "mailbox", Value: m.Email, ID: strconv.Itoa(m.ID)})
			}
			return kit.List(c, ui.TableSpec[option]{
				Noun:  "options",
				Total: ui.Unknown, Page: ui.Unpaged,
				Columns: []ui.Column[option]{
					{Header: "KIND", Cell: func(o option) string { return o.Kind }},
					{Header: "VALUE", Flex: true, Cell: func(o option) string { return o.Value }},
					{Header: "ID", Cell: func(o option) string { return o.ID }},
				},
			}, rows, nil)
		}),
	}
}
