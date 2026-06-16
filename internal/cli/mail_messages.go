package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

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
				_, _ = fmt.Fprintf(c.R().Stdout, "ID:      %s\n\n", msg.ID)
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
	var to, subject, body string
	c := &cobra.Command{
		Use: "send", Short: "Send a message",
		RunE: run([]Step{stepAuth}, func(c *Ctx) error {
			if to == "" {
				return fmt.Errorf("--to is required")
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
			if c.App.DryRun {
				c.R().Info(fmt.Sprintf("dry-run: would send to %s subject %q (%d bytes)", to, subject, len(body)))
				return nil
			}
			u, err := c.App.Unlock(c.Ctx)
			if err != nil {
				return err
			}
			if err := c.App.Mail.Send(c.Ctx, u, to, subject, body); err != nil {
				return err
			}
			c.R().Success("Message sent.")
			return nil
		}),
	}
	c.Flags().StringVar(&to, "to", "", "Recipient email")
	c.Flags().StringVar(&subject, "subject", "", "Subject")
	c.Flags().StringVar(&body, "body", "", "Message body (use - for stdin)")
	return c
}

// collectMessageIDs unions explicit REFs with messages matched by filters.
func collectMessageIDs(c *Ctx, args []string, f *msgFilter) ([]string, error) {
	var ids []string
	refs, err := resolvePrefixes(c.App, args)
	if err != nil {
		return nil, err
	}
	if len(refs) == 1 && mailsvc.LooksLikeID(refs[0]) {
		if err := c.App.Mail.AssertMessageKind(c.Ctx, refs[0]); err != nil {
			var wte *mailsvc.WrongTableError
			if errors.As(err, &wte) {
				return nil, err
			}
		}
	}
	for _, arg := range refs {
		id, err := c.App.Mail.Resolve(c.Ctx, arg)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if f.set() {
		if f.onlyAll() {
			c.R().Info("--all with no other filter will affect every message in the account. Add --folder to scope it.")
		}
		search, err := f.toSearch()
		if err != nil {
			return nil, err
		}
		msgs, _, err := c.App.Mail.Search(c.Ctx, search)
		if err != nil {
			return nil, err
		}
		for _, m := range msgs {
			ids = append(ids, m.ID)
		}
	}
	if len(args) == 0 && !f.set() {
		return nil, fmt.Errorf("no messages selected: pass REF(s) or a filter (e.g. --unread, --from, --older-than); use --all to target an entire folder")
	}
	return dedupe(ids), nil
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
	return run([]Step{stepAuth}, func(c *Ctx) error {
		ids, err := collectMessageIDs(c, c.Args, f)
		if err != nil {
			return handleWrongTable(err, otherVerb)
		}
		if c.App.DryRun {
			c.R().Info(fmt.Sprintf("dry-run: would affect %d message(s)", len(ids)))
			for _, id := range ids {
				_, _ = fmt.Fprintln(c.R().Stderr, "  "+id)
			}
			return nil
		}
		if err := do(c, ids); err != nil {
			return err
		}
		c.R().Success(fmt.Sprintf(successFmt, len(ids)))
		return nil
	})
}
