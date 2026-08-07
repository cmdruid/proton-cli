package drive

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/roman-16/proton-cli/internal/cli/kit"
	drivesvc "github.com/roman-16/proton-cli/internal/service/drive"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/roman-16/proton-cli/internal/units"
	"github.com/spf13/cobra"
)

// Photos live on their own volume, so they are addressed by REF - their link ID -
// rather than by path.

func photosCmd() *cobra.Command {
	c := &cobra.Command{Use: "photos", Short: "Your photo library"}
	c.AddCommand(photosListCmd(), photosUploadCmd(), photosDownloadCmd(),
		photoTagVerb("favorite", "Mark photos as favourites", ui.Favorited),
		photoTagVerb("unfavorite", "Remove photos from favourites", ui.Unfavorited),
		photosRemoveCmd("trash", "Move photos to the trash", ui.Trashed, false),
		photosRemoveCmd("delete", "Delete photos permanently", ui.Deleted, true),
		albumsCmd())
	return c
}

func photoColumns() []ui.Column[drivesvc.Photo] {
	return []ui.Column[drivesvc.Photo]{
		{Header: "ID", ID: true, Cell: func(p drivesvc.Photo) string { return p.LinkID }},
		{Header: "CAPTURED", Cell: func(p drivesvc.Photo) string { return units.Time(p.CaptureTime) }},
		{Header: "TAGS", Flex: true, Cell: func(p drivesvc.Photo) string {
			return strings.Join(p.Tags, ", ")
		}},
	}
}

func photosListCmd() *cobra.Command {
	var album string
	tag := &kit.Enum{
		Name: "tag", Usage: "Show only photos with this tag",
		Values: []string{"favorites", "screenshots", "videos", "live-photos",
			"motion-photos", "selfies", "portraits", "bursts", "panoramas", "raw"},
	}
	c := &cobra.Command{
		Use:   "list",
		Short: "List photos",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepAuth}, func(c *kit.Invocation) error {
			dc, err := photosContext(c)
			if err != nil {
				return err
			}
			// An album is a place to look, so it is a flag on list rather than a
			// verb of its own.
			if album != "" {
				ref, err := kit.Expand(c.App, album)
				if err != nil {
					return err
				}
				photos, err := c.App.Drive.AlbumItems(c.Ctx, dc, ref)
				if err != nil {
					return err
				}
				return listPhotos(c, photos)
			}
			tagID, filter := 0, false
			if tag.Set() {
				name, err := tag.Value()
				if err != nil {
					return err
				}
				id, err := drivesvc.ParseTag(name)
				if err != nil {
					return err
				}
				tagID, filter = id, true
			}
			photos, err := c.App.Drive.PhotosList(c.Ctx, dc, tagID, filter)
			if err != nil {
				return err
			}
			return listPhotos(c, photos)
		}),
	}
	tag.Register(c)
	c.Flags().StringVar(&album, "album", "", "Show only what is in this album, by ID")
	return c
}

func listPhotos(c *kit.Invocation, photos []drivesvc.Photo) error {
	return kit.List(c, ui.TableSpec[drivesvc.Photo]{
		Noun: "photos", Columns: photoColumns(),
		Total: ui.Unknown, Page: ui.Unpaged,
	}, photos, func(p drivesvc.Photo) []string { return []string{p.LinkID} })
}

func photosUploadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upload SRC",
		Short: "Upload a photo to the library",
		Args:  cobra.ExactArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepAuth}, func(c *kit.Invocation) error {
			dc, err := photosContext(c)
			if err != nil {
				return err
			}
			src := c.Args[0]
			fi, err := os.Stat(src)
			if err != nil {
				return err
			}
			name := filepath.Base(src)
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Uploaded, Count: 1, Name: name,
				Detail: "to your photo library", Extra: map[string]any{"size": fi.Size()},
			}, func() error {
				f, err := os.Open(src)
				if err != nil {
					return err
				}
				defer func() { _ = f.Close() }()
				return c.App.Drive.PhotoUpload(c.Ctx, dc, name, f, fi.ModTime().Unix(),
					drivesvc.UploadOptions{
						Label: "Uploading " + name, Progress: ui.NewProgress(c.UI()),
						TotalHint: fi.Size(),
					})
			})
		}),
	}
}

func photosDownloadCmd() *cobra.Command {
	var dest kit.Destination
	c := &cobra.Command{
		Use:   "download REF",
		Short: "Download a photo",
		Args:  cobra.ExactArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand}, func(c *kit.Invocation) error {
			if err := dest.Validate(true); err != nil {
				return err
			}
			dc, err := photosContext(c)
			if err != nil {
				return err
			}
			if dest.Stdout() {
				_, err := c.App.Drive.PhotoDownload(c.Ctx, dc, c.Args[0], c.UI().Out,
					drivesvc.DownloadOptions{Label: "Downloading"})
				return err
			}
			// A photo's own filename only becomes known once the download starts,
			// so the bytes land in a temporary file beside the destination and are
			// renamed when the name - and a collision-free path - is known.
			var final string
			if err := kit.Mutate(c, ui.ResultSpec{
				Action: ui.Downloaded, Kind: "photos", Count: 1,
				Detail: "to " + dest.Describe(),
			}, func() error {
				dir, err := dest.Dir()
				if err != nil {
					return err
				}
				tmp, err := os.CreateTemp(dir, ".proton-cli-photo-*")
				if err != nil {
					return err
				}
				name, derr := c.App.Drive.PhotoDownload(c.Ctx, dc, c.Args[0], tmp,
					drivesvc.DownloadOptions{Label: "Downloading", Progress: ui.NewProgress(c.UI())})
				_ = tmp.Close()
				if derr != nil {
					_ = os.Remove(tmp.Name())
					return derr
				}
				target, perr := dest.Reserve(name)
				if perr != nil {
					_ = os.Remove(tmp.Name())
					return perr
				}
				if err := os.Rename(tmp.Name(), target); err != nil {
					_ = os.Remove(tmp.Name())
					return err
				}
				final = target
				return nil
			}); err != nil {
				return err
			}
			_ = final
			return nil
		}),
	}
	dest.Register(c)
	return c
}

func photoTagVerb(use, short string, action ui.Action) *cobra.Command {
	return &cobra.Command{
		Use:   use + " REF...",
		Short: short,
		Args:  cobra.MinimumNArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand}, func(c *kit.Invocation) error {
			dc, err := photosContext(c)
			if err != nil {
				return err
			}
			var copied int
			if err := kit.Mutate(c, ui.ResultSpec{
				Action: action, Kind: "photos", Count: len(c.Args), IDs: c.Args,
			}, func() error {
				if use == "favorite" {
					n, err := c.App.Drive.PhotosFavorite(c.Ctx, dc, c.Args)
					copied = n
					return err
				}
				return c.App.Drive.PhotosUnfavorite(c.Ctx, dc, c.Args)
			}); err != nil {
				return err
			}
			if copied > 0 {
				// A photo shared into an album is not yours to tag, so Proton
				// copies it into your timeline first. Saying so avoids a surprise
				// duplicate later.
				c.Note("%s had to be copied into your own timeline first.",
					ui.Quantity(copied, "photos"))
			}
			return nil
		}),
	}
}

func photosRemoveCmd(use, short string, action ui.Action, permanent bool) *cobra.Command {
	return &cobra.Command{
		Use:   use + " REF...",
		Short: short,
		Args:  cobra.MinimumNArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand}, func(c *kit.Invocation) error {
			dc, err := photosContext(c)
			if err != nil {
				return err
			}
			detail := ""
			if !permanent {
				detail = "to trash"
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: action, Kind: "photos", Count: len(c.Args), IDs: c.Args, Detail: detail,
			}, func() error {
				return c.App.Drive.PhotosDelete(c.Ctx, dc, c.Args, permanent)
			})
		}),
	}
}

// ── albums ──

func albumsCmd() *cobra.Command {
	c := &cobra.Command{Use: "albums", Short: "Photo albums"}
	c.AddCommand(albumsListCmd(), albumsCreateCmd(), albumsDeleteCmd(),
		albumMembersCmd("add", "Put photos into an album", ui.Added, "into"),
		albumMembersCmd("remove", "Take photos out of an album", ui.Removed, "from"))
	return c
}

func albumsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List albums",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepAuth}, func(c *kit.Invocation) error {
			dc, err := photosContext(c)
			if err != nil {
				return err
			}
			albums, err := c.App.Drive.AlbumsList(c.Ctx, dc)
			if err != nil {
				return err
			}
			return kit.List(c, ui.TableSpec[drivesvc.Album]{
				Noun:  "albums",
				Total: ui.Unknown, Page: ui.Unpaged,
				Columns: []ui.Column[drivesvc.Album]{
					{Header: "ID", ID: true, Cell: func(a drivesvc.Album) string { return a.LinkID }},
					{Header: "NAME", Flex: true, Cell: func(a drivesvc.Album) string { return a.Name }},
					{Header: "PHOTOS", Right: true, Cell: func(a drivesvc.Album) string {
						return strconv.Itoa(a.PhotoCount)
					}},
				},
			}, albums, func(a drivesvc.Album) []string { return []string{a.LinkID} })
		}),
	}
}

func albumsCreateCmd() *cobra.Command {
	var name string
	c := &cobra.Command{
		Use:   "create",
		Short: "Create an album",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepAuth}, func(c *kit.Invocation) error {
			dc, err := photosContext(c)
			if err != nil {
				return err
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Created, Kind: "albums", Count: 1, Name: name,
			}, func() error { return c.App.Drive.AlbumCreate(c.Ctx, dc, name) })
		}),
	}
	c.Flags().StringVar(&name, "name", "", "Name for the new album")
	_ = c.MarkFlagRequired("name")
	return c
}

func albumsDeleteCmd() *cobra.Command {
	var withPhotos bool
	c := &cobra.Command{
		Use:   "delete REF...",
		Short: "Delete albums",
		Args:  cobra.MinimumNArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand}, func(c *kit.Invocation) error {
			dc, err := photosContext(c)
			if err != nil {
				return err
			}
			detail := "keeping their photos"
			if withPhotos {
				detail = "and trashing their photos"
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Deleted, Kind: "albums", Count: len(c.Args), IDs: c.Args, Detail: detail,
			}, func() error {
				for _, id := range c.Args {
					if err := c.App.Drive.AlbumDelete(c.Ctx, dc, id, withPhotos); err != nil {
						return err
					}
				}
				return nil
			})
		}),
	}
	c.Flags().BoolVar(&withPhotos, "delete-photos", false, "Also move the album's photos to the trash")
	return c
}

// albumMembersCmd builds `albums add` and `albums remove`. The album comes first
// because the command lives in the albums collection.
func albumMembersCmd(use, short string, action ui.Action, preposition string) *cobra.Command {
	return &cobra.Command{
		Use:   use + " REF PHOTO_REF...",
		Short: short,
		Args:  cobra.MinimumNArgs(2),
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand}, func(c *kit.Invocation) error {
			dc, err := photosContext(c)
			if err != nil {
				return err
			}
			album, photos := c.Args[0], kit.Dedupe(c.Args[1:])
			return kit.Mutate(c, ui.ResultSpec{
				Action: action, Kind: "photos", Count: len(photos), IDs: photos,
				Detail: preposition + " the album",
			}, func() error {
				if use == "add" {
					return c.App.Drive.AlbumAddPhotos(c.Ctx, dc, album, photos)
				}
				return c.App.Drive.AlbumRemovePhotos(c.Ctx, dc, album, photos)
			})
		}),
	}
}
