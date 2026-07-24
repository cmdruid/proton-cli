//go:build !windows

package selfmanage

import "os"

// removeBinary unlinks the executable. Unlinking a running binary is allowed on
// Unix: the process continues from the already-open inode.
func removeBinary(path string) error {
	return os.Remove(path)
}
