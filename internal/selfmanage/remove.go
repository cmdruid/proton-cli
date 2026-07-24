package selfmanage

import "os"

// Remove deletes the proton-cli binary at exePath. On Unix the running file is
// unlinked directly (a process keeps running from the open inode); on Windows
// it is renamed aside and a detached helper deletes it once this process exits.
//
// When dedicatedDir is non-empty it is removed too, but only if it became empty
// - it names a directory that belongs solely to this install (the Windows
// installer's own folder), never a shared bin directory.
func Remove(exePath, dedicatedDir string) error {
	if err := removeBinary(exePath); err != nil {
		return err
	}
	if dedicatedDir != "" {
		_ = os.Remove(dedicatedDir) // best-effort: succeeds only when empty
	}
	return nil
}
