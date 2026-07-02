package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/roman-16/proton-cli/internal/crypto/ical"
	"github.com/roman-16/proton-cli/internal/render"
	mailsvc "github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/roman-16/proton-cli/internal/view"
	"github.com/spf13/cobra"
)

func messagesCmd() *cobra.Command {
	c := &cobra.Command{Use: "messages", Short: "Manage messages"}
	c.AddCommand(msgListCmd(), msgSearchCmd(), msgReadCmd(), msgSendCmd(), msgTrashCmd(), msgDeleteCmd(), msgMoveCmd(), msgMarkCmd(), msgStarCmd(), msgUnstarCmd())
	return c
}

func messageColumns() []view.Column[mailsvc.Message] {
	return []view.Column[mailsvc.Message]{
		{Header: "ID", ID: true, Cell: func(m mailsvc.Message) string { return m.ID }},
		{Header: "FROM", Cell: func(m mailsvc.Message) string {
			if m.FromName != "" {
				return m.FromName
			}
			return m.FromAddress
		}},
		{Header: "SUBJECT", Cell: func(m mailsvc.Message) string { return m.Subject }},
		{Header: "DATE", Cell: func(m mailsvc.Message) string { return time.Unix(m.Time, 0).Local().Format("2006-01-02 15:04") }},
		{Header: "⚑", Cell: func(m mailsvc.Message) string {
			flags := ""
			if m.Unread == 1 {
				flags += "●"
			}
			if m.NumAttachments > 0 {
				flags += "📎"
			}
			return flags
		}},
	}
}

func msgListCmd() *cobra.Command {
	var opts mailsvc.ListOptions
	c := &cobra.Command{
		Use: "list", Short: "List messages",
		RunE: run([]Step{stepAuth}, func(c *Ctx) error {
			msgs, total, err := c.App.Mail.List(c.Ctx, opts)
			if err != nil {
				return err
			}
			hasMore := (opts.Page+1)*opts.PageSize < total
			wrap := struct {
				Total    int               `json:"total"`
				Page     int               `json:"page"`
				PageSize int               `json:"page_size"`
				HasMore  bool              `json:"has_more"`
				Messages []mailsvc.Message `json:"messages"`
			}{total, opts.Page, opts.PageSize, hasMore, msgs}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[mailsvc.Message]{
				Columns:  messageColumns(),
				CacheIDs: func(m mailsvc.Message) []string { return []string{m.ID} },
				Footer:   func(n int) string { return render.PaginationFooter("messages", total, opts.Page, opts.PageSize, n) },
				JSON:     wrap,
			}, msgs)
		}),
	}
	c.Flags().StringVar(&opts.Folder, "folder", "inbox", "Folder (inbox, sent, drafts, trash, spam, archive, starred, all)")
	c.Flags().IntVar(&opts.Page, "page", 0, "Page number (0-based)")
	c.Flags().IntVar(&opts.PageSize, "page-size", 25, "Messages per page")
	c.Flags().BoolVar(&opts.Unread, "unread", false, "Show only unread messages")
	return c
}

func msgSearchCmd() *cobra.Command {
	var opts mailsvc.SearchOptions
	c := &cobra.Command{
		Use: "search", Short: "Search messages",
		RunE: run([]Step{stepAuth}, func(c *Ctx) error {
			msgs, _, err := c.App.Mail.Search(c.Ctx, opts)
			if err != nil {
				return err
			}
			if len(msgs) == 0 {
				hintFromTo(c, "messages", opts)
			}
			limited := opts.Limit > 0 && len(msgs) >= opts.Limit
			wrap := struct {
				Total    int               `json:"total"`
				Results  int               `json:"results"`
				Limited  bool              `json:"limited"`
				Messages []mailsvc.Message `json:"messages"`
			}{len(msgs), len(msgs), limited, msgs}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[mailsvc.Message]{
				Columns:  messageColumns(),
				CacheIDs: func(m mailsvc.Message) []string { return []string{m.ID} },
				Footer:   func(n int) string { return render.SearchFooter(n, opts.Limit) },
				JSON:     wrap,
			}, msgs)
		}),
	}
	registerSearchFlags(c, &opts, "all")
	return c
}

func msgReadCmd() *cobra.Command {
	var format string
	var includeInline, bodyOnly, stripQuotes bool
	c := &cobra.Command{
		Use: "read REF", Short: "Read a message (decrypted)",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Ctx) error {
			if format != "text" && format != "html" && format != "raw" {
				return fmt.Errorf("unknown --format %q (use text, html, raw)", format)
			}
			id, err := c.App.Mail.Resolve(c.Ctx, c.Args[0])
			if err != nil {
				return err
			}
			u, err := c.App.Unlock(c.Ctx)
			if err != nil {
				return err
			}
			msg, err := c.App.Mail.Read(c.Ctx, u, id)
			if err != nil {
				return handleWrongTable(err, "read")
			}
			if c.R().Format != render.FormatText {
				return c.R().Object(msg)
			}
			showMetadata := format == "text" && !bodyOnly
			if showMetadata {
				_, _ = fmt.Fprintf(c.R().Stdout, "Subject: %s\n", msg.Subject)
				if s, ok := msg.Sender["Address"].(string); ok {
					_, _ = fmt.Fprintf(c.R().Stdout, "From:    %s\n", s)
				}
				for _, t := range msg.ToList {
					if s, ok := t["Address"].(string); ok {
						_, _ = fmt.Fprintf(c.R().Stdout, "To:      %s\n", s)
					}
				}
				_, _ = fmt.Fprintf(c.R().Stdout, "ID:      %s\n", msg.ID)
				if msg.Signature != "" {
					_, _ = fmt.Fprintf(c.R().Stdout, "Sig:     %s\n", sigText(msg.Signature))
				}
				_, _ = fmt.Fprintln(c.R().Stdout)
			}
			body := msg.Body
			if stripQuotes {
				if render.IsHTML(msg.MIMEType) {
					body = render.StripHTMLQuotes(body)
				} else {
					body = render.StripPlaintextQuotes(body)
				}
			}
			if format == "text" && render.IsHTML(msg.MIMEType) {
				body = render.HTMLToText(body)
			}
			_, _ = fmt.Fprintln(c.R().Stdout, body)
			if showMetadata {
				if footer := renderAttachmentsFooter(msg.Attachments, includeInline); footer != "" {
					_, _ = io.WriteString(c.R().Stdout, footer)
				}
			}
			return nil
		}),
	}
	c.Flags().StringVar(&format, "format", "text", "Body format: text, html, raw")
	c.Flags().BoolVar(&includeInline, "include-inline", false, "Include inline attachments (e.g. signature graphics) in the footer")
	c.Flags().BoolVar(&bodyOnly, "body-only", false, "Suppress headers and attachments footer; output the body only (default for --format html|raw)")
	c.Flags().BoolVar(&stripQuotes, "strip-quotes", false, "Remove quoted reply blocks from the body (heuristic)")
	return c
}

func msgSendCmd() *cobra.Command {
	var to, cc, bcc, attach, attachInline []string
	var subject, body, sendAt, expires, eoPassword, eoPasswordHint string
	var html bool
	c := &cobra.Command{
		Use: "send", Short: "Send a message",
		RunE: run([]Step{stepAuth}, func(c *Ctx) error {
			if len(to)+len(cc)+len(bcc) == 0 {
				return fmt.Errorf("at least one recipient is required (--to, --cc, or --bcc)")
			}
			if subject == "" {
				return fmt.Errorf("--subject is required")
			}
			if body == "-" {
				b, err := io.ReadAll(os.Stdin)
				if err != nil {
					return err
				}
				body = string(b)
			}
			if body == "" {
				return fmt.Errorf("--body is required (use - for stdin)")
			}
			for _, p := range attach {
				if _, err := os.Stat(p); err != nil {
					return fmt.Errorf("attachment %s: %w", p, err)
				}
			}
			for _, p := range attachInline {
				if _, err := os.Stat(p); err != nil {
					return fmt.Errorf("inline attachment %s: %w", p, err)
				}
			}
			if len(attachInline) > 0 && !html {
				return fmt.Errorf("--attach-inline requires --html (inline images need an HTML body)")
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would send to %d recipient(s) subject %q (%d bytes, %d attachment(s))", len(to)+len(cc)+len(bcc), subject, len(body), len(attach)))
				return nil
			}
			opts := mailsvc.SendOptions{To: to, CC: cc, BCC: bcc, Subject: subject, Body: body, HTML: html, Attachments: attach, InlineAttach: attachInline, EOPassword: eoPassword, EOPasswordHint: eoPasswordHint}
			if sendAt != "" {
				t, err := ical.ParseTime(sendAt)
				if err != nil {
					return fmt.Errorf("invalid --send-at: %w", err)
				}
				opts.DeliveryTime = t.Unix()
			}
			if expires != "" {
				d, err := render.ParseDuration(expires)
				if err != nil {
					return fmt.Errorf("invalid --expires: %w", err)
				}
				opts.ExpiresInSeconds = int(d.Seconds())
			}
			u, err := c.App.Unlock(c.Ctx)
			if err != nil {
				return err
			}
			// Consult Contacts for pinned keys per recipient. A pinned key means
			// we encrypt to it (E2EE / PGP-MIME) rather than sending cleartext.
			for _, email := range dedupe(append(append(append([]string{}, to...), cc...), bcc...)) {
				pin, err := c.App.Contacts.PinnedKeysFor(c.Ctx, u, email)
				if err != nil {
					return err
				}
				if pin == nil {
					continue
				}
				if opts.PinnedKeys == nil {
					opts.PinnedKeys = map[string]*mailsvc.PinnedRecipient{}
				}
				opts.PinnedKeys[email] = &mailsvc.PinnedRecipient{
					ArmoredKeys:       pin.ArmoredKeys,
					Encrypt:           pin.Encrypt,
					Sign:              pin.Sign,
					Scheme:            pin.Scheme,
					SignatureVerified: pin.SignatureVerified,
				}
			}
			if err := c.App.Mail.Send(c.Ctx, u, opts); err != nil {
				return err
			}
			c.R().Success("Message sent.")
			return nil
		}),
	}
	c.Flags().StringArrayVar(&to, "to", nil, "Recipient email (repeatable)")
	c.Flags().StringArrayVar(&cc, "cc", nil, "CC recipient (repeatable)")
	c.Flags().StringArrayVar(&bcc, "bcc", nil, "BCC recipient (repeatable)")
	c.Flags().StringVar(&subject, "subject", "", "Subject")
	c.Flags().StringVar(&body, "body", "", "Message body (use - for stdin)")
	c.Flags().BoolVar(&html, "html", false, "Send the body as text/html instead of text/plain")
	c.Flags().StringVar(&sendAt, "send-at", "", "Schedule delivery (RFC3339 or YYYY-MM-DDTHH:MM)")
	c.Flags().StringVar(&expires, "expires", "", "Self-destruct after DURATION (e.g. 7d, 24h)")
	c.Flags().StringArrayVar(&attach, "attach", nil, "File to attach (repeatable)")
	c.Flags().StringArrayVar(&attachInline, "attach-inline", nil, "Image embedded inline in the HTML body via Content-ID (repeatable; requires --html)")
	c.Flags().StringVar(&eoPassword, "eo-password", "", "Password-protect the message for non-Proton recipients (Encrypted Outside; defaults to a 28-day expiry)")
	c.Flags().StringVar(&eoPasswordHint, "eo-password-hint", "", "Optional hint shown to Encrypted Outside recipients")
	return c
}

func collectMessageIDs(c *Ctx, args []string, f *msgFilter) ([]string, error) {
	return collectMailIDs(c, args, f, mailIDCollector{
		noun: "message", plural: "messages",
		assertKind: c.App.Mail.AssertMessageKind,
		resolve:    c.App.Mail.Resolve,
		searchIDs: func(ctx context.Context, opts mailsvc.SearchOptions) ([]string, error) {
			msgs, _, err := c.App.Mail.Search(ctx, opts)
			if err != nil {
				return nil, err
			}
			ids := make([]string, len(msgs))
			for i, m := range msgs {
				ids[i] = m.ID
			}
			return ids, nil
		},
	})
}

func msgTrashCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use: "trash [REF...]", Short: "Move messages to trash",
		RunE: bulkMessageAction(&f, "Moved %d message(s) to trash.", "trash", func(c *Ctx, ids []string) error {
			return c.App.Mail.Trash(c.Ctx, ids)
		}),
	}
	f.register(c)
	return c
}

func msgDeleteCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use: "delete [REF...]", Short: "Permanently delete messages",
		RunE: bulkMessageAction(&f, "Permanently deleted %d message(s).", "delete", func(c *Ctx, ids []string) error {
			return c.App.Mail.Delete(c.Ctx, ids)
		}),
	}
	f.register(c)
	return c
}

func msgMoveCmd() *cobra.Command {
	var dest string
	var f msgFilter
	c := &cobra.Command{
		Use: "move [REF...]", Short: "Move messages to a folder",
		RunE: run([]Step{stepAuth}, func(c *Ctx) error {
			ids, err := collectMessageIDs(c, c.Args, &f)
			if err != nil {
				return handleWrongTable(err, "move")
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would move %d message(s) to %s", len(ids), dest))
				return nil
			}
			if err := c.App.Mail.Move(c.Ctx, ids, dest); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Moved %d message(s) to %s.", len(ids), dest))
			return nil
		}),
	}
	c.Flags().StringVar(&dest, "dest", "", "Destination folder (inbox, sent, drafts, trash, spam, archive, starred, or a label ID)")
	_ = c.MarkFlagRequired("dest")
	f.register(c)
	return c
}

func msgMarkCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use:       "mark ACTION [REF...]",
		Short:     "Mark messages (ACTION: read|unread)",
		Args:      cobra.MinimumNArgs(1),
		ValidArgs: []string{"read", "unread"},
		RunE: run([]Step{stepAuth}, func(c *Ctx) error {
			action := strings.ToLower(c.Args[0])
			rest := c.Args[1:]
			if action != "read" && action != "unread" {
				return fmt.Errorf("unknown action %q (use: read, unread)", action)
			}
			ids, err := collectMessageIDs(c, rest, &f)
			if err != nil {
				return handleWrongTable(err, "mark "+action)
			}
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would mark %d message(s) as %s", len(ids), action))
				return nil
			}
			if err := c.App.Mail.Mark(c.Ctx, ids, action == "read", action == "unread", false, false); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Marked %d message(s) as %s.", len(ids), action))
			return nil
		}),
	}
	f.register(c)
	return c
}

func msgStarCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use: "star [REF...]", Short: "Add a star to messages",
		RunE: bulkMessageAction(&f, "Starred %d message(s).", "star", func(c *Ctx, ids []string) error {
			return c.App.Mail.Mark(c.Ctx, ids, false, false, true, false)
		}),
	}
	f.register(c)
	return c
}

func msgUnstarCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use: "unstar [REF...]", Short: "Remove a star from messages",
		RunE: bulkMessageAction(&f, "Unstarred %d message(s).", "unstar", func(c *Ctx, ids []string) error {
			return c.App.Mail.Mark(c.Ctx, ids, false, false, false, true)
		}),
	}
	f.register(c)
	return c
}

func bulkMessageAction(f *msgFilter, successFmt, otherVerb string, do func(c *Ctx, ids []string) error) func(*cobra.Command, []string) error {
	return bulkMailAction(collectMessageIDs, "message", f, successFmt, otherVerb, do)
}
