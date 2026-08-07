package mail

import (
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
		filtersListCmd(), filtersCreateCmd(), filtersUpdateCmd(), filtersDeleteCmd(),
		filterToggleCmd("enable", "Enable filters", ui.Enabled,
			func(c *kit.Invocation, id string) error { return c.App.Mail.FilterEnable(c.Ctx, id) }),
		filterToggleCmd("disable", "Disable filters", ui.Disabled,
			func(c *kit.Invocation, id string) error { return c.App.Mail.FilterDisable(c.Ctx, id) }),
	)
	return c
}

func filtersListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your filters",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepAuth}, func(c *kit.Invocation) error {
			filters, err := c.App.Mail.FiltersList(c.Ctx)
			if err != nil {
				return err
			}
			return kit.List(c, ui.TableSpec[mailsvc.Filter]{
				Noun:  "filters",
				Total: ui.Unknown, Page: ui.Unpaged,
				Columns: []ui.Column[mailsvc.Filter]{
					{Header: "ID", ID: true, Cell: func(f mailsvc.Filter) string { return f.ID }},
					{Header: "NAME", Flex: true, Cell: func(f mailsvc.Filter) string { return f.Name }},
					{Header: "ENABLED", Cell: func(f mailsvc.Filter) string { return yesNo(f.Status == 1) }},
					{Header: "VERSION", Right: true, Cell: func(f mailsvc.Filter) string {
						return strconv.Itoa(f.Version)
					}},
				},
			}, filters, func(f mailsvc.Filter) []string { return []string{f.ID} })
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
			script, err := kit.ReadTextArg(sieve, "--sieve")
			if err != nil {
				return err
			}
			if name == "" || script == "" {
				return kit.Fail("A filter needs a name and a script.").
					Hint("--name \"Archive invoices\"", "--sieve '…', or --sieve - to read stdin.")
			}
			status := 1
			if disabled {
				status = 0
			}
			return kit.Create(c, ui.ResultSpec{
				Action: ui.Created, Kind: "filters", Name: name,
			}, func() (string, error) {
				return c.App.Mail.FilterCreate(c.Ctx, name, script, status)
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
			script, err := kit.ReadTextArg(sieve, "--sieve")
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

func filtersDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete REF...",
		Short: "Delete filters",
		Args:  cobra.MinimumNArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand}, func(c *kit.Invocation) error {
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Deleted, Kind: "filters", Count: len(c.Args), IDs: c.Args,
			}, func() error {
				for _, id := range c.Args {
					if err := c.App.Mail.FilterDelete(c.Ctx, id); err != nil {
						return err
					}
				}
				return nil
			})
		}),
	}
}

func filterToggleCmd(use, short string, action ui.Action, apply func(*kit.Invocation, string) error) *cobra.Command {
	return &cobra.Command{
		Use:   use + " REF...",
		Short: short,
		Args:  cobra.MinimumNArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand}, func(c *kit.Invocation) error {
			return kit.Mutate(c, ui.ResultSpec{
				Action: action, Kind: "filters", Count: len(c.Args), IDs: c.Args,
			}, func() error {
				for _, id := range c.Args {
					if err := apply(c, id); err != nil {
						return err
					}
				}
				return nil
			})
		}),
	}
}
