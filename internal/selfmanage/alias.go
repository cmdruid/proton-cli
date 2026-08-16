package selfmanage

import "os"

// An install answers to two names: the program, and an alias beside it in the
// same directory. Which of the two is the real file is not fixed - an installer
// writes the program and links the alias, while a copy that arrived under the
// other name keeps it - and it does not need to be, because either shape
// answers to both.

// AliasFiles returns the file names the alias may occupy on this platform, the
// one to create first.
//
// Unix carries it as a symlink under the plain name, and has nothing else to
// consider. Windows has two: an installer writes a .cmd shim, because a symlink
// there needs a privilege an ordinary install does not have, while a copy
// unpacked from the release zip carries a small launcher named .exe, because an
// archive cannot hold a link at all.
func AliasFiles(name string) []string { return aliasFiles(name) }

// LinkAlias makes path resolve to target, replacing whatever is at path.
func LinkAlias(target, path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return linkAlias(target, path)
}
