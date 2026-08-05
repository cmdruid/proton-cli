package cli

import (
	"fmt"

	mailsvc "github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/roman-16/proton-cli/internal/view"
	"github.com/spf13/cobra"
)

// ── mail settings filters ──

func mailFiltersCmd() *cobra.Command {
	c := &cobra.Command{Use: "filters", Short: "Manage Sieve filters"}
	c.AddCommand(filtersListCmd(), filtersCreateCmd(), filtersUpdateCmd(),
		filterVerbCmd("delete", "Delete a filter", "Deleted filter %s.",
			func(c *Invocation, id string) error { return c.App.Mail.FilterDelete(c.Ctx, id) }),
		filterVerbCmd("enable", "Enable a filter", "Enabled filter %s.",
			func(c *Invocation, id string) error { return c.App.Mail.FilterEnable(c.Ctx, id) }),
		filterVerbCmd("disable", "Disable a filter", "Disabled filter %s.",
			func(c *Invocation, id string) error { return c.App.Mail.FilterDisable(c.Ctx, id) }),
	)
	return c
}

func filtersListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List filters",
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			filters, err := c.App.Mail.FiltersList(c.Ctx)
			if err != nil {
				return err
			}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[mailsvc.Filter]{
				Columns: []view.Column[mailsvc.Filter]{
					{Header: "ID", ID: true, Cell: func(f mailsvc.Filter) string { return f.ID }},
					{Header: "STATUS", Cell: func(f mailsvc.Filter) string { return enabledText(f.Status) }},
					{Header: "NAME", Cell: func(f mailsvc.Filter) string { return f.Name }},
					{Header: "VERSION", Cell: func(f mailsvc.Filter) string { return fmt.Sprintf("%d", f.Version) }},
				},
				CacheIDs: func(f mailsvc.Filter) []string { return []string{f.ID} },
			}, filters)
		}),
	}
}

func filtersCreateCmd() *cobra.Command {
	var name, sieve string
	var disabled bool
	c := &cobra.Command{
		Use: "create", Short: "Create a Sieve filter",
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			script, err := readTextArg(sieve, "--sieve")
			if err != nil {
				return err
			}
			if name == "" || script == "" {
				return fmt.Errorf("--name and --sieve are required (--sieve - reads stdin)")
			}
			if c.dryRun("create filter %q", name) {
				return nil
			}
			status := 1
			if disabled {
				status = 0
			}
			id, err := c.App.Mail.FilterCreate(c.Ctx, name, script, status)
			if err != nil {
				return err
			}
			c.R().ID(id, fmt.Sprintf("Created filter %q", name))
			return nil
		}),
	}
	c.Flags().StringVar(&name, "name", "", "Filter name")
	c.Flags().StringVar(&sieve, "sieve", "", "Sieve script (use - for stdin)")
	c.Flags().BoolVar(&disabled, "disabled", false, "Create the filter without enabling it")
	return c
}

func filtersUpdateCmd() *cobra.Command {
	var name, sieve string
	c := &cobra.Command{
		Use: "update FILTER_ID", Short: "Update a filter's name or Sieve script",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			script, err := readTextArg(sieve, "--sieve")
			if err != nil {
				return err
			}
			if name == "" && script == "" {
				return fmt.Errorf("nothing to update: pass --name or --sieve")
			}
			if c.dryRun("update filter %s", c.Args[0]) {
				return nil
			}
			if err := c.App.Mail.FilterUpdate(c.Ctx, c.Args[0], name, script); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Updated filter %s.", c.Args[0]))
			return nil
		}),
	}
	c.Flags().StringVar(&name, "name", "", "New filter name")
	c.Flags().StringVar(&sieve, "sieve", "", "New Sieve script (use - for stdin)")
	return c
}

func filterVerbCmd(use, short, successFmt string, fn func(*Invocation, string) error) *cobra.Command {
	return &cobra.Command{
		Use: use + " FILTER_ID", Short: short,
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			id := c.Args[0]
			if c.dryRun("%s filter %s", use, id) {
				return nil
			}
			if err := fn(c, id); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf(successFmt, id))
			return nil
		}),
	}
}

func enabledText(status int) string {
	if status == 1 {
		return "enabled"
	}
	return "disabled"
}
