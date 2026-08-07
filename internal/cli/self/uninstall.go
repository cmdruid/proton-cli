package self

import (
	"fmt"
	"github.com/roman-16/proton-cli/internal/cli/kit"
	"os"
	"path/filepath"
	"runtime"

	"github.com/roman-16/proton-cli/internal/account/session"
	"github.com/roman-16/proton-cli/internal/selfmanage"
	"github.com/spf13/cobra"
)

// UninstallCmd removes a manually installed binary.
func UninstallCmd() *cobra.Command {
	var yes, purge bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove a curl/PowerShell-installed proton-cli",
		Long: `Remove a proton-cli binary that was installed with the curl or PowerShell
installer (or downloaded manually).

It refuses to touch a package-managed install (apt, dnf, apk, AUR,
Homebrew, winget, npm, Nix) and tells you the right command instead.

Only the binary is removed by default. Pass --purge to also delete local
data (saved sessions and the ID cache) under your config directory.

Examples:
  proton-cli uninstall          # preview what would be removed
  proton-cli uninstall --yes    # remove the binary
  proton-cli uninstall --yes --purge   # also remove local data`,
		Args: cobra.NoArgs,
		RunE: kit.Run(nil, func(c *kit.Invocation) error {
			return runUninstall(c, yes, purge)
		}),
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Actually remove (without it, prints what would be removed)")
	cmd.Flags().BoolVar(&purge, "purge", false, "Also remove local data (saved sessions and ID cache)")
	return cmd
}

type uninstallResult struct {
	Binary  string `json:"binary"`
	Data    string `json:"data,omitempty"`
	Removed bool   `json:"removed"`
}

func runUninstall(c *kit.Invocation, yes, purge bool) error {
	u := c.UI()

	exe, err := resolveExe()
	if err != nil {
		return err
	}
	if err := guardManaged(exe, actionUninstall); err != nil {
		return err
	}

	dataDir := ""
	if purge {
		if d, err := session.Dir(); err == nil {
			dataDir = d
		}
	}

	confirmed := yes && !c.App.DryRun
	if !confirmed {
		if u.Format.Machine() {
			return kit.Object(c, uninstallResult{Binary: exe, Data: dataDir, Removed: false})
		}
		u.Note("Would remove:")
		u.Note("  " + exe)
		if dataDir != "" {
			u.Note("  " + dataDir + " (saved sessions, ID cache)")
		}
		if !c.App.DryRun {
			u.Note("Re-run with --yes to remove.")
		}
		return nil
	}

	if err := selfmanage.Remove(exe, dedicatedDir(exe)); err != nil {
		return selfManageError(err, exe, actionUninstall)
	}
	if dataDir != "" {
		if err := os.RemoveAll(dataDir); err != nil {
			u.Note(fmt.Sprintf("warning: could not remove %s: %v", dataDir, err))
		}
	}

	if u.Format.Machine() {
		return kit.Object(c, uninstallResult{Binary: exe, Data: dataDir, Removed: true})
	}
	u.Note("Removed " + exe)
	if runtime.GOOS == "windows" {
		u.Note("A temporary copy will be cleaned up after this process exits.")
	}
	if dataDir != "" {
		u.Note("Removed " + dataDir)
		u.Note("Sign out this session from Proton (web/app) to invalidate any tokens it held.")
	} else {
		u.Note("Saved sessions and cache were left in place (use --purge to remove them).")
	}
	return nil
}

// dedicatedDir returns the install directory when it belongs solely to
// proton-cli (the Windows installer's ...\Programs\proton-cli), so uninstall
// can remove it once empty. Shared bin directories (e.g. ~/.local/bin) yield "".
func dedicatedDir(exe string) string {
	if runtime.GOOS != "windows" {
		return ""
	}
	dir := filepath.Dir(exe)
	if filepath.Base(dir) == "proton-cli" {
		return dir
	}
	return ""
}
