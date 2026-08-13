// Package contacts is the `proton-cli contacts` tree.
//
// The app hosts its primary collection's verbs directly - `contacts list`, not
// `contacts contacts list` - under a rule that applies to exactly one app: an
// app whose name is already the plural of its primary collection needs no second
// level to say so. Groups and pinned keys are secondary collections and do get
// their own level.
package contacts

import (
	"context"

	"github.com/roman-16/proton-cli/internal/cli/kit"
	ctsvc "github.com/roman-16/proton-cli/internal/service/contacts"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	c := &cobra.Command{
		Use:   "contacts",
		Short: "Contacts, their groups and their pinned keys",
	}
	c.AddCommand(listCmd(), getCmd(), createCmd(), updateCmd(), deleteCmd(), keysCmd(), groupsCmd())
	return c
}

// columns is the contact table. ID leads, as it does in every collection, so the
// thing you paste into the next command is always in the same place.
func columns() []ui.Column[ctsvc.Contact] {
	return []ui.Column[ctsvc.Contact]{
		{Header: "ID", ID: true, Cell: func(c ctsvc.Contact) string { return c.ID }},
		{Header: "NAME", Flex: true, Cell: func(c ctsvc.Contact) string { return c.Name }},
		{Header: "EMAIL", Flex: true, Cell: func(c ctsvc.Contact) string { return c.Email }},
		{Header: "PHONE", Cell: func(c ctsvc.Contact) string { return c.Phone }},
	}
}

func spec() ui.TableSpec[ctsvc.Contact] {
	return ui.TableSpec[ctsvc.Contact]{
		Noun: "contacts", Columns: columns(),
		Total: ui.Unknown, Page: ui.Unpaged,
	}
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List contacts",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepUnlock}, func(c *kit.Invocation) error {
			all, err := c.App.Contacts.List(c.Ctx, c.U)
			if err != nil {
				return err
			}
			return kit.List(c, spec(), all, func(ct ctsvc.Contact) []string { return []string{ct.ID} })
		}),
	}
}

func getCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get REF",
		Short: "Show one contact in full",
		Args:  cobra.ExactArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepExpand, kit.StepUnlock}, func(c *kit.Invocation) error {
			id, err := c.App.Contacts.Resolve(c.Ctx, c.U, c.Args[0])
			if err != nil {
				return err
			}
			ct, err := c.App.Contacts.Get(c.Ctx, c.U, id)
			if err != nil {
				return err
			}
			fields := []ui.Field{{Label: "Name", Value: ct.Name}}
			for _, e := range ct.Emails {
				fields = append(fields, ui.Field{Label: "Email", Value: e})
			}
			for _, p := range ct.Phones {
				fields = append(fields, ui.Field{Label: "Phone", Value: p})
			}
			fields = append(fields,
				ui.Field{Label: "Organization", Value: ct.Org},
				ui.Field{Label: "Job Title", Value: ct.Title},
				ui.Field{Label: "Birthday", Value: ct.Birthday},
				ui.Field{Label: "Address", Value: ct.Address},
				ui.Field{Label: "Website", Value: ct.URL},
				ui.Field{Label: "Note", Value: ct.Note},
				ui.Field{Label: "Signature", Value: string(ct.Signature), Always: true},
				ui.Field{Label: "ID", Value: ct.ID, ID: true},
			)
			return kit.Show(c, ui.RecordSpec{Object: ct, Fields: fields})
		}),
	}
}

// details are the fields a contact carries. create and update share them so the
// two commands can never drift apart on what a contact is.
type details struct {
	nc ctsvc.NewContact
}

func (d *details) register(c *cobra.Command, verb string) {
	f := c.Flags()
	f.StringVar(&d.nc.Name, "name", "", verb+" the contact's name")
	f.StringArrayVar(&d.nc.Emails, "email", nil, verb+" an email address (repeatable)")
	f.StringArrayVar(&d.nc.Phones, "phone", nil, verb+" a phone number (repeatable)")
	f.StringVar(&d.nc.Org, "organization", "", verb+" the organization")
	f.StringVar(&d.nc.Title, "job-title", "", verb+" the job title")
	f.StringVar(&d.nc.Birthday, "birthday", "", verb+" the birthday (e.g. 1990-01-31)")
	f.StringVar(&d.nc.Address, "address", "", verb+" the postal address")
	f.StringVar(&d.nc.URL, "website", "", verb+" the website")
	f.StringVar(&d.nc.Note, "note", "", verb+" the note")
}

func createCmd() *cobra.Command {
	var d details
	c := &cobra.Command{
		Use:   "create",
		Short: "Create a contact",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepUnlock}, func(c *kit.Invocation) error {
			if d.nc.Name == "" && len(d.nc.Emails) == 0 {
				return kit.Fail("A contact needs at least a name or an email address.").
					Hint("--name \"Jane Roe\"", "--email jane@example.com")
			}
			return kit.Create(c, ui.ResultSpec{
				Action: ui.Created, Kind: "contacts", Name: d.nc.Name,
			}, func() (string, error) {
				return c.App.Contacts.Create(c.Ctx, c.U, d.nc)
			})
		}),
	}
	d.register(c, "Set")
	return c
}

func updateCmd() *cobra.Command {
	var d details
	c := &cobra.Command{
		Use:   "update REF",
		Short: "Change a contact's details",
		Long: "Change a contact's details.\n\n" +
			"Only what you pass is replaced. --email and --phone replace the whole list\n" +
			"rather than adding to it, so pass every address you want the contact to keep.",
		Args: cobra.ExactArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepExpand, kit.StepUnlock}, func(c *kit.Invocation) error {
			id, err := c.App.Contacts.Resolve(c.Ctx, c.U, c.Args[0])
			if err != nil {
				return err
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Updated, Kind: "contacts", Count: 1,
				Name: d.nc.Name, IDs: []string{id},
			}, func() error {
				return c.App.Contacts.Update(c.Ctx, c.U, id, d.nc)
			})
		}),
	}
	d.register(c, "Replace")
	return c
}

func deleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete REF...",
		Short: "Delete contacts",
		Args:  cobra.MinimumNArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepExpand, kit.StepUnlock}, func(c *kit.Invocation) error {
			sel, err := kit.Select(c, kit.Selector[ctsvc.Contact]{
				Noun:    "contacts",
				Columns: columns(),
				IDOf:    func(ct ctsvc.Contact) string { return ct.ID },
				ByRef: func(ctx context.Context, ref string) (ctsvc.Contact, error) {
					id, err := c.App.Contacts.Resolve(ctx, c.U, ref)
					if err != nil {
						return ctsvc.Contact{}, err
					}
					ct, err := c.App.Contacts.Get(ctx, c.U, id)
					if err != nil {
						return ctsvc.Contact{}, err
					}
					return *ct, nil
				},
			})
			if err != nil {
				return err
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Deleted, Kind: "contacts", Count: sel.Len(), IDs: sel.IDs,
				Preview: sel.Preview(),
			}, func() error {
				return c.App.Contacts.Delete(c.Ctx, sel.IDs)
			})
		}),
	}
}
