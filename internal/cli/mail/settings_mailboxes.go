package mail

import (
	"github.com/roman-16/proton-cli/internal/cli/kit"
	mailsvc "github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/spf13/cobra"
)

// Folders and labels are two trees, not one.
//
// Proton's settings page is called "Folders and labels" and shows two separate
// lists, because they are separate things: a message lives in one folder and
// carries any number of labels. Keeping them apart is what stops `move` from
// quietly accepting a label and doing something else.
//
// It also makes the pairing obvious: `move --to` takes what
// `settings folders list` shows, and `label --label` takes what
// `settings labels list` shows.

func foldersCmd() *cobra.Command {
	return mailboxTree("folders", "Folders, which a message lives in", true)
}

func labelsCmd() *cobra.Command {
	return mailboxTree("labels", "Labels, which a message carries", false)
}

func mailboxTree(noun, short string, folder bool) *cobra.Command {
	c := &cobra.Command{Use: noun, Short: short}
	c.AddCommand(
		mailboxListCmd(noun, folder),
		mailboxCreateCmd(noun, folder),
		mailboxUpdateCmd(noun, folder),
		mailboxDeleteCmd(noun),
	)
	return c
}

func mailboxListCmd(noun string, folder bool) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your " + noun,
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepAuth}, func(c *kit.Invocation) error {
			labels, folders, err := c.App.Mail.LabelsList(c.Ctx)
			if err != nil {
				return err
			}
			rows := labels
			cols := []ui.Column[mailsvc.Label]{
				{Header: "ID", ID: true, Cell: func(l mailsvc.Label) string { return l.ID }},
				{Header: "NAME", Flex: true, Cell: func(l mailsvc.Label) string { return l.Name }},
				{Header: "COLOR", Cell: func(l mailsvc.Label) string { return l.Color }},
			}
			if folder {
				rows = folders
				// A folder can nest, so its path is the part a user needs to tell
				// two same-named subfolders apart.
				cols = append(cols, ui.Column[mailsvc.Label]{
					Header: "PATH", Flex: true,
					Cell: func(l mailsvc.Label) string { return l.Path },
				})
			}
			return kit.List(c, ui.TableSpec[mailsvc.Label]{
				Noun: noun, Columns: cols, Total: ui.Unknown, Page: ui.Unpaged,
			}, rows, func(l mailsvc.Label) []string { return []string{l.ID} })
		}),
	}
}

func mailboxCreateCmd(noun string, folder bool) *cobra.Command {
	var name, parent string
	color := &kit.Color{Name: "color", Default: kit.DefaultAccentColor}
	c := &cobra.Command{
		Use:   "create",
		Short: "Create a " + ui.Singular(noun),
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepAuth}, func(c *kit.Invocation) error {
			if name == "" {
				return kit.Fail("A %s needs a name.", ui.Singular(noun)).Hint("--name Work")
			}
			return kit.Create(c, ui.ResultSpec{
				Action: ui.Created, Kind: noun, Name: name,
			}, func() (string, error) {
				return c.App.Mail.LabelCreate(c.Ctx, name, color.Value(), folder, parent)
			})
		}),
	}
	c.Flags().StringVar(&name, "name", "", "Name for the new "+ui.Singular(noun))
	color.Register(c)
	if folder {
		c.Flags().StringVar(&parent, "parent", "", "Put it inside this folder, by ID")
	}
	return c
}

func mailboxUpdateCmd(noun string, folder bool) *cobra.Command {
	var name, parent string
	color := &kit.Color{Name: "color", Usage: "New accent color, as a hex value"}
	c := &cobra.Command{
		Use:   "update REF",
		Short: "Rename or recolor a " + ui.Singular(noun),
		Args:  cobra.ExactArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand}, func(c *kit.Invocation) error {
			if name == "" && !color.Set() && parent == "" {
				return kit.Fail("Nothing to change.").Hint("pass --name or --color.")
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Updated, Kind: noun, Count: 1, Name: name,
				IDs: []string{c.Args[0]},
			}, func() error {
				return c.App.Mail.LabelUpdate(c.Ctx, c.Args[0], name, color.Value(), parent)
			})
		}),
	}
	c.Flags().StringVar(&name, "name", "", "New name")
	color.Register(c)
	if folder {
		c.Flags().StringVar(&parent, "parent", "", "Move it inside this folder, by ID")
	}
	return c
}

func mailboxDeleteCmd(noun string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete REF...",
		Short: "Delete " + noun,
		Args:  cobra.MinimumNArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand}, func(c *kit.Invocation) error {
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Deleted, Kind: noun, Count: len(c.Args), IDs: c.Args,
			}, func() error { return c.App.Mail.LabelDelete(c.Ctx, c.Args) })
		}),
	}
}
