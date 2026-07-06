package cli

import "path/filepath"

// matchGlob reports whether name matches the shell-style glob. Empty matches all.
func matchGlob(pattern, name string) bool {
	if pattern == "" {
		return true
	}
	ok, err := filepath.Match(pattern, name)
	if err != nil {
		return false
	}
	return ok
}

// dedupe removes duplicate strings, preserving order.
func dedupe(ss []string) []string {
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
