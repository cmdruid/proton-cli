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

// EnvHelper names a helper executable to run instead of the embedded one.
//
// It is what lets the helper be a file somebody else owns. A packager can
// install it properly and give it the GTK environment a webview needs, rather
// than putting that environment on the CLI and having every child process the
// CLI spawns - a pager, an editor - inherit it. Somebody on a host where the
// shipped helper cannot render can build one and point at it, which is the
// escape hatch scripts/build-hv-helpers.sh describes.
const EnvHelper = "PROTON_HV_HELPER"

// helperPath returns the absolute path of a helper to run.
//
// The override is read before the embedded bytes, so a build that embeds none -
// anything built without -tags embed_hv, which is what `go install` produces -
// can still verify when it is pointed at one.
func helperPath() (string, error) {
	if named := os.Getenv(EnvHelper); named != "" {
		return namedHelper(named)
	}
	return extractHelper()
}

// namedHelper checks the override before anything tries to run it, so a typo
// reads as a typo rather than as human verification being unavailable.
func namedHelper(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("hv: %s: %w", EnvHelper, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("hv: %s names %s, which cannot be read: %w", EnvHelper, abs, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("hv: %s names %s, which is a directory", EnvHelper, abs)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("hv: %s names %s, which is not executable", EnvHelper, abs)
	}
	return abs, nil
}

// extractHelper writes the embedded helper bytes to the user's cache
// directory if not already there, and returns the absolute path to
// the executable. The cached file is content-addressed by sha256 so
// upgrading proton automatically replaces stale helpers.
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
	base := "proton-hv-" + hash
	if runtime.GOOS == "windows" {
		base += ".exe"
	}
	return base
}
