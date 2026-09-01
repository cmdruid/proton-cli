package confirm

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Everywhere is the scope that means every command, as a configuration file
// spells it.
const Everywhere = "*"

const denySuffix = "=deny"

// Parse reads the one-line form: a comma-separated list of directives, each
// `[path:]class[=deny]`.
//
//	mutations
//	pass:all
//	deletions=deny
//	mail drafts send:all=deny
//
// It is the form a flag and an environment variable can carry, and it says
// exactly what the file's two maps say - the same policy, written on one line
// because a shell has nowhere to put a document.
func Parse(s string) (Source, error) {
	var source Source
	for _, raw := range strings.Split(s, ",") {
		token := strings.TrimSpace(raw)
		if token == "" {
			continue
		}
		d, err := parseDirective(token)
		if err != nil {
			return nil, err
		}
		source = append(source, d)
	}
	return source, nil
}

func parseDirective(token string) (Directive, error) {
	outcome := Ask
	if rest, found := strings.CutSuffix(token, denySuffix); found {
		outcome, token = Deny, strings.TrimSpace(rest)
	}

	scope, className := Everywhere, token
	if path, name, found := strings.Cut(token, ":"); found {
		scope, className = strings.TrimSpace(path), strings.TrimSpace(name)
	}
	if className == "" {
		return Directive{}, fmt.Errorf("%q names a scope but no class; one of %s has to follow the colon",
			token, ClassList())
	}
	class, err := ParseClass(className)
	if err != nil {
		return Directive{}, err
	}
	return Directive{Path: parsePath(scope), Class: class, Outcome: outcome}, nil
}

func parsePath(scope string) []string {
	if scope == Everywhere {
		return nil
	}
	return strings.Fields(scope)
}

// Document is the `confirm` section of a configuration file: two maps from a
// scope to the class of command that scope stops for.
//
//	confirm:
//	  ask:
//	    "*": mutations
//	    pass: all
//	  deny:
//	    "*": deletions
//	    drive: all
type Document struct {
	Ask  map[string]string `yaml:"ask"`
	Deny map[string]string `yaml:"deny"`
}

// Source turns the document into directives.
//
// The scopes are walked in sorted order so that a file with two mistakes in it
// reports the same one every time.
func (d Document) Source() (Source, error) {
	var source Source
	for _, m := range []struct {
		scopes  map[string]string
		outcome Outcome
		field   string
	}{
		{d.Ask, Ask, "ask"},
		{d.Deny, Deny, "deny"},
	} {
		for _, scope := range slices.Sorted(maps.Keys(m.scopes)) {
			class, err := ParseClass(m.scopes[scope])
			if err != nil {
				return nil, fmt.Errorf("confirm.%s.%s: %w", m.field, scope, err)
			}
			source = append(source, Directive{Path: parsePath(scope), Class: class, Outcome: m.outcome})
		}
	}
	return source, nil
}
