package selfmanage

import (
	"encoding/json"
	"os"

	"github.com/cmdruid/proton-cli/internal/config"
	"path/filepath"
	"time"
)

// Interval is how long a lookup stands before another is worth making, which is
// also how often a reader can be told what it found.
//
// A day, because that is the shape of the answer: a release is worth hearing
// about once, when you sit down, and hearing about it again before you have had
// a chance to act on it is nagging. Anything shorter buys nothing - a release is
// not urgent to the minute - and anything longer leaves a security fix sitting
// unmentioned on a machine with no package manager to bring it.
const Interval = 24 * time.Hour

// Check is when the last lookup was attempted, and the whole of what is
// remembered between runs.
type Check struct {
	CheckedAt time.Time `json:"checked_at"`
}

// Due reports whether the last attempt has stood long enough to make another. A
// file that is missing, unreadable or nonsense reads as due, which is what makes
// a first run behave like a stale one.
func (c Check) Due(now time.Time) bool { return now.Sub(c.CheckedAt) >= Interval }

// Watched reports whether this install is one that could act on a new release.
//
// Only a copy that can replace itself qualifies: a package manager owns its own
// updates and would refuse this one, so telling a Homebrew user about a release
// is telling them to run a command that will decline. A development build has no
// version to compare.
func Watched(exePath, version string) bool {
	if version == "" || version == "dev" {
		return false
	}
	return Classify(exePath) == KindStandalone
}

// CheckPath is where the last attempt is remembered, beside the sessions and the
// ID cache. It is not profile-scoped: which binary is installed on this machine
// has nothing to do with whose account is being used on it.
func CheckPath() string {
	dir, err := config.Dir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "update-check.json")
}

// LoadCheck reads the stored attempt. Anything wrong with the file reads as no
// attempt at all: this is a courtesy, and a courtesy that can fail a command is
// worse than no courtesy.
func LoadCheck(path string) Check {
	source, err := os.ReadFile(path)
	if err != nil {
		return Check{}
	}
	var c Check
	if err := json.Unmarshal(source, &c); err != nil {
		return Check{}
	}
	return c
}

// SaveCheck writes the attempt atomically, through a fixed temporary name so a
// run cut short leaves one stale file rather than a growing pile of them.
func SaveCheck(path string, c Check) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	encoded, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
