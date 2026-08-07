// Package account is the `proton-cli account` tree: the account itself, its
// settings, and the session this machine holds.
//
// It is a first-class app, the way Proton treats it: account settings live at
// account.proton.me, not inside Mail.
package account

import (
	"fmt"

	"github.com/roman-16/proton-cli/internal/account/session"
	"github.com/roman-16/proton-cli/internal/cli/kit"
	acctsvc "github.com/roman-16/proton-cli/internal/service/account"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/roman-16/proton-cli/internal/units"
	"github.com/spf13/cobra"
)

// New builds the account tree.
func New() *cobra.Command {
	c := &cobra.Command{
		Use:   "account",
		Short: "Your Proton account, its settings and your session",
	}
	c.AddCommand(getCmd(), loginCmd(), logoutCmd(), profilesCmd(), sessionsCmd(), settingsCmd())
	return c
}

// ── account get ──

// state is what `account get` answers: not just who the account is, but whether
// this machine can currently act as it. Those are the same question to a user,
// so they are one command rather than an `account get` plus an `account status`.
type state struct {
	*acctsvc.Account
	Profile  string   `json:"profile"`
	Session  string   `json:"session"`
	Unlocked bool     `json:"unlocked"`
	Scopes   []string `json:"scopes,omitempty"`
}

func getCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Show the account, its storage and this machine's session",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepAuth}, func(c *kit.Invocation) error {
			acct, err := c.App.Account.Get(c.Ctx)
			if err != nil {
				return err
			}
			// Reaching this point means the session worked, which is the only
			// honest way to report that it is valid.
			st := state{Account: acct, Profile: c.App.Profile, Session: "valid"}
			if sess, err := session.Load(c.App.Profile); err == nil {
				st.Unlocked = sess.Unlocked()
			}
			// Scopes are informative rather than essential: an account whose
			// session cannot list them is still worth reporting.
			if scopes, err := c.App.API.Scopes(c.Ctx); err == nil {
				st.Scopes = scopes
			}

			return kit.Show(c, ui.RecordSpec{
				Object: st,
				Fields: []ui.Field{
					{Label: "Email", Value: acct.Email},
					{Label: "Name", Value: acct.DisplayName},
					{Label: "Storage", Value: storage(acct.UsedSpace, acct.MaxSpace)},
					{Label: "Max Upload", Value: units.Size(acct.MaxUpload)},
					{Label: "Profile", Value: st.Profile},
					{Label: "Session", Value: st.Session, Always: true},
					{Label: "Unlocked", Value: yesNo(st.Unlocked), Always: true},
					{Label: "ID", Value: acct.ID, ID: true},
				},
			})
		}),
	}
}

// storage renders usage as a person reads it: how much of how much, and the
// share that represents.
func storage(used, max int64) string {
	if max <= 0 {
		return units.Size(used)
	}
	return fmt.Sprintf("%s of %s (%.0f%%)", units.Size(used), units.Size(max),
		100*float64(used)/float64(max))
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// ── account login / logout ──

func loginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Sign in and save the session for this profile",
		Long: "Sign in and save the session for this profile.\n\n" +
			"Anything not already set by a flag or an environment variable is asked for,\n" +
			"as long as this is a terminal. Signing in also unlocks your keys, so the\n" +
			"password is needed once per machine and not again.",
		Args: cobra.NoArgs,
		RunE: kit.Run(nil, func(c *kit.Invocation) error {
			if err := c.App.Login(c.Ctx); err != nil {
				return err
			}
			acct, err := c.App.Account.Get(c.Ctx)
			if err != nil {
				return err
			}
			c.App.RememberIdentity(acct.ID, acct.Email)
			if err := c.App.SaveSession(); err != nil {
				return err
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.SignedIn, Count: 1, Name: acct.Email,
				Detail: fmt.Sprintf("(profile %q)", c.App.Profile),
			}, func() error { return nil })
		}),
	}
}

func logoutCmd() *cobra.Command {
	var all, revoke bool
	c := &cobra.Command{
		Use:   "logout",
		Short: "Discard the saved session for this profile",
		Long: "Discard the saved session for this profile.\n\n" +
			"The sealed key password on disk is useless without the session, so removing\n" +
			"the file is enough to make it unreadable. --revoke additionally invalidates\n" +
			"the session at Proton, which is what signing out in a Proton app does.",
		Args: cobra.NoArgs,
		RunE: kit.Run(nil, func(c *kit.Invocation) error {
			targets := []string{c.App.Profile}
			if all {
				profiles, err := session.Profiles()
				if err != nil {
					return err
				}
				targets = targets[:0]
				for _, p := range profiles {
					targets = append(targets, p.Name)
				}
			}
			if len(targets) == 0 {
				return kit.Mutate(c, ui.ResultSpec{Action: ui.SignedOut, Kind: "profiles"},
					func() error { return nil })
			}

			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.SignedOut, Kind: "profiles", Count: len(targets),
				Name: single(targets), IDs: targets,
			}, func() error {
				// Revoking needs the session that is about to be deleted, so it
				// happens first and only for the active profile: the others'
				// tokens are not loaded.
				if revoke {
					if err := c.App.Authenticate(c.Ctx); err == nil {
						uid, _, _ := c.App.API.Tokens()
						if err := c.App.API.RevokeSession(c.Ctx, uid); err != nil {
							return err
						}
					}
				}
				for _, name := range targets {
					if err := session.Clear(name); err != nil {
						return err
					}
				}
				return nil
			})
		}),
	}
	c.Flags().BoolVar(&all, "all", false, "Sign out of every profile on this machine")
	c.Flags().BoolVar(&revoke, "revoke", false, "Also invalidate the session at Proton")
	return c
}

// single returns the sole element of ss, or "" when there is more than one, so a
// confirmation can name one thing and count many.
func single(ss []string) string {
	if len(ss) == 1 {
		return ss[0]
	}
	return ""
}
