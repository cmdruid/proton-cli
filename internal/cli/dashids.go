package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/pflag"
)

// preprocessArgs walks argv looking for the first token shaped like a Proton
// ID with a leading '-' (≥60 chars, ends "==", URL-safe base64). It injects
// "--" immediately before it so cobra treats the rest as positional. A literal
// "--" already present leaves argv untouched.
func preprocessArgs(args []string) []string {
	for i := 1; i < len(args); i++ {
		if args[i] == "--" {
			return args
		}
		if looksLikeDashedProtonID(args[i]) {
			out := make([]string, 0, len(args)+1)
			out = append(out, args[:i]...)
			out = append(out, "--")
			out = append(out, args[i:]...)
			return out
		}
	}
	return args
}

// looksLikeDashedProtonID reports whether s starts with a single '-' and is
// otherwise shaped like an ID Proton issues.
//
// There are two such shapes, and both are checked exactly rather than loosely,
// because inserting "--" in front of something that was meant as a flag value
// would be worse than the collision it avoids:
//
//   - what most things are named by: at least 60 characters of URL-safe base64,
//     padded to a multiple of four, so ending "==";
//   - what a Drive share invitation is named by: sixteen bytes unpadded, which
//     is exactly 22 characters and never contains "=".
func looksLikeDashedProtonID(s string) bool {
	if s == "" || s[0] != '-' || (len(s) > 1 && s[1] == '-') {
		return false
	}
	if !isBase64URL(s[1:]) {
		return false
	}
	switch {
	case len(s) >= 60 && strings.HasSuffix(s, "=="):
		return true
	case len(s) == 22 && !strings.Contains(s, "="):
		return true
	}
	return false
}

// mightBeDashedReference is the looser question, asked only to decide whether to
// explain "--" instead of letting cobra talk about shorthand flags.
//
// Advice can afford to be generous where the rewriting above cannot: a short ID
// is eight characters counting the dash, which is also a length a cluster of
// shorthand flags could have, so it is worth mentioning but not worth acting on.
func mightBeDashedReference(s string) bool {
	return len(s) >= 8 && s[0] == '-' && s[1] != '-' && isBase64URL(s[1:])
}

func isBase64URL(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-' || c == '_' || c == '=':
		default:
			return false
		}
	}
	return true
}

// rewrapFlagError translates the two failure modes where a leading-dash
// Proton ID collides with flag parsing into a hint mentioning `--`.
func rewrapFlagError(err error, argv []string) error {
	if err == nil {
		return err
	}

	var nee *pflag.NotExistError
	if errors.As(err, &nee) {
		if shorts := nee.GetSpecifiedShortnames(); shorts != "" {
			token := "-" + shorts
			if mightBeDashedReference(token) {
				return fmt.Errorf(
					"that argument looks like a flag because it starts with '-'.\n"+
						"       If it is an ID, insert -- before it:\n"+
						"         proton-cli ... -- %s", token)
			}
		}
	}

	msg := err.Error()
	if strings.Contains(msg, "accepts ") && strings.Contains(msg, "arg(s)") {
		for _, a := range argv[1:] {
			if looksLikeDashedProtonID(a) {
				return fmt.Errorf(
					"%w\n"+
						"Hint: %q starts with '-' so it is auto-protected with -- before it.\n"+
						"      Any flags after the ID then become positional arguments. Put flags\n"+
						"      before the ID, or insert -- before it explicitly:\n"+
						"        proton-cli ... --flag value -- %s",
					err, a, a)
			}
		}
	}

	return err
}
