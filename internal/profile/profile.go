// Package profile is the name of a local session slot.
//
// A profile names files: the saved session and the ID cache both live at a path
// built from it. So it is a type with one validating constructor rather than a
// string, because a string reaches the filesystem through several callers and only
// one of them has to forget for `--profile ../../elsewhere` to write outside the
// directory it was meant to.
package profile

import (
	"fmt"
	"strings"
)

// Default is the profile a command acts as when none is named.
const Default = "default"

// maxBytes bounds the name so it cannot exceed what a filesystem accepts once the
// extension is added.
const maxBytes = 64

// Name is a validated profile name. The zero value is not usable; build one with
// Parse.
type Name struct{ value string }

// String is the name, or the default when the value is unset.
func (n Name) String() string {
	if n.value == "" {
		return Default
	}
	return n.value
}

// FileName is the name as a single path element, which is the only way a profile
// is allowed to become part of a path.
func (n Name) FileName(ext string) string { return n.String() + ext }

// Parse validates a profile name. An empty name selects the default.
//
// The accepted set is deliberately small and portable: ASCII letters, digits, dot,
// underscore and hyphen, starting with a letter or a digit. That rules out path
// separators, the relative names, leading dots, control characters and anything a
// Windows filesystem would refuse, which is every way a name can escape the
// directory it belongs in.
func Parse(name string) (Name, error) {
	if name == "" {
		return Name{}, nil
	}
	if len(name) > maxBytes {
		return Name{}, fmt.Errorf("a profile name can be at most %d characters, and %q is %d",
			maxBytes, name, len(name))
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		alphanumeric := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
		if i == 0 && !alphanumeric {
			return Name{}, fmt.Errorf("a profile name has to start with a letter or a digit, and %q starts with %q",
				name, string(c))
		}
		if !alphanumeric && c != '.' && c != '_' && c != '-' {
			return Name{}, fmt.Errorf("a profile name can hold letters, digits, %q, %q and %q, and %q holds %q",
				".", "_", "-", name, printable(c))
		}
	}
	return Name{value: name}, nil
}

// MustParse is Parse for names this package's own callers control.
func MustParse(name string) Name {
	n, err := Parse(name)
	if err != nil {
		panic(err)
	}
	return n
}

// printable renders a byte for an error message without putting a control
// character into the terminal.
func printable(c byte) string {
	if c < 0x20 || c == 0x7f {
		return fmt.Sprintf("\\x%02x", c)
	}
	return string(c)
}

// Equal reports whether two names select the same profile.
func (n Name) Equal(o Name) bool { return n.String() == o.String() }

// IsDefault reports whether this is the unnamed profile.
func (n Name) IsDefault() bool { return n.String() == Default }

// Names parses several names, reporting the first that is not one.
func Names(raw []string) ([]Name, error) {
	out := make([]Name, 0, len(raw))
	for _, r := range raw {
		n, err := Parse(strings.TrimSpace(r))
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}
