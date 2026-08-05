package cli

import (
	"fmt"

	"github.com/roman-16/proton-cli/internal/render"
	mailsvc "github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/roman-16/proton-cli/internal/view"
	"github.com/spf13/cobra"
)

// ── mail settings addresses ──

func mailAddressesCmd() *cobra.Command {
	c := &cobra.Command{Use: "addresses", Short: "Manage your addresses, display names and signatures"}
	c.AddCommand(addressesListCmd(), addressesGetCmd(), addressesUpdateCmd())
	return c
}

func addressesListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List the email addresses on the account",
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			addrs, err := c.App.Mail.AddressesList(c.Ctx)
			if err != nil {
				return err
			}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[mailsvc.Address]{
				Columns: []view.Column[mailsvc.Address]{
					{Header: "ID", ID: true, Cell: func(a mailsvc.Address) string { return a.ID }},
					{Header: "EMAIL", Cell: func(a mailsvc.Address) string { return a.Email }},
					{Header: "DISPLAY_NAME", Cell: func(a mailsvc.Address) string { return a.DisplayName }},
					{Header: "STATUS", Cell: func(a mailsvc.Address) string {
						if a.Status == 1 {
							return "active"
						}
						return "disabled"
					}},
					{Header: "TYPE", Cell: func(a mailsvc.Address) string { return addressType(a.Type) }},
					{Header: "SIG", Accent: true, Cell: func(a mailsvc.Address) string {
						if a.Signature != "" {
							return "✓"
						}
						return ""
					}},
				},
				CacheIDs: func(a mailsvc.Address) []string { return []string{a.ID} },
			}, addrs)
		}),
	}
}

func addressesGetCmd() *cobra.Command {
	return &cobra.Command{
		Use: "get REF", Short: "Show one address, including its signature",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			a, err := c.App.Mail.ResolveAddress(c.Ctx, c.Args[0])
			if err != nil {
				return err
			}
			if c.R().Format != render.FormatText {
				return c.R().Object(a)
			}
			p := fieldPrinter(c, 14)
			p("Email", a.Email)
			p("ID", a.ID)
			p("Display Name", a.DisplayName)
			p("Type", addressType(a.Type))
			p("Status", map[bool]string{true: "active", false: "disabled"}[a.Status == 1])
			p("Can Send", onOffText(boolInt(a.CanSend())))
			if a.Signature == "" {
				p("Signature", "(none)")
				return nil
			}
			_, _ = fmt.Fprintf(c.R().Stdout, "\nSignature:\n%s\n", render.HTMLToText(a.Signature))
			return nil
		}),
	}
}

func addressesUpdateCmd() *cobra.Command {
	var displayName, signature string
	var html, clear bool
	c := &cobra.Command{
		Use:   "update REF",
		Short: "Set an address's display name or signature",
		Long: "Set the display name recipients see and the signature appended to mail\n" +
			"sent from this address.\n\n" +
			"Proton stores signatures as HTML. Plain text is escaped and its newlines\n" +
			"become line breaks; pass --html to supply markup yourself.",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			setName, setSig := c.changed("display-name"), c.changed("signature")
			if clear && setSig {
				return fmt.Errorf("--clear-signature and --signature are mutually exclusive")
			}
			if !setName && !setSig && !clear {
				return fmt.Errorf("nothing to update: pass --display-name, --signature, or --clear-signature")
			}
			a, err := c.App.Mail.ResolveAddress(c.Ctx, c.Args[0])
			if err != nil {
				return err
			}
			var namePtr, sigPtr *string
			if setName {
				namePtr = &displayName
			}
			switch {
			case clear:
				empty := ""
				sigPtr = &empty
			case setSig:
				text, err := readTextArg(signature, "--signature")
				if err != nil {
					return err
				}
				if !html {
					text = render.TextToHTML(text)
				}
				sigPtr = &text
			}
			if c.dryRun("update address %s", a.Email) {
				return nil
			}
			if err := c.App.Mail.AddressUpdate(c.Ctx, a.ID, namePtr, sigPtr); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Updated %s.", a.Email))
			return nil
		}),
	}
	c.Flags().StringVar(&displayName, "display-name", "", "Name recipients see next to the address")
	c.Flags().StringVar(&signature, "signature", "", "Signature appended to mail from this address (use - for stdin)")
	c.Flags().BoolVar(&html, "html", false, "Treat --signature as HTML instead of escaping it and converting newlines")
	c.Flags().BoolVar(&clear, "clear-signature", false, "Remove the signature")
	return c
}

func addressType(t int) string {
	switch t {
	case 1:
		return "original"
	case 2:
		return "alias"
	case 3:
		return "custom"
	case 4:
		return "premium"
	case 5:
		return "external"
	}
	return "unknown"
}
