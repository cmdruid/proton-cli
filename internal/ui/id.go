package ui

import "strings"

// ShortIDLen is how many leading characters of a Proton ID identify it in
// interactive output. Eight is short enough to read back from the screen and
// long enough that collisions inside one account are vanishingly rare; when two
// cached IDs do collide, resolution reports the ambiguity rather than guessing.
const ShortIDLen = 8

// Short truncates a Proton ID for display. No ellipsis is appended: a short ID
// is the canonical interactive form, meant to be pasted straight back into the
// next command, so it must stay copy-safe.
//
// A compound reference ("SHARE/ITEM") is shortened segment by segment, keeping
// it a single pasteable token. What follows an "@" names one occurrence of a
// recurring event rather than an ID, and is what tells two rows of the same
// series apart, so it is carried over whole.
func Short(id string, short bool) string {
	if !short || id == "" {
		return id
	}
	ref, occurrence, recurring := strings.Cut(id, "@")
	parts := strings.Split(ref, "/")
	for i, p := range parts {
		parts[i] = truncID(p)
	}
	ref = strings.Join(parts, "/")
	if !recurring {
		return ref
	}
	return ref + "@" + occurrence
}

func truncID(id string) string {
	if len(id) <= ShortIDLen {
		return id
	}
	return id[:ShortIDLen]
}
