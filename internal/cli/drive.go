package cli

import (
	drivesvc "github.com/roman-16/proton-cli/internal/service/drive"
	"github.com/spf13/cobra"
)

func newDriveCmd() *cobra.Command {
	c := &cobra.Command{Use: "drive", Short: "Drive operations"}
	c.AddCommand(driveItemsCmd(), driveFoldersCmd(), driveTrashCmd(), driveShareCmd(),
		driveInvitationsCmd(), drivePhotosCmd(), driveSettingsCmd())
	return c
}

func driveCtx(c *Invocation) (*drivesvc.Context, error) {
	u, err := c.App.Unlock(c.Ctx)
	if err != nil {
		return nil, err
	}
	return c.App.Drive.Resolve(c.Ctx, u)
}

func driveTypeLabel(t int) string {
	if t == 1 {
		return "DIR "
	}
	return "FILE"
}

// ── drive items ──

// ── drive folders ──

// ── drive share ──

// ── drive invitations ──

// ── drive trash ──
