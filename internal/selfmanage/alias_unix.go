//go:build !windows

package selfmanage

import (
	"os"
	"path/filepath"
)

func aliasFiles(name string) []string { return []string{name} }

// The symlink is relative, so moving the install directory keeps it intact.
func linkAlias(target, path string) error {
	return os.Symlink(filepath.Base(target), path)
}
