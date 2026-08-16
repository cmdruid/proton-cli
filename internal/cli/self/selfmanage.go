package self

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/roman-16/proton-cli/internal/cli/kit"
	"github.com/roman-16/proton-cli/internal/selfmanage"
	"github.com/roman-16/proton-cli/internal/ui"
)

// selfAction is the operation a self-management command performs on the running
// binary; it selects the right wording in shared guards and hints.
type selfAction int

const (
	actionUpdate selfAction = iota
	actionUninstall
)

// resolveExe returns the path of the running binary with symlinks resolved, so
// the operation acts on the real file rather than a symlink to it.
func resolveExe() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", kit.Fail("Could not locate the running binary: %v", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// aliasesBeside returns every file the install's other name may occupy next to
// exe, the one to create first, or nil when exe is named neither of them and so
// is not an install this tool made.
func aliasesBeside(exe string) []string {
	dir, base := filepath.Split(exe)
	var other string
	switch strings.TrimSuffix(base, filepath.Ext(base)) {
	case kit.Program:
		other = kit.Alias
	case kit.Alias:
		other = kit.Program
	default:
		return nil
	}
	var paths []string
	for _, name := range selfmanage.AliasFiles(other) {
		paths = append(paths, filepath.Join(dir, name))
	}
	return paths
}

// linkAlias puts the install's other name beside exe, and says so when that
// name was not there before, which is the one moment it is worth a line.
func linkAlias(c *kit.Invocation, exe string) {
	paths := aliasesBeside(exe)
	if len(paths) == 0 {
		return
	}
	alias := paths[0]
	_, err := os.Lstat(alias)
	fresh := err != nil
	if err := selfmanage.LinkAlias(exe, alias); err != nil {
		c.Warn("Could not put %s beside %s: %v", filepath.Base(alias), filepath.Base(exe), err)
		return
	}
	if fresh {
		c.Note("%s %s → %s", ui.GlyphSuccess, filepath.Base(alias), filepath.Base(exe))
	}
}

// guardManaged refuses to modify a package-managed install, naming the right
// command for the requested action instead.
//
// The refusal is a problem with a remedy, like every other refusal in the CLI:
// the manager that owns this copy is stated, and what to run instead is a line
// under Try rather than a clause buried in the sentence.
func guardManaged(exe string, action selfAction) error {
	kind := selfmanage.Classify(exe)
	if kind == selfmanage.KindStandalone {
		return nil
	}
	via, remedy := managedHint(kind, action)
	problem := kit.Fail("proton was installed with %s, which owns this copy.", via)
	if kind == selfmanage.KindNix {
		problem = kit.Fail("proton was installed with %s, which owns %s.", via, exe)
	}
	return problem.Hint(remedy)
}

// managedHint maps a package-managed install to a human name and the remedy that
// performs the requested action through that manager.
func managedHint(kind selfmanage.Kind, action selfAction) (via, remedy string) {
	update := action == actionUpdate
	switch kind {
	case selfmanage.KindNix:
		if update {
			return "Nix", "update it through your flake or nixpkgs configuration"
		}
		return "Nix", "remove it through your flake or nixpkgs configuration"
	case selfmanage.KindHomebrew:
		if update {
			return "Homebrew", "brew upgrade --cask proton-cli"
		}
		return "Homebrew", "brew uninstall --cask proton-cli"
	case selfmanage.KindNpm:
		if update {
			return "npm", "npm update -g @roman-16/proton-cli"
		}
		return "npm", "npm uninstall -g @roman-16/proton-cli"
	case selfmanage.KindWinget:
		if update {
			return "winget", "winget upgrade Roman-16.ProtonCLI"
		}
		return "winget", "winget uninstall Roman-16.ProtonCLI"
	}
	return "", ""
}

// permissionRemedies name the likely package manager for the current OS, used
// when an operation is refused because the target is not user-writable (a system
// or root-owned install).
func permissionRemedies(action selfAction) []string {
	uninstall := action == actionUninstall
	switch runtime.GOOS {
	case "darwin":
		if uninstall {
			return []string{"brew uninstall --cask proton-cli"}
		}
		return []string{"brew upgrade --cask proton-cli"}
	case "windows":
		if uninstall {
			return []string{"winget uninstall Roman-16.ProtonCLI"}
		}
		return []string{"winget upgrade Roman-16.ProtonCLI"}
	default:
		if uninstall {
			return []string{
				"use your distribution's package manager, one of:",
				"  sudo pacman -R proton-cli",
				"  sudo apt remove proton-cli",
				"  sudo dnf remove proton-cli",
				"  sudo apk del proton-cli",
			}
		}
		return []string{"update it with the package manager that installed it (pacman/AUR, apt, dnf, apk, …)"}
	}
}

// selfManageError turns a failed replace/remove into an actionable message,
// recognising the package-managed / root-owned case (a non-writable target).
func selfManageError(err error, exe string, action selfAction) error {
	if errors.Is(err, os.ErrPermission) {
		verb := "replace"
		if action == actionUninstall {
			verb = "remove"
		}
		return kit.Fail("Cannot %s %s: permission denied, so this looks like a system or "+
			"package-managed install.", verb, exe).Hint(permissionRemedies(action)...)
	}
	if action == actionUninstall {
		return fmt.Errorf("remove %s: %w", exe, err)
	}
	return fmt.Errorf("install the update: %w", err)
}
