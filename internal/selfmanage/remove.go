package selfmanage

import "os"

// Remove deletes an install: every name in aliases, the program at exePath, and
// dedicatedDir when it became empty.
//
// On Unix the running file is unlinked directly (a process keeps running from
// the open inode); on Windows it is renamed aside and a detached helper deletes
// it once this process exits.
//
// dedicatedDir names a directory that belongs solely to this install (the
// Windows installer's own folder), never a shared bin directory, and is removed
// only if nothing else is left in it.
func Remove(exePath string, aliases []string, dedicatedDir string) error {
	for _, alias := range aliases {
		if err := os.Remove(alias); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := removeBinary(exePath); err != nil {
		return err
	}
	if dedicatedDir != "" {
		_ = os.Remove(dedicatedDir) // best-effort: succeeds only when empty
	}
	return nil
}
