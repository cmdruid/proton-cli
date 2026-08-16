// Command proton-cli runs the proton binary sitting next to it, passing
// everything through untouched.
//
// It exists for one reason: an archive cannot carry a link. Every channel that
// installs by running something - the package managers, the installers, `proton
// update` - gives the second name as a link to the program and never builds
// this. The Windows zip has no such step, and winget shims exactly the
// executables the release declares, so on that one path the second name has to
// be an executable of its own.
package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func main() {
	os.Exit(run())
}

func run() int {
	program, err := beside("proton")
	if err != nil {
		return fail(err)
	}
	cmd := exec.Command(program, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// Ctrl-C reaches the whole console process group, so the program handles it
	// itself and this only has to wait for what that decided.
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		return fail(err)
	}
	return 0
}

// beside resolves name in the directory this executable really lives in.
// Symlinks are followed first, because the name that started this may be a shim
// somewhere else entirely - which is exactly what winget installs.
func beside(name string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(exe), name), nil
}

func fail(err error) int {
	_, _ = os.Stderr.WriteString("proton-cli: " + err.Error() + "\n")
	return 1
}
