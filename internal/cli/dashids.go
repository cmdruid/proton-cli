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
// otherwise shaped like a Proton ID (≥60 chars, ends "==", URL-safe base64).
func looksLikeDashedProtonID(s string) bool {
	if len(s) < 60 {
		return false
	}
	if s[0] != '-' || (len(s) > 1 && s[1] == '-') {
		return false
	}
	if !strings.HasSuffix(s, "==") {
		return false
	}
	for i := 1; i < len(s); i++ {
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
			if looksLikeDashedProtonID(token) {
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
