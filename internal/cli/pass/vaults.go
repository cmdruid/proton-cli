package pass

import (
	"context"
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

func vaultColumns() []ui.Column[passsvc.Vault] {
	return []ui.Column[passsvc.Vault]{
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
	}
}

func vaultList(c *kit.Invocation) *kit.Lookup[passsvc.Vault] {
	return &kit.Lookup[passsvc.Vault]{
		Kind:   "vault",
		Load:   func(ctx context.Context) ([]passsvc.Vault, error) { return c.App.Pass.VaultsList(ctx, c.U) },
		ID:     func(v passsvc.Vault) string { return v.ShareID },
		Handle: func(v passsvc.Vault) string { return v.Name },
	}
}

func vaultsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your vaults",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepUnlock}, func(c *kit.Invocation) error {
			vaults, err := vaultList(c).Rows(c.Ctx)
			if err != nil {
				return err
			}
			return kit.List(c, ui.TableSpec[passsvc.Vault]{
				Noun: "vaults", Columns: vaultColumns(),
				Total: ui.Unknown, Page: ui.Unpaged,
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
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand, kit.StepUnlock}, func(c *kit.Invocation) error {
			sel, err := kit.SelectFrom(c, "vaults", vaultColumns(), vaultList(c))
			if err != nil {
				return err
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Deleted, Kind: "vaults", Count: sel.Len(), IDs: sel.IDs,
				Name:    kit.Sole(sel.Rows, func(v passsvc.Vault) string { return v.Name }),
				Preview: sel.Preview(),
			}, func() error {
				for _, id := range sel.IDs {
					if err := c.App.Pass.VaultDelete(c.Ctx, id); err != nil {
						return err
					}
				}
				return nil
			})
		}),
	}
}
