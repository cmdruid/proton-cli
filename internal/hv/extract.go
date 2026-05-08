package hv

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// extractHelper writes the embedded helper bytes to the user's cache
// directory if not already there, and returns the absolute path to
// the executable. The cached file is content-addressed by sha256 so
// upgrading proton-cli automatically replaces stale helpers.
//
// Returns ErrHelperMissing if no helper was embedded for this OS/arch
// (i.e. the binary was built without -tags embed_hv).
func extractHelper() (string, error) {
	bin := helperBinary // populated by embed_<os>_<arch>.go in tagged builds
	if len(bin) == 0 {
		return "", ErrHelperMissing
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("hv: locating user cache: %w", err)
	}
	dir := filepath.Join(cacheDir, "proton-cli")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("hv: creating cache dir: %w", err)
	}

	// Content-addressed filename: short hash of the embedded bytes.
	// Different binary => different filename, so updates land cleanly
	// next to (rather than overwrite) the old one. Stale files are
	// fine: only the current version is referenced.
	sum := sha256.Sum256(bin)
	short := hex.EncodeToString(sum[:6])
	name := helperFilename(short)
	target := filepath.Join(dir, name)

	if _, err := os.Stat(target); err == nil {
		return target, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("hv: stat helper: %w", err)
	}

	// Atomic write: tempfile + rename. Avoids a half-written helper
	// being exec'd by a concurrent CLI invocation.
	tmp, err := os.CreateTemp(dir, "hv-helper-*.tmp")
	if err != nil {
		return "", fmt.Errorf("hv: creating temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(bin); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("hv: writing helper: %w", err)
	}
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("hv: chmod helper: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("hv: closing helper tmp: %w", err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return "", fmt.Errorf("hv: renaming helper into place: %w", err)
	}
	return target, nil
}

// helperFilename builds the on-disk filename for the cached helper,
// including a content hash and the .exe suffix on Windows.
func helperFilename(hash string) string {
	base := "proton-cli-hv-" + hash
	if runtime.GOOS == "windows" {
		base += ".exe"
	}
	return base
}
