package ui

import "github.com/cmdruid/proton-cli/internal/ref"

// Short renders a reference the way a terminal shows it. No ellipsis is
// appended: a short ID is the canonical interactive form, meant to be pasted
// straight back into the next command, so it must stay copy-safe.
//
// The notation is internal/ref's rather than this file's, which is the whole
// point: what a listing prints and what the next command will accept are then
// one decision instead of two that happen to match.
func Short(id string, short bool) string {
	if !short {
		return id
	}
	return ref.Shorten(id)
}
