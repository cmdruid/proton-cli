package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	drivesvc "github.com/roman-16/proton-cli/internal/service/drive"
	"github.com/roman-16/proton-cli/internal/units"
	"github.com/roman-16/proton-cli/internal/view"
	"github.com/spf13/cobra"
)

func drivePhotosCtx(c *Invocation) (*drivesvc.Context, error) {
	u, err := c.App.Unlock(c.Ctx)
	if err != nil {
		return nil, err
	}
	return c.App.Drive.ResolvePhotos(c.Ctx, u)
}

func photoTagsLabel(tags []string) string {
	if len(tags) == 0 {
		return "-"
	}
	return strings.Join(tags, ",")
}

func drivePhotosCmd() *cobra.Command {
	c := &cobra.Command{Use: "photos", Short: "Photo library operations"}
	c.AddCommand(photosListCmd(), photosDownloadCmd(), photosUploadCmd(), photosTrashCmd(), photosDeleteCmd(), photosFavoriteCmd(), photosUnfavoriteCmd(), photosAlbumsCmd())
	return c
}

func photosFavoriteCmd() *cobra.Command {
	return &cobra.Command{
		Use: "favorite PHOTO_LINK_ID...", Short: "Mark photos as favorite",
		Args: cobra.MinimumNArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			if c.dryRun("favorite %d photo(s)", len(c.Args)) {
				return nil
			}
			copied, err := c.App.Drive.PhotosFavorite(c.Ctx, dc, c.Args)
			if err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Favorited %d photo(s).", len(c.Args)))
			if copied > 0 {
				c.R().Info(fmt.Sprintf("Note: %d photo(s) were copied into your timeline and favorited there.", copied))
			}
			return nil
		}),
	}
}

func photosUnfavoriteCmd() *cobra.Command {
	return &cobra.Command{
		Use: "unfavorite PHOTO_LINK_ID...", Short: "Remove photos from favorites",
		Args: cobra.MinimumNArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			if c.dryRun("unfavorite %d photo(s)", len(c.Args)) {
				return nil
			}
			if err := c.App.Drive.PhotosUnfavorite(c.Ctx, dc, c.Args); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Unfavorited %d photo(s).", len(c.Args)))
			return nil
		}),
	}
}

func photosTrashCmd() *cobra.Command {
	return photosRemoveCmd("trash", "Move photos to the trash", false)
}

func photosDeleteCmd() *cobra.Command {
	return photosRemoveCmd("delete", "Permanently delete photos", true)
}

// photosRemoveCmd builds the shared photos trash/delete command. permanent=false
// moves to the trash (reversible); permanent=true purges outright.
func photosRemoveCmd(use, short string, permanent bool) *cobra.Command {
	return &cobra.Command{
		Use: use + " PHOTO_LINK_ID...", Short: short,
		Args: cobra.MinimumNArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			if c.dryRun("%s %d photo(s)", map[bool]string{true: "permanently delete", false: "trash"}[permanent], len(c.Args)) {
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
}

func photosListCmd() *cobra.Command {
	var tag string
	cmd := &cobra.Command{
		Use: "list", Short: "List photos",
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			tagID, filter := 0, false
			if tag != "" {
				id, err := drivesvc.ParseTag(tag)
				if err != nil {
					return err
				}
				tagID, filter = id, true
			}
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			photos, err := c.App.Drive.PhotosList(c.Ctx, dc, tagID, filter)
			if err != nil {
				return err
			}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[drivesvc.Photo]{
				Columns: []view.Column[drivesvc.Photo]{
					{Header: "CAPTURED", Cell: func(p drivesvc.Photo) string { return units.Time(p.CaptureTime) }},
					{Header: "TAGS", Cell: func(p drivesvc.Photo) string { return photoTagsLabel(p.Tags) }},
					{Header: "LINK_ID", ID: true, Cell: func(p drivesvc.Photo) string { return p.LinkID }},
				},
				CacheIDs: func(p drivesvc.Photo) []string { return []string{p.LinkID} },
			}, photos)
		}),
	}
	cmd.Flags().StringVar(&tag, "tags", "", "Filter by tag: favorites, screenshots, videos, live-photos, motion-photos, selfies, portraits, bursts, panoramas, raw")
	return cmd
}

func photosDownloadCmd() *cobra.Command {
	var output, outputDir string
	var force bool
	cmd := &cobra.Command{
		Use: "download LINK_ID", Short: "Download a photo (--output - writes to stdout)",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			// A photo's own filename is only known after the download, so stream
			// straight to stdout, else download to a temp file next to the final
			// destination and rename once the name (and collision-free path) is known.
			if output == "-" {
				_, err := c.App.Drive.PhotoDownload(c.Ctx, dc, c.Args[0], c.R().Stdout, drivesvc.DownloadOptions{Label: "Downloading", Quiet: true})
				return err
			}
			if outputDir != "" {
				if err := ensureDir(outputDir); err != nil {
					return err
				}
			}
			tempDir := "."
			if output != "" {
				tempDir = filepath.Dir(output)
			} else if outputDir != "" {
				tempDir = outputDir
			}
			tmp, err := os.CreateTemp(tempDir, ".pcli-photo-*")
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
			target, _, perr := pickDownloadPath(name, output, outputDir, force)
			if perr != nil {
				_ = os.Remove(tmpName)
				return perr
			}
			if err := os.Rename(tmpName, target); err != nil {
				_ = os.Remove(tmpName)
				return err
			}
			c.R().Success(fmt.Sprintf("Downloaded %s", target))
			return nil
		}),
	}
	cmd.Flags().StringVar(&output, "output", "", "Output file (- for stdout); errors on an existing file")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory; uses the photo's own name (auto-suffix on collision)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing destination")
	return cmd
}

func photosUploadCmd() *cobra.Command {
	return &cobra.Command{
		Use: "upload FILE", Short: "Upload a photo to the library",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			path := c.Args[0]
			fi, err := os.Stat(path)
			if err != nil {
				return err
			}
			if c.dryRun("upload photo %s (%s)", path, units.Size(fi.Size())) {
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
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
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
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			if c.dryRun("create album %q", name) {
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
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			if c.dryRun("delete album %s", c.Args[0]) {
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
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
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
					{Header: "CAPTURED", Cell: func(p drivesvc.Photo) string { return units.Time(p.CaptureTime) }},
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
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			if c.dryRun("add %d photo(s) to album %s", len(c.Args)-1, c.Args[0]) {
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
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			dc, err := drivePhotosCtx(c)
			if err != nil {
				return err
			}
			if c.dryRun("remove %d photo(s) from album %s", len(c.Args)-1, c.Args[0]) {
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
