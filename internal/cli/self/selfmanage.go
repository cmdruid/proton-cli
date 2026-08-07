package self

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/roman-16/proton-cli/internal/selfmanage"
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
		return "", fmt.Errorf("locate executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// guardManaged refuses to modify a package-managed install, naming the right
// command for the requested action instead.
func guardManaged(exe string, action selfAction) error {
	kind := selfmanage.Classify(exe)
	if kind == selfmanage.KindStandalone {
		return nil
	}
	via, cmd := managedHint(kind, action)
	if kind == selfmanage.KindNix {
		return fmt.Errorf("proton-cli was installed via %s (%s); %s", via, exe, cmd)
	}
	return fmt.Errorf("proton-cli was installed via %s; %s", via, cmd)
}

// managedHint maps a package-managed install to a human name and the command
// that performs the requested action through that manager.
func managedHint(kind selfmanage.Kind, action selfAction) (via, cmd string) {
	update := action == actionUpdate
	switch kind {
	case selfmanage.KindNix:
		if update {
			return "Nix", "update it through your flake or nixpkgs configuration"
		}
		return "Nix", "remove it through your flake or nixpkgs configuration"
	case selfmanage.KindHomebrew:
		if update {
			return "Homebrew", "run `brew upgrade --cask proton-cli`"
		}
		return "Homebrew", "run `brew uninstall --cask proton-cli`"
	case selfmanage.KindNpm:
		if update {
			return "npm", "run `npm update -g @roman-16/proton-cli`"
		}
		return "npm", "run `npm uninstall -g @roman-16/proton-cli`"
	case selfmanage.KindWinget:
		if update {
			return "winget", "run `winget upgrade Roman-16.ProtonCLI`"
		}
		return "winget", "run `winget uninstall Roman-16.ProtonCLI`"
	}
	return "", ""
}

// permissionHint names the likely package manager for the current OS, used when
// an operation is refused because the target is not user-writable (a system or
// root-owned install).
func permissionHint(action selfAction) string {
	switch runtime.GOOS {
	case "darwin":
		if action == actionUninstall {
			return "Homebrew (`brew uninstall --cask proton-cli`)"
		}
		return "Homebrew (`brew upgrade --cask proton-cli`)"
	case "windows":
		if action == actionUninstall {
			return "winget (`winget uninstall Roman-16.ProtonCLI`)"
		}
		return "winget (`winget upgrade Roman-16.ProtonCLI`)"
	default:
		if action == actionUninstall {
			return "your distribution's package manager (e.g. sudo pacman -R proton-cli, apt remove proton-cli, dnf remove proton-cli, apk del proton-cli)"
		}
		return "your distribution's package manager (pacman/AUR, apt, dnf, apk, ...)"
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
		return fmt.Errorf("cannot %s %s: permission denied. It looks like a system or package-managed "+
			"install; use %s", verb, exe, permissionHint(action))
	}
	if action == actionUninstall {
		return fmt.Errorf("remove %s: %w", exe, err)
	}
	return fmt.Errorf("install update: %w", err)
}
