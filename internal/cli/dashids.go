package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/roman-16/proton-cli/internal/ref"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Roughly one ID in sixty-four begins with a dash, because Proton's IDs are
// base64 and '-' is one of its sixty-four characters. Every one of those is a
// reference the CLI printed and cannot read back, since argv gives a leading
// dash to the flag parser first.
//
// The fix is to insert "--" in front of it, and the whole difficulty is knowing
// when to. What counts as a reference is not decided here: internal/ref owns
// that, so the grammar that prints a reference and the grammar that reads one
// back cannot drift apart, which is how a listing came to show IDs that the very
// next command rejected.

// preprocessArgs walks argv and protects the first leading-dash reference by
// inserting "--" before it, so cobra treats the rest as positional. A literal
// "--" already present leaves argv untouched.
//
// Two things stop it, and both are answered by asking the command rather than by
// guessing at the token.
//
// A reference that is a flag's value is left alone, because it was never in
// danger: pflag reads the token after `--album` as its value whatever it starts
// with. Inserting there is what would do the damage, handing "--" to the flag and
// stranding the reference as a positional argument.
//
// A token that really does name shorthand flags is left alone too - which is the
// exact form of a question that cannot be answered by shape. "-Qt-s7R_" is a
// real Drive link shortened for display, and it is also, in principle, a run of
// eight shorthand flags. It is not in practice, because this CLI defines three
// shorthands in total, and asking the command settles it with certainty instead
// of with a length threshold.
func preprocessArgs(args []string) []string {
	for i := 1; i < len(args); i++ {
		if args[i] == "--" {
			return args
		}
		if !ref.Plausible(args[i]) || !worthProtecting(args[1:i], args[i]) {
			continue
		}
		out := make([]string, 0, len(args)+1)
		out = append(out, args[:i]...)
		out = append(out, "--")
		out = append(out, args[i:]...)
		return out
	}
	return args
}

// worthProtecting decides whether the candidate reaching the command named by
// path is a positional reference that flag parsing would otherwise eat.
//
// path is everything between the program name and the candidate. A tree that
// cannot be resolved falls back to what can be known without one.
func worthProtecting(path []string, candidate string) bool {
	// Probed on a tree of its own: Find leaves state behind, and the tree that
	// answers the command has to start clean.
	cmd, _, err := newRoot().Find(path)
	if err != nil || cmd == nil {
		return ref.Unambiguous(candidate)
	}
	return !consumesNextArgument(cmd, path) && !namesShorthandFlags(cmd, candidate)
}

// consumesNextArgument reports whether the token before the candidate is a flag
// that will take it as its value.
//
// The answer differs per flag: `--album X` takes a value and `--unread X` does
// not, so after `--unread` a reference really is positional and really does need
// protecting.
func consumesNextArgument(cmd *cobra.Command, path []string) bool {
	if len(path) == 0 {
		return false
	}
	prev := path[len(path)-1]
	if !strings.HasPrefix(prev, "-") || prev == "-" || strings.Contains(prev, "=") {
		return false
	}
	var flag *pflag.Flag
	if name := strings.TrimPrefix(prev, "--"); name != prev {
		flag = cmd.Flags().Lookup(name)
	} else if short := prev[1:]; len(short) == 1 {
		flag = cmd.Flags().ShorthandLookup(short)
	}
	if flag == nil {
		return false
	}
	// A boolean flag never consumes the next argument, so what follows one is a
	// positional and is exactly what the protection exists for.
	return flag.NoOptDefVal == ""
}

// namesShorthandFlags reports whether s could be parsed as a run of shorthand
// flags this command actually has, which is the one reading that must win over
// "it is a reference".
//
// pflag stops at the first shorthand that takes a value and treats the remainder
// of the token as that value, so `-ojson` is `--output json` and everything
// after the 'o' is beyond question.
func namesShorthandFlags(cmd *cobra.Command, s string) bool {
	for i := 1; i < len(s); i++ {
		flag := cmd.Flags().ShorthandLookup(s[i : i+1])
		if flag == nil {
			return false
		}
		if flag.NoOptDefVal == "" {
			return true
		}
	}
	return true
}

// rewrapFlagError translates the two failure modes where a leading-dash
// reference collides with flag parsing into a hint mentioning `--`.
func rewrapFlagError(err error, argv []string) error {
	if err == nil {
		return err
	}

	var nee *pflag.NotExistError
	if errors.As(err, &nee) {
		if shorts := nee.GetSpecifiedShortnames(); shorts != "" {
			token := "-" + shorts
			if ref.Plausible(token) {
				return fmt.Errorf(
					"that argument looks like a flag because it starts with '-'.\n"+
						"       If it is an ID, insert -- before it:\n"+
						"         proton ... -- %s", token)
			}
		}
	}

	msg := err.Error()
	if strings.Contains(msg, "accepts ") && strings.Contains(msg, "arg(s)") {
		for _, a := range argv[1:] {
			if ref.Unambiguous(a) {
				return fmt.Errorf(
					"%w\n"+
						"Hint: %q starts with '-' so it is auto-protected with -- before it.\n"+
						"      Any flags after the ID then become positional arguments. Put flags\n"+
						"      before the ID, or insert -- before it explicitly:\n"+
						"        proton ... --flag value -- %s",
					err, a, a)
			}
		}
	}

	return err
}
