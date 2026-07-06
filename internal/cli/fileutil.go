package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// writeMode controls how writeFileSafe handles a pre-existing destination.
type writeMode int

const (
	// writeError fails if the path exists (user supplied an explicit dest).
	writeError writeMode = iota
	// writeAutoSuffix tries name_1.ext, name_2.ext, … on collision.
	writeAutoSuffix
	// writeForce overwrites unconditionally.
	writeForce
)

const maxSuffix = 1000

// writeFileSafe writes data to path according to mode, returning the path
// actually written (may differ under writeAutoSuffix). Parent dirs are not
// created.
func writeFileSafe(path string, data []byte, perm os.FileMode, mode writeMode) (string, error) {
	target := path
	switch mode {
	case writeForce:
	case writeAutoSuffix:
		if fileExists(path) {
			suffixed, err := suffixedName(path)
			if err != nil {
				return "", err
			}
			target = suffixed
		}
	case writeError:
		if fileExists(path) {
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

// suffixedName returns the first free path of the form name_N.ext. Splits on
// the LAST '.' so "archive.tar.gz" → "archive.tar_1.gz".
func suffixedName(path string) (string, error) {
	dir, base := filepath.Split(path)
	stem, ext := splitLastDot(base)
	for i := 1; i <= maxSuffix; i++ {
		var candidate string
		if ext == "" {
			candidate = filepath.Join(dir, fmt.Sprintf("%s_%d", stem, i))
		} else {
			candidate = filepath.Join(dir, fmt.Sprintf("%s_%d.%s", stem, i, ext))
		}
		if !fileExists(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no free suffix for %s after %d attempts", path, maxSuffix)
}

// splitLastDot splits "archive.tar.gz" → ("archive.tar","gz"). A leading dot
// is part of the stem (".bashrc" → (".bashrc","")).
func splitLastDot(name string) (stem, ext string) {
	i := strings.LastIndex(name, ".")
	if i <= 0 {
		return name, ""
	}
	return name[:i], name[i+1:]
}

// pickDownloadPath resolves where a download writes under the shared download
// model, returning (path, stdout). "--output -" is stdout; "--output PATH" is an
// explicit file (errors on an existing file unless force); otherwise the item's
// own name is written into --output-dir, or the current directory, auto-suffixed
// on collision unless force.
func pickDownloadPath(name, output, outputDir string, force bool) (path string, stdout bool, err error) {
	if output == "-" {
		return "", true, nil
	}
	if output != "" {
		if !force && fileExists(output) {
			return "", false, fmt.Errorf("destination %s exists; use --force to overwrite", output)
		}
		return output, false, nil
	}
	path = filepath.Join(outputDir, name) // outputDir may be empty (current dir)
	if !force && fileExists(path) {
		s, serr := suffixedName(path)
		if serr != nil {
			return "", false, serr
		}
		path = s
	}
	return path, false, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return !errors.Is(err, fs.ErrNotExist)
}
