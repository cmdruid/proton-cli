package self

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/roman-16/proton-cli/internal/account/session"
	"github.com/roman-16/proton-cli/internal/cli/kit"
	"github.com/roman-16/proton-cli/internal/selfmanage"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/spf13/cobra"
)

// UninstallCmd removes a manually installed binary.
//
// It is a mutation like any other, so it reports through kit.Mutate and inherits
// what that guarantees: --dry-run describes it without doing it, and being
// unable to take it back is what makes it stop for a yes. There is no local
// --yes here; the global one means "proceed without asking" everywhere,
// including here.
func UninstallCmd() *cobra.Command {
	var purge bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove a curl/PowerShell-installed proton-cli",
		Long: `Remove a proton-cli binary that was installed with the curl or PowerShell
installer (or downloaded manually).

It refuses to touch a package-managed install (apt, dnf, apk, AUR,
Homebrew, winget, npm, Nix) and tells you the right command instead.

Only the binary is removed by default. Pass --purge to also delete local
data (saved sessions and the ID cache) under your config directory.`,
		Args: cobra.NoArgs,
		RunE: kit.Run(nil, func(c *kit.Invocation) error {
			return runUninstall(c, purge)
		}),
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "Also remove local data (saved sessions and ID cache)")
	return cmd
}

func runUninstall(c *kit.Invocation, purge bool) error {
	exe, err := resolveExe()
	if err != nil {
		return err
	}
	// A package-managed install is refused before anything else, so the question
	// is never asked about a removal that was never going to happen.
	if err := guardManaged(exe, actionUninstall); err != nil {
		return err
	}

	dataDir := ""
	if purge {
		if d, err := session.Dir(); err == nil {
			dataDir = d
		}
	}
	detail := ""
	if dataDir != "" {
		detail = "with its saved sessions and ID cache"
	}

	if err := kit.Mutate(c, ui.ResultSpec{
		Action: ui.Uninstalled, Count: 1, Name: exe, Detail: detail,
		Extra: map[string]any{"binary": exe, "data": dataDir},
	}, func() error {
		if err := selfmanage.Remove(exe, dedicatedDir(exe)); err != nil {
			return selfManageError(err, exe, actionUninstall)
		}
		if dataDir != "" {
			if err := os.RemoveAll(dataDir); err != nil {
				c.Warn("Could not remove %s: %v", dataDir, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if c.App.DryRun {
		return nil
	}

	if runtime.GOOS == "windows" {
		c.Note("A temporary copy will be cleaned up after this process exits.")
	}
	// What is left behind is the part worth stopping on: the binary is gone, so
	// nothing here can remove the credential afterwards.
	if dataDir != "" {
		c.Warn("The session this machine held is still valid at Proton.\n" +
			"Sign it out from any Proton app to invalidate the tokens it carried.")
	} else {
		if d, err := session.Dir(); err == nil {
			c.Warn("Your saved session is still on this machine, at %s.\n"+
				"Re-run with --purge to remove it, and sign the session out from any Proton app.", d)
		}
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
