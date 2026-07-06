package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/roman-16/proton-cli/internal/render"
	drivesvc "github.com/roman-16/proton-cli/internal/service/drive"
	"github.com/roman-16/proton-cli/internal/units"
	"github.com/roman-16/proton-cli/internal/view"
	"github.com/spf13/cobra"
)

func driveItemsCmd() *cobra.Command {
	c := &cobra.Command{Use: "items", Short: "Manage files and folders"}
	c.AddCommand(itemsListCmd(), itemsInfoCmd(), itemsUploadCmd(), itemsDownloadCmd(), itemsRenameCmd(), itemsMoveCmd(), itemsCopyCmd(), itemsTrashCmd(), itemsDeleteCmd(), itemsRevisionsCmd())
	return c
}

func itemsInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use: "info PATH", Short: "Show metadata for a file or folder",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			dc, err := driveCtx(c)
			if err != nil {
				return err
			}
			info, err := c.App.Drive.Info(c.Ctx, dc, c.Args[0])
			if err != nil {
				return err
			}
			if c.R().Format != render.FormatText {
				return c.R().Object(info)
			}
			out := c.R().Stdout
			p := func(k, v string) { _, _ = fmt.Fprintf(out, "%-14s %s\n", k+":", v) }
			p("name", info.Name)
			p("location", info.Location)
			p("type", info.Type)
			if info.MIMEType != "" {
				p("mime_type", info.MIMEType)
			}
			if info.CreatedBy != "" {
				p("created_by", info.CreatedBy)
			}
			p("signature", info.Signature)
			p("uploaded", units.Time(info.Uploaded))
			if info.Modified != 0 {
				p("modified", units.Time(info.Modified))
			}
			p("size", units.Size(info.Size))
			if info.OriginalSize != 0 {
				p("original_size", units.Size(info.OriginalSize))
			}
			if info.SHA1 != "" {
				p("sha1", info.SHA1)
			}
			p("shared", yesNo(info.Shared))
			p("link_id", info.LinkID)
			return nil
		}),
	}
}

func itemsListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list [PATH]", Short: "List folder contents (decrypted names)",
		Args: cobra.MaximumNArgs(1),
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			dc, err := driveCtx(c)
			if err != nil {
				return err
			}
			path := "/"
			if len(c.Args) > 0 {
				path = c.Args[0]
			}
			children, err := c.App.Drive.List(c.Ctx, dc, path)
			if err != nil {
				return err
			}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[drivesvc.Child]{
				Columns: []view.Column[drivesvc.Child]{
					{Header: "TYPE", Cell: func(ch drivesvc.Child) string { return driveTypeLabel(ch.Type) }},
					{Header: "SIZE", Cell: func(ch drivesvc.Child) string { return units.Size(ch.Size) }},
					{Header: "NAME", Cell: func(ch drivesvc.Child) string { return ch.Name }},
					{Header: "LINK_ID", ID: true, Cell: func(ch drivesvc.Child) string { return ch.LinkID }},
				},
				CacheIDs: func(ch drivesvc.Child) []string { return []string{ch.LinkID} },
			}, children)
		}),
	}
}

func itemsUploadCmd() *cobra.Command {
	var recursive bool
	c := &cobra.Command{
		Use: "upload SRC [DEST]", Short: "Upload a file (SRC=- reads from stdin)",
		Args: cobra.RangeArgs(1, 2),
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			dc, err := driveCtx(c)
			if err != nil {
				return err
			}
			src := c.Args[0]
			dest := "/"
			if len(c.Args) >= 2 {
				dest = c.Args[1]
			}
			if recursive {
				if src == "-" {
					return fmt.Errorf("--recursive is not supported with stdin")
				}
				return uploadRecursive(c, dc, src, dest)
			}
			return uploadOne(c, dc, src, dest)
		}),
	}
	c.Flags().BoolVar(&recursive, "recursive", false, "Recursively upload a directory")
	return c
}

func uploadOne(c *Invocation, dc *drivesvc.Context, src, dest string) error {
	var r io.Reader
	var size int64
	var name string
	if src == "-" {
		r = os.Stdin
		name = fmt.Sprintf("stdin-%d", time.Now().Unix())
	} else {
		fi, err := os.Stat(src)
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return fmt.Errorf("%s is a directory (use --recursive)", src)
		}
		f, err := os.Open(src)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		r = f
		size = fi.Size()
		name = filepath.Base(src)
	}
	if c.dryRun("upload %s → %s/%s (%s)", src, dest, name, units.Size(size)) {
		return nil
	}
	if err := c.App.Drive.Upload(c.Ctx, dc, dest, name, r, drivesvc.UploadOptions{
		Label: fmt.Sprintf("Uploading %s", name), Quiet: c.R().Quiet, TotalHint: size,
	}); err != nil {
		return err
	}
	c.R().Success(fmt.Sprintf("Uploaded %s", name))
	return nil
}

func uploadRecursive(c *Invocation, dc *drivesvc.Context, src, dest string) error {
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	baseName := filepath.Base(srcAbs)
	top := filepath.ToSlash(filepath.Join(dest, baseName))
	if !c.App.DryRun {
		if err := c.App.Drive.CreateFolder(c.Ctx, dc, top); err != nil {
			return err
		}
	}
	return filepath.Walk(srcAbs, func(p string, info os.FileInfo, walkErr error) error {
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
		remote := filepath.ToSlash(filepath.Join(top, rel))
		if info.IsDir() {
			if c.App.DryRun {
				c.R().Info("dry-run: would mkdir " + remote)
				return nil
			}
			return c.App.Drive.CreateFolder(c.Ctx, dc, remote)
		}
		remoteParent := filepath.ToSlash(filepath.Dir(remote))
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		if c.dryRun("upload %s → %s (%s)", p, remote, units.Size(info.Size())) {
			return nil
		}
		return c.App.Drive.Upload(c.Ctx, dc, remoteParent, filepath.Base(p), f, drivesvc.UploadOptions{
			Label: "Uploading " + filepath.Base(p), Quiet: c.R().Quiet, TotalHint: info.Size(),
		})
	})
}

func itemsDownloadCmd() *cobra.Command {
	var output, outputDir string
	var force bool
	c := &cobra.Command{
		Use: "download PATH", Short: "Download a file (--output - writes to stdout)",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			dc, err := driveCtx(c)
			if err != nil {
				return err
			}
			src := c.Args[0]
			if c.dryRun("download %s", src) {
				return nil
			}
			if outputDir != "" {
				if err := ensureDir(outputDir); err != nil {
					return err
				}
			}
			target, toStdout, err := pickDownloadPath(filepath.Base(src), output, outputDir, force)
			if err != nil {
				return err
			}
			out := io.Writer(os.Stdout)
			if !toStdout {
				f, err := os.Create(target)
				if err != nil {
					return err
				}
				defer func() { _ = f.Close() }()
				out = f
			}
			if err := c.App.Drive.Download(c.Ctx, dc, src, out, drivesvc.DownloadOptions{
				Label: "Downloading " + filepath.Base(src), Quiet: c.R().Quiet || toStdout,
			}); err != nil {
				return err
			}
			if !toStdout {
				c.R().Success(fmt.Sprintf("Downloaded to %s", target))
			}
			return nil
		}),
	}
	c.Flags().StringVar(&output, "output", "", "Output file (- for stdout); errors on an existing file")
	c.Flags().StringVar(&outputDir, "output-dir", "", "Output directory; uses the file's own name (auto-suffix on collision)")
	c.Flags().BoolVar(&force, "force", false, "Overwrite an existing destination")
	return c
}

func itemsRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use: "rename PATH NEW_NAME", Short: "Rename a file or folder",
		Args: cobra.ExactArgs(2),
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			dc, err := driveCtx(c)
			if err != nil {
				return err
			}
			if c.dryRun("rename %s → %s", c.Args[0], c.Args[1]) {
				return nil
			}
			if err := c.App.Drive.Rename(c.Ctx, dc, c.Args[0], c.Args[1]); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Renamed to %s", c.Args[1]))
			return nil
		}),
	}
}

func itemsMoveCmd() *cobra.Command {
	return &cobra.Command{
		Use: "move SRC DEST_FOLDER", Short: "Move a file or folder",
		Args: cobra.ExactArgs(2),
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			dc, err := driveCtx(c)
			if err != nil {
				return err
			}
			if c.dryRun("move %s → %s", c.Args[0], c.Args[1]) {
				return nil
			}
			if err := c.App.Drive.Move(c.Ctx, dc, c.Args[0], c.Args[1]); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Moved %s → %s", c.Args[0], c.Args[1]))
			return nil
		}),
	}
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

func itemsRevisionsCmd() *cobra.Command {
	c := &cobra.Command{Use: "revisions", Short: "List and restore file revisions"}
	c.AddCommand(&cobra.Command{
		Use: "list PATH", Short: "List a file's revisions",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			dc, err := driveCtx(c)
			if err != nil {
				return err
			}
			revs, err := c.App.Drive.RevisionsList(c.Ctx, dc, c.Args[0])
			if err != nil {
				return err
			}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[drivesvc.Revision]{
				Columns: []view.Column[drivesvc.Revision]{
					{Header: "REVISION_ID", ID: true, Cell: func(r drivesvc.Revision) string { return r.ID }},
					{Header: "STATE", Cell: func(r drivesvc.Revision) string { return revisionState(r.State) }},
					{Header: "SIZE", Cell: func(r drivesvc.Revision) string { return units.Size(r.Size) }},
					{Header: "CREATED", Cell: func(r drivesvc.Revision) string { return units.Time(r.CreateTime) }},
				},
				CacheIDs: func(r drivesvc.Revision) []string { return []string{r.ID} },
			}, revs)
		}),
	})
	c.AddCommand(&cobra.Command{
		Use: "restore PATH REVISION_ID", Short: "Restore a file to an earlier revision",
		Args: cobra.ExactArgs(2),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			dc, err := driveCtx(c)
			if err != nil {
				return err
			}
			if c.dryRun("restore %s to revision %s", c.Args[0], c.Args[1]) {
				return nil
			}
			if err := c.App.Drive.RevisionRestore(c.Ctx, dc, c.Args[0], c.Args[1]); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Restored %s to revision %s", c.Args[0], c.Args[1]))
			return nil
		}),
	})
	return c
}

func itemsCopyCmd() *cobra.Command {
	return &cobra.Command{
		Use: "copy SRC DEST_FOLDER", Short: "Copy a file into another folder",
		Args: cobra.ExactArgs(2),
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			dc, err := driveCtx(c)
			if err != nil {
				return err
			}
			if c.dryRun("copy %s → %s", c.Args[0], c.Args[1]) {
				return nil
			}
			if err := c.App.Drive.Copy(c.Ctx, dc, c.Args[0], c.Args[1]); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Copied %s → %s", c.Args[0], c.Args[1]))
			return nil
		}),
	}
}

func itemsTrashCmd() *cobra.Command {
	return driveRemoveCmd("trash", "Move files or folders to the trash", false)
}

func itemsDeleteCmd() *cobra.Command {
	return driveRemoveCmd("delete", "Permanently delete files or folders", true)
}

// driveRemoveCmd builds the shared items trash/delete command. permanent=false
// moves to the trash (reversible); permanent=true deletes outright.
func driveRemoveCmd(use, short string, permanent bool) *cobra.Command {
	var recursive, all bool
	var pattern, largerThan, scope, olderThan, newerThan string
	c := &cobra.Command{
		Use:   use + " [PATH...]",
		Short: short,
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			dc, err := driveCtx(c)
			if err != nil {
				return err
			}

			filtersSet := pattern != "" || largerThan != "" || all || scope != "" || olderThan != "" || newerThan != ""
			if len(c.Args) == 0 && !filtersSet {
				return fmt.Errorf("no paths selected: pass PATH(s) or a filter (--pattern, --larger-than, --older-than, --newer-than, --scope); use --all with --scope to target an entire subtree")
			}

			targets := append([]string{}, c.Args...)

			if filtersSet {
				if all && scope == "" && pattern == "" && largerThan == "" && olderThan == "" && newerThan == "" {
					return fmt.Errorf("--all requires --scope or a filter (e.g. --scope / to target the whole drive)")
				}
				root := scope
				if root == "" {
					root = "/"
				}
				var minSize int64
				if largerThan != "" {
					n, err := units.ParseSize(largerThan)
					if err != nil {
						return err
					}
					minSize = n
				}
				var olderCutoff, newerCutoff int64
				if olderThan != "" {
					d, err := units.ParseDuration(olderThan)
					if err != nil {
						return fmt.Errorf("invalid --older-than: %w", err)
					}
					olderCutoff = time.Now().Add(-d).Unix()
				}
				if newerThan != "" {
					d, err := units.ParseDuration(newerThan)
					if err != nil {
						return fmt.Errorf("invalid --newer-than: %w", err)
					}
					newerCutoff = time.Now().Add(-d).Unix()
				}
				children, err := c.App.Drive.Walk(c.Ctx, dc, root)
				if err != nil {
					return err
				}
				for _, ch := range children {
					if !recursive && strings.Count(strings.TrimPrefix(ch.Path, root), "/") > 1 {
						continue
					}
					if ch.Type != 2 && (minSize > 0 || olderCutoff != 0 || newerCutoff != 0) {
						continue
					}
					if pattern != "" && !matchGlob(pattern, ch.Name) {
						continue
					}
					if minSize > 0 && ch.Size < minSize {
						continue
					}
					if olderCutoff != 0 && ch.ModifyTime > olderCutoff {
						continue
					}
					if newerCutoff != 0 && ch.ModifyTime < newerCutoff {
						continue
					}
					targets = append(targets, ch.Path)
				}
			}

			targets = dedupe(targets)
			if len(targets) == 0 {
				c.R().Info("Nothing to delete.")
				return nil
			}

			if c.App.DryRun {
				label := "dry-run: would trash"
				if permanent {
					label = "dry-run: would permanently delete"
				}
				c.R().Info(fmt.Sprintf("%s %d item(s):", label, len(targets)))
				for _, t := range targets {
					_, _ = fmt.Fprintln(c.R().Stderr, "  "+t)
				}
				return nil
			}
			for _, p := range targets {
				if err := c.App.Drive.Delete(c.Ctx, dc, p, permanent); err != nil {
					return err
				}
			}
			verb := "Moved to trash"
			if permanent {
				verb = "Permanently deleted"
			}
			c.R().Success(fmt.Sprintf("%s %d item(s)", verb, len(targets)))
			return nil
		}),
	}
	c.Flags().StringVar(&pattern, "pattern", "", "Match items by glob pattern (shell-style, e.g. *.tmp)")
	c.Flags().StringVar(&largerThan, "larger-than", "", "Match files larger than SIZE (e.g. 100MB, 2GB)")
	c.Flags().StringVar(&olderThan, "older-than", "", "Match files not modified within DURATION (e.g. 30d, 2w)")
	c.Flags().StringVar(&newerThan, "newer-than", "", "Match files modified within DURATION")
	c.Flags().StringVar(&scope, "scope", "", "Limit filtered deletion to this subtree (default: /)")
	c.Flags().BoolVar(&recursive, "recursive", false, "Descend into subfolders when applying filters")
	c.Flags().BoolVar(&all, "all", false, "Confirm matching every item in the scope (requires --scope or a filter)")
	return c
}

func driveFoldersCmd() *cobra.Command {
	c := &cobra.Command{Use: "folders", Short: "Manage folders"}
	c.AddCommand(&cobra.Command{
		Use: "create PATH", Short: "Create a folder",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			dc, err := driveCtx(c)
			if err != nil {
				return err
			}
			if c.dryRun("create folder %s", c.Args[0]) {
				return nil
			}
			if err := c.App.Drive.CreateFolder(c.Ctx, dc, c.Args[0]); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Created folder %s", c.Args[0]))
			return nil
		}),
	})
	return c
}
