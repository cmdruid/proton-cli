package pass

import (
	"strconv"

	"github.com/roman-16/proton-cli/internal/cli/kit"
	passsvc "github.com/roman-16/proton-cli/internal/service/pass"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/spf13/cobra"
)

func vaultsCmd() *cobra.Command {
	c := &cobra.Command{Use: "vaults", Short: "The vaults your items live in"}
	c.AddCommand(vaultsListCmd(), vaultsCreateCmd(), vaultsUpdateCmd(), vaultsDeleteCmd())
	return c
}

func vaultsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your vaults",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepUnlock}, func(c *kit.Invocation) error {
			vaults, err := c.App.Pass.VaultsList(c.Ctx, c.U)
			if err != nil {
				return err
			}
			return kit.List(c, ui.TableSpec[passsvc.Vault]{
				Noun:  "vaults",
				Total: ui.Unknown, Page: ui.Unpaged,
				Columns: []ui.Column[passsvc.Vault]{
					{Header: "ID", ID: true, Cell: func(v passsvc.Vault) string { return v.ShareID }},
					{Header: "NAME", Flex: true, Cell: func(v passsvc.Vault) string {
						if v.Name == "" {
							return "(could not be decrypted)"
						}
						return v.Name
					}},
					{Header: "MEMBERS", Right: true, Cell: func(v passsvc.Vault) string {
						return strconv.Itoa(v.Members)
					}},
					{Header: "OWNER", Cell: func(v passsvc.Vault) string { return yesNo(v.Owner) }},
					{Header: "SHARED", Cell: func(v passsvc.Vault) string { return yesNo(v.Shared) }},
				},
			}, vaults, func(v passsvc.Vault) []string { return []string{v.ShareID} })
		}),
	}
}

func vaultsCreateCmd() *cobra.Command {
	var name string
	c := &cobra.Command{
		Use:   "create",
		Short: "Create a vault",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepUnlock}, func(c *kit.Invocation) error {
			if name == "" {
				return kit.Fail("A vault needs a name.").Hint("--name Work")
			}
			return kit.Create(c, ui.ResultSpec{
				Action: ui.Created, Kind: "vaults", Name: name,
			}, func() (string, error) {
				return c.App.Pass.VaultCreate(c.Ctx, c.U, name)
			})
		}),
	}
	c.Flags().StringVar(&name, "name", "", "Name for the new vault")
	return c
}

func vaultsUpdateCmd() *cobra.Command {
	var name string
	c := &cobra.Command{
		Use:   "update REF",
		Short: "Rename a vault",
		Args:  cobra.ExactArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand, kit.StepUnlock}, func(c *kit.Invocation) error {
			if name == "" {
				return kit.Fail("Nothing to change.").Hint("pass --name.")
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Updated, Kind: "vaults", Count: 1, Name: name,
				IDs: []string{c.Args[0]},
			}, func() error {
				return c.App.Pass.VaultEdit(c.Ctx, c.U, c.Args[0], name)
			})
		}),
	}
	c.Flags().StringVar(&name, "name", "", "New name")
	return c
}

func vaultsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete REF...",
		Short: "Delete vaults, and everything in them",
		Args:  cobra.MinimumNArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand}, func(c *kit.Invocation) error {
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Deleted, Kind: "vaults", Count: len(c.Args), IDs: c.Args,
			}, func() error {
				for _, id := range c.Args {
					if err := c.App.Pass.VaultDelete(c.Ctx, id); err != nil {
						return err
					}
				}
				return nil
			})
		}),
	}
}
