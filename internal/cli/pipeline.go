package cli

import (
	"context"

	"github.com/roman-16/proton-cli/internal/app"
	"github.com/roman-16/proton-cli/internal/crypto/keys"
	"github.com/roman-16/proton-cli/internal/render"
	"github.com/spf13/cobra"
)

// Ctx is the prepared per-invocation context handed to command handlers. The
// requirement steps populate it (auth, unlock, resolved args) before the
// handler runs, collapsing the boilerplate the old cmd layer repeated.
type Ctx struct {
	Ctx  context.Context
	App  *app.App
	U    *keys.Unlocked
	Args []string
}

// R is the renderer for this invocation.
func (c *Ctx) R() *render.Renderer { return c.App.R }

// short reports whether IDs should be shortened (interactive TTY, full IDs off).
func (c *Ctx) short() bool { return c.App.R.IsTTY() && !c.App.FullIDs }

// Step is a requirement run before the handler. The first failing step aborts.
type Step func(*Ctx) error

// Handler is a prepared command body.
type Handler func(*Ctx) error

// stepAuth ensures the client is authenticated.
func stepAuth(c *Ctx) error { return c.App.Authenticate(c.Ctx) }

// stepUnlock ensures the PGP key hierarchy is unlocked, caching it on c.U.
func stepUnlock(c *Ctx) error {
	u, err := c.App.Unlock(c.Ctx)
	if err != nil {
		return err
	}
	c.U = u
	return nil
}

// stepResolve expands short-ID prefixes in the positional args. It is a no-op
// for non-ID-shaped tokens, so it is safe to apply uniformly.
func stepResolve(c *Ctx) error {
	expanded, err := resolvePrefixes(c.App, c.Args)
	if err != nil {
		return err
	}
	c.Args = expanded
	return nil
}

// run builds a cobra RunE that prepares a Ctx, runs the steps in order, then
// invokes the handler.
func run(steps []Step, h Handler) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		c := &Ctx{Ctx: cmd.Context(), App: app.From(cmd.Context()), Args: args}
		for _, s := range steps {
			if err := s(c); err != nil {
				return err
			}
		}
		return h(c)
	}
}
