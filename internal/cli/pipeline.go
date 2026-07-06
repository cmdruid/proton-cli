package cli

import (
	"context"
	"fmt"

	"github.com/roman-16/proton-cli/internal/account/keys"
	"github.com/roman-16/proton-cli/internal/app"
	"github.com/roman-16/proton-cli/internal/render"
	"github.com/spf13/cobra"
)

// Invocation is the prepared per-command context handed to handlers. The
// requirement steps populate it (auth, unlock, resolved args) before the
// handler runs, collapsing the boilerplate each command would otherwise repeat.
type Invocation struct {
	Ctx  context.Context
	App  *app.App
	U    *keys.Unlocked
	Args []string
}

func (c *Invocation) R() *render.Renderer { return c.App.R }

func (c *Invocation) short() bool { return c.App.R.IsTTY() && !c.App.FullIDs }

// dryRun reports whether --dry-run is active and, when it is, prints the
// standard "dry-run: would <msg>" line. Handlers use it as:
//
//	if c.dryRun("delete %d item(s)", n) { return nil }
func (c *Invocation) dryRun(format string, a ...any) bool {
	if !c.App.DryRun {
		return false
	}
	c.R().Info("dry-run: would " + fmt.Sprintf(format, a...))
	return true
}

// Step is a requirement run before the handler. The first failing step aborts.
type Step func(*Invocation) error

type Handler func(*Invocation) error

func stepAuth(c *Invocation) error { return c.App.Authenticate(c.Ctx) }

// stepUnlock unlocks the key hierarchy up front and stashes it in c.U.
//
// Convention: use stepUnlock (and read c.U) when a command ALWAYS needs the
// keys. When unlock must happen only after resolving arguments, call
// c.App.Unlock(c.Ctx) inline instead - it memoizes, so the extra call is free.
func stepUnlock(c *Invocation) error {
	u, err := c.App.Unlock(c.Ctx)
	if err != nil {
		return err
	}
	c.U = u
	return nil
}

// stepResolve expands short-ID prefixes in the positional args. It is a no-op
// for non-ID-shaped tokens, so it is safe to apply uniformly.
func stepResolve(c *Invocation) error {
	expanded, err := resolvePrefixes(c.App, c.Args)
	if err != nil {
		return err
	}
	c.Args = expanded
	return nil
}

func run(steps []Step, h Handler) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		c := &Invocation{Ctx: cmd.Context(), App: app.From(cmd.Context()), Args: args}
		for _, s := range steps {
			if err := s(c); err != nil {
				return err
			}
		}
		return h(c)
	}
}
