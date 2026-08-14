package drive

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/roman-16/proton-cli/internal/cli/kit"
	"github.com/roman-16/proton-cli/internal/errs"
	drivesvc "github.com/roman-16/proton-cli/internal/service/drive"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/roman-16/proton-cli/internal/units"
	"github.com/spf13/cobra"
)

func itemsCmd() *cobra.Command {
	c := &cobra.Command{Use: "items", Short: "Files and folders"}
	c.AddCommand(itemsListCmd(), itemsGetCmd(), itemsUploadCmd(), itemsDownloadCmd(),
		itemsUpdateCmd(), itemsMoveCmd(), itemsCopyCmd(),
		itemsTrashCmd(), itemsDeleteCmd(), revisionsCmd())
	return c
}

func childColumns() []ui.Column[drivesvc.Child] {
	return []ui.Column[drivesvc.Child]{
		{Header: "ID", ID: true, Cell: func(ch drivesvc.Child) string { return ch.LinkID }},
		{Header: "TYPE", Cell: func(ch drivesvc.Child) string { return ch.Type }},
		{Header: "SIZE", Right: true, Cell: func(ch drivesvc.Child) string {
			// A folder has no size of its own. Blank is how every other column
			// says "nothing here"; a placeholder glyph would read like a value.
			if ch.Type == drivesvc.TypeFolder {
				return ""
			}
			return units.Size(ch.Size)
		}},
		{Header: "MODIFIED", Cell: func(ch drivesvc.Child) string { return units.Time(ch.ModifyTime) }},
		{Header: "NAME", Flex: true, Cell: func(ch drivesvc.Child) string { return ch.Name }},
	}
}

func itemsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list [PATH]",
		Short: "List what is in a folder",
		Args:  cobra.MaximumNArgs(1),
		RunE: kit.Run(nil, func(c *kit.Invocation) error {
			dc, err := context(c)
			if err != nil {
				return err
			}
			at := "/"
			if len(c.Args) > 0 {
				at = c.Args[0]
			}
			children, err := c.App.Drive.List(c.Ctx, dc, at)
			if err != nil {
				return err
			}
			return kit.List(c, ui.TableSpec[drivesvc.Child]{
				Noun: "items", Columns: childColumns(),
				Total: ui.Unknown, Page: ui.Unpaged,
			}, children, func(ch drivesvc.Child) []string { return []string{ch.LinkID} })
		}),
	}
}

func itemsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get PATH",
		Short: "Show a file or folder's details",
		Args:  cobra.ExactArgs(1),
		RunE: kit.Run(nil, func(c *kit.Invocation) error {
			dc, err := context(c)
			if err != nil {
				return err
			}
			info, err := c.App.Drive.Info(c.Ctx, dc, c.Args[0])
			if err != nil {
				return err
			}
			fields := []ui.Field{
				{Label: "Name", Value: info.Name},
				{Label: "Location", Value: info.Location},
				{Label: "Type", Value: info.Type},
				{Label: "MIME Type", Value: info.MIMEType},
				{Label: "Created By", Value: info.CreatedBy},
				{Label: "Signature", Value: info.Signature, Always: true},
				{Label: "Uploaded", Value: units.Time(info.Uploaded)},
				{Label: "Modified", Value: units.Time(info.Modified)},
				{Label: "Size", Value: units.Size(info.Size)},
			}
			if info.OriginalSize != 0 && info.OriginalSize != info.Size {
				fields = append(fields, ui.Field{Label: "Original Size", Value: units.Size(info.OriginalSize)})
			}
			fields = append(fields,
				ui.Field{Label: "SHA-1", Value: info.SHA1},
				ui.Field{Label: "Shared", Value: yesNo(info.Shared), Always: true},
				ui.Field{Label: "ID", Value: info.LinkID, ID: true},
			)
			return kit.Show(c, ui.RecordSpec{Object: info, Fields: fields})
		}),
	}
}

// ── moving bytes ──

func itemsUploadCmd() *cobra.Command {
	var recursive bool
	ifExists := kit.Enum{
		Name:   "if-exists",
		Usage:  "What to do when the folder already has that name",
		Values: []string{"rename", "replace", "skip"},
	}
	c := &cobra.Command{
		Use:   "upload SRC [DEST]",
		Short: "Upload a file or directory",
		Long: "Upload a file or directory.\n\n" +
			"SRC of - reads standard input, which needs DEST to name the file, since a\n" +
			"stream has no name of its own.\n\n" +
			"A name already taken is refused, so nothing is overwritten by accident.\n" +
			"--if-exists answers the question instead:\n\n" +
			"  replace  write the bytes as a new revision, so the file keeps its\n" +
			"           history and `items revisions list` shows both\n" +
			"  rename   keep both, adding a number to the name being uploaded\n" +
			"  skip     leave what is there alone and upload nothing\n\n" +
			"With --recursive the answer applies to every file, and folders already\n" +
			"there are used rather than refused.",
		Args: cobra.RangeArgs(1, 2),
		RunE: kit.Run(nil, func(c *kit.Invocation) error {
			dc, err := context(c)
			if err != nil {
				return err
			}
			src := c.Args[0]
			dest := "/"
			if len(c.Args) >= 2 {
				dest = c.Args[1]
			}
			choice, err := ifExists.Value()
			if err != nil {
				return err
			}
			onConflict := drivesvc.OnConflict(choice)
			if recursive {
				if src == "-" {
					return kit.Fail("--recursive cannot read from standard input.")
				}
				return uploadTree(c, dc, src, dest, onConflict)
			}
			return uploadOne(c, dc, src, dest, onConflict)
		}),
	}
	c.Flags().BoolVar(&recursive, "recursive", false, "Upload a directory and everything under it")
	ifExists.Register(c)
	return c
}

func uploadOne(c *kit.Invocation, dc *drivesvc.Context, src, dest string, on drivesvc.OnConflict) error {
	var r io.Reader
	var size int64
	var name string

	if src == "-" {
		stdin, err := c.App.Stdin("SRC -")
		if err != nil {
			return err
		}
		r = stdin
		name = fmt.Sprintf("stdin-%d", time.Now().Unix())
		// A stream has no name, so DEST carries it: an existing folder receives
		// the generated name, and any other path is parent plus new file name.
		resolved, err := c.App.Drive.ResolvePath(c.Ctx, dc, dest)
		var notFound *errs.NotFound
		if err != nil && !errors.As(err, &notFound) {
			return err
		}
		if err != nil || !resolved.IsFolder {
			name = path.Base(dest)
			dest = path.Dir(dest)
		}
	} else {
		fi, err := os.Stat(src)
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return kit.Fail("%s is a directory.", src).Hint("--recursive to upload it and its contents.")
		}
		f, err := os.Open(src)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		r, size, name = f, fi.Size(), filepath.Base(src)
	}

	plan, err := c.App.Drive.PlanUpload(c.Ctx, dc, dest, name, on)
	if err != nil {
		return err
	}
	// What the plan says is what gets reported, so a skip is a count of nothing
	// rather than a claim to have uploaded something, and a dry run promises the
	// name the file will really end up with.
	spec := ui.ResultSpec{
		Action: ui.Uploaded, Count: 1, Name: plan.Name,
		Detail: "to " + dest, Extra: map[string]any{"size": size},
	}
	switch {
	case plan.Nothing:
		spec.Count = 0
		spec.Detail = fmt.Sprintf("- %s already has %s", dest, name)
	case plan.Revision:
		spec.Detail = "to " + dest + " as a new revision"
	}
	return sayHowToGoAhead(kit.Mutate(c, spec, func() error {
		return c.App.Drive.Upload(c.Ctx, dc, plan, r, drivesvc.UploadOptions{
			Label: "Uploading " + plan.Name, Progress: ui.NewProgress(c.UI()), TotalHint: size,
		})
	}))
}

// sayHowToGoAhead turns Proton's refusal to write over a name into the question
// its own client asks, with the three answers this one takes.
func sayHowToGoAhead(err error) error {
	var exists *errs.Exists
	if !errors.As(err, &exists) {
		return err
	}
	exists.Answers = []string{
		"--if-exists replace to write the bytes as a new revision of it",
		"--if-exists rename to keep both",
		"--if-exists skip to leave it alone",
	}
	return exists
}

// uploadTree mirrors a local directory into Drive, creating folders as it goes.
func uploadTree(c *kit.Invocation, dc *drivesvc.Context, src, dest string, on drivesvc.OnConflict) error {
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	top := filepath.ToSlash(filepath.Join(dest, filepath.Base(srcAbs)))

	// Walk first so the confirmation can report what was actually found, and so a
	// dry run lists it without creating anything.
	type upload struct {
		local  string
		remote string
		dir    bool
		size   int64
	}
	var plan []upload
	if err := filepath.Walk(srcAbs, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == srcAbs {
			return nil
		}
		rel, err := filepath.Rel(srcAbs, p)
		if err != nil {
			return err
		}
		plan = append(plan, upload{
			local:  p,
			remote: filepath.ToSlash(filepath.Join(top, rel)),
			dir:    info.IsDir(),
			size:   info.Size(),
		})
		return nil
	}); err != nil {
		return err
	}

	files := 0
	for _, u := range plan {
		if !u.dir {
			files++
		}
	}
	// A folder that is already there is the container the answer is about, not the
	// thing being written, so with an answer given it is used rather than refused.
	makeFolder := func(path string) error {
		err := c.App.Drive.CreateFolder(c.Ctx, dc, path)
		var exists *errs.Exists
		if on != drivesvc.ConflictRefuse && errors.As(err, &exists) {
			return nil
		}
		return err
	}
	return sayHowToGoAhead(kit.Mutate(c, ui.ResultSpec{
		Action: ui.Uploaded, Kind: "items", Count: files,
		Detail: "to " + top + treeConflictClause(on),
	}, func() error {
		if err := makeFolder(top); err != nil {
			return err
		}
		for _, u := range plan {
			if u.dir {
				if err := makeFolder(u.remote); err != nil {
					return err
				}
				continue
			}
			dir, name := filepath.ToSlash(filepath.Dir(u.remote)), filepath.Base(u.remote)
			filePlan, err := c.App.Drive.PlanUpload(c.Ctx, dc, dir, name, on)
			if err != nil {
				return err
			}
			if filePlan.Nothing {
				continue
			}
			f, err := os.Open(u.local)
			if err != nil {
				return err
			}
			err = c.App.Drive.Upload(c.Ctx, dc, filePlan, f, drivesvc.UploadOptions{
				Label:    "Uploading " + filePlan.Name,
				Progress: ui.NewProgress(c.UI()), TotalHint: u.size,
			})
			_ = f.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}))
}

// treeConflictClause says what a tree upload did about the names already taken,
// since the count alone cannot: it counts what the local tree holds.
func treeConflictClause(on drivesvc.OnConflict) string {
	switch on {
	case drivesvc.ConflictSkip:
		return ", keeping what is already there"
	case drivesvc.ConflictReplace:
		return ", replacing what is already there"
	case drivesvc.ConflictRename:
		return ", keeping both where a name is taken"
	}
	return ""
}

func itemsDownloadCmd() *cobra.Command {
	var dest kit.Destination
	c := &cobra.Command{
		Use:   "download PATH",
		Short: "Download a file",
		Args:  cobra.ExactArgs(1),
		RunE: kit.Run(nil, func(c *kit.Invocation) error {
			if err := dest.Validate(true); err != nil {
				return err
			}
			dc, err := context(c)
			if err != nil {
				return err
			}
			src := c.Args[0]
			name := path.Base(src)

			if dest.Stdout() {
				// Streaming to stdout means the bar would compete with the
				// payload's own consumer for the terminal, so it stays off.
				return c.App.Drive.Download(c.Ctx, dc, src, c.UI().Out, drivesvc.DownloadOptions{
					Label: "Downloading " + name, OnSignatureIssue: signatureIssue(c, name),
				})
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Downloaded, Count: 1, Name: name,
				Detail: "to " + dest.Describe(),
			}, func() error {
				// A download that fails its integrity checks part way through must
				// not leave a plausible-looking file behind, so the bytes land
				// beside the destination and are moved into place at the end.
				_, err := dest.Stream(c, name, func(w io.Writer) error {
					return c.App.Drive.Download(c.Ctx, dc, src, w, drivesvc.DownloadOptions{
						Label: "Downloading " + name, Progress: ui.NewProgress(c.UI()),
						OnSignatureIssue: signatureIssue(c, name),
					})
				})
				return err
			})
		}),
	}
	dest.Register(c)
	return c
}

// signatureIssue reports a block whose author signature does not check out.
//
// The content is already known to be what the revision was signed for, so this is
// not a reason to refuse the file; it is a reason to say who cannot be confirmed as
// having written it. Once said, it is not said again for every remaining block.
func signatureIssue(c *kit.Invocation, name string) func(int, string) {
	reported := false
	return func(index int, verdict string) {
		if reported {
			return
		}
		reported = true
		c.Note("%s downloaded, but the signature on block %d is %s, so who wrote it cannot be confirmed.",
			name, index, verdict)
	}
}

// ── organising ──

func itemsUpdateCmd() *cobra.Command {
	var name string
	c := &cobra.Command{
		Use:   "update PATH",
		Short: "Rename a file or folder",
		Long: "Rename a file or folder.\n\n" +
			"A name is a field like any other, so changing it is `update --name` rather\n" +
			"than a verb of its own. To put something somewhere else, use `move`.",
		Args: cobra.ExactArgs(1),
		RunE: kit.Run(nil, func(c *kit.Invocation) error {
			dc, err := context(c)
			if err != nil {
				return err
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Updated, Count: 1, Name: path.Base(c.Args[0]),
				Detail: "to " + name,
			}, func() error {
				return c.App.Drive.Rename(c.Ctx, dc, c.Args[0], name)
			})
		}),
	}
	c.Flags().StringVar(&name, "name", "", "New name, without a path")
	_ = c.MarkFlagRequired("name")
	return c
}

func itemsMoveCmd() *cobra.Command {
	return relocateCmd("move", "Move files or folders into another folder", ui.Moved,
		func(c *kit.Invocation, dc *drivesvc.Context, src, into string) error {
			return c.App.Drive.Move(c.Ctx, dc, src, into)
		})
}

func itemsCopyCmd() *cobra.Command {
	return relocateCmd("copy", "Copy files into another folder", ui.Copied,
		func(c *kit.Invocation, dc *drivesvc.Context, src, into string) error {
			return c.App.Drive.Copy(c.Ctx, dc, src, into)
		})
}

// relocateCmd builds move and copy, which differ only in whether the original
// stays. Both take the selection model, so a filtered move is as available as a
// filtered trash.
func relocateCmd(use, short string, action ui.Action,
	apply func(*kit.Invocation, *drivesvc.Context, string, string) error) *cobra.Command {
	var f filters
	var into string
	c := &cobra.Command{
		Use:   use + " [PATH...]",
		Short: short,
		RunE: kit.Run([]kit.Step{kit.StepSelection(f.set, filterHint, itemScope)}, func(c *kit.Invocation) error {
			dc, err := context(c)
			if err != nil {
				return err
			}
			sel, err := selectItems(c, dc, &f)
			if err != nil {
				return err
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: action, Kind: "items", Count: sel.Len(), IDs: sel.IDs,
				Detail: "into " + into, Preview: sel.Preview(),
			}, func() error {
				for _, row := range sel.Rows {
					if err := apply(c, dc, row.Path, into); err != nil {
						return err
					}
				}
				return nil
			})
		}),
	}
	c.Flags().StringVar(&into, "into", "", "Destination folder")
	_ = c.MarkFlagRequired("into")
	f.register(c)
	return c
}

func itemsTrashCmd() *cobra.Command {
	return removeCmd("trash", "Move files or folders to the trash", ui.Trashed, false)
}

func itemsDeleteCmd() *cobra.Command {
	return removeCmd("delete", "Delete files or folders permanently", ui.Deleted, true)
}

func removeCmd(use, short string, action ui.Action, permanent bool) *cobra.Command {
	var f filters
	c := &cobra.Command{
		Use:   use + " [PATH...]",
		Short: short,
		RunE: kit.Run([]kit.Step{kit.StepSelection(f.set, filterHint, itemScope)}, func(c *kit.Invocation) error {
			dc, err := context(c)
			if err != nil {
				return err
			}
			sel, err := selectItems(c, dc, &f)
			if err != nil {
				return err
			}
			detail := ""
			if !permanent {
				detail = "to trash"
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: action, Kind: "items", Count: sel.Len(), IDs: sel.IDs,
				Detail: detail, Preview: sel.Preview(),
			}, func() error {
				for _, row := range sel.Rows {
					if err := c.App.Drive.Delete(c.Ctx, dc, row.Path, permanent); err != nil {
						return err
					}
				}
				return nil
			})
		}),
	}
	f.register(c)
	return c
}

// ── revisions ──

func revisionsCmd() *cobra.Command {
	c := &cobra.Command{Use: "revisions", Short: "Earlier versions of a file"}
	c.AddCommand(revisionsListCmd(), revisionsRestoreCmd())
	return c
}

func revisionState(state int) string {
	switch state {
	case 0:
		return "draft"
	case 1:
		return "active"
	case 2:
		return "inactive"
	}
	return "unknown"
}

func revisionsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list PATH",
		Short: "List a file's earlier versions",
		Args:  cobra.ExactArgs(1),
		RunE: kit.Run(nil, func(c *kit.Invocation) error {
			dc, err := context(c)
			if err != nil {
				return err
			}
			revs, err := c.App.Drive.RevisionsList(c.Ctx, dc, c.Args[0])
			if err != nil {
				return err
			}
			return kit.List(c, ui.TableSpec[drivesvc.Revision]{
				Noun:  "revisions",
				Total: ui.Unknown, Page: ui.Unpaged,
				Columns: []ui.Column[drivesvc.Revision]{
					{Header: "ID", ID: true, Cell: func(r drivesvc.Revision) string { return r.ID }},
					{Header: "STATE", Cell: func(r drivesvc.Revision) string { return revisionState(r.State) }},
					{Header: "SIZE", Right: true, Cell: func(r drivesvc.Revision) string { return units.Size(r.Size) }},
					{Header: "CREATED", Cell: func(r drivesvc.Revision) string { return units.Time(r.CreateTime) }},
				},
			}, revs, func(r drivesvc.Revision) []string { return []string{r.ID} })
		}),
	}
}

func revisionsRestoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore PATH REVISION_REF",
		Short: "Restore a file to an earlier version",
		Args:  cobra.ExactArgs(2),
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			dc, err := context(c)
			if err != nil {
				return err
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Restored, Count: 1, Name: path.Base(c.Args[0]),
				Detail: "to an earlier revision", IDs: []string{c.Args[1]},
			}, func() error {
				return c.App.Drive.RevisionRestore(c.Ctx, dc, c.Args[0], c.Args[1])
			})
		}),
	}
}

// ── folders ──

func foldersCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "folders",
		Short: "Creating folders",
		Long: "Creating folders.\n\n" +
			"Every other folder operation is an item operation, because Drive treats files\n" +
			"and folders alike: rename, move, trash and delete all live under `items`.",
	}
	c.AddCommand(&cobra.Command{
		Use:   "create PATH",
		Short: "Create a folder, and any missing folder above it",
		Args:  cobra.ExactArgs(1),
		RunE: kit.Run(nil, func(c *kit.Invocation) error {
			dc, err := context(c)
			if err != nil {
				return err
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Created, Kind: "folders", Count: 1, Name: c.Args[0],
			}, func() error {
				return c.App.Drive.CreateFolder(c.Ctx, dc, c.Args[0])
			})
		}),
	})
	return c
}
