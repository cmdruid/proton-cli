package pass

import (
	stdctx "context"
	"time"

	"github.com/roman-16/proton-cli/internal/cli/kit"
	passsvc "github.com/roman-16/proton-cli/internal/service/pass"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/roman-16/proton-cli/internal/units"
	"github.com/spf13/cobra"
)

// itemTypes are the kinds of item Pass stores, spelled the way a CLI spells
// things. Proton's own names are camelCase (creditCard, sshKey); kebab-case is the
// convention everywhere else here, so these are the CLI's spelling of the same set.
var itemTypes = []string{"login", "note", "credit-card", "wifi", "ssh-key", "identity", "alias", "custom"}

func itemsCmd() *cobra.Command {
	c := &cobra.Command{Use: "items", Short: "Logins, notes, cards and the rest"}
	c.AddCommand(itemsListCmd(), itemsGetCmd(), itemsCreateCmd(), itemsUpdateCmd(),
		itemsTrashCmd(), itemsDeleteCmd())
	return c
}

func itemColumns() []ui.Column[passsvc.Item] {
	return []ui.Column[passsvc.Item]{
		{Header: "ID", ID: true, Cell: itemRef},
		{Header: "TYPE", Cell: func(it passsvc.Item) string { return it.Type }},
		{Header: "NAME", Flex: true, Cell: func(it passsvc.Item) string { return it.Name }},
		{Header: "USERNAME", Flex: true, Cell: func(it passsvc.Item) string {
			if it.Username != "" {
				return it.Username
			}
			return it.Email
		}},
		{Header: "MODIFIED", Cell: func(it passsvc.Item) string { return units.Time(it.ModifyTime) }},
	}
}

func itemsListCmd() *cobra.Command {
	var vault string
	itemType := &kit.Enum{Name: "type", Usage: "Show only this kind of item", Values: itemTypes}
	c := &cobra.Command{
		Use:   "list",
		Short: "List items across your vaults",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepUnlock}, func(c *kit.Invocation) error {
			kind, err := itemType.Value()
			if err != nil {
				return err
			}
			vaultRef, err := kit.Expand(c.App, vault)
			if err != nil {
				return err
			}
			items, err := c.App.Pass.ItemsList(c.Ctx, c.U, vaultRef)
			if err != nil {
				return err
			}
			if kind != "" {
				items = keepType(items, kind)
			}
			return kit.List(c, ui.TableSpec[passsvc.Item]{
				Noun: "items", Columns: itemColumns(),
				Total: ui.Unknown, Page: ui.Unpaged,
			}, items, func(it passsvc.Item) []string { return []string{it.ShareID, it.ItemID} })
		}),
	}
	c.Flags().StringVar(&vault, "vault", "", "Show only this vault, by name or ID")
	itemType.Register(c)
	return c
}

func itemsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get REF",
		Short: "Show one item, decrypted",
		Long: "Show one item, decrypted.\n\n" +
			"Passwords, TOTP secrets and private keys are printed in full: this is the\n" +
			"command for reading a secret, so it does not hide one.",
		Args: cobra.ExactArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepExpand, kit.StepUnlock}, func(c *kit.Invocation) error {
			shareID, itemID, err := resolveItem(c, c.Args[0])
			if err != nil {
				return err
			}
			it, err := c.App.Pass.ItemGet(c.Ctx, c.U, shareID, itemID)
			if err != nil {
				return err
			}
			return kit.Show(c, ui.RecordSpec{Object: it, Fields: itemFields(it)})
		}),
	}
}

// itemFields is the record for one item. Every kind shares it: an empty field is
// dropped, so a note shows a note's fields and a card shows a card's without
// either needing a layout of its own.
func itemFields(it *passsvc.Item) []ui.Field {
	fields := []ui.Field{
		{Label: "Type", Value: it.Type},
		{Label: "Name", Value: it.Name},
		{Label: "Username", Value: it.Username},
		{Label: "Email", Value: it.Email},
		{Label: "Password", Value: it.Password},
		{Label: "TOTP", Value: it.TOTP},
	}
	for _, u := range it.URLs {
		fields = append(fields, ui.Field{Label: "URL", Value: u})
	}
	fields = append(fields,
		ui.Field{Label: "Cardholder", Value: it.Holder},
		ui.Field{Label: "Number", Value: it.Number},
		ui.Field{Label: "Expiry", Value: it.Expiry},
		ui.Field{Label: "CVV", Value: it.CVV},
		ui.Field{Label: "PIN", Value: it.PIN},
		ui.Field{Label: "SSID", Value: it.SSID},
		ui.Field{Label: "Public Key", Value: it.PublicKey},
		ui.Field{Label: "Private Key", Value: it.PrivateKey},
		ui.Field{Label: "Full Name", Value: it.FullName},
		ui.Field{Label: "First Name", Value: it.FirstName},
		ui.Field{Label: "Last Name", Value: it.LastName},
		ui.Field{Label: "Phone", Value: it.Phone},
		ui.Field{Label: "Organization", Value: it.Organization},
		ui.Field{Label: "Job Title", Value: it.JobTitle},
		ui.Field{Label: "Address", Value: it.StreetAddress},
		ui.Field{Label: "City", Value: it.City},
		ui.Field{Label: "Postal Code", Value: it.PostalCode},
		ui.Field{Label: "Country", Value: it.Country},
		ui.Field{Label: "Birthdate", Value: it.Birthdate},
		ui.Field{Label: "Website", Value: it.Website},
		ui.Field{Label: "Note", Value: it.Note},
	)
	for _, f := range it.Fields {
		fields = append(fields, ui.Field{Label: f.Name, Value: f.Value})
	}
	return append(fields, ui.Field{Label: "ID", Value: itemRef(*it), ID: true})
}

// fields are everything an item can carry. create and update share them so the two
// commands cannot drift on what an item is.
type fields struct {
	nc passsvc.NewItem
}

func (d *fields) register(c *cobra.Command, verb string) {
	f := c.Flags()
	f.StringVar(&d.nc.Name, "name", "", verb+" the item's name")
	f.StringVar(&d.nc.Username, "username", "", verb+" the username (login)")
	f.StringVar(&d.nc.Password, "password", "", verb+" the password (login, wifi)")
	f.StringVar(&d.nc.Email, "email", "", verb+" the email address (login)")
	f.StringVar(&d.nc.URL, "url", "", verb+" the URL (login)")
	f.StringVar(&d.nc.TOTP, "totp-uri", "", verb+" the TOTP URI or secret (login)")
	f.StringVar(&d.nc.Note, "note", "", verb+" the note")
	f.StringVar(&d.nc.Holder, "holder", "", verb+" the cardholder's name (credit-card)")
	f.StringVar(&d.nc.Number, "number", "", verb+" the card number (credit-card)")
	f.StringVar(&d.nc.Expiry, "expiry", "", verb+" the card expiry, YYYY-MM (credit-card)")
	f.StringVar(&d.nc.CVV, "cvv", "", verb+" the card's CVV (credit-card)")
	f.StringVar(&d.nc.PIN, "pin", "", verb+" the card's PIN (credit-card)")
	f.StringVar(&d.nc.SSID, "ssid", "", verb+" the network name (wifi)")
	f.StringVar(&d.nc.PrivateKey, "private-key", "", verb+" the private key (ssh-key)")
	f.StringVar(&d.nc.PublicKey, "public-key", "", verb+" the public key (ssh-key)")
	f.StringVar(&d.nc.FullName, "full-name", "", verb+" the full name (identity)")
	f.StringVar(&d.nc.FirstName, "first-name", "", verb+" the first name (identity)")
	f.StringVar(&d.nc.LastName, "last-name", "", verb+" the last name (identity)")
	f.StringVar(&d.nc.PhoneNumber, "phone", "", verb+" the phone number (identity)")
	f.StringVar(&d.nc.Organization, "organization", "", verb+" the organization (identity)")
	f.StringVar(&d.nc.JobTitle, "job-title", "", verb+" the job title (identity)")
	f.StringVar(&d.nc.StreetAddress, "address", "", verb+" the street address (identity)")
	f.StringVar(&d.nc.City, "city", "", verb+" the city (identity)")
	f.StringVar(&d.nc.PostalCode, "postal-code", "", verb+" the postal code (identity)")
	f.StringVar(&d.nc.Country, "country", "", verb+" the country (identity)")
	f.StringVar(&d.nc.Birthdate, "birthdate", "", verb+" the birthdate (identity)")
	f.StringVar(&d.nc.Website, "website", "", verb+" the website (identity)")
}

func itemsCreateCmd() *cobra.Command {
	var d fields
	var vault string
	itemType := &kit.Enum{
		Name: "type", Usage: "What kind of item", Default: "login", Values: itemTypes,
	}
	security := &kit.Enum{
		Name: "security", Usage: "Wi-Fi security (wifi)",
		Values: []string{"WPA", "WPA2", "WPA3", "WEP"},
	}
	c := &cobra.Command{
		Use:   "create",
		Short: "Create an item",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepUnlock}, func(c *kit.Invocation) error {
			kind, err := itemType.Value()
			if err != nil {
				return err
			}
			wifi, err := security.Value()
			if err != nil {
				return err
			}
			if d.nc.Name == "" {
				return kit.Fail("An item needs a name.").Hint("--name GitHub")
			}
			d.nc.Type, d.nc.WifiSecurity = kind, wifi
			shareID, err := resolveVault(c, vault)
			if err != nil {
				return err
			}
			return kit.Create(c, ui.ResultSpec{
				Action: ui.Created, Kind: "items", Name: d.nc.Name,
				Extra: map[string]any{"type": kind},
			}, func() (string, error) {
				itemID, err := c.App.Pass.ItemCreate(c.Ctx, c.U, shareID, d.nc)
				if err != nil {
					return "", err
				}
				return kit.JoinPair(shareID, itemID), nil
			})
		}),
	}
	d.register(c, "Set")
	itemType.Register(c)
	security.Register(c)
	c.Flags().StringVar(&vault, "vault", "", "Which vault, by name or ID (default: your first)")
	c.Flags().StringArrayVar(&d.nc.Fields, "field", nil, "Custom field NAME=VALUE (repeatable)")
	c.Flags().StringArrayVar(&d.nc.HiddenFields, "hidden", nil, "Hidden custom field NAME=VALUE (repeatable)")
	return c
}

func itemsUpdateCmd() *cobra.Command {
	var d fields
	security := &kit.Enum{
		Name: "security", Usage: "Wi-Fi security (wifi)",
		Values: []string{"WPA", "WPA2", "WPA3", "WEP"},
	}
	c := &cobra.Command{
		Use:   "update REF",
		Short: "Change an item's fields",
		Args:  cobra.ExactArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepExpand, kit.StepUnlock}, func(c *kit.Invocation) error {
			wifi, err := security.Value()
			if err != nil {
				return err
			}
			shareID, itemID, err := resolveItem(c, c.Args[0])
			if err != nil {
				return err
			}
			patch := passsvc.Patch{
				Name: d.nc.Name, Username: d.nc.Username, Password: d.nc.Password,
				Email: d.nc.Email, URL: d.nc.URL, TOTP: d.nc.TOTP, Note: d.nc.Note,
				Holder: d.nc.Holder, Number: d.nc.Number, Expiry: d.nc.Expiry,
				CVV: d.nc.CVV, PIN: d.nc.PIN, SSID: d.nc.SSID, WifiSecurity: wifi,
				PrivateKey: d.nc.PrivateKey, PublicKey: d.nc.PublicKey,
				FullName: d.nc.FullName, FirstName: d.nc.FirstName, LastName: d.nc.LastName,
				PhoneNumber: d.nc.PhoneNumber, Organization: d.nc.Organization,
				JobTitle: d.nc.JobTitle, StreetAddress: d.nc.StreetAddress,
				City: d.nc.City, PostalCode: d.nc.PostalCode, Country: d.nc.Country,
				Birthdate: d.nc.Birthdate, Website: d.nc.Website,
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Updated, Kind: "items", Count: 1, Name: d.nc.Name,
				IDs: []string{kit.JoinPair(shareID, itemID)},
			}, func() error {
				return c.App.Pass.ItemEdit(c.Ctx, c.U, shareID, itemID, patch)
			})
		}),
	}
	d.register(c, "Replace")
	security.Register(c)
	return c
}

// ── removing ──

func itemsTrashCmd() *cobra.Command {
	return bulkItemCmd("trash", "Move items to the trash", ui.Trashed, "to trash",
		func(c *kit.Invocation, share, item string) error {
			return c.App.Pass.ItemTrash(c.Ctx, share, item)
		})
}

func itemsDeleteCmd() *cobra.Command {
	return bulkItemCmd("delete", "Delete items permanently", ui.Deleted, "",
		func(c *kit.Invocation, share, item string) error {
			return c.App.Pass.ItemDelete(c.Ctx, share, item)
		})
}

func bulkItemCmd(use, short string, action ui.Action, detail string,
	apply func(*kit.Invocation, string, string) error) *cobra.Command {
	var f filters
	c := &cobra.Command{
		Use:   use + " [REF...]",
		Short: short,
		RunE: kit.Run([]kit.Step{
			kit.StepSelection(f.set, itemFilterHint, itemScope), kit.StepExpand, kit.StepUnlock,
		}, func(c *kit.Invocation) error {
			sel, err := selectItems(c, &f)
			if err != nil {
				return err
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: action, Kind: "items", Count: sel.Len(), IDs: sel.IDs,
				Detail: detail, Preview: sel.Preview(),
			}, func() error {
				for _, it := range sel.Rows {
					if err := apply(c, it.ShareID, it.ItemID); err != nil {
						return err
					}
				}
				return nil
			})
		}),
	}
	f.register(c)
	return c
}

// ── selection ──

type filters struct {
	vault    string
	itemType *kit.Enum
	age      kit.Range
	all      bool
}

func (f *filters) register(c *cobra.Command) {
	f.itemType = &kit.Enum{Name: "type", Usage: "Match only this kind of item", Values: itemTypes}
	f.itemType.Register(c)
	fl := c.Flags()
	fl.StringVar(&f.vault, "vault", "", "Match only this vault, by name or ID")
	f.age.Register(fl, "items")
	fl.BoolVar(&f.all, "all", false, "Confirm that no narrowing filter means everything in scope")
}

func (f *filters) set() bool {
	return f.vault != "" || f.itemType.Set() || f.age.Set() || f.all
}

const (
	itemFilterHint = "--vault, --type or --older-than"
	// itemScope is what --all covers when nothing narrows it.
	itemScope = "a whole vault"
)

func selectItems(c *kit.Invocation, f *filters) (kit.Selection[passsvc.Item], error) {
	if f.all && f.vault == "" && !f.itemType.Set() && !f.age.Set() {
		c.Note("--all with no other filter covers every vault. Add --vault to narrow it.")
	}
	sel := kit.Selector[passsvc.Item]{
		Noun:       "items",
		Columns:    itemColumns(),
		IDOf:       itemRef,
		FilterHint: itemFilterHint,
		Scope:      itemScope,
		ByRef: func(ctx stdctx.Context, ref string) (passsvc.Item, error) {
			shareID, itemID, err := resolveItem(c, ref)
			if err != nil {
				return passsvc.Item{}, err
			}
			it, err := c.App.Pass.ItemGet(ctx, c.U, shareID, itemID)
			if err != nil {
				return passsvc.Item{}, err
			}
			return *it, nil
		},
	}
	if f.set() {
		sel.ByFilter = func(ctx stdctx.Context) ([]passsvc.Item, error) {
			return matchItems(ctx, c, f)
		}
	}
	return kit.Select(c, sel)
}

func matchItems(ctx stdctx.Context, c *kit.Invocation, f *filters) ([]passsvc.Item, error) {
	kind, err := f.itemType.Value()
	if err != nil {
		return nil, err
	}
	var olderThan, newerThan int64
	if f.age.OlderThan != "" {
		d, err := units.ParseDuration(f.age.OlderThan)
		if err != nil {
			return nil, kit.Fail("--older-than: %v", err)
		}
		olderThan = time.Now().Add(-d).Unix()
	}
	if f.age.NewerThan != "" {
		d, err := units.ParseDuration(f.age.NewerThan)
		if err != nil {
			return nil, kit.Fail("--newer-than: %v", err)
		}
		newerThan = time.Now().Add(-d).Unix()
	}
	vaultRef, err := kit.Expand(c.App, f.vault)
	if err != nil {
		return nil, err
	}
	items, err := c.App.Pass.ItemsList(ctx, c.U, vaultRef)
	if err != nil {
		return nil, err
	}
	out := make([]passsvc.Item, 0, len(items))
	for _, it := range items {
		if kind != "" && it.Type != kind {
			continue
		}
		if olderThan != 0 && it.ModifyTime > olderThan {
			continue
		}
		if newerThan != 0 && it.ModifyTime < newerThan {
			continue
		}
		out = append(out, it)
	}
	return out, nil
}

func keepType(items []passsvc.Item, kind string) []passsvc.Item {
	out := items[:0]
	for _, it := range items {
		if it.Type == kind {
			out = append(out, it)
		}
	}
	return out
}
