package cli

import (
	"fmt"

	"github.com/roman-16/proton-cli/internal/render"
	drivesvc "github.com/roman-16/proton-cli/internal/service/drive"
	"github.com/roman-16/proton-cli/internal/units"
	"github.com/roman-16/proton-cli/internal/view"
	"github.com/spf13/cobra"
)

func driveTrashCmd() *cobra.Command {
	c := &cobra.Command{Use: "trash", Short: "Manage the drive trash"}
	c.AddCommand(&cobra.Command{
		Use: "list", Short: "List trashed items",
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			dc, err := driveCtx(c)
			if err != nil {
				return err
			}
			entries, err := c.App.Drive.TrashList(c.Ctx, dc)
			if err != nil {
				return err
			}
			if c.R().Format == render.FormatText && len(entries) == 0 {
				c.R().Info("(trash is empty)")
				return nil
			}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[drivesvc.TrashEntry]{
				Columns: []view.Column[drivesvc.TrashEntry]{
					{Header: "LINK_ID", ID: true, Cell: func(e drivesvc.TrashEntry) string { return e.LinkID }},
					{Header: "TYPE", Cell: func(e drivesvc.TrashEntry) string { return driveTypeLabel(e.Type) }},
					{Header: "SIZE", Cell: func(e drivesvc.TrashEntry) string { return units.Size(e.Size) }},
				},
				CacheIDs: func(e drivesvc.TrashEntry) []string { return []string{e.LinkID} },
			}, entries)
		}),
	})
	c.AddCommand(&cobra.Command{
		Use: "restore LINK_ID...", Short: "Restore items from trash (IDs only)",
		Args: cobra.MinimumNArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			dc, err := driveCtx(c)
			if err != nil {
				return err
			}
			if c.dryRun("restore %d item(s)", len(c.Args)) {
				return nil
			}
			if err := c.App.Drive.TrashRestore(c.Ctx, dc, c.Args); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Restored %d item(s)", len(c.Args)))
			return nil
		}),
	})
	c.AddCommand(&cobra.Command{
		Use: "empty", Short: "Empty the trash",
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			dc, err := driveCtx(c)
			if err != nil {
				return err
			}
			if c.dryRun("empty trash") {
				return nil
			}
			if err := c.App.Drive.TrashEmpty(c.Ctx, dc); err != nil {
				return err
			}
			c.R().Success("Trash emptied.")
			return nil
		}),
	})
	return c
}
