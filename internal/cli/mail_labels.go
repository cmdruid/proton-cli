package cli

import (
	"fmt"

	"github.com/roman-16/proton-cli/internal/render"
	mailsvc "github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/roman-16/proton-cli/internal/view"
	"github.com/spf13/cobra"
)

// ── mail labels ──

func labelsCmd() *cobra.Command {
	c := &cobra.Command{Use: "labels", Short: "Manage labels and folders"}
	c.AddCommand(&cobra.Command{
		Use: "list", Short: "List labels and folders",
		RunE: run([]Step{stepAuth}, func(c *Ctx) error {
			labels, folders, err := c.App.Mail.LabelsList(c.Ctx)
			if err != nil {
				return err
			}
			if c.R().Format != render.FormatText {
				return c.R().Object(map[string]any{"Labels": labels, "Folders": folders})
			}
			type row struct {
				id, kind, name, color, path string
			}
			var rows []row
			ids := make([]string, 0, len(folders)+len(labels))
			for _, l := range folders {
				rows = append(rows, row{l.ID, "FOLDER", l.Name, l.Color, l.Path})
				ids = append(ids, l.ID)
			}
			for _, l := range labels {
				rows = append(rows, row{l.ID, "LABEL", l.Name, l.Color, ""})
				ids = append(ids, l.ID)
			}
			if c.App.IDCache != nil && len(ids) > 0 {
				_ = c.App.IDCache.Save(ids...)
			}
			return view.Render(c.R(), c.short(), nil, view.List[row]{
				Columns: []view.Column[row]{
					{Header: "ID", ID: true, Cell: func(r row) string { return r.id }},
					{Header: "TYPE", Cell: func(r row) string { return r.kind }},
					{Header: "NAME", Cell: func(r row) string { return r.name }},
					{Header: "COLOR", Cell: func(r row) string { return r.color }},
					{Header: "PATH", Cell: func(r row) string { return r.path }},
				},
			}, rows)
		}),
	})
	var createName, createColor, createParent string
	var createFolder bool
	createCmd := &cobra.Command{
		Use: "create", Short: "Create a label or folder",
		RunE: run([]Step{stepAuth}, func(c *Ctx) error {
			if createName == "" {
				return fmt.Errorf("--name is required")
			}
			if err := validateAccentColor(createColor); err != nil {
				return err
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would create label %q", createName))
				return nil
			}
			id, err := c.App.Mail.LabelCreate(c.Ctx, createName, createColor, createFolder, createParent)
			if err != nil {
				return err
			}
			kind := "Label"
			if createFolder {
				kind = "Folder"
			}
			c.R().ID(id, fmt.Sprintf("Created %s %q", kind, createName))
			return nil
		}),
	}
	createCmd.Flags().StringVar(&createName, "name", "", "Label name")
	createCmd.Flags().StringVar(&createColor, "color", "#8080FF", "Label color (hex; must be a Proton accent color)")
	createCmd.Flags().BoolVar(&createFolder, "folder", false, "Create a folder instead of a label")
	createCmd.Flags().StringVar(&createParent, "parent", "", "Parent folder ID (folders only)")
	c.AddCommand(createCmd)

	var updName, updColor, updParent string
	updateCmd := &cobra.Command{
		Use: "update LABEL_ID", Short: "Update a label or folder (name, color, parent)",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Ctx) error {
			if updName == "" && updColor == "" && updParent == "" {
				return fmt.Errorf("nothing to update: pass --name, --color, or --parent")
			}
			if err := validateAccentColor(updColor); err != nil {
				return err
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would update label %s", c.Args[0]))
				return nil
			}
			if err := c.App.Mail.LabelUpdate(c.Ctx, c.Args[0], updName, updColor, updParent); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Updated label %s.", c.Args[0]))
			return nil
		}),
	}
	updateCmd.Flags().StringVar(&updName, "name", "", "New name")
	updateCmd.Flags().StringVar(&updColor, "color", "", "New color (hex; must be a Proton accent color)")
	updateCmd.Flags().StringVar(&updParent, "parent", "", "New parent folder ID (folders only)")
	c.AddCommand(updateCmd)

	c.AddCommand(&cobra.Command{
		Use: "delete LABEL_ID...", Short: "Delete labels or folders",
		Args: cobra.MinimumNArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Ctx) error {
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would delete %d label(s)", len(c.Args)))
				return nil
			}
			if err := c.App.Mail.LabelDelete(c.Ctx, c.Args); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Deleted %d label(s)/folder(s).", len(c.Args)))
			return nil
		}),
	})
	return c
}

// ── mail filters ──

func filtersCmd() *cobra.Command {
	c := &cobra.Command{Use: "filters", Short: "Manage Sieve filters"}
	c.AddCommand(&cobra.Command{
		Use: "list", Short: "List filters",
		RunE: run([]Step{stepAuth}, func(c *Ctx) error {
			filters, err := c.App.Mail.FiltersList(c.Ctx)
			if err != nil {
				return err
			}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[mailsvc.Filter]{
				Columns: []view.Column[mailsvc.Filter]{
					{Header: "ID", ID: true, Cell: func(f mailsvc.Filter) string { return f.ID }},
					{Header: "STATUS", Cell: func(f mailsvc.Filter) string {
						if f.Status == 1 {
							return "enabled"
						}
						return "disabled"
					}},
					{Header: "NAME", Cell: func(f mailsvc.Filter) string { return f.Name }},
					{Header: "VERSION", Cell: func(f mailsvc.Filter) string { return fmt.Sprintf("%d", f.Version) }},
				},
				CacheIDs: func(f mailsvc.Filter) []string { return []string{f.ID} },
			}, filters)
		}),
	})
	var fName, fSieve string
	var fStatus int
	createCmd := &cobra.Command{
		Use: "create", Short: "Create a sieve filter",
		RunE: run([]Step{stepAuth}, func(c *Ctx) error {
			if fName == "" || fSieve == "" {
				return fmt.Errorf("--name and --sieve are required")
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would create filter %q", fName))
				return nil
			}
			id, err := c.App.Mail.FilterCreate(c.Ctx, fName, fSieve, fStatus)
			if err != nil {
				return err
			}
			c.R().ID(id, fmt.Sprintf("Created filter %q", fName))
			return nil
		}),
	}
	createCmd.Flags().StringVar(&fName, "name", "", "Filter name")
	createCmd.Flags().StringVar(&fSieve, "sieve", "", "Sieve script")
	createCmd.Flags().IntVar(&fStatus, "status", 1, "Status (1=enabled, 0=disabled)")
	c.AddCommand(createCmd)

	var fuName, fuSieve string
	updateCmd := &cobra.Command{
		Use: "update FILTER_ID", Short: "Update a filter's name or sieve script",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Ctx) error {
			if fuName == "" && fuSieve == "" {
				return fmt.Errorf("nothing to update: pass --name or --sieve")
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would update filter %s", c.Args[0]))
				return nil
			}
			if err := c.App.Mail.FilterUpdate(c.Ctx, c.Args[0], fuName, fuSieve); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Updated filter %s.", c.Args[0]))
			return nil
		}),
	}
	updateCmd.Flags().StringVar(&fuName, "name", "", "New filter name")
	updateCmd.Flags().StringVar(&fuSieve, "sieve", "", "New sieve script")
	c.AddCommand(updateCmd)

	c.AddCommand(filterSingleArg("delete", "Delete a filter", func(c *Ctx, id string) error { return c.App.Mail.FilterDelete(c.Ctx, id) }, "Deleted filter %s."))
	c.AddCommand(filterSingleArg("enable", "Enable a filter", func(c *Ctx, id string) error { return c.App.Mail.FilterEnable(c.Ctx, id) }, "Enabled filter %s."))
	c.AddCommand(filterSingleArg("disable", "Disable a filter", func(c *Ctx, id string) error { return c.App.Mail.FilterDisable(c.Ctx, id) }, "Disabled filter %s."))
	return c
}

func filterSingleArg(use, short string, fn func(c *Ctx, id string) error, successFmt string) *cobra.Command {
	return &cobra.Command{
		Use: use + " FILTER_ID", Short: short,
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Ctx) error {
			id := c.Args[0]
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would %s filter %s", use, id))
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

// ── mail addresses ──

func addressesCmd() *cobra.Command {
	c := &cobra.Command{Use: "addresses", Short: "Manage email addresses"}
	c.AddCommand(&cobra.Command{
		Use: "list", Short: "List email addresses on the account",
		RunE: run([]Step{stepAuth}, func(c *Ctx) error {
			addrs, err := c.App.Mail.AddressesList(c.Ctx)
			if err != nil {
				return err
			}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[mailsvc.Address]{
				Columns: []view.Column[mailsvc.Address]{
					{Header: "ID", ID: true, Cell: func(a mailsvc.Address) string { return a.ID }},
					{Header: "EMAIL", Cell: func(a mailsvc.Address) string { return a.Email }},
					{Header: "DISPLAY_NAME", Cell: func(a mailsvc.Address) string { return a.DisplayName }},
					{Header: "STATUS", Cell: func(a mailsvc.Address) string {
						if a.Status == 1 {
							return "active"
						}
						return "disabled"
					}},
					{Header: "TYPE", Cell: func(a mailsvc.Address) string { return addressType(a.Type) }},
				},
				CacheIDs: func(a mailsvc.Address) []string { return []string{a.ID} },
			}, addrs)
		}),
	})
	return c
}

func addressType(t int) string {
	switch t {
	case 1:
		return "original"
	case 2:
		return "alias"
	case 3:
		return "custom"
	case 4:
		return "premium"
	case 5:
		return "external"
	}
	return "unknown"
}
