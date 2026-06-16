package cli

import (
	"fmt"
	"time"

	"github.com/roman-16/proton-cli/internal/crypto/keys"
	"github.com/roman-16/proton-cli/internal/render"
	passsvc "github.com/roman-16/proton-cli/internal/service/pass"
	"github.com/roman-16/proton-cli/internal/view"
	"github.com/spf13/cobra"
)

func newPassCmd() *cobra.Command {
	c := &cobra.Command{Use: "pass", Short: "Password manager operations"}
	c.AddCommand(passItemsCmd(), passVaultsCmd(), passAliasCmd())
	return c
}

// ── pass items ──

func passItemsCmd() *cobra.Command {
	c := &cobra.Command{Use: "items", Short: "Manage Pass items"}

	var listVault string
	list := &cobra.Command{
		Use: "list", Short: "List Pass items across vaults",
		RunE: run([]Step{stepAuth, stepUnlock}, func(c *Ctx) error {
			listVaultRef, err := resolvePrefix(c.App, listVault)
			if err != nil {
				return err
			}
			items, err := c.App.Pass.ItemsList(c.Ctx, c.U, listVaultRef)
			if err != nil {
				return err
			}
			if err := view.Render(c.R(), c.short(), c.App.IDCache, view.List[passsvc.Item]{
				Columns: []view.Column[passsvc.Item]{
					{Header: "TYPE", Cell: func(it passsvc.Item) string { return it.Type }},
					{Header: "NAME", Cell: func(it passsvc.Item) string { return it.Name }},
					{Header: "USERNAME", Cell: func(it passsvc.Item) string {
						if it.Username != "" {
							return it.Username
						}
						return it.Email
					}},
					{Header: "SHARE_ID", ID: true, Cell: func(it passsvc.Item) string { return it.ShareID }},
					{Header: "ITEM_ID", ID: true, Cell: func(it passsvc.Item) string { return it.ItemID }},
				},
				CacheIDs: func(it passsvc.Item) []string { return []string{it.ShareID, it.ItemID} },
			}, items); err != nil {
				return err
			}
			if c.R().Format == render.FormatText {
				_, _ = fmt.Fprintf(c.R().Stderr, "\n%d item(s)\n", len(items))
			}
			return nil
		}),
	}
	list.Flags().StringVar(&listVault, "vault", "", "Filter by vault name or ID")
	c.AddCommand(list)

	c.AddCommand(&cobra.Command{
		Use: "get {SHARE_ID ITEM_ID | SEARCH}", Short: "Get a Pass item (decrypted)",
		Args: cobra.RangeArgs(1, 2),
		RunE: run([]Step{stepAuth, stepResolve, stepUnlock}, func(c *Ctx) error {
			shareID, itemID, err := c.App.Pass.ResolveItem(c.Ctx, c.U, c.Args)
			if err != nil {
				return err
			}
			it, err := c.App.Pass.ItemGet(c.Ctx, c.U, shareID, itemID)
			if err != nil {
				return err
			}
			if c.R().Format != render.FormatText {
				return c.R().Object(it)
			}
			printPassItem(c, it)
			return nil
		}),
	})

	var nc passsvc.NewItem
	var createVault string
	create := &cobra.Command{
		Use: "create", Short: "Create a Pass item (login, note, card)",
		RunE: run([]Step{stepAuth, stepUnlock}, func(c *Ctx) error {
			if nc.Name == "" {
				return fmt.Errorf("--name is required")
			}
			createVaultRef, err := resolvePrefix(c.App, createVault)
			if err != nil {
				return err
			}
			shareID, err := c.App.Pass.ResolveVault(c.Ctx, c.U, createVaultRef)
			if err != nil {
				return err
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would create %s %q in vault %s", nc.Type, nc.Name, shareID))
				return nil
			}
			id, err := c.App.Pass.ItemCreate(c.Ctx, c.U, shareID, nc)
			if err != nil {
				return err
			}
			c.R().ID(id, fmt.Sprintf("Created %s %q", nc.Type, nc.Name))
			return nil
		}),
	}
	create.Flags().StringVar(&nc.Type, "type", "login", "Item type (login, note, card)")
	create.Flags().StringVar(&nc.Name, "name", "", "Item name")
	create.Flags().StringVar(&nc.Username, "username", "", "Username (login)")
	create.Flags().StringVar(&nc.Password, "password", "", "Password (login)")
	create.Flags().StringVar(&nc.Email, "email", "", "Email (login)")
	create.Flags().StringVar(&nc.URL, "url", "", "URL (login)")
	create.Flags().StringVar(&nc.Note, "note", "", "Note")
	create.Flags().StringVar(&nc.Holder, "holder", "", "Cardholder name (card)")
	create.Flags().StringVar(&nc.Number, "number", "", "Card number (card)")
	create.Flags().StringVar(&nc.Expiry, "expiry", "", "Card expiry YYYY-MM (card)")
	create.Flags().StringVar(&nc.CVV, "cvv", "", "Card CVV (card)")
	create.Flags().StringVar(&nc.PIN, "pin", "", "Card PIN (card)")
	create.Flags().StringVar(&createVault, "vault", "", "Vault name or ID (default: first vault)")
	c.AddCommand(create)

	var patch passsvc.Patch
	edit := &cobra.Command{
		Use: "edit {SHARE_ID ITEM_ID | SEARCH}", Short: "Edit a Pass item",
		Args: cobra.RangeArgs(1, 2),
		RunE: run([]Step{stepAuth, stepResolve, stepUnlock}, func(c *Ctx) error {
			shareID, itemID, err := c.App.Pass.ResolveItem(c.Ctx, c.U, c.Args)
			if err != nil {
				return err
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would edit %s/%s", shareID, itemID))
				return nil
			}
			if err := c.App.Pass.ItemEdit(c.Ctx, c.U, shareID, itemID, patch); err != nil {
				return err
			}
			c.R().Success("Item updated.")
			return nil
		}),
	}
	edit.Flags().StringVar(&patch.Name, "name", "", "New name")
	edit.Flags().StringVar(&patch.Username, "username", "", "New username")
	edit.Flags().StringVar(&patch.Password, "password", "", "New password")
	edit.Flags().StringVar(&patch.Email, "email", "", "New email")
	edit.Flags().StringVar(&patch.URL, "url", "", "New URL")
	edit.Flags().StringVar(&patch.Note, "note", "", "New note")
	c.AddCommand(edit)

	c.AddCommand(simpleItemCmd("restore", "Restore an item from trash", func(c *Ctx, share, item string) error {
		return c.App.Pass.ItemRestore(c.Ctx, share, item)
	}, "Item restored."))
	c.AddCommand(bulkItemCmd("trash", "Move items to trash", func(c *Ctx, share, item string) error {
		return c.App.Pass.ItemTrash(c.Ctx, share, item)
	}, "Trashed %d item(s)."))
	c.AddCommand(bulkItemCmd("delete", "Permanently delete items", func(c *Ctx, share, item string) error {
		return c.App.Pass.ItemDelete(c.Ctx, share, item)
	}, "Deleted %d item(s)."))
	return c
}

func printPassItem(c *Ctx, it *passsvc.Item) {
	out := c.R().Stdout
	_, _ = fmt.Fprintf(out, "Type:     %s\n", it.Type)
	_, _ = fmt.Fprintf(out, "Name:     %s\n", it.Name)
	field := func(label, v string) {
		if v != "" {
			_, _ = fmt.Fprintf(out, "%-9s %s\n", label+":", v)
		}
	}
	field("Username", it.Username)
	field("Email", it.Email)
	field("Password", it.Password)
	field("TOTP", it.TOTP)
	for _, u := range it.URLs {
		_, _ = fmt.Fprintf(out, "URL:      %s\n", u)
	}
	field("Holder", it.Holder)
	field("Number", it.Number)
	field("Expiry", it.Expiry)
	field("CVV", it.CVV)
	field("PIN", it.PIN)
	field("SSID", it.SSID)
	field("Note", it.Note)
	_, _ = fmt.Fprintf(out, "ID:       %s\n", it.ItemID)
	_, _ = fmt.Fprintf(out, "Share:    %s\n", it.ShareID)
}

func simpleItemCmd(use, short string, fn func(c *Ctx, share, item string) error, success string) *cobra.Command {
	return &cobra.Command{
		Use: use + " {SHARE_ID ITEM_ID | SEARCH}", Short: short,
		Args: cobra.RangeArgs(1, 2),
		RunE: run([]Step{stepAuth, stepResolve, stepUnlock}, func(c *Ctx) error {
			shareID, itemID, err := c.App.Pass.ResolveItem(c.Ctx, c.U, c.Args)
			if err != nil {
				return err
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would %s %s/%s", use, shareID, itemID))
				return nil
			}
			if err := fn(c, shareID, itemID); err != nil {
				return err
			}
			c.R().Success(success)
			return nil
		}),
	}
}

type itemFilter struct {
	vault, itemType      string
	olderThan, newerThan string
	all                  bool
}

func (f *itemFilter) register(c *cobra.Command) {
	c.Flags().StringVar(&f.vault, "vault", "", "Filter by vault name or ID")
	c.Flags().StringVar(&f.itemType, "type", "", "Filter by item type (login, note, credit_card, alias, identity, ssh_key, wifi, custom)")
	c.Flags().StringVar(&f.olderThan, "older-than", "", "Match items not modified within DURATION (e.g. 30d, 2w, 1h)")
	c.Flags().StringVar(&f.newerThan, "newer-than", "", "Match items modified within DURATION")
	c.Flags().BoolVar(&f.all, "all", false, "Confirm matching every item in the scope (required when no other filter is set)")
}

func (f *itemFilter) set() bool {
	return f.vault != "" || f.itemType != "" || f.olderThan != "" || f.newerThan != "" || f.all
}

func collectItemIDs(c *Ctx, u *keys.Unlocked, args []string, f *itemFilter) ([][2]string, error) {
	var pairs [][2]string
	refs, err := resolvePrefixes(c.App, args)
	if err != nil {
		return nil, err
	}
	if len(refs) == 2 {
		pairs = append(pairs, [2]string{refs[0], refs[1]})
	} else if len(refs) == 1 {
		shareID, itemID, err := c.App.Pass.ResolveItem(c.Ctx, u, refs)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, [2]string{shareID, itemID})
	}

	if f.set() {
		var olderCutoff, newerCutoff int64
		if f.olderThan != "" {
			d, err := render.ParseDuration(f.olderThan)
			if err != nil {
				return nil, fmt.Errorf("invalid --older-than: %w", err)
			}
			olderCutoff = time.Now().Add(-d).Unix()
		}
		if f.newerThan != "" {
			d, err := render.ParseDuration(f.newerThan)
			if err != nil {
				return nil, fmt.Errorf("invalid --newer-than: %w", err)
			}
			newerCutoff = time.Now().Add(-d).Unix()
		}
		vaultRef, err := resolvePrefix(c.App, f.vault)
		if err != nil {
			return nil, err
		}
		items, err := c.App.Pass.ItemsList(c.Ctx, u, vaultRef)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			if f.itemType != "" && it.Type != f.itemType {
				continue
			}
			if olderCutoff != 0 && it.ModifyTime > olderCutoff {
				continue
			}
			if newerCutoff != 0 && it.ModifyTime < newerCutoff {
				continue
			}
			pairs = append(pairs, [2]string{it.ShareID, it.ItemID})
		}
	}

	if len(args) == 0 && !f.set() {
		return nil, fmt.Errorf("no items selected: pass an item argument or a filter (--vault, --type); use --all to target an entire vault")
	}

	seen := make(map[string]struct{}, len(pairs))
	out := make([][2]string, 0, len(pairs))
	for _, p := range pairs {
		key := p[0] + "/" + p[1]
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, p)
	}
	return out, nil
}

func bulkItemCmd(use, short string, fn func(c *Ctx, share, item string) error, successFmt string) *cobra.Command {
	var f itemFilter
	c := &cobra.Command{
		Use:   use + " [SHARE_ID ITEM_ID | SEARCH]",
		Short: short,
		Args:  cobra.RangeArgs(0, 2),
		RunE: run([]Step{stepAuth, stepResolve, stepUnlock}, func(c *Ctx) error {
			pairs, err := collectItemIDs(c, c.U, c.Args, &f)
			if err != nil {
				return err
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would %s %d item(s)", use, len(pairs)))
				for _, p := range pairs {
					_, _ = fmt.Fprintf(c.R().Stderr, "  %s/%s\n", p[0], p[1])
				}
				return nil
			}
			for _, p := range pairs {
				if err := fn(c, p[0], p[1]); err != nil {
					return err
				}
			}
			c.R().Success(fmt.Sprintf(successFmt, len(pairs)))
			return nil
		}),
	}
	f.register(c)
	return c
}

// ── pass vaults ──

func passVaultsCmd() *cobra.Command {
	c := &cobra.Command{Use: "vaults", Short: "Manage Pass vaults"}
	c.AddCommand(&cobra.Command{
		Use: "list", Short: "List vaults",
		RunE: run([]Step{stepAuth, stepUnlock}, func(c *Ctx) error {
			vaults, err := c.App.Pass.VaultsList(c.Ctx, c.U)
			if err != nil {
				return err
			}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[passsvc.Vault]{
				Columns: []view.Column[passsvc.Vault]{
					{Header: "SHARE_ID", ID: true, Cell: func(v passsvc.Vault) string { return v.ShareID }},
					{Header: "NAME", Cell: func(v passsvc.Vault) string {
						if v.Name == "" {
							return "(encrypted)"
						}
						return v.Name
					}},
					{Header: "MEMBERS", Cell: func(v passsvc.Vault) string { return fmt.Sprintf("%d", v.Members) }},
					{Header: "OWNER", Cell: func(v passsvc.Vault) string { return yesNo(v.Owner) }},
					{Header: "SHARED", Cell: func(v passsvc.Vault) string { return yesNo(v.Shared) }},
				},
				CacheIDs: func(v passsvc.Vault) []string { return []string{v.ShareID} },
			}, vaults)
		}),
	})
	var name string
	create := &cobra.Command{
		Use: "create", Short: "Create a vault",
		RunE: run([]Step{stepAuth, stepUnlock}, func(c *Ctx) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would create vault %q", name))
				return nil
			}
			id, err := c.App.Pass.VaultCreate(c.Ctx, c.U, name)
			if err != nil {
				return err
			}
			c.R().ID(id, fmt.Sprintf("Created vault %q", name))
			return nil
		}),
	}
	create.Flags().StringVar(&name, "name", "", "Vault name")
	c.AddCommand(create)

	c.AddCommand(&cobra.Command{
		Use: "delete SHARE_ID", Short: "Delete a vault",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Ctx) error {
			shareID := c.Args[0]
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would delete vault %s", shareID))
				return nil
			}
			if err := c.App.Pass.VaultDelete(c.Ctx, shareID); err != nil {
				return err
			}
			c.R().Success("Vault deleted.")
			return nil
		}),
	})
	return c
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// ── pass alias ──

func passAliasCmd() *cobra.Command {
	c := &cobra.Command{Use: "alias", Short: "Manage aliases"}
	c.AddCommand(&cobra.Command{
		Use: "options", Short: "List available alias suffixes and mailboxes",
		RunE: run([]Step{stepAuth, stepUnlock}, func(c *Ctx) error {
			shareID, err := c.App.Pass.ResolveVault(c.Ctx, c.U, "")
			if err != nil {
				return err
			}
			sx, mx, err := c.App.Pass.AliasOptions(c.Ctx, shareID)
			if err != nil {
				return err
			}
			if c.R().Format != render.FormatText {
				return c.R().Object(map[string]any{"Suffixes": sx, "Mailboxes": mx})
			}
			_, _ = fmt.Fprintln(c.R().Stdout, "Suffixes:")
			for _, s := range sx {
				_, _ = fmt.Fprintf(c.R().Stdout, "  %s\n", s.Suffix)
			}
			_, _ = fmt.Fprintln(c.R().Stdout, "\nMailboxes:")
			for _, m := range mx {
				_, _ = fmt.Fprintf(c.R().Stdout, "  %s (ID: %d)\n", m.Email, m.ID)
			}
			return nil
		}),
	})
	var prefix, suffix, mailbox, name, vault string
	create := &cobra.Command{
		Use: "create", Short: "Create an alias",
		RunE: run([]Step{stepAuth, stepUnlock}, func(c *Ctx) error {
			if prefix == "" {
				return fmt.Errorf("--prefix is required")
			}
			vaultRef, err := resolvePrefix(c.App, vault)
			if err != nil {
				return err
			}
			shareID, err := c.App.Pass.ResolveVault(c.Ctx, c.U, vaultRef)
			if err != nil {
				return err
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would create alias %s@%s", prefix, suffix))
				return nil
			}
			id, err := c.App.Pass.AliasCreate(c.Ctx, c.U, shareID, prefix, suffix, mailbox, name)
			if err != nil {
				return err
			}
			c.R().ID(id, "Alias created.")
			return nil
		}),
	}
	create.Flags().StringVar(&prefix, "prefix", "", "Alias prefix (before @)")
	create.Flags().StringVar(&suffix, "suffix", "", "Alias suffix (e.g. @passmail.net)")
	create.Flags().StringVar(&mailbox, "mailbox", "", "Mailbox email to forward to")
	create.Flags().StringVar(&name, "name", "", "Display name for the alias item")
	create.Flags().StringVar(&vault, "vault", "", "Vault name or ID (default: first vault)")
	c.AddCommand(create)
	return c
}
