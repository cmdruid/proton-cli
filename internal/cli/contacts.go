package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/roman-16/proton-cli/internal/render"
	ctsvc "github.com/roman-16/proton-cli/internal/service/contacts"
	"github.com/roman-16/proton-cli/internal/view"
	"github.com/spf13/cobra"
)

func newContactsCmd() *cobra.Command {
	c := &cobra.Command{Use: "contacts", Short: "Contact operations"}
	c.AddCommand(contactsListCmd(), contactsGetCmd(), contactsCreateCmd(), contactsUpdateCmd(), contactsDeleteCmd(), contactsPinKeyCmd(), contactsUnpinKeyCmd(), contactsGroupsCmd())
	return c
}

func readArmoredKey(path string) (string, error) {
	var (
		data []byte
		err  error
	)
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// resolveContactEmail picks which of a contact's emails a key operation targets:
// the --email flag if given, the sole email if there is one, else an error
// asking the user to disambiguate.
func resolveContactEmail(c *Ctx, id, emailFlag string) (string, error) {
	if emailFlag != "" {
		return emailFlag, nil
	}
	ct, err := c.App.Contacts.Get(c.Ctx, c.U, id)
	if err != nil {
		return "", err
	}
	switch len(ct.Emails) {
	case 0:
		return "", fmt.Errorf("contact has no email address; pass --email")
	case 1:
		return ct.Emails[0], nil
	default:
		return "", fmt.Errorf("contact has %d email addresses; pass --email to choose one", len(ct.Emails))
	}
}

func contactsPinKeyCmd() *cobra.Command {
	var keyPath, email, scheme string
	var noEncrypt bool
	c := &cobra.Command{
		Use: "pin-key REF", Short: "Pin a public key to a contact so mail to them is encrypted to it",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve, stepUnlock}, func(c *Ctx) error {
			if keyPath == "" {
				return fmt.Errorf("--key is required (armored public key file, or - for stdin)")
			}
			if scheme != "" && scheme != "pgp-mime" && scheme != "pgp-inline" {
				return fmt.Errorf("invalid --scheme %q (use pgp-mime or pgp-inline)", scheme)
			}
			armored, err := readArmoredKey(keyPath)
			if err != nil {
				return err
			}
			id, err := c.App.Contacts.Resolve(c.Ctx, c.U, c.Args[0])
			if err != nil {
				return err
			}
			target, err := resolveContactEmail(c, id, email)
			if err != nil {
				return err
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would pin a key for %s", target))
				return nil
			}
			var enc *bool
			if noEncrypt {
				f := false
				enc = &f
			}
			if err := c.App.Contacts.PinKey(c.Ctx, c.U, id, target, armored, enc, nil, scheme); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Pinned key for %s", target))
			return nil
		}),
	}
	c.Flags().StringVar(&keyPath, "key", "", "Armored public key file (- for stdin)")
	c.Flags().StringVar(&email, "email", "", "Which of the contact's emails to pin the key to")
	c.Flags().StringVar(&scheme, "scheme", "", "PGP scheme for external recipients: pgp-mime (default) or pgp-inline")
	c.Flags().BoolVar(&noEncrypt, "no-encrypt", false, "Store the key but leave encryption disabled (x-pm-encrypt:false)")
	return c
}

func contactsUnpinKeyCmd() *cobra.Command {
	var email string
	c := &cobra.Command{
		Use: "unpin-key REF", Short: "Remove pinned key(s) from a contact",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve, stepUnlock}, func(c *Ctx) error {
			id, err := c.App.Contacts.Resolve(c.Ctx, c.U, c.Args[0])
			if err != nil {
				return err
			}
			target, err := resolveContactEmail(c, id, email)
			if err != nil {
				return err
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would remove pinned key(s) for %s", target))
				return nil
			}
			if err := c.App.Contacts.UnpinKey(c.Ctx, c.U, id, target); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Removed pinned key(s) for %s", target))
			return nil
		}),
	}
	c.Flags().StringVar(&email, "email", "", "Which of the contact's emails to unpin")
	return c
}

func contactsGroupsCmd() *cobra.Command {
	c := &cobra.Command{Use: "groups", Short: "Manage contact groups"}

	c.AddCommand(&cobra.Command{
		Use: "list", Short: "List contact groups",
		RunE: run([]Step{stepAuth}, func(c *Ctx) error {
			groups, err := c.App.Contacts.GroupsList(c.Ctx)
			if err != nil {
				return err
			}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[ctsvc.Group]{
				Columns: []view.Column[ctsvc.Group]{
					{Header: "ID", ID: true, Cell: func(g ctsvc.Group) string { return g.ID }},
					{Header: "NAME", Cell: func(g ctsvc.Group) string { return g.Name }},
					{Header: "COLOR", Cell: func(g ctsvc.Group) string { return g.Color }},
				},
				CacheIDs: func(g ctsvc.Group) []string { return []string{g.ID} },
			}, groups)
		}),
	})

	var gName, gColor string
	create := &cobra.Command{
		Use: "create", Short: "Create a contact group",
		RunE: run([]Step{stepAuth}, func(c *Ctx) error {
			if gName == "" {
				return fmt.Errorf("--name is required")
			}
			if err := validateAccentColor(gColor); err != nil {
				return err
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would create group %q", gName))
				return nil
			}
			id, err := c.App.Contacts.GroupCreate(c.Ctx, gName, gColor)
			if err != nil {
				return err
			}
			c.R().ID(id, fmt.Sprintf("Created group %q", gName))
			return nil
		}),
	}
	create.Flags().StringVar(&gName, "name", "", "Group name")
	create.Flags().StringVar(&gColor, "color", "#8080FF", "Group color (hex; must be a Proton accent color)")
	c.AddCommand(create)

	c.AddCommand(&cobra.Command{
		Use: "delete GROUP_ID", Short: "Delete a contact group",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Ctx) error {
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would delete group %s", c.Args[0]))
				return nil
			}
			if err := c.App.Contacts.GroupDelete(c.Ctx, c.Args[0]); err != nil {
				return err
			}
			c.R().Success("Group deleted.")
			return nil
		}),
	})

	c.AddCommand(contactsGroupMembersCmd("add", "Add contacts to a group", func(c *Ctx, gid string, ids []string) error {
		return c.App.Contacts.GroupAdd(c.Ctx, gid, ids)
	}, "Added %d contact(s) to group."))
	c.AddCommand(contactsGroupMembersCmd("remove", "Remove contacts from a group", func(c *Ctx, gid string, ids []string) error {
		return c.App.Contacts.GroupRemove(c.Ctx, gid, ids)
	}, "Removed %d contact(s) from group."))

	return c
}

func contactsGroupMembersCmd(use, short string, fn func(*Ctx, string, []string) error, successFmt string) *cobra.Command {
	return &cobra.Command{
		Use: use + " GROUP_ID CONTACT_REF...", Short: short,
		Args: cobra.MinimumNArgs(2),
		RunE: run([]Step{stepAuth, stepResolve, stepUnlock}, func(c *Ctx) error {
			groupID := c.Args[0]
			ids := make([]string, 0, len(c.Args)-1)
			for _, ref := range c.Args[1:] {
				id, err := c.App.Contacts.Resolve(c.Ctx, c.U, ref)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would %s %d contact(s)", use, len(ids)))
				return nil
			}
			if err := fn(c, groupID, ids); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf(successFmt, len(ids)))
			return nil
		}),
	}
}

func contactsListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List contacts (decrypted)",
		RunE: run([]Step{stepAuth, stepUnlock}, func(c *Ctx) error {
			contacts, err := c.App.Contacts.List(c.Ctx, c.U)
			if err != nil {
				return err
			}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[ctsvc.Contact]{
				Columns: []view.Column[ctsvc.Contact]{
					{Header: "ID", ID: true, Cell: func(ct ctsvc.Contact) string { return ct.ID }},
					{Header: "NAME", Cell: func(ct ctsvc.Contact) string { return ct.Name }},
					{Header: "EMAIL", Cell: func(ct ctsvc.Contact) string { return ct.Email }},
					{Header: "PHONE", Cell: func(ct ctsvc.Contact) string { return ct.Phone }},
				},
				CacheIDs: func(ct ctsvc.Contact) []string { return []string{ct.ID} },
			}, contacts)
		}),
	}
}

func contactsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use: "get REF", Short: "Get a contact (decrypted)",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve, stepUnlock}, func(c *Ctx) error {
			id, err := c.App.Contacts.Resolve(c.Ctx, c.U, c.Args[0])
			if err != nil {
				return err
			}
			ct, err := c.App.Contacts.Get(c.Ctx, c.U, id)
			if err != nil {
				return err
			}
			if c.R().Format != render.FormatText {
				return c.R().Object(ct)
			}
			out := c.R().Stdout
			_, _ = fmt.Fprintf(out, "ID:    %s\n", ct.ID)
			if ct.Name != "" {
				_, _ = fmt.Fprintf(out, "Name:  %s\n", ct.Name)
			}
			for _, e := range ct.Emails {
				_, _ = fmt.Fprintf(out, "Email: %s\n", e)
			}
			for _, p := range ct.Phones {
				_, _ = fmt.Fprintf(out, "Phone: %s\n", p)
			}
			if ct.Org != "" {
				_, _ = fmt.Fprintf(out, "Org:   %s\n", ct.Org)
			}
			if ct.Title != "" {
				_, _ = fmt.Fprintf(out, "Title: %s\n", ct.Title)
			}
			if ct.Birthday != "" {
				_, _ = fmt.Fprintf(out, "Bday:  %s\n", ct.Birthday)
			}
			if ct.Address != "" {
				_, _ = fmt.Fprintf(out, "Addr:  %s\n", ct.Address)
			}
			if ct.URL != "" {
				_, _ = fmt.Fprintf(out, "URL:   %s\n", ct.URL)
			}
			if ct.Note != "" {
				_, _ = fmt.Fprintf(out, "Note:  %s\n", ct.Note)
			}
			if ct.Signature != "" {
				_, _ = fmt.Fprintf(out, "Sig:   %s\n", sigText(ct.Signature))
			}
			return nil
		}),
	}
}

func contactsCreateCmd() *cobra.Command {
	var nc ctsvc.NewContact
	c := &cobra.Command{
		Use: "create", Short: "Create a contact",
		RunE: run([]Step{stepAuth, stepUnlock}, func(c *Ctx) error {
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would create contact %q", nc.Name))
				return nil
			}
			id, err := c.App.Contacts.Create(c.Ctx, c.U, nc)
			if err != nil {
				return err
			}
			c.R().ID(id, fmt.Sprintf("Created contact %q", nc.Name))
			return nil
		}),
	}
	c.Flags().StringVar(&nc.Name, "name", "", "Contact name")
	c.Flags().StringArrayVar(&nc.Emails, "email", nil, "Contact email (repeatable)")
	c.Flags().StringArrayVar(&nc.Phones, "phone", nil, "Contact phone (repeatable)")
	c.Flags().StringVar(&nc.Org, "org", "", "Contact organization")
	c.Flags().StringVar(&nc.Title, "title", "", "Job title")
	c.Flags().StringVar(&nc.Birthday, "birthday", "", "Birthday (e.g. 1990-01-31)")
	c.Flags().StringVar(&nc.Address, "address", "", "Postal address")
	c.Flags().StringVar(&nc.URL, "url", "", "Website URL")
	c.Flags().StringVar(&nc.Note, "note", "", "Contact note")
	return c
}

func contactsUpdateCmd() *cobra.Command {
	var nc ctsvc.NewContact
	c := &cobra.Command{
		Use: "update REF", Short: "Update a contact",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve, stepUnlock}, func(c *Ctx) error {
			id, err := c.App.Contacts.Resolve(c.Ctx, c.U, c.Args[0])
			if err != nil {
				return err
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would update contact %s", id))
				return nil
			}
			if err := c.App.Contacts.Update(c.Ctx, c.U, id, nc); err != nil {
				return err
			}
			c.R().Success("Contact updated.")
			return nil
		}),
	}
	c.Flags().StringVar(&nc.Name, "name", "", "New name")
	c.Flags().StringArrayVar(&nc.Emails, "email", nil, "New email(s) (repeatable; replaces existing)")
	c.Flags().StringArrayVar(&nc.Phones, "phone", nil, "New phone(s) (repeatable; replaces existing)")
	c.Flags().StringVar(&nc.Org, "org", "", "New organization")
	c.Flags().StringVar(&nc.Title, "title", "", "New job title")
	c.Flags().StringVar(&nc.Birthday, "birthday", "", "New birthday")
	c.Flags().StringVar(&nc.Address, "address", "", "New postal address")
	c.Flags().StringVar(&nc.URL, "url", "", "New website URL")
	c.Flags().StringVar(&nc.Note, "note", "", "New note")
	return c
}

func contactsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use: "delete REF...", Short: "Delete contacts",
		Args: cobra.MinimumNArgs(1),
		RunE: run([]Step{stepAuth, stepResolve, stepUnlock}, func(c *Ctx) error {
			ids := make([]string, 0, len(c.Args))
			for _, ref := range c.Args {
				id, err := c.App.Contacts.Resolve(c.Ctx, c.U, ref)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would delete %d contact(s)", len(ids)))
				return nil
			}
			if err := c.App.Contacts.Delete(c.Ctx, ids); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Deleted %d contact(s).", len(ids)))
			return nil
		}),
	}
}
