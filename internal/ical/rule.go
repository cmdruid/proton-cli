package ical

import "strings"

// A recurrence rule is edited in place, as text.
//
// Round-tripping a rule through a parser would normalise it: parts reordered,
// defaults dropped, a rule the user typed coming back spelled differently. Since
// the only edits the CLI makes are to COUNT and UNTIL, they are made to the text
// and everything else survives exactly as it was written.

type rulePart struct{ key, value string }

func ruleParts(rule string) []rulePart {
	var out []rulePart
	for _, part := range strings.Split(rule, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, _ := strings.Cut(part, "=")
		out = append(out, rulePart{key: strings.TrimSpace(key), value: value})
	}
	return out
}

func joinRule(parts []rulePart) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, p.key+"="+p.value)
	}
	return strings.Join(out, ";")
}

// RuleValue returns the value of one rule part, or "".
func RuleValue(rule, key string) string {
	for _, p := range ruleParts(rule) {
		if strings.EqualFold(p.key, key) {
			return p.value
		}
	}
	return ""
}

// RuleWith sets a rule part, replacing it where it already appears and appending
// it where it does not.
func RuleWith(rule, key, value string) string {
	parts := ruleParts(rule)
	for i := range parts {
		if strings.EqualFold(parts[i].key, key) {
			parts[i].value = value
			return joinRule(parts)
		}
	}
	return joinRule(append(parts, rulePart{key: key, value: value}))
}

// RuleWithout removes rule parts.
func RuleWithout(rule string, keys ...string) string {
	parts := ruleParts(rule)
	kept := parts[:0]
	for _, p := range parts {
		drop := false
		for _, k := range keys {
			if strings.EqualFold(p.key, k) {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, p)
		}
	}
	return joinRule(kept)
}

// ruleCount is the rule's COUNT, or 0 when it does not end after a fixed number
// of occurrences.
func ruleCount(rule string) int {
	n := 0
	for _, c := range RuleValue(rule, "COUNT") {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
