package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/roman-16/proton-cli/internal/render"
	drivesvc "github.com/roman-16/proton-cli/internal/service/drive"
	"github.com/roman-16/proton-cli/internal/view"
	"github.com/spf13/cobra"
)

func drivePhotosCtx(c *Ctx) (*drivesvc.Context, error) {
	u, err := c.App.Unlock(c.Ctx)
	if err != nil {
		return nil, err
	}
	return c.App.Drive.ResolvePhotos(c.Ctx, u)
}

func photoTagsLabel(tags []int) string {
	if len(tags) == 0 {
		return "-"
	}
	parts := make([]string, len(tags))
	for i, t := range tags {
		parts[i] = strconv.Itoa(t)
	}
	return strings.Join(parts, ",")
}

func drivePhotosCmd() *cobra.Command {
	c := &cobra.Command{Use: "photos", Short: "Photo library operations"}
	c.AddCommand(photosListCmd(), photosDownloadCmd(), photosUploadCmd(), photosDeleteCmd(), photosAlbumsCmd(), photosTagsCmd())
	return c
}

func photosDeleteCmd() *cobra.Command {
	var permanent bool
	cmd := &cobra.Command{
		Use: "delete PHOTO_LINK_ID...", Short: "Move photos to the trash (or purge with --permanent)",
		Args: cobra.MinimumNArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Ctx) error {
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would %s %d photo(s)", map[bool]string{true: "permanently delete", false: "trash"}[permanent], len(c.Args)))
				return nil
			}
			if err := c.App.Drive.PhotosDelete(c.Ctx, dc, c.Args, permanent); err != nil {
				return err
			}
			verb := "Trashed"
			if permanent {
				verb = "Deleted"
			}
			c.R().Success(fmt.Sprintf("%s %d photo(s).", verb, len(c.Args)))
			return nil
		}),
	}
	cmd.Flags().BoolVar(&permanent, "permanent", false, "Permanently delete (purge from trash) instead of just trashing")
	return cmd
}

func photosListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List photos",
		RunE: run([]Step{stepAuth}, func(c *Ctx) error {
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			photos, err := c.App.Drive.PhotosList(c.Ctx, dc)
			if err != nil {
				return err
			}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[drivesvc.Photo]{
				Columns: []view.Column[drivesvc.Photo]{
					{Header: "CAPTURED", Cell: func(p drivesvc.Photo) string { return render.Time(p.CaptureTime) }},
					{Header: "TAGS", Cell: func(p drivesvc.Photo) string { return photoTagsLabel(p.Tags) }},
					{Header: "LINK_ID", ID: true, Cell: func(p drivesvc.Photo) string { return p.LinkID }},
				},
				CacheIDs: func(p drivesvc.Photo) []string { return []string{p.LinkID} },
			}, photos)
		}),
	}
}

func photosDownloadCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use: "download LINK_ID", Short: "Download a photo by link ID",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Ctx) error {
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			dir := "."
			dest := out
			if fi, statErr := os.Stat(out); out == "" || (statErr == nil && fi.IsDir()) {
				if out != "" {
					dir = out
				}
				dest = ""
			}
			tmp, err := os.CreateTemp(dir, ".pcli-photo-*")
			if err != nil {
				return err
			}
			tmpName := tmp.Name()
			name, derr := c.App.Drive.PhotoDownload(c.Ctx, dc, c.Args[0], tmp, drivesvc.DownloadOptions{Label: "Downloading", Quiet: c.R().Quiet})
			_ = tmp.Close()
			if derr != nil {
				_ = os.Remove(tmpName)
				return derr
			}
			if dest == "" {
				dest = filepath.Join(dir, name)
			}
			if err := os.Rename(tmpName, dest); err != nil {
				_ = os.Remove(tmpName)
				return err
			}
			c.R().Success(fmt.Sprintf("Downloaded %s", dest))
			return nil
		}),
	}
	cmd.Flags().StringVar(&out, "out", "", "Output file or directory (default: current directory, original name)")
	return cmd
}

func photosUploadCmd() *cobra.Command {
	return &cobra.Command{
		Use: "upload FILE", Short: "Upload a photo to the library",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth}, func(c *Ctx) error {
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			path := c.Args[0]
			fi, err := os.Stat(path)
			if err != nil {
				return err
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would upload photo %s (%s)", path, render.Size(fi.Size())))
				return nil
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()
			name := filepath.Base(path)
			if err := c.App.Drive.PhotoUpload(c.Ctx, dc, name, f, fi.ModTime().Unix(), drivesvc.UploadOptions{
				Label: "Uploading " + name, Quiet: c.R().Quiet, TotalHint: fi.Size(),
			}); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Uploaded %s", name))
			return nil
		}),
	}
}

func photosAlbumsCmd() *cobra.Command {
	c := &cobra.Command{Use: "albums", Short: "Manage photo albums"}

	c.AddCommand(&cobra.Command{
		Use: "list", Short: "List albums",
		RunE: run([]Step{stepAuth}, func(c *Ctx) error {
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			albums, err := c.App.Drive.AlbumsList(c.Ctx, dc)
			if err != nil {
				return err
			}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[drivesvc.Album]{
				Columns: []view.Column[drivesvc.Album]{
					{Header: "PHOTOS", Cell: func(a drivesvc.Album) string { return strconv.Itoa(a.PhotoCount) }},
					{Header: "NAME", Cell: func(a drivesvc.Album) string { return a.Name }},
					{Header: "LINK_ID", ID: true, Cell: func(a drivesvc.Album) string { return a.LinkID }},
				},
				CacheIDs: func(a drivesvc.Album) []string { return []string{a.LinkID} },
			}, albums)
		}),
	})

	var name string
	create := &cobra.Command{
		Use: "create", Short: "Create an album",
		RunE: run([]Step{stepAuth}, func(c *Ctx) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would create album %q", name))
				return nil
			}
			if err := c.App.Drive.AlbumCreate(c.Ctx, dc, name); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Created album %q", name))
			return nil
		}),
	}
	create.Flags().StringVar(&name, "name", "", "Album name")
	c.AddCommand(create)

	var deletePhotos bool
	del := &cobra.Command{
		Use: "delete ALBUM_LINK_ID", Short: "Delete an album",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Ctx) error {
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would delete album %s", c.Args[0]))
				return nil
			}
			if err := c.App.Drive.AlbumDelete(c.Ctx, dc, c.Args[0], deletePhotos); err != nil {
				return err
			}
			c.R().Success("Album deleted.")
			return nil
		}),
	}
	del.Flags().BoolVar(&deletePhotos, "delete-photos", false, "Also trash the album's photos")
	c.AddCommand(del)

	c.AddCommand(&cobra.Command{
		Use: "items ALBUM_LINK_ID", Short: "List photos in an album",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Ctx) error {
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			photos, err := c.App.Drive.AlbumItems(c.Ctx, dc, c.Args[0])
			if err != nil {
				return err
			}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[drivesvc.Photo]{
				Columns: []view.Column[drivesvc.Photo]{
					{Header: "CAPTURED", Cell: func(p drivesvc.Photo) string { return render.Time(p.CaptureTime) }},
					{Header: "TAGS", Cell: func(p drivesvc.Photo) string { return photoTagsLabel(p.Tags) }},
					{Header: "LINK_ID", ID: true, Cell: func(p drivesvc.Photo) string { return p.LinkID }},
				},
				CacheIDs: func(p drivesvc.Photo) []string { return []string{p.LinkID} },
			}, photos)
		}),
	})

	c.AddCommand(&cobra.Command{
		Use: "add ALBUM_LINK_ID PHOTO_LINK_ID...", Short: "Add photos to an album",
		Args: cobra.MinimumNArgs(2),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Ctx) error {
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would add %d photo(s) to album %s", len(c.Args)-1, c.Args[0]))
				return nil
			}
			if err := c.App.Drive.AlbumAddPhotos(c.Ctx, dc, c.Args[0], c.Args[1:]); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Added %d photo(s) to album.", len(c.Args)-1))
			return nil
		}),
	})

	c.AddCommand(&cobra.Command{
		Use: "remove ALBUM_LINK_ID PHOTO_LINK_ID...", Short: "Remove photos from an album",
		Args: cobra.MinimumNArgs(2),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Ctx) error {
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would remove %d photo(s) from album %s", len(c.Args)-1, c.Args[0]))
				return nil
			}
			if err := c.App.Drive.AlbumRemovePhotos(c.Ctx, dc, c.Args[0], c.Args[1:]); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Removed %d photo(s) from album.", len(c.Args)-1))
			return nil
		}),
	})

	return c
}

func photosTagsCmd() *cobra.Command {
	c := &cobra.Command{Use: "tags", Short: "Manage photo tags"}
	c.AddCommand(&cobra.Command{
		Use: "remove PHOTO_LINK_ID TAG...", Short: "Remove classification tags from a photo",
		Args: cobra.MinimumNArgs(2),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Ctx) error {
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			tags := make([]int, 0, len(c.Args)-1)
			for _, a := range c.Args[1:] {
				t, err := strconv.Atoi(a)
				if err != nil {
					return fmt.Errorf("tag must be an integer: %q", a)
				}
				tags = append(tags, t)
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would remove %d tag(s) from %s", len(tags), c.Args[0]))
				return nil
			}
			if err := c.App.Drive.PhotoTagsRemove(c.Ctx, dc, c.Args[0], tags); err != nil {
				return err
			}
			c.R().Success("Tags removed.")
			return nil
		}),
	})
	return c
}
