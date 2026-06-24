package cli

import (
	"fmt"

	"github.com/roman-16/proton-cli/internal/render"
	ctsvc "github.com/roman-16/proton-cli/internal/service/contacts"
	"github.com/roman-16/proton-cli/internal/view"
	"github.com/spf13/cobra"
)

func newContactsCmd() *cobra.Command {
	c := &cobra.Command{Use: "contacts", Short: "Contact operations"}
	c.AddCommand(contactsListCmd(), contactsGetCmd(), contactsCreateCmd(), contactsUpdateCmd(), contactsDeleteCmd())
	return c
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
			if ct.Email != "" {
				_, _ = fmt.Fprintf(out, "Email: %s\n", ct.Email)
			}
			if ct.Phone != "" {
				_, _ = fmt.Fprintf(out, "Phone: %s\n", ct.Phone)
			}
			if ct.Org != "" {
				_, _ = fmt.Fprintf(out, "Org:   %s\n", ct.Org)
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
	c.Flags().StringVar(&nc.Email, "email", "", "Contact email")
	c.Flags().StringVar(&nc.Phone, "phone", "", "Contact phone")
	c.Flags().StringVar(&nc.Org, "org", "", "Contact organization")
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
	c.Flags().StringVar(&nc.Email, "email", "", "New email")
	c.Flags().StringVar(&nc.Phone, "phone", "", "New phone")
	c.Flags().StringVar(&nc.Org, "org", "", "New organization")
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
