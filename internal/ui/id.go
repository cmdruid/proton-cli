package ui

import (
	"strings"

	"github.com/roman-16/proton-cli/internal/ref"
)

// ShortIDLen is how many leading characters of a Proton ID identify it in
// interactive output.
const ShortIDLen = ref.ShortLen

// Short truncates a Proton reference for display. No ellipsis is appended: a
// short ID is the canonical interactive form, meant to be pasted straight back
// into the next command, so it must stay copy-safe.
//
// A compound reference ("SHARE/ITEM") is shortened segment by segment, keeping
// it a single pasteable token. What follows an "@" names one occurrence of a
// recurring event rather than an ID, and is what tells two rows of the same
// series apart, so it is carried over whole.
//
// The notation is internal/ref's rather than this file's, which is the whole
// point: what a listing prints and what the next command will accept are then
// one decision instead of two that happen to match. They once did not, and the
// symptom was a Pass listing whose rows could not be pasted back.
func Short(id string, short bool) string {
	if !short || id == "" {
		return id
	}
	parts, occurrence := ref.Split(id)
	for i, p := range parts {
		parts[i] = truncID(p)
	}
	out := ref.Join(parts...)
	if occurrence == "" && !strings.Contains(id, ref.Occurrence) {
		return out
	}
	return out + ref.Occurrence + occurrence
}

func truncID(id string) string {
	if len(id) <= ShortIDLen {
		return id
	}
	return id[:ShortIDLen]
}
