package ui

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/roman-16/proton-cli/internal/errs"
)

// tryLabel is the fixed heading for remedies. Its width sets the indent of
// continuation lines, so a multi-line remedy stays visually attached to it.
const tryLabel = "Try:   "

// WriteError renders an error the way every error is rendered: one line stating
// the problem, then an indented "Try:" block when the error knows a remedy.
//
//	Error: That ID is a conversation, not a message.
//	Try:   proton-cli mail conversations get 5bH2mQxK
//
// Wrapped context ("upload x: open y: no such file") is left intact - it is the
// chain of what was being attempted, which is exactly what a reader needs.
func WriteError(w io.Writer, err error, style Style) {
	if err == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "%s %s\n", style.Paint(Danger, "Error:"), errs.Sentence(err.Error()))

	var hinter errs.Hinter
	if !errors.As(err, &hinter) {
		return
	}
	hints := hinter.Hints()
	if len(hints) == 0 {
		return
	}
	indent := strings.Repeat(" ", len(tryLabel))
	for i, h := range hints {
		prefix := indent
		if i == 0 {
			prefix = style.Paint(Muted, tryLabel)
		}
		_, _ = fmt.Fprintf(w, "%s%s\n", prefix, h)
	}
}

// Fail writes an error to the UI's stderr. It is the single exit path for a
// failed command, so no handler formats an error itself.
func (u *UI) Fail(err error) { WriteError(u.Err, err, u.errStyle) }
