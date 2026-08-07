package kit

import (
	"context"
	"errors"
	"strings"

	"github.com/roman-16/proton-cli/internal/account/keys"
	"github.com/roman-16/proton-cli/internal/app"
	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/idcache"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/spf13/cobra"
)

// Invocation is the prepared context handed to a command body. The steps a
// command declares populate it - authenticate, unlock, expand short IDs - so no
// handler repeats that preamble.
type Invocation struct {
	Ctx  context.Context
	App  *app.App
	Args []string
	// U is the unlocked key hierarchy, present when the command declared
	// StepUnlock.
	U *keys.Unlocked
	// Cmd is the running command, so a handler can tell a flag left alone from
	// one explicitly set to its zero value.
	Cmd *cobra.Command
}

// UI is the renderer for this invocation. It is the only way a command produces
// output.
func (c *Invocation) UI() *ui.UI { return c.App.UI }

// Changed reports whether the user passed the named flag, however they set it.
func (c *Invocation) Changed(flag string) bool {
	return c.Cmd != nil && c.Cmd.Flags().Changed(flag)
}

// Step is a requirement satisfied before the body runs. The first failure aborts.
type Step func(*Invocation) error

// Handler is a command body.
type Handler func(*Invocation) error

// StepAuth makes sure a usable session exists, signing in if none does.
func StepAuth(c *Invocation) error { return c.App.Authenticate(c.Ctx) }

// StepUnlock decrypts the key hierarchy up front and puts it in U.
//
// Declare it when the command always needs keys. When unlocking should wait
// until arguments have been resolved, call c.App.Unlock inline instead: it
// memoises, so the extra call costs nothing.
func StepUnlock(c *Invocation) error {
	u, err := c.App.Unlock(c.Ctx)
	if err != nil {
		return err
	}
	c.U = u
	return nil
}

// StepExpand turns short IDs in the positional arguments into full ones. It
// leaves anything that is not short-ID-shaped alone, so applying it everywhere is
// safe.
func StepExpand(c *Invocation) error {
	out := make([]string, len(c.Args))
	for i, a := range c.Args {
		full, err := Expand(c.App, a)
		if err != nil {
			return err
		}
		out[i] = full
	}
	c.Args = out
	return nil
}

// Run wires a command body to cobra, running the declared steps first.
//
// Before any step, every enum flag the command declared is checked. Local
// validation preceding the network is a rule rather than a habit: a value that
// could never have been sent should not first cost a sign-in to discover.
func Run(steps []Step, h Handler) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := validateFlags(cmd); err != nil {
			return err
		}
		c := &Invocation{Ctx: cmd.Context(), App: app.From(cmd.Context()), Args: args, Cmd: cmd}
		for _, s := range steps {
			if err := s(c); err != nil {
				return err
			}
		}
		return h(c)
	}
}

// ── responses ──

// List renders a collection and remembers the IDs it showed, so a short ID read
// off this output resolves on the next command.
//
// Caching here rather than in the ui package is deliberate: remembering an ID is
// something this CLI does about Proton, not something a table does.
func List[T any](c *Invocation, spec ui.TableSpec[T], items []T, ids func(T) []string) error {
	if ids != nil && c.App.IDCache != nil && len(items) > 0 {
		seen := make([]string, 0, len(items))
		for _, it := range items {
			seen = append(seen, ids(it)...)
		}
		_ = c.App.IDCache.Save(seen...)
	}
	return ui.Table(c.UI(), spec, items)
}

// Show renders one object.
func Show(c *Invocation, spec ui.RecordSpec) error { return ui.Record(c.UI(), spec) }

// Read renders decrypted content meant to be read.
func Read(c *Invocation, spec ui.DocumentSpec) error { return ui.Document(c.UI(), spec) }

// Confirm stops for a yes before something sweeping enough that a typo would be
// expensive, and returns an error unless it gets one.
//
// --dry-run needs no answer, because it asks the same question in a safer form.
// --yes is the answer given in advance, which is also the only way through in a
// script: a prompt nobody can see is a hang, so an unattended run is told what
// to add rather than left waiting.
func Confirm(c *Invocation, question string) error {
	if c.App.DryRun || c.App.Yes {
		return nil
	}
	if !c.UI().CanPrompt() {
		return Fail("%s", question).
			Hint("--yes to confirm, or --dry-run to see what it would touch.")
	}
	ok, err := c.UI().Confirm(question)
	if err != nil {
		return err
	}
	if !ok {
		return Fail("Cancelled.")
	}
	return nil
}

// Mutate performs a change and reports it - or, under --dry-run, reports what it
// would have done and changes nothing.
//
// Routing every mutation through here is what makes --dry-run a property of the
// CLI rather than a flag each command has to remember to check.
func Mutate(c *Invocation, spec ui.ResultSpec, apply func() error) error {
	if c.App.DryRun {
		spec.DryRun = true
		return ui.Result(c.UI(), spec)
	}
	if err := apply(); err != nil {
		return err
	}
	return ui.Result(c.UI(), spec)
}

// Create makes one thing and reports its new ID.
//
// It is separate from Mutate because a creation's identity does not exist until
// the work is done, and because that ID goes to stdout: `ID=$(proton-cli ...
// create ...)` is the whole reason the split between the streams matters.
func Create(c *Invocation, spec ui.ResultSpec, apply func() (string, error)) error {
	spec.Count = 1
	if c.App.DryRun {
		spec.DryRun = true
		return ui.Result(c.UI(), spec)
	}
	id, err := apply()
	if err != nil {
		return err
	}
	spec.IDs = []string{id}
	spec.EmitID = true
	return ui.Result(c.UI(), spec)
}

// ── references ──

// Expand turns an eight-character short ID into the full one using the local
// cache.
//
// Anything that is not short-ID-shaped, and anything the cache has not seen,
// passes through untouched: it may well be a name or a subject that the service
// layer will resolve. An ambiguous prefix is an error, never a guess.
func Expand(a *app.App, ref string) (string, error) {
	if ref == "" || idcache.IsFullID(ref) || !idcache.IsShortID(ref) {
		return ref, nil
	}
	if a == nil || a.IDCache == nil {
		return ref, nil
	}
	full, err := a.IDCache.Resolve(ref)
	if err == nil {
		return full, nil
	}
	var amb *idcache.AmbiguousError
	if errors.As(err, &amb) {
		lines := []string{"use one of:"}
		for _, cand := range amb.Candidates {
			lines = append(lines, "  "+cand)
		}
		return "", errs.Problemf("%q matches %d cached IDs.", ref, len(amb.Candidates)).
			Hint(lines...).Exit(4)
	}
	if errors.Is(err, idcache.ErrNotFound) {
		return ref, nil
	}
	return "", err
}

// Pair splits a compound reference into its two halves.
//
// Proton addresses some things with two IDs - a Pass item lives in a share, an
// event lives in a calendar. Writing them as one slash-separated token keeps
// every command to a single REF, and is unambiguous because Proton's IDs are
// base64url and never contain a slash.
//
// A reference with no slash is returned as the second half with an empty first,
// since that is the shape a human handle takes.
func Pair(ref string) (first, second string) {
	if i := strings.IndexByte(ref, '/'); i >= 0 {
		return ref[:i], ref[i+1:]
	}
	return "", ref
}

// ExpandPair splits a compound reference and expands each half, so a short ID
// works on either side of the slash.
//
// StepExpand cannot do this. A slash is not part of an ID, so a compound
// reference never looks short to it - and a Drive path is full of slashes too,
// so a step that applies to every argument would take paths apart as well. Only
// a command that knows it is holding two IDs can safely separate them.
func ExpandPair(a *app.App, ref string) (first, second string, err error) {
	first, second = Pair(ref)
	if first == "" {
		return "", second, nil
	}
	if first, err = Expand(a, first); err != nil {
		return "", "", err
	}
	if second, err = Expand(a, second); err != nil {
		return "", "", err
	}
	return first, second, nil
}

// JoinPair renders a compound reference as the single token a user pastes back.
func JoinPair(first, second string) string {
	if first == "" {
		return second
	}
	return first + "/" + second
}

// Dedupe removes repeated strings, preserving order. Selections union explicit
// references with filter matches, so an overlap is expected rather than an error.
func Dedupe(ss []string) []string {
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

// Note reports something incidental on stderr.
func (c *Invocation) Note(format string, a ...any) { c.UI().Notef(format, a...) }

// Fail starts a user error. It is the same as errs.Problemf, re-exported so a
// command package needs one import for the whole vocabulary.
func Fail(format string, a ...any) *errs.Problem { return errs.Problemf(format, a...) }

// Object writes v in the machine format. It is for the handful of results that
// have no table, record or mutation shape of their own - the self-management
// commands, which report on the binary rather than on an account.
func Object(c *Invocation, v any) error { return ui.Record(c.UI(), ui.RecordSpec{Object: v}) }
