//go:build windows

package selfmanage

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// removeBinary renames the running executable out of the way, then launches a
// detached helper that waits for this process to exit and deletes the leftover.
// Windows locks a mapped image against deletion, but renaming it on the same
// volume is allowed, so the install location is freed immediately.
func removeBinary(path string) error {
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("proton-uninstall-%d.exe", os.Getpid()))
	_ = os.Remove(tmp)
	if err := os.Rename(path, tmp); err != nil {
		return err
	}
	scheduleSelfDelete(tmp)
	return nil
}

func scheduleSelfDelete(path string) {
	script := fmt.Sprintf(
		"Wait-Process -Id %s -ErrorAction SilentlyContinue; Remove-Item -LiteralPath %s -Force -ErrorAction SilentlyContinue",
		strconv.Itoa(os.Getpid()), quotePowerShell(path))
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NO_WINDOW,
	}
	_ = cmd.Start()
}

// quotePowerShell wraps s in a single-quoted PowerShell literal, doubling any
// embedded single quotes.
func quotePowerShell(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
