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
// handler runs, collapsing the boilerplate each command would otherwise repeat.
type Ctx struct {
	Ctx  context.Context
	App  *app.App
	U    *keys.Unlocked
	Args []string
}

func (c *Ctx) R() *render.Renderer { return c.App.R }

func (c *Ctx) short() bool { return c.App.R.IsTTY() && !c.App.FullIDs }

// Step is a requirement run before the handler. The first failing step aborts.
type Step func(*Ctx) error

type Handler func(*Ctx) error

func stepAuth(c *Ctx) error { return c.App.Authenticate(c.Ctx) }

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
