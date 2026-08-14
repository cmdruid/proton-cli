// Package drive is the `proton-cli drive` tree.
//
// Drive addresses things two ways, and both are real. A file or folder that
// exists in the tree is named by its PATH, because that is how a person thinks
// about it and how Proton's own API resolves it. Something with no place in the
// tree - a trashed item, a photo, an album - is named by REF, its link ID, which
// shortens on a terminal like every other reference.
package drive

import (
	"github.com/roman-16/proton-cli/internal/cli/kit"
	drivesvc "github.com/roman-16/proton-cli/internal/service/drive"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	c := &cobra.Command{
		Use:   "drive",
		Short: "Files and folders in Drive",
	}
	c.AddCommand(itemsCmd(), foldersCmd(), trashCmd(), shareCmd(), invitationsCmd(),
		photosCmd(), settingsCmd())
	return c
}

// context opens the Drive volume. Every command needs it, and it is memoised on
// the service, so asking for it repeatedly is free.
func context(c *kit.Invocation) (*drivesvc.Context, error) {
	return c.App.Drive.Resolve(c.Ctx)
}

// photosContext opens the photo volume, which Proton keeps separate from the
// file tree.
func photosContext(c *kit.Invocation) (*drivesvc.Context, error) {
	return c.App.Drive.ResolvePhotos(c.Ctx)
}

// itemType names what a link is, in Proton's own words rather than in the API's
// integers. No padding: alignment is the table's job, not the value's.
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
