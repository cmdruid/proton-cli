package ui

import (
	"fmt"
	"strings"
)

// Action names a mutation in the three grammatical forms the CLI needs: the
// past tense for a confirmation, the infinitive for a dry-run preview, and a
// stable key for machine output.
//
// The set below is the complete vocabulary of things this CLI does. A command
// that wants a word not in this list is a command that has invented one.
type Action struct {
	Past string // "Moved"  → "✓ Moved 3 messages to trash."
	Verb string // "move"   → "Dry run - would move 3 messages to trash:"
	Key  string // "moved"  → {"action": "moved"}
}

var (
	Created      = Action{"Created", "create", "created"}
	Updated      = Action{"Updated", "update", "updated"}
	Deleted      = Action{"Deleted", "delete", "deleted"}
	Trashed      = Action{"Moved", "move", "trashed"}
	Restored     = Action{"Restored", "restore", "restored"}
	Emptied      = Action{"Emptied", "empty", "emptied"}
	Moved        = Action{"Moved", "move", "moved"}
	Copied       = Action{"Copied", "copy", "copied"}
	Uploaded     = Action{"Uploaded", "upload", "uploaded"}
	Downloaded   = Action{"Downloaded", "download", "downloaded"}
	Exported     = Action{"Exported", "export", "exported"}
	Sent         = Action{"Sent", "send", "sent"}
	Scheduled    = Action{"Scheduled", "schedule", "scheduled"}
	Unscheduled  = Action{"Unscheduled", "unschedule", "unscheduled"}
	Saved        = Action{"Saved", "save", "saved"}
	Labelled     = Action{"Labelled", "label", "labelled"}
	Unlabelled   = Action{"Unlabelled", "unlabel", "unlabelled"}
	Starred      = Action{"Starred", "star", "starred"}
	Unstarred    = Action{"Unstarred", "unstar", "unstarred"}
	MarkedRead   = Action{"Marked", "mark", "marked_read"}
	MarkedUnread = Action{"Marked", "mark", "marked_unread"}
	Enabled      = Action{"Enabled", "enable", "enabled"}
	Disabled     = Action{"Disabled", "disable", "disabled"}
	Linked       = Action{"Created", "create", "linked"}
	Unlinked     = Action{"Removed", "remove", "unlinked"}
	Added        = Action{"Added", "add", "added"}
	Removed      = Action{"Removed", "remove", "removed"}
	Accepted     = Action{"Accepted", "accept", "accepted"}
	Declined     = Action{"Declined", "decline", "declined"}
	Favorited    = Action{"Favorited", "favorite", "favorited"}
	Unfavorited  = Action{"Unfavorited", "unfavorite", "unfavorited"}
	Pinned       = Action{"Pinned", "pin", "pinned"}
	Unpinned     = Action{"Removed", "remove", "unpinned"}
	Responded    = Action{"Responded", "respond", "responded"}
	Set          = Action{"Set", "set", "set"}
	Invited      = Action{"Invited", "invite", "invited"}
	Revoked      = Action{"Revoked", "revoke", "revoked"}
	SignedIn     = Action{"Signed in", "sign in", "signed_in"}
	SignedOut    = Action{"Signed out", "sign out", "signed_out"}
)

// Actions is the vocabulary, for the conformance test to check against.
var Actions = []Action{
	Created, Updated, Deleted, Trashed, Restored, Emptied, Moved, Copied,
	Uploaded, Downloaded, Exported, Sent, Scheduled, Unscheduled, Saved,
	Labelled, Unlabelled, Starred, Unstarred, MarkedRead, MarkedUnread,
	Enabled, Disabled, Linked, Unlinked, Added, Removed, Accepted, Declined,
	Favorited, Unfavorited, Pinned, Unpinned, Responded, Set, Invited, Revoked,
	SignedIn, SignedOut,
}

// ResultSpec describes what a mutation did.
type ResultSpec struct {
	Action Action
	// Kind is the affected collection's plural noun ("messages"), matching
	// TableSpec.Noun so both halves of a command speak the same word.
	Kind  string
	Count int
	IDs   []string
	// Name is the affected thing's own name, used instead of a count when
	// exactly one thing was touched and naming it is more useful.
	Name string
	// Detail is a trailing clause: "to trash", "to /Documents", "as read".
	Detail string
	// EmitID prints the first ID on Out in text mode, so a script can capture
	// what was just created with a plain assignment.
	EmitID bool
	// DryRun switches to the preview form. Preview, when set, draws the
	// selection that would have been affected.
	DryRun  bool
	Preview func(*UI) error
	// Extra adds fields to the machine-format object.
	Extra map[string]any
}

// Result reports a mutation. In text mode the new ID (if any) goes to Out and
// the confirmation to Err, so a redirect captures the ID alone. In a machine
// format the whole result goes to Out, so --output json always means JSON.
func Result(u *UI, spec ResultSpec) error {
	if u.Format.Machine() {
		return u.encode(spec.object())
	}

	if spec.DryRun {
		_, _ = fmt.Fprintf(u.Err, "%s\n", spec.dryRunLine())
		if spec.Preview != nil {
			_, _ = fmt.Fprintln(u.Err)
			return spec.Preview(u.preview())
		}
		return nil
	}

	if spec.EmitID && len(spec.IDs) > 0 && spec.IDs[0] != "" {
		_, _ = fmt.Fprintln(u.Out, spec.IDs[0])
	}
	if !u.Quiet {
		_, _ = fmt.Fprintf(u.Err, "%s %s\n", u.errTheme.Success(GlyphSuccess), spec.message())
	}
	return nil
}

// message composes the confirmation. Three shapes, chosen by what the caller
// actually knows:
//
//	Created label "Work".            one thing, and its name is worth saying
//	Uploaded report.pdf to /Docs.    one thing named, with no useful kind word
//	Moved 3 messages to trash.       a count
func (s ResultSpec) message() string {
	var b strings.Builder
	b.WriteString(s.Action.Past)
	b.WriteByte(' ')
	switch {
	case s.Count == 0:
		b.Reset()
		b.WriteString("Nothing to ")
		b.WriteString(s.Action.Verb)
	case s.Name != "" && s.Count == 1 && s.Kind != "":
		b.WriteString(Singular(s.Kind))
		b.WriteString(` "`)
		b.WriteString(s.Name)
		b.WriteString(`"`)
	case s.Name != "" && s.Count == 1:
		b.WriteString(s.Name)
	default:
		b.WriteString(Quantity(s.Count, s.Kind))
	}
	if s.Detail != "" {
		b.WriteByte(' ')
		b.WriteString(s.Detail)
	}
	return b.String() + "."
}

func (s ResultSpec) dryRunLine() string {
	subject := Quantity(s.Count, s.Kind)
	if s.Name != "" && s.Count == 1 {
		subject = s.Name
		if s.Kind != "" {
			subject = Singular(s.Kind) + ` "` + s.Name + `"`
		}
	}
	line := fmt.Sprintf("Dry run - would %s %s", s.Action.Verb, subject)
	if s.Detail != "" {
		line += " " + s.Detail
	}
	if s.Preview != nil && s.Count > 0 {
		return line + ":"
	}
	return line + "."
}

func (s ResultSpec) object() map[string]any {
	obj := map[string]any{
		"action":  s.Action.Key,
		"count":   s.Count,
		"dry_run": s.DryRun,
	}
	if s.Kind != "" {
		obj["kind"] = Singular(s.Kind)
	}
	if len(s.IDs) > 0 {
		obj["ids"] = s.IDs
	}
	if s.Name != "" {
		obj["name"] = s.Name
	}
	for k, v := range s.Extra {
		obj[k] = v
	}
	return obj
}
