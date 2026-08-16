//go:build windows

package selfmanage

import (
	"os"
	"path/filepath"
)

func aliasFiles(name string) []string { return []string{name + ".cmd", name + ".exe"} }

// The shim resolves the program relative to its own directory, so moving the
// install keeps it working. cmd.exe exits with the errorlevel of the last
// command it ran, which is what carries the program's exit code back out.
func linkAlias(target, path string) error {
	shim := "@echo off\r\n\"%~dp0" + filepath.Base(target) + "\" %*\r\n"
	return os.WriteFile(path, []byte(shim), 0o755)
}
