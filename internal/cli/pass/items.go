package pass

import (
	stdctx "context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cmdruid/proton-cli/internal/cli/kit"
	"github.com/cmdruid/proton-cli/internal/otp"
	passsvc "github.com/cmdruid/proton-cli/internal/service/pass"
	"github.com/cmdruid/proton-cli/internal/ui"
	"github.com/cmdruid/proton-cli/internal/units"
	"github.com/spf13/cobra"
)

// itemTypes are the kinds of item Pass stores, spelled the way a CLI spells
// things. Proton's own names are camelCase (creditCard, sshKey); kebab-case is the
// convention everywhere else here, so these are the CLI's spelling of the same set.
var itemTypes = []string{"login", "note", "credit-card", "wifi", "ssh-key", "identity", "alias", "custom"}

func itemsCmd() *cobra.Command {
	c := &cobra.Command{Use: "items", Short: "Logins, notes, cards and the rest"}
	c.AddCommand(itemsListCmd(), itemsGetCmd(), itemsCreateCmd(), itemsUpdateCmd(),
		itemsRevisionsCmd(), itemsTOTPCmd(),
		itemsPinCmd("pin", "Keep items at the top of the list", ui.Pinned, true),
		itemsPinCmd("unpin", "Stop keeping items at the top", ui.Unpinned, false),
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

// itemOrder is how a Pass listing may be ordered. Every vault's items arrive as
// one batch and are decrypted here, so the whole set is in hand.
func itemOrder() kit.Comparators[passsvc.Item] {
	return kit.Comparators[passsvc.Item]{
		"name":     func(a, b passsvc.Item) int { return kit.Fold(a.Name, b.Name) },
		"type":     func(a, b passsvc.Item) int { return kit.Fold(a.Type, b.Type) },
		"modified": func(a, b passsvc.Item) int { return kit.Ints(a.ModifyTime, b.ModifyTime) },
		"created":  func(a, b passsvc.Item) int { return kit.Ints(a.CreateTime, b.CreateTime) },
	}
}

func itemsListCmd() *cobra.Command {
	var f filters
	var page kit.Page
	var order kit.Order
	c := &cobra.Command{
		Use:   "list",
		Short: "List items across your vaults",
		Long: "List items across your vaults.\n\n" +
			"The filters are the same ones trash and delete take, so a selection can be\n" +
			"worked out here before it is handed to a verb that acts on it.",
		Args: cobra.NoArgs,
		RunE: kit.Run(nil, func(c *kit.Invocation) error {
			items, err := matchItems(c.Ctx, c, &f)
			if err != nil {
				return err
			}
			if err := kit.Sort(order, items, itemOrder()); err != nil {
				return err
			}
			rows, total := kit.Slice(page, items)
			return kit.List(c, ui.TableSpec[passsvc.Item]{
				Noun: "items", Columns: itemColumns(),
				Total: total, Page: page.Number, PageSize: page.Size,
				Filtered: f.narrowed(),
			}, rows, func(it passsvc.Item) []string { return []string{it.ShareID, it.ItemID} })
		}),
	}
	f.registerNarrowing(c)
	order.Register(c, "name", "type", "modified", "created")
	page.Register(c, "items")
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
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			shareID, itemID, err := resolveItem(c, c.Args[0])
			if err != nil {
				return err
			}
			it, err := c.App.Pass.ItemGet(c.Ctx, shareID, itemID)
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
		{Label: "Alias", Value: it.Alias},
		{Label: "Status", Value: it.AliasStatus},
	}
	for _, m := range it.AliasMailboxes {
		fields = append(fields, ui.Field{Label: "Forwards To", Value: m})
	}
	fields = append(fields,
		ui.Field{Label: "Display Name", Value: it.AliasDisplayName},
		ui.Field{Label: "SimpleLogin Note", Value: it.AliasNote},
		ui.Field{Label: "Activity", Value: activity(it.AliasActivity)},
		ui.Field{Label: "Password", Value: it.Password},
		ui.Field{Label: "TOTP", Value: it.TOTP},
	)
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
	)
	// An identity's fields are declared once in the service, so the record shows
	// whatever the declaration holds rather than a second list that can drift.
	for _, f := range passsvc.IdentityFields {
		fields = append(fields, ui.Field{Label: f.Label, Value: it.Identity[f.Flag]})
	}
	fields = append(fields, ui.Field{Label: "Note", Value: it.Note})
	// A field is labelled the way --field accepts it, so what a record shows can
	// be handed straight back to a command.
	for _, f := range it.Fields {
		fields = append(fields, ui.Field{Label: f.Ref(), Value: f.Value})
	}
	return append(fields, ui.Field{Label: "ID", Value: itemRef(*it), ID: true})
}

// activity is what an alias has carried lately, over the fourteen days Proton
// counts. An alias nobody has written to yet reports zeros, which is an answer;
// anything that is not an alias reports nothing.
func activity(a *passsvc.AliasActivity) string {
	if a == nil {
		return ""
	}
	return fmt.Sprintf("%d forwarded, %d replied, %d blocked (last 14 days)",
		a.Forwarded, a.Replied, a.Blocked)
}

// sharedWithEveryItem names the identity fields that already have a flag, because
// the idea is not particular to an identity.
var sharedWithEveryItem = map[string]bool{"email": true}

// checkFields refuses a field nothing can read, and a section on an item that
// has no place for one. Each refusal points at its own fix.
//
// An empty type is one not yet known, which is what an edit has until the item
// is fetched: the grammar can still be judged, the placement cannot.
func (d *fields) checkFields(itemType string) error {
	// A two-factor secret is a two-factor secret whichever flag carried it, so
	// --totp-uri is held to what --totp-field is: a value no code can come out of
	// is refused here rather than stored and found useless later.
	if d.nc.TOTP != "" {
		if _, err := otp.Parse(d.nc.TOTP); err != nil {
			return kit.Fail("--totp-uri: %v.", err).
				Hint("an otpauth:// URI, or the secret on its own")
		}
	}
	if err := passsvc.CheckFields(d.nc.ExtraFields); err != nil {
		return kit.Fail("%s.", capitalise(err.Error())).
			Hint("--field NAME=VALUE", "--field SECTION/NAME=VALUE")
	}
	if itemType == "" {
		return nil
	}
	if err := passsvc.CheckSections(itemType, d.nc.ExtraFields); err != nil {
		return kit.Fail("%s.", capitalise(err.Error())).
			Hint("drop the section, or --type custom")
	}
	return nil
}

// capitalise makes a service's error read as the sentence a refusal is.
func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// identityValues is what the identity flags were given, keyed by flag. The
// shared ones are folded back in, so an identity built from --email carries it.
func (d *fields) identityValues() map[string]string {
	out := make(map[string]string, len(d.identity)+len(sharedWithEveryItem))
	for flag, v := range d.identity {
		if *v != "" {
			out[flag] = *v
		}
	}
	if d.nc.Email != "" {
		out["email"] = d.nc.Email
	}
	return out
}

// fields are everything an item can carry. create and update share them so the two
// commands cannot drift on what an item is.
type fields struct {
	nc passsvc.NewItem
	// identity holds one string per declared identity field. The set is the
	// service's to declare, so this is a map rather than thirty named members
	// that would have to be kept in step with it.
	identity map[string]*string
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
	// A field states its own section, so the flag stays one self-contained token
	// that can be given in any order and read back exactly as it was written.
	f.StringArrayVar(&d.nc.ExtraFields.Text, "field", nil,
		verb+" a custom field, as NAME=VALUE or SECTION/NAME=VALUE (repeatable)")
	f.StringArrayVar(&d.nc.ExtraFields.Hidden, "hidden", nil,
		verb+" a hidden custom field, as NAME=VALUE or SECTION/NAME=VALUE (repeatable)")
	// --totp is the two-factor code a re-authentication asks for, so a field
	// holding a secret that generates one needs a name of its own.
	f.StringArrayVar(&d.nc.ExtraFields.TOTP, "totp-field", nil,
		verb+" a custom field holding a two-factor secret, as NAME=URI (repeatable)")
	f.StringVar(&d.nc.Holder, "holder", "", verb+" the cardholder's name (credit-card)")
	f.StringVar(&d.nc.Number, "number", "", verb+" the card number (credit-card)")
	f.StringVar(&d.nc.Expiry, "expiry", "", verb+" the card expiry, YYYY-MM (credit-card)")
	f.StringVar(&d.nc.CVV, "cvv", "", verb+" the card's CVV (credit-card)")
	f.StringVar(&d.nc.PIN, "pin", "", verb+" the card's PIN (credit-card)")
	f.StringVar(&d.nc.SSID, "ssid", "", verb+" the network name (wifi)")
	f.StringVar(&d.nc.PrivateKey, "private-key", "", verb+" the private key (ssh-key)")
	f.StringVar(&d.nc.PublicKey, "public-key", "", verb+" the public key (ssh-key)")
	// An identity's fields are declared once in the service; the flags are made
	// from that declaration so the two can never offer different sets.
	d.identity = make(map[string]*string, len(passsvc.IdentityFields))
	for _, id := range passsvc.IdentityFields {
		// An address is an address whichever kind of item holds one, so an
		// identity's email is the same --email a login takes rather than a second
		// flag meaning the same thing.
		if sharedWithEveryItem[id.Flag] {
			continue
		}
		v := new(string)
		d.identity[id.Flag] = v
		f.StringVar(v, id.Flag, "", verb+" the "+strings.ToLower(id.Label)+" (identity)")
	}
}

// aliasFields are the two an alias carries beside the item's own, which only an
// edit takes: an alias is born from `aliases create`, since Proton and not this
// CLI decides what its address is.
type aliasFields struct {
	patch passsvc.AliasPatch
}

func (a *aliasFields) register(c *cobra.Command) {
	f := c.Flags()
	f.StringArrayVar(&a.patch.Mailboxes, "mailbox", nil,
		"Replace where mail to it arrives (alias, repeatable)")
	f.StringVar(&a.patch.DisplayName, "display-name", "",
		"Replace the name recipients see on mail from it (alias)")
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
		// What a field says, and whether this kind of item has a section to put it
		// under, are both answerable from the command line alone.
		RunE: kit.Run([]kit.Step{func(*kit.Invocation) error {
			kind, err := itemType.Value()
			if err != nil {
				return err
			}
			return d.checkFields(kind)
		}}, func(c *kit.Invocation) error {
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
			d.nc.Identity = d.identityValues()
			shareID, err := resolveVault(c, vault)
			if err != nil {
				return err
			}
			return kit.Create(c, ui.ResultSpec{
				Action: ui.Created, Kind: "items", Name: d.nc.Name,
				Extra: map[string]any{"type": kind},
			}, func() (string, error) {
				itemID, err := c.App.Pass.ItemCreate(c.Ctx, shareID, d.nc)
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
	return c
}

func itemsUpdateCmd() *cobra.Command {
	var d fields
	var a aliasFields
	security := &kit.Enum{
		Name: "security", Usage: "Wi-Fi security (wifi)",
		Values: []string{"WPA", "WPA2", "WPA3", "WEP"},
	}
	c := &cobra.Command{
		Use:   "update REF",
		Short: "Change an item's fields",
		Args:  cobra.ExactArgs(1),
		// Whether the item has a section to put a field under depends on what
		// kind it is, which is not known until it is read; what a field says is
		// known now.
		RunE: kit.Run([]kit.Step{kit.StepExpand, func(*kit.Invocation) error {
			return d.checkFields("")
		}}, func(c *kit.Invocation) error {
			wifi, err := security.Value()
			if err != nil {
				return err
			}
			shareID, itemID, err := resolveItem(c, c.Args[0])
			if err != nil {
				return err
			}
			patch := passsvc.Patch{
				Scalars: passsvc.Scalars{
					Name: d.nc.Name, Username: d.nc.Username, Password: d.nc.Password,
					Email: d.nc.Email, URL: d.nc.URL, TOTP: d.nc.TOTP, Note: d.nc.Note,
					Holder: d.nc.Holder, Number: d.nc.Number, Expiry: d.nc.Expiry,
					CVV: d.nc.CVV, PIN: d.nc.PIN, SSID: d.nc.SSID, WifiSecurity: wifi,
					PrivateKey: d.nc.PrivateKey, PublicKey: d.nc.PublicKey,
				},
				Identity:    d.identityValues(),
				ExtraFields: d.nc.ExtraFields,
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Updated, Kind: "items", Count: 1, Name: d.nc.Name,
				IDs: []string{kit.JoinPair(shareID, itemID)},
			}, func() error {
				if err := c.App.Pass.AliasEdit(c.Ctx, shareID, itemID, a.patch); err != nil {
					return err
				}
				return c.App.Pass.ItemEdit(c.Ctx, shareID, itemID, patch)
			})
		}),
	}
	d.register(c, "Replace")
	a.register(c)
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
			kit.StepSelection(f.set, itemFilterHint, itemScope), kit.StepExpand,
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

// registerNarrowing adds the flags that say which items, and nothing else. See
// the same method on Drive's filters for why `list` shares them.
func (f *filters) registerNarrowing(c *cobra.Command) {
	f.itemType = &kit.Enum{Name: "type", Usage: "Match only this kind of item", Values: itemTypes}
	f.itemType.Register(c)
	fl := c.Flags()
	fl.StringVar(&f.vault, "vault", "", "Match only this vault, by name or ID")
	f.age.Register(fl, "items")
}

func (f *filters) register(c *cobra.Command) {
	f.registerNarrowing(c)
	kit.All(c.Flags(), &f.all)
}

// narrowed reports whether the user asked for a subset of what is there.
func (f *filters) narrowed() bool {
	return f.vault != "" || f.itemType.Set() || f.age.Set()
}

func (f *filters) set() bool { return f.narrowed() || f.all }

const (
	itemFilterHint = "--vault, --type or --older-than"
	// itemScope is what --all covers when nothing narrows it.
	itemScope = "a whole vault"
)

func selectItems(c *kit.Invocation, f *filters) (kit.Selection[passsvc.Item], error) {
	if f.all && f.vault == "" && !f.itemType.Set() && !f.age.Set() {
		c.Warn("--all with no other filter covers every vault. Add --vault to narrow it.")
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
			it, err := c.App.Pass.ItemGet(ctx, shareID, itemID)
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
	items, err := c.App.Pass.ItemsList(ctx, vaultRef)
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

// ── pinning and history ──

// Pinning puts an item at the top of the list. It carries no content, so nothing
// is encrypted: it is the vault recording that one of its items is wanted often.
func itemsPinCmd(use, short string, action ui.Action, pinned bool) *cobra.Command {
	return &cobra.Command{
		Use:   use + " REF...",
		Short: short,
		Args:  cobra.MinimumNArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			type target struct{ share, item, name string }
			targets := make([]target, 0, len(c.Args))
			for _, ref := range c.Args {
				shareID, itemID, err := resolveItem(c, ref)
				if err != nil {
					return err
				}
				it, err := c.App.Pass.ItemGet(c.Ctx, shareID, itemID)
				if err != nil {
					return err
				}
				targets = append(targets, target{shareID, itemID, it.Name})
			}
			ids := make([]string, 0, len(targets))
			for _, t := range targets {
				ids = append(ids, kit.JoinPair(t.share, t.item))
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: action, Kind: "items", Count: len(targets), IDs: ids,
				Name: kit.Sole(targets, func(t target) string { return t.name }),
			}, func() error {
				for _, t := range targets {
					if err := c.App.Pass.ItemPin(c.Ctx, t.share, t.item, pinned); err != nil {
						return err
					}
				}
				return nil
			})
		}),
	}
}

// An item's revisions are what it used to be. Pass keeps every edit, so a
// password changed by mistake is recoverable by reading what it was.
//
// The word is `revisions`, as it is in Drive: the same idea gets the same word,
// whichever app it is in.
func itemsRevisionsCmd() *cobra.Command {
	c := &cobra.Command{Use: "revisions", Short: "Earlier versions of an item"}
	c.AddCommand(itemsRevisionsListCmd())
	return c
}

func itemsRevisionsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list REF",
		Short: "Show what an item used to be",
		Long: "Show what an item used to be.\n\n" +
			"Pass keeps every edit, so a password changed by mistake can be read back.\n" +
			"Newest first. Use --output json for the full contents of each revision.",
		Args: cobra.ExactArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			shareID, itemID, err := resolveItem(c, c.Args[0])
			if err != nil {
				return err
			}
			revs, err := c.App.Pass.ItemHistory(c.Ctx, shareID, itemID)
			if err != nil {
				return err
			}
			return kit.List(c, ui.TableSpec[passsvc.Revision]{
				Noun:  "revisions",
				Total: ui.Unknown, Page: ui.Unpaged,
				Columns: []ui.Column[passsvc.Revision]{
					{Header: "REVISION", Right: true, Cell: func(r passsvc.Revision) string {
						return strconv.Itoa(r.Revision)
					}},
					{Header: "CHANGED", Cell: func(r passsvc.Revision) string {
						return units.Time(r.ModifyTime)
					}},
					{Header: "NAME", Flex: true, Cell: func(r passsvc.Revision) string {
						if r.Item == nil {
							return "(cannot be read)"
						}
						return r.Item.Name
					}},
					{Header: "USERNAME", Flex: true, Cell: func(r passsvc.Revision) string {
						if r.Item == nil {
							return ""
						}
						if r.Item.Username != "" {
							return r.Item.Username
						}
						return r.Item.Email
					}},
				},
			}, revs, nil)
		}),
	}
}

// ── the code behind a stored secret ──

// Pass stores the TOTP secret, not the code, so every client works the code out
// for itself. `items get` prints the secret because that is the thing being
// stored; this prints what it currently stands for, which is what a script or a
// login prompt wants.
func itemsTOTPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "totp REF",
		Short: "Print the current two-factor code for an item",
		Long: "Print the current two-factor code for an item.\n\n" +
			"How long the code has left is reported beside it, because a code about to\n" +
			"expire is one worth waiting out.\n\n" +
			"For a script: --output json, then read .code.",
		Args: cobra.ExactArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			shareID, itemID, err := resolveItem(c, c.Args[0])
			if err != nil {
				return err
			}
			it, err := c.App.Pass.ItemGet(c.Ctx, shareID, itemID)
			if err != nil {
				return err
			}
			secret := it.TOTP
			if secret == "" {
				// A custom field can hold one too, which is where a second factor
				// for the same login usually lands.
				for _, f := range it.Fields {
					if f.Type == "totp" && f.Value != "" {
						secret = f.Value
						break
					}
				}
			}
			if secret == "" {
				return kit.Fail("%s carries no two-factor secret.", it.Name).
					Hint("`pass items update " + c.Args[0] + " --totp-uri …` stores one.")
			}
			s, err := otp.Parse(secret)
			if err != nil {
				return kit.Fail("%v", err)
			}
			code, err := s.Now()
			if err != nil {
				return kit.Fail("%v", err)
			}
			return kit.Show(c, ui.RecordSpec{
				Object: code,
				Fields: []ui.Field{
					{Label: "Code", Value: code.Code},
					{Label: "Expires In", Value: ui.Quantity(code.Expires, "seconds")},
				},
			})
		}),
	}
}
