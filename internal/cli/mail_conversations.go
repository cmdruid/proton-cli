package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/roman-16/proton-cli/internal/render"
	mailsvc "github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/roman-16/proton-cli/internal/view"
	"github.com/spf13/cobra"
)

func conversationsCmd() *cobra.Command {
	c := &cobra.Command{Use: "conversations", Short: "Manage conversations (threads)"}
	c.AddCommand(
		convListCmd(), convSearchCmd(), convReadCmd(),
		convTrashCmd(), convDeleteCmd(), convMoveCmd(),
		convMarkCmd(), convStarCmd(), convUnstarCmd(),
		convAttachmentsCmd(),
	)
	return c
}

func conversationColumns() []view.Column[mailsvc.Conversation] {
	return []view.Column[mailsvc.Conversation]{
		{Header: "ID", ID: true, Cell: func(c mailsvc.Conversation) string { return c.ID }},
		{Header: "FROM", Cell: func(c mailsvc.Conversation) string {
			if len(c.Senders) > 0 {
				if n, ok := c.Senders[0]["Name"].(string); ok && n != "" {
					return n
				}
				if a, ok := c.Senders[0]["Address"].(string); ok {
					return a
				}
			}
			return ""
		}},
		{Header: "SUBJECT", Cell: func(c mailsvc.Conversation) string { return c.Subject }},
		{Header: "#", Cell: func(c mailsvc.Conversation) string { return fmt.Sprintf("%d", c.NumMessages) }},
		{Header: "DATE", Cell: func(c mailsvc.Conversation) string { return time.Unix(c.Time, 0).Local().Format("2006-01-02 15:04") }},
		{Header: "⚑", Accent: true, Cell: func(c mailsvc.Conversation) string {
			flags := ""
			if c.NumUnread > 0 {
				flags += "●"
			}
			if c.NumAttachments > 0 {
				flags += "📎"
			}
			return flags
		}},
	}
}

func convListCmd() *cobra.Command {
	var opts mailsvc.ListOptions
	c := &cobra.Command{
		Use: "list", Short: "List conversations",
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			convs, total, err := c.App.Mail.ConversationsList(c.Ctx, opts)
			if err != nil {
				return err
			}
			hasMore := (opts.Page+1)*opts.PageSize < total
			wrap := struct {
				Total         int                    `json:"total"`
				Page          int                    `json:"page"`
				PageSize      int                    `json:"page_size"`
				HasMore       bool                   `json:"has_more"`
				Conversations []mailsvc.Conversation `json:"conversations"`
			}{total, opts.Page, opts.PageSize, hasMore, convs}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[mailsvc.Conversation]{
				Columns:  conversationColumns(),
				CacheIDs: func(cv mailsvc.Conversation) []string { return []string{cv.ID} },
				Footer: func(n int) string {
					return render.PaginationFooter("conversations", total, opts.Page, opts.PageSize, n)
				},
				JSON: wrap,
			}, convs)
		}),
	}
	c.Flags().StringVar(&opts.Folder, "folder", "inbox", "Folder (inbox, sent, drafts, trash, spam, archive, starred, all)")
	c.Flags().IntVar(&opts.Page, "page", 0, "Page number (0-based)")
	c.Flags().IntVar(&opts.PageSize, "page-size", 25, "Conversations per page")
	c.Flags().BoolVar(&opts.Unread, "unread", false, "Show only conversations with unread messages")
	return c
}

func convSearchCmd() *cobra.Command {
	var opts mailsvc.SearchOptions
	c := &cobra.Command{
		Use: "search", Short: "Search conversations",
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			convs, _, err := c.App.Mail.ConversationsSearch(c.Ctx, opts)
			if err != nil {
				return err
			}
			if len(convs) == 0 {
				hintFromTo(c, "conversations", opts)
			}
			limited := opts.Limit > 0 && len(convs) >= opts.Limit
			wrap := struct {
				Total         int                    `json:"total"`
				Results       int                    `json:"results"`
				Limited       bool                   `json:"limited"`
				Conversations []mailsvc.Conversation `json:"conversations"`
			}{len(convs), len(convs), limited, convs}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[mailsvc.Conversation]{
				Columns:  conversationColumns(),
				CacheIDs: func(cv mailsvc.Conversation) []string { return []string{cv.ID} },
				Footer:   func(n int) string { return render.SearchFooter(n, opts.Limit) },
				JSON:     wrap,
			}, convs)
		}),
	}
	registerSearchFlags(c, &opts, "all")
	return c
}

func convReadCmd() *cobra.Command {
	var format string
	var includeInline, bodyOnly, stripQuotes, summary bool
	c := &cobra.Command{
		Use: "read REF", Short: "Read a conversation (full thread, decrypted)",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			id, err := c.App.Mail.ResolveConversation(c.Ctx, c.Args[0])
			if err != nil {
				return err
			}
			u, err := c.App.Unlock(c.Ctx)
			if err != nil {
				return err
			}
			conv, err := c.App.Mail.ConversationRead(c.Ctx, u, id)
			if err != nil {
				return handleWrongTable(err, "read")
			}
			if c.R().Format != render.FormatText {
				return c.R().Object(conv)
			}
			if summary {
				return renderConversationSummary(c, conv)
			}
			return renderConversationText(c, conv, format, includeInline, bodyOnly, stripQuotes)
		}),
	}
	c.Flags().StringVar(&format, "format", "text", "Body format: text, html, raw")
	c.Flags().BoolVar(&includeInline, "include-inline", false, "Include inline attachments (e.g. signature graphics) in per-message footers")
	c.Flags().BoolVar(&bodyOnly, "body-only", false, "Suppress envelope, dividers, headers, and per-message footers; concatenate bodies (default for --format html|raw)")
	c.Flags().BoolVar(&stripQuotes, "strip-quotes", false, "Remove quoted reply blocks from each message body (heuristic)")
	c.Flags().BoolVar(&summary, "summary", false, "One-line preview per message; implies --strip-quotes")
	return c
}

func convTrashCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use: "trash [REF...]", Short: "Move conversations to trash",
		RunE: bulkConversationAction(&f, "Moved %d conversation(s) to trash.", "trash", func(c *Invocation, ids []string) error {
			return c.App.Mail.ConversationsTrash(c.Ctx, ids)
		}),
	}
	f.register(c)
	return c
}

func convDeleteCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use: "delete [REF...]", Short: "Permanently delete conversations",
		RunE: bulkConversationAction(&f, "Permanently deleted %d conversation(s).", "delete", func(c *Invocation, ids []string) error {
			return c.App.Mail.ConversationsDelete(c.Ctx, ids, f.folder)
		}),
	}
	f.register(c)
	return c
}

func convMoveCmd() *cobra.Command {
	var dest string
	var f msgFilter
	c := &cobra.Command{
		Use: "move [REF...]", Short: "Move conversations to a folder",
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			ids, err := collectConversationIDs(c, c.Args, &f)
			if err != nil {
				return handleWrongTable(err, "move")
			}
			if c.dryRun("move %d conversation(s) to %s", len(ids), dest) {
				return nil
			}
			if err := c.App.Mail.ConversationsMove(c.Ctx, ids, dest); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Moved %d conversation(s) to %s.", len(ids), dest))
			return nil
		}),
	}
	c.Flags().StringVar(&dest, "dest", "", "Destination folder (inbox, sent, drafts, trash, spam, archive, starred, or a label ID)")
	_ = c.MarkFlagRequired("dest")
	f.register(c)
	return c
}

func convMarkCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use:       "mark ACTION [REF...]",
		Short:     "Mark conversations (ACTION: read|unread)",
		Args:      cobra.MinimumNArgs(1),
		ValidArgs: []string{"read", "unread"},
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			action := strings.ToLower(c.Args[0])
			rest := c.Args[1:]
			if action != "read" && action != "unread" {
				return fmt.Errorf("unknown action %q (use: read, unread)", action)
			}
			ids, err := collectConversationIDs(c, rest, &f)
			if err != nil {
				return handleWrongTable(err, "mark "+action)
			}
			if c.dryRun("mark %d conversation(s) as %s", len(ids), action) {
				return nil
			}
			if err := c.App.Mail.ConversationsMark(c.Ctx, ids, action == "read", action == "unread", false, false, f.folder); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Marked %d conversation(s) as %s.", len(ids), action))
			return nil
		}),
	}
	f.register(c)
	return c
}

func convStarCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use: "star [REF...]", Short: "Add a star to conversations",
		RunE: bulkConversationAction(&f, "Starred %d conversation(s).", "star", func(c *Invocation, ids []string) error {
			return c.App.Mail.ConversationsMark(c.Ctx, ids, false, false, true, false, f.folder)
		}),
	}
	f.register(c)
	return c
}

func convUnstarCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use: "unstar [REF...]", Short: "Remove a star from conversations",
		RunE: bulkConversationAction(&f, "Unstarred %d conversation(s).", "unstar", func(c *Invocation, ids []string) error {
			return c.App.Mail.ConversationsMark(c.Ctx, ids, false, false, false, true, f.folder)
		}),
	}
	f.register(c)
	return c
}

func bulkConversationAction(f *msgFilter, successFmt, otherVerb string, do func(c *Invocation, ids []string) error) func(*cobra.Command, []string) error {
	return bulkMailAction(collectConversationIDs, "conversation", f, successFmt, otherVerb, do)
}

func collectConversationIDs(c *Invocation, args []string, f *msgFilter) ([]string, error) {
	return collectMailIDs(c, args, f, mailIDCollector{
		noun: "conversation", plural: "conversations",
		assertKind: c.App.Mail.AssertConversationKind,
		resolve:    c.App.Mail.ResolveConversation,
		searchIDs: func(ctx context.Context, opts mailsvc.SearchOptions) ([]string, error) {
			convs, _, err := c.App.Mail.ConversationsSearch(ctx, opts)
			if err != nil {
				return nil, err
			}
			ids := make([]string, len(convs))
			for i, cv := range convs {
				ids[i] = cv.ID
			}
			return ids, nil
		},
	})
}

func renderConversationText(c *Invocation, conv *mailsvc.ConversationFull, format string, includeInline, bodyOnly, stripQuotes bool) error {
	if format != "text" && format != "html" && format != "raw" {
		return fmt.Errorf("unknown --format %q (use text, html, raw)", format)
	}
	n := len(conv.Messages)
	showMetadata := format == "text" && !bodyOnly
	if showMetadata {
		_, _ = fmt.Fprintf(c.R().Stdout, "Subject:      %s\n", conv.Conversation.Subject)
		_, _ = fmt.Fprintf(c.R().Stdout, "Conversation: %s\n", conv.Conversation.ID)
		_, _ = fmt.Fprintf(c.R().Stdout, "Messages:     %d\n\n", n)
	}
	divider := strings.Repeat("─", 56)
	for i, m := range conv.Messages {
		if showMetadata {
			_, _ = fmt.Fprintf(c.R().Stdout, "─── %d/%d %s\n", i+1, n, divider)
			if s, ok := m.Sender["Address"].(string); ok {
				name, _ := m.Sender["Name"].(string)
				if name != "" {
					_, _ = fmt.Fprintf(c.R().Stdout, "From: %s <%s>\n", name, s)
				} else {
					_, _ = fmt.Fprintf(c.R().Stdout, "From: %s\n", s)
				}
			}
			for _, t := range m.ToList {
				if s, ok := t["Address"].(string); ok {
					_, _ = fmt.Fprintf(c.R().Stdout, "To:   %s\n", s)
				}
			}
			if m.Time > 0 {
				_, _ = fmt.Fprintf(c.R().Stdout, "Date: %s\n", time.Unix(m.Time, 0).Local().Format("2006-01-02 15:04"))
			}
			_, _ = fmt.Fprintf(c.R().Stdout, "ID:   %s\n", m.ID)
			if m.Signature != "" {
				_, _ = fmt.Fprintf(c.R().Stdout, "Sig:  %s\n", sigText(m.Signature))
			}
			_, _ = fmt.Fprintln(c.R().Stdout)
		}
		body := m.Body
		if stripQuotes {
			if render.IsHTML(m.MIMEType) {
				body = render.StripHTMLQuotes(body)
			} else {
				body = render.StripPlaintextQuotes(body)
			}
		}
		if format == "text" && render.IsHTML(m.MIMEType) {
			body = render.HTMLToText(body)
		}
		_, _ = fmt.Fprintln(c.R().Stdout, body)
		if showMetadata {
			if footer := renderAttachmentsFooter(m.Attachments, includeInline); footer != "" {
				_, _ = io.WriteString(c.R().Stdout, footer)
			}
		}
		if i < n-1 {
			_, _ = fmt.Fprintln(c.R().Stdout)
		}
	}
	return nil
}

func renderConversationSummary(c *Invocation, conv *mailsvc.ConversationFull) error {
	n := len(conv.Messages)
	for i, m := range conv.Messages {
		addr := ""
		if s, ok := m.Sender["Address"].(string); ok {
			addr = s
		}
		date := ""
		if m.Time > 0 {
			date = time.Unix(m.Time, 0).Local().Format("2006-01-02 15:04")
		}
		preview := render.MessagePreview(m.Body, m.MIMEType)
		attCount := nonInlineAttachmentCount(m.Attachments)
		attTag := ""
		if attCount > 0 {
			attTag = fmt.Sprintf("(%d attachments)", attCount)
		}
		if preview == "" {
			if attCount > 0 {
				preview = attTag
				attTag = ""
			} else {
				preview = "(quoted reply, body empty after strip)"
			}
		}
		line := fmt.Sprintf("%d/%d  %s  %s  %s", i+1, n, date, addr, preview)
		if attTag != "" {
			line += "  " + attTag
		}
		_, _ = fmt.Fprintln(c.R().Stdout, line)
	}
	return nil
}

func nonInlineAttachmentCount(atts []mailsvc.Attachment) int {
	n := 0
	for _, a := range atts {
		if !a.IsInline() {
			n++
		}
	}
	return n
}
