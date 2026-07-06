package cli

import (
	"fmt"

	"github.com/roman-16/proton-cli/internal/render"
	drivesvc "github.com/roman-16/proton-cli/internal/service/drive"
	"github.com/roman-16/proton-cli/internal/units"
	"github.com/roman-16/proton-cli/internal/view"
	"github.com/spf13/cobra"
)

func driveShareCmd() *cobra.Command {
	c := &cobra.Command{Use: "share", Short: "Manage sharing (public links and members)"}
	c.AddCommand(shareStatusCmd(), shareLinkCmd(), shareUnlinkCmd(), shareAddCmd(), shareRemoveCmd())
	return c
}

func shareStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use: "status PATH", Short: "Show how a file or folder is shared",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			dc, err := driveCtx(c)
			if err != nil {
				return err
			}
			st, err := c.App.Drive.ShareStatusOf(c.Ctx, dc, c.Args[0])
			if err != nil {
				return err
			}
			if c.R().Format != render.FormatText {
				return c.R().Object(st)
			}
			if len(st.Links) == 0 && len(st.Members) == 0 && len(st.Invitees) == 0 {
				c.R().Info("Not shared.")
				return nil
			}
			out := c.R().Stdout
			if len(st.Links) > 0 {
				_, _ = fmt.Fprintln(out, "Public links:")
				for _, l := range st.Links {
					perm := "view"
					if l.CanEdit {
						perm = "edit"
					}
					exp := "never"
					if l.ExpireTime != nil {
						exp = units.Time(*l.ExpireTime)
					}
					_, _ = fmt.Fprintf(out, "  %s  (%s, expires %s, %d accesses)\n", l.URL, perm, exp, l.NumAccesses)
				}
			}
			if len(st.Members) > 0 {
				_, _ = fmt.Fprintln(out, "Members:")
				for _, m := range st.Members {
					_, _ = fmt.Fprintf(out, "  %-32s %s\n", m.Email, m.Role)
				}
			}
			if len(st.Invitees) > 0 {
				_, _ = fmt.Fprintln(out, "Pending invitations:")
				for _, p := range st.Invitees {
					_, _ = fmt.Fprintf(out, "  %-32s %s (pending)\n", p.Email, p.Role)
				}
			}
			return nil
		}),
	}
}

func shareLinkCmd() *cobra.Command {
	var edit bool
	var expires, password string
	c := &cobra.Command{
		Use:   "link PATH",
		Short: "Create or show the public link for a file or folder",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := drivesvc.LinkOptions{}
			if cmd.Flags().Changed("edit") {
				opts.SetEdit, opts.CanEdit = true, edit
			}
			if cmd.Flags().Changed("expires") {
				d, err := units.ParseDuration(expires)
				if err != nil {
					return fmt.Errorf("invalid --expires: %w", err)
				}
				opts.SetExpiry, opts.ExpireSeconds = true, int(d.Seconds())
			}
			if cmd.Flags().Changed("password") {
				opts.SetPassword, opts.CustomPassword = true, password
			}
			return run([]Step{stepAuth}, func(c *Invocation) error {
				dc, err := driveCtx(c)
				if err != nil {
					return err
				}
				if c.dryRun("create/update public link for %s", args[0]) {
					return nil
				}
				link, err := c.App.Drive.EnsureLink(c.Ctx, dc, args[0], opts)
				if err != nil {
					return err
				}
				if c.R().Format != render.FormatText {
					return c.R().Object(link)
				}
				_, _ = fmt.Fprintln(c.R().Stdout, link.URL)
				if link.CustomPassword != "" {
					c.R().Info("Password recipients must enter: " + link.CustomPassword)
				}
				return nil
			})(cmd, args)
		},
	}
	c.Flags().BoolVar(&edit, "edit", false, "Allow editing (default: view only)")
	c.Flags().StringVar(&expires, "expires", "", "Link expiration (e.g. 7d, 2w, 6mo)")
	c.Flags().StringVar(&password, "password", "", "Require a custom password to open the link")
	return c
}

func shareUnlinkCmd() *cobra.Command {
	return &cobra.Command{
		Use: "unlink PATH", Short: "Remove the public link(s) for a file or folder",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			dc, err := driveCtx(c)
			if err != nil {
				return err
			}
			if c.App.DryRun {
				n, err := c.App.Drive.CountLinks(c.Ctx, dc, c.Args[0])
				if err != nil {
					return err
				}
				c.R().Info(fmt.Sprintf("dry-run: would remove %d public link(s)", n))
				return nil
			}
			n, err := c.App.Drive.RemoveLinks(c.Ctx, dc, c.Args[0])
			if err != nil {
				return err
			}
			if n == 0 {
				c.R().Info("No public links to remove.")
				return nil
			}
			c.R().Success(fmt.Sprintf("Removed %d public link(s)", n))
			return nil
		}),
	}
}

func shareAddCmd() *cobra.Command {
	var edit bool
	var message string
	c := &cobra.Command{
		Use: "add PATH EMAIL", Short: "Invite a Proton user to a file or folder",
		Args: cobra.ExactArgs(2),
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			dc, err := driveCtx(c)
			if err != nil {
				return err
			}
			if c.dryRun("invite %s to %s", c.Args[1], c.Args[0]) {
				return nil
			}
			if err := c.App.Drive.InviteMember(c.Ctx, dc, c.Args[0], c.Args[1], edit, message); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Invited %s", c.Args[1]))
			return nil
		}),
	}
	c.Flags().BoolVar(&edit, "edit", false, "Allow editing (default: view only)")
	c.Flags().StringVar(&message, "message", "", "Optional note included in the invitation email")
	return c
}

func shareRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use: "remove PATH EMAIL", Short: "Revoke a member or cancel a pending invitation",
		Args: cobra.ExactArgs(2),
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			dc, err := driveCtx(c)
			if err != nil {
				return err
			}
			if c.dryRun("revoke access for %s on %s", c.Args[1], c.Args[0]) {
				return nil
			}
			if err := c.App.Drive.RemoveMember(c.Ctx, dc, c.Args[0], c.Args[1]); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Revoked access for %s", c.Args[1]))
			return nil
		}),
	}
}

func driveInvitationsCmd() *cobra.Command {
	c := &cobra.Command{Use: "invitations", Short: "Manage incoming share invitations"}
	c.AddCommand(invitationsListCmd(), invitationsAcceptCmd(), invitationsRejectCmd())
	return c
}

func invitationsListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List pending incoming share invitations",
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			invitations, err := c.App.Drive.ListInvitations(c.Ctx)
			if err != nil {
				return err
			}
			if c.R().Format == render.FormatText && len(invitations) == 0 {
				c.R().Info("No pending invitations.")
				return nil
			}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[drivesvc.Invitation]{
				Columns: []view.Column[drivesvc.Invitation]{
					{Header: "INVITATION_ID", ID: true, Cell: func(i drivesvc.Invitation) string { return i.InvitationID }},
					{Header: "FROM", Cell: func(i drivesvc.Invitation) string { return i.InviterEmail }},
					{Header: "ROLE", Cell: func(i drivesvc.Invitation) string { return i.Role }},
					{Header: "CREATED", Cell: func(i drivesvc.Invitation) string { return units.Time(i.CreateTime) }},
				},
				CacheIDs: func(i drivesvc.Invitation) []string { return []string{i.InvitationID} },
			}, invitations)
		}),
	}
}

func invitationsAcceptCmd() *cobra.Command {
	return &cobra.Command{
		Use: "accept INVITATION_ID", Short: "Accept a pending share invitation",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			if c.dryRun("accept invitation %s", c.Args[0]) {
				return nil
			}
			u, err := c.App.Unlock(c.Ctx)
			if err != nil {
				return err
			}
			if err := c.App.Drive.AcceptInvitation(c.Ctx, u, c.Args[0]); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Accepted invitation %s", c.Args[0]))
			return nil
		}),
	}
}

func invitationsRejectCmd() *cobra.Command {
	return &cobra.Command{
		Use: "reject INVITATION_ID", Short: "Reject a pending share invitation",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			if c.dryRun("reject invitation %s", c.Args[0]) {
				return nil
			}
			if err := c.App.Drive.RejectInvitation(c.Ctx, c.Args[0]); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Rejected invitation %s", c.Args[0]))
			return nil
		}),
	}
}
