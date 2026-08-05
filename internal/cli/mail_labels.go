package cli

import (
	"fmt"

	"github.com/roman-16/proton-cli/internal/render"
	"github.com/roman-16/proton-cli/internal/view"
	"github.com/spf13/cobra"
)

// ── mail settings labels ──

func mailLabelsCmd() *cobra.Command {
	c := &cobra.Command{Use: "labels", Short: "Manage labels and folders"}
	c.AddCommand(labelsListCmd(), labelsCreateCmd(), labelsUpdateCmd(), labelsDeleteCmd())
	return c
}

// labelRow flattens labels and folders into one table, since they share an ID
// space and a user thinks of them as one list.
type labelRow struct {
	id, kind, name, color, path string
}

func labelsListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List labels and folders",
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			labels, folders, err := c.App.Mail.LabelsList(c.Ctx)
			if err != nil {
				return err
			}
			if c.R().Format != render.FormatText {
				return c.R().Object(map[string]any{"labels": labels, "folders": folders})
			}
			rows := make([]labelRow, 0, len(folders)+len(labels))
			for _, l := range folders {
				rows = append(rows, labelRow{l.ID, "FOLDER", l.Name, l.Color, l.Path})
			}
			for _, l := range labels {
				rows = append(rows, labelRow{l.ID, "LABEL", l.Name, l.Color, ""})
			}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[labelRow]{
				Columns: []view.Column[labelRow]{
					{Header: "ID", ID: true, Cell: func(r labelRow) string { return r.id }},
					{Header: "TYPE", Cell: func(r labelRow) string { return r.kind }},
					{Header: "NAME", Cell: func(r labelRow) string { return r.name }},
					{Header: "COLOR", Cell: func(r labelRow) string { return r.color }},
					{Header: "PATH", Cell: func(r labelRow) string { return r.path }},
				},
				CacheIDs: func(r labelRow) []string { return []string{r.id} },
			}, rows)
		}),
	}
}

func labelsCreateCmd() *cobra.Command {
	var name, color, parent string
	var folder bool
	c := &cobra.Command{
		Use: "create", Short: "Create a label or folder",
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if err := validateAccentColor(color); err != nil {
				return err
			}
			if c.dryRun("create label %q", name) {
				return nil
			}
			id, err := c.App.Mail.LabelCreate(c.Ctx, name, color, folder, parent)
			if err != nil {
				return err
			}
			kind := "Label"
			if folder {
				kind = "Folder"
			}
			c.R().ID(id, fmt.Sprintf("Created %s %q", kind, name))
			return nil
		}),
	}
	c.Flags().StringVar(&name, "name", "", "Label name")
	c.Flags().StringVar(&color, "color", "#8080FF", "Label color (hex; must be a Proton accent color)")
	c.Flags().BoolVar(&folder, "folder", false, "Create a folder instead of a label")
	c.Flags().StringVar(&parent, "parent", "", "Parent folder ID (folders only)")
	return c
}

func labelsUpdateCmd() *cobra.Command {
	var name, color, parent string
	c := &cobra.Command{
		Use: "update LABEL_ID", Short: "Update a label or folder (name, color, parent)",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			if name == "" && color == "" && parent == "" {
				return fmt.Errorf("nothing to update: pass --name, --color, or --parent")
			}
			if err := validateAccentColor(color); err != nil {
				return err
			}
			if c.dryRun("update label %s", c.Args[0]) {
				return nil
			}
			if err := c.App.Mail.LabelUpdate(c.Ctx, c.Args[0], name, color, parent); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Updated label %s.", c.Args[0]))
			return nil
		}),
	}
	c.Flags().StringVar(&name, "name", "", "New name")
	c.Flags().StringVar(&color, "color", "", "New color (hex; must be a Proton accent color)")
	c.Flags().StringVar(&parent, "parent", "", "New parent folder ID (folders only)")
	return c
}

func labelsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use: "delete LABEL_ID...", Short: "Delete labels or folders",
		Args: cobra.MinimumNArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			if c.dryRun("delete %d label(s)", len(c.Args)) {
				return nil
			}
			if err := c.App.Mail.LabelDelete(c.Ctx, c.Args); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Deleted %d label(s)/folder(s).", len(c.Args)))
			return nil
		}),
	}
}
