package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// binary is the proton-cli to drive. `just` builds it first; PROTON_CLI points
// somewhere else when the integration suite runs its own build.
func binary() string {
	if v := os.Getenv("PROTON_CLI"); v != "" {
		return v
	}
	return "./proton-cli"
}

// command builds a CLI invocation as one profile.
func command(profile string, args ...string) *exec.Cmd {
	cmd := exec.Command(binary(), args...)
	cmd.Env = append(os.Environ(), "PROTON_PROFILE="+profile, "PROTON_NO_INPUT=1")
	return cmd
}

// run invokes the CLI as one profile and returns stdout. stderr is folded into
// the error, because that is where the CLI explains itself.
func run(profile string, args ...string) (string, error) {
	var out, errb bytes.Buffer
	cmd := command(profile, args...)
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), fmt.Errorf("%s: %s", strings.Join(args, " "), firstLine(msg))
	}
	return out.String(), nil
}

// rows reads a collection. Every list is an envelope keyed by its plural noun,
// so this takes whichever array it finds rather than making each caller name it.
func rows(profile string, args ...string) ([]map[string]any, error) {
	out, err := run(profile, append([]string{"--output", "json"}, args...)...)
	if err != nil {
		return nil, err
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		return nil, fmt.Errorf("%s: not an envelope: %w", strings.Join(args, " "), err)
	}
	for _, raw := range envelope {
		var list []map[string]any
		if json.Unmarshal(raw, &list) == nil {
			return list, nil
		}
	}
	return nil, nil
}

// find returns the row whose key holds want, and whether there was one.
func find(list []map[string]any, key, want string) (map[string]any, bool) {
	for _, r := range list {
		if str(r[key]) == want {
			return r, true
		}
	}
	return nil, false
}

// str renders a JSON value for comparison. Numbers arrive as float64, and a
// whole one should read as "1" rather than "1e+00".
func str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%v", t)
	case bool:
		return fmt.Sprintf("%t", t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
