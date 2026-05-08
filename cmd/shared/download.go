package shared

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// WriteMode controls how WriteFileSafe handles a pre-existing destination.
type WriteMode int

const (
	// WriteError fails with an error if path exists. Use when the user
	// supplied an explicit destination — silent overwrite would be bad.
	WriteError WriteMode = iota

	// WriteAutoSuffix tries name_1.ext, name_2.ext, … on collision (browser
	// convention). Use when the destination was synthesised from an inbound
	// name (e.g. an attachment's own filename), where the user has not
	// pinned a specific path.
	WriteAutoSuffix

	// WriteForce overwrites unconditionally.
	WriteForce
)

// MaxSuffix is the largest integer SuffixedName will try before giving up.
const MaxSuffix = 1000

// WriteFileSafe writes data to path according to mode. Returns the path
// actually written, which may differ from the input path under WriteAutoSuffix.
//
// Parent directories are NOT created — callers are responsible for ensuring
// the parent exists (or for creating it explicitly with os.MkdirAll).
func WriteFileSafe(path string, data []byte, perm os.FileMode, mode WriteMode) (string, error) {
	target := path
	switch mode {
	case WriteForce:
		// no-op; os.WriteFile overwrites
	case WriteAutoSuffix:
		if exists(path) {
			suffixed, err := SuffixedName(path)
			if err != nil {
				return "", err
			}
			target = suffixed
		}
	case WriteError:
		if exists(path) {
			return "", fmt.Errorf("destination %s exists; use --force to overwrite", path)
		}
	default:
		return "", fmt.Errorf("unknown write mode %d", mode)
	}
	if err := os.WriteFile(target, data, perm); err != nil {
		return "", err
	}
	return target, nil
}

// SuffixedName returns the first path of the form name_N.ext (1 ≤ N ≤
// MaxSuffix) that does not exist on disk. Splits on the LAST '.' so that
// "archive.tar.gz" yields "archive.tar_1.gz", matching browser behaviour.
// Names without an extension get a plain "_N" appended.
//
// Returns an error when MaxSuffix is exhausted without finding a free name.
func SuffixedName(path string) (string, error) {
	dir, base := filepath.Split(path)
	stem, ext := splitLastDot(base)
	for i := 1; i <= MaxSuffix; i++ {
		var candidate string
		if ext == "" {
			candidate = filepath.Join(dir, fmt.Sprintf("%s_%d", stem, i))
		} else {
			candidate = filepath.Join(dir, fmt.Sprintf("%s_%d.%s", stem, i, ext))
		}
		if !exists(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no free suffix for %s after %d attempts", path, MaxSuffix)
}

// splitLastDot splits "archive.tar.gz" into ("archive.tar", "gz"). Hidden
// files like ".bashrc" are kept whole (returned as ("",".bashrc")? No — we
// treat the leading dot as part of the stem, so ".bashrc" → ("", ".bashrc")).
// Specifically: a leading dot does NOT count as an extension separator, so
// ".bashrc" yields stem=".bashrc", ext="".
func splitLastDot(name string) (stem, ext string) {
	i := strings.LastIndex(name, ".")
	if i <= 0 {
		// No dot, or dot at index 0 (hidden file with no extension).
		return name, ""
	}
	return name[:i], name[i+1:]
}

// exists reports whether path resolves to an existing filesystem entry.
// Non-IsNotExist errors are treated as "exists" defensively.
func exists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return !errors.Is(err, fs.ErrNotExist)
}
