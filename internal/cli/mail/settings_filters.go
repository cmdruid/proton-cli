package mail

import (
	"context"
	"strconv"

	"github.com/roman-16/proton-cli/internal/cli/kit"
	mailsvc "github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/spf13/cobra"
)

// Server-side Sieve filters, the same ones the web client creates.

func filtersCmd() *cobra.Command {
	c := &cobra.Command{Use: "filters", Short: "Server-side Sieve filters"}
	c.AddCommand(
		filtersListCmd(), filtersCreateCmd(), filtersUpdateCmd(),
		filterVerbCmd("delete", "Delete filters", ui.Deleted,
			func(c *kit.Invocation, id string) error { return c.App.Mail.FilterDelete(c.Ctx, id) }),
		filterVerbCmd("enable", "Enable filters", ui.Enabled,
			func(c *kit.Invocation, id string) error { return c.App.Mail.FilterEnable(c.Ctx, id) }),
		filterVerbCmd("disable", "Disable filters", ui.Disabled,
			func(c *kit.Invocation, id string) error { return c.App.Mail.FilterDisable(c.Ctx, id) }),
	)
	return c
}

func filterColumns() []ui.Column[mailsvc.Filter] {
	return []ui.Column[mailsvc.Filter]{
		{Header: "ID", ID: true, Cell: func(f mailsvc.Filter) string { return f.ID }},
		{Header: "NAME", Flex: true, Cell: func(f mailsvc.Filter) string { return f.Name }},
		{Header: "ENABLED", Cell: func(f mailsvc.Filter) string { return yesNo(f.Status == 1) }},
		{Header: "VERSION", Right: true, Cell: func(f mailsvc.Filter) string {
			return strconv.Itoa(f.Version)
		}},
	}
}

func filterList(c *kit.Invocation) *kit.Lookup[mailsvc.Filter] {
	return &kit.Lookup[mailsvc.Filter]{
		Kind:   "filter",
		Load:   func(ctx context.Context) ([]mailsvc.Filter, error) { return c.App.Mail.FiltersList(ctx) },
		ID:     func(f mailsvc.Filter) string { return f.ID },
		Handle: func(f mailsvc.Filter) string { return f.Name },
	}
}

func filtersListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your filters",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepAuth}, func(c *kit.Invocation) error {
			rows, err := filterList(c).Rows(c.Ctx)
			if err != nil {
				return err
			}
			return kit.List(c, ui.TableSpec[mailsvc.Filter]{
				Noun: "filters", Columns: filterColumns(),
				Total: ui.Unknown, Page: ui.Unpaged,
			}, rows, func(f mailsvc.Filter) []string { return []string{f.ID} })
		}),
	}
}

func filtersCreateCmd() *cobra.Command {
	var name, sieve string
	var disabled bool
	c := &cobra.Command{
		Use:   "create",
		Short: "Create a Sieve filter",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepAuth}, func(c *kit.Invocation) error {
			script, err := kit.ReadTextArg(c, sieve, "--sieve")
			if err != nil {
				return err
			}
			if name == "" || script == "" {
				return kit.Fail("A filter needs a name and a script.").
					Hint("--name \"Archive invoices\"", "--sieve '…', or --sieve - to read stdin.")
			}
			return kit.Create(c, ui.ResultSpec{
				Action: ui.Created, Kind: "filters", Name: name,
			}, func() (string, error) {
				id, err := c.App.Mail.FilterCreate(c.Ctx, name, script)
				if err != nil || !disabled {
					return id, err
				}
				return id, c.App.Mail.FilterDisable(c.Ctx, id)
			})
		}),
	}
	c.Flags().StringVar(&name, "name", "", "Name for the new filter")
	c.Flags().StringVar(&sieve, "sieve", "", "Sieve script (- reads stdin)")
	c.Flags().BoolVar(&disabled, "disabled", false, "Create it without turning it on")
	return c
}

func filtersUpdateCmd() *cobra.Command {
	var name, sieve string
	c := &cobra.Command{
		Use:   "update REF",
		Short: "Change a filter's name or script",
		Args:  cobra.ExactArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand}, func(c *kit.Invocation) error {
			script, err := kit.ReadTextArg(c, sieve, "--sieve")
			if err != nil {
				return err
			}
			if name == "" && script == "" {
				return kit.Fail("Nothing to change.").Hint("pass --name or --sieve.")
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Updated, Kind: "filters", Count: 1, Name: name,
				IDs: []string{c.Args[0]},
			}, func() error {
				return c.App.Mail.FilterUpdate(c.Ctx, c.Args[0], name, script)
			})
		}),
	}
	c.Flags().StringVar(&name, "name", "", "New name")
	c.Flags().StringVar(&sieve, "sieve", "", "New Sieve script (- reads stdin)")
	return c
}

// filterVerbCmd builds every verb that acts on named filters. They differ only
// in what they do to each one, so they share how the filters are found - which
// is what lets `delete Newsletters` work as well as `delete FILTER_ID`.
func filterVerbCmd(use, short string, action ui.Action, apply func(*kit.Invocation, string) error) *cobra.Command {
	return &cobra.Command{
		Use:   use + " REF...",
		Short: short,
		Args:  cobra.MinimumNArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand}, func(c *kit.Invocation) error {
			sel, err := kit.SelectFrom(c, "filters", filterColumns(), filterList(c))
			if err != nil {
				return err
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: action, Kind: "filters", Count: sel.Len(), IDs: sel.IDs,
				Name:    kit.Sole(sel.Rows, func(f mailsvc.Filter) string { return f.Name }),
				Preview: sel.Preview(),
			}, func() error {
				for _, id := range sel.IDs {
					if err := apply(c, id); err != nil {
						return err
					}
				}
				return nil
			})
		}),
	}
}
