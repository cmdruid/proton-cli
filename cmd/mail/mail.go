// Package mail implements the `mail` subcommand tree.
package mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/roman-16/proton-cli/cmd/shared"
	"github.com/roman-16/proton-cli/internal/app"
	"github.com/roman-16/proton-cli/internal/keys"
	"github.com/roman-16/proton-cli/internal/render"
	mailsvc "github.com/roman-16/proton-cli/internal/services/mail"
	"github.com/spf13/cobra"
)

// NewCmd returns the root `mail` command.
func NewCmd() *cobra.Command {
	c := &cobra.Command{Use: "mail", Short: "Mail operations"}
	c.AddCommand(messagesCmd(), conversationsCmd(), attachmentsCmd(), labelsCmd(), filtersCmd(), addressesCmd())
	return c
}

// handleWrongTable inspects err for *mailsvc.WrongTableError and, on a match,
// returns an app.Exit(3, ...) wrapping a copy-pasteable redirect hint to the
// other command tree. Non-WrongTableError errors are returned unchanged.
//
// otherVerb is the verb on the other tree (e.g. "read", "trash", "mark read").
func handleWrongTable(err error, otherVerb string) error {
	if err == nil {
		return nil
	}
	var wte *mailsvc.WrongTableError
	if !errors.As(err, &wte) {
		return err
	}
	var otherTree string
	switch wte.Kind {
	case "conversation":
		otherTree = "mail conversations"
	case "message":
		otherTree = "mail messages"
	default:
		return err
	}
	return app.Exit(3, fmt.Errorf(
		"that ID is a %s, not a %s. Run: proton-cli %s %s %s",
		wte.Kind, oppositeKind(wte.Kind), otherTree, otherVerb, wte.ID))
}

func oppositeKind(k string) string {
	if k == "conversation" {
		return "message"
	}
	return "conversation"
}

// hintFromTo emits a stderr hint suggesting --keyword when a search by
// --from or --to returned zero results without --keyword. Proton's server
// matches the email address only on those filters; --keyword broadens the
// search to display names and message content, which is almost always what
// the user wanted.
//
// noun is "messages" or "conversations" — used to construct the redirect
// command in the hint.
func hintFromTo(a *app.App, noun string, opts mailsvc.SearchOptions) {
	if opts.Keyword != "" || (opts.From == "" && opts.To == "") {
		return
	}
	flag := "--from"
	val := opts.From
	if val == "" {
		flag = "--to"
		val = opts.To
	}
	a.R.Info(fmt.Sprintf(
		"Hint: %s matches the email address only. To also search display\n"+
			"      names and message content, try:\n"+
			"        proton-cli mail %s search --keyword %s",
		flag, noun, val))
}

// ── mail messages ──

func messagesCmd() *cobra.Command {
	c := &cobra.Command{Use: "messages", Short: "Manage messages"}
	c.AddCommand(msgListCmd(), msgSearchCmd(), msgReadCmd(), msgSendCmd(), msgTrashCmd(), msgDeleteCmd(), msgMoveCmd(), msgMarkCmd(), msgStarCmd(), msgUnstarCmd())
	return c
}

func msgListCmd() *cobra.Command {
	var opts mailsvc.ListOptions
	c := &cobra.Command{
		Use: "list", Short: "List messages",
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			msgs, total, err := a.Mail.List(cmd.Context(), opts)
			if err != nil {
				return err
			}
			return renderListMessages(a, msgs, total, opts)
		},
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
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			msgs, _, err := a.Mail.Search(cmd.Context(), opts)
			if err != nil {
				return err
			}
			if len(msgs) == 0 {
				hintFromTo(a, "messages", opts)
			}
			return renderSearchMessages(a, msgs, opts.Limit)
		},
	}
	c.Flags().StringVar(&opts.Keyword, "keyword", "", "Search keyword")
	c.Flags().StringVar(&opts.From, "from", "", "Filter by sender email address (use --keyword to also match display names)")
	c.Flags().StringVar(&opts.To, "to", "", "Filter by recipient email address (use --keyword to also match display names)")
	c.Flags().StringVar(&opts.Subject, "subject", "", "Filter by subject")
	c.Flags().StringVar(&opts.After, "after", "", "After date (YYYY-MM-DD)")
	c.Flags().StringVar(&opts.Before, "before", "", "Before date (YYYY-MM-DD)")
	c.Flags().StringVar(&opts.Folder, "folder", "all", "Folder to search in")
	c.Flags().IntVar(&opts.Limit, "limit", 25, "Max results")
	return c
}

func msgReadCmd() *cobra.Command {
	var format string
	var includeInline, bodyOnly, stripQuotes bool
	c := &cobra.Command{
		Use: "read REF", Short: "Read a message (decrypted)",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != "text" && format != "html" && format != "raw" {
				return fmt.Errorf("unknown --format %q (use text, html, raw)", format)
			}
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			ref, err := shared.ResolvePrefix(a, args[0])
			if err != nil {
				return err
			}
			id, err := a.Mail.Resolve(cmd.Context(), ref)
			if err != nil {
				return app.Exit(resolveExit(err), err)
			}
			u, err := a.Unlock(cmd.Context())
			if err != nil {
				return err
			}
			msg, err := a.Mail.Read(cmd.Context(), u, id)
			if err != nil {
				return handleWrongTable(err, "read")
			}
			if a.R.Format != render.FormatText {
				return a.R.Object(msg)
			}

			showMetadata := format == "text" && !bodyOnly
			if showMetadata {
				_, _ = fmt.Fprintf(a.R.Stdout, "Subject: %s\n", msg.Subject)
				if s, ok := msg.Sender["Address"].(string); ok {
					_, _ = fmt.Fprintf(a.R.Stdout, "From:    %s\n", s)
				}
				for _, t := range msg.ToList {
					if s, ok := t["Address"].(string); ok {
						_, _ = fmt.Fprintf(a.R.Stdout, "To:      %s\n", s)
					}
				}
				_, _ = fmt.Fprintf(a.R.Stdout, "ID:      %s\n\n", msg.ID)
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
			_, _ = fmt.Fprintln(a.R.Stdout, body)
			if showMetadata {
				if footer := renderAttachmentsFooter(msg.Attachments, includeInline); footer != "" {
					_, _ = io.WriteString(a.R.Stdout, footer)
				}
			}
			return nil
		},
	}
	c.Flags().StringVar(&format, "format", "text", "Body format: text, html, raw")
	c.Flags().BoolVar(&includeInline, "include-inline", false,
		"Include inline attachments (e.g. signature graphics) in the footer")
	c.Flags().BoolVar(&bodyOnly, "body-only", false,
		"Suppress headers and attachments footer; output the body only (default for --format html|raw)")
	c.Flags().BoolVar(&stripQuotes, "strip-quotes", false,
		"Remove quoted reply blocks from the body (heuristic; some non-standard quoting styles may be preserved)")
	return c
}

// renderAttachmentsFooter returns the text-mode footer block for a message's
// attachments, or "" when nothing is to be shown. The returned string starts
// with a blank line and a "---" separator, lists each visible attachment as
// `  - <name>  (<size>)  ID: <id>` (with a trailing `  (inline)` tag when
// includeInline=true and the attachment is inline), and ends with a newline.
//
//	includeInline=false: lists only Disposition != "inline".
//	includeInline=true:  lists all; inline entries get the (inline) tag.
func renderAttachmentsFooter(atts []mailsvc.Attachment, includeInline bool) string {
	visible := atts
	if !includeInline {
		visible = mailsvc.FilterInline(atts)
	}
	if len(visible) == 0 {
		return ""
	}
	sizes := make([]string, len(visible))
	var maxName, maxSize int
	for i, a := range visible {
		sizes[i] = render.Size(a.Size)
		if n := utf8.RuneCountInString(a.Name); n > maxName {
			maxName = n
		}
		if n := utf8.RuneCountInString(sizes[i]); n > maxSize {
			maxSize = n
		}
	}
	var b strings.Builder
	b.WriteString("\n---\n")
	fmt.Fprintf(&b, "Attachments (%d):\n", len(visible))
	for i, a := range visible {
		// Pad name to align the size column; pad the (size) cell to align
		// the ID column. Padding goes AFTER the closing paren so columns
		// line up cleanly when sizes have different lengths.
		sizeCell := "(" + sizes[i] + ")" + strings.Repeat(" ", maxSize-utf8.RuneCountInString(sizes[i]))
		fmt.Fprintf(&b, "  - %s  %s  ID: %s", padName(a.Name, maxName), sizeCell, a.ID)
		if a.IsInline() {
			b.WriteString("  (inline)")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// padName right-pads s with spaces to width runes. Rune-count aware so unicode
// filenames don't shift the size column off-grid.
func padName(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

func msgSendCmd() *cobra.Command {
	var to, subject, body string
	c := &cobra.Command{
		Use: "send", Short: "Send a message",
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
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
			if a.DryRun {
				a.R.Info(fmt.Sprintf("dry-run: would send to %s subject %q (%d bytes)", to, subject, len(body)))
				return nil
			}
			u, err := a.Unlock(cmd.Context())
			if err != nil {
				return err
			}
			if err := a.Mail.Send(cmd.Context(), u, to, subject, body); err != nil {
				return err
			}
			a.R.Success("Message sent.")
			return nil
		},
	}
	c.Flags().StringVar(&to, "to", "", "Recipient email")
	c.Flags().StringVar(&subject, "subject", "", "Subject")
	c.Flags().StringVar(&body, "body", "", "Message body (use - for stdin)")
	return c
}

// msgFilter collects batch-filter flags accepted by trash/delete/move/mark.
type msgFilter struct {
	unread                             bool
	from, to, subject, keyword, folder string
	olderThan, newerThan               string
	all                                bool
	limit                              int
}

func (f *msgFilter) register(c *cobra.Command) {
	c.Flags().BoolVar(&f.unread, "unread", false, "Match unread messages")
	c.Flags().StringVar(&f.from, "from", "", "Match sender")
	c.Flags().StringVar(&f.to, "to", "", "Match recipient")
	c.Flags().StringVar(&f.subject, "subject", "", "Match subject")
	c.Flags().StringVar(&f.keyword, "keyword", "", "Match keyword")
	c.Flags().StringVar(&f.folder, "folder", "", "Scope to a folder")
	c.Flags().StringVar(&f.olderThan, "older-than", "", "Match messages older than DURATION (e.g. 30d, 2w, 1h)")
	c.Flags().StringVar(&f.newerThan, "newer-than", "", "Match messages newer than DURATION")
	c.Flags().BoolVar(&f.all, "all", false, "Confirm matching every message in the scope (required when no other filter is set)")
	c.Flags().IntVar(&f.limit, "limit", 150, "Maximum messages to affect when using filters (Proton caps at 150 per page)")
}

func (f *msgFilter) set() bool {
	return f.unread || f.from != "" || f.to != "" || f.subject != "" || f.keyword != "" ||
		f.folder != "" || f.olderThan != "" || f.newerThan != "" || f.all
}

func (f *msgFilter) toSearch() (mailsvc.SearchOptions, error) {
	opts := mailsvc.SearchOptions{
		Keyword: f.keyword, From: f.from, To: f.to, Subject: f.subject,
		Folder: f.folder, Limit: f.limit, Unread: f.unread,
	}
	if opts.Folder == "" {
		opts.Folder = "all"
	}
	if f.olderThan != "" {
		d, err := render.ParseDuration(f.olderThan)
		if err != nil {
			return opts, fmt.Errorf("invalid --older-than: %w", err)
		}
		opts.Before = time.Now().Add(-d).Format("2006-01-02")
	}
	if f.newerThan != "" {
		d, err := render.ParseDuration(f.newerThan)
		if err != nil {
			return opts, fmt.Errorf("invalid --newer-than: %w", err)
		}
		opts.After = time.Now().Add(-d).Format("2006-01-02")
	}
	return opts, nil
}

// collectMessageIDs unions explicit REFs with messages matched by filters.
// On a single ID-shaped REF it pre-flights via AssertMessageKind so that
// pasting a conversation ID yields a redirect hint instead of a silent no-op.
func collectMessageIDs(cmd *cobra.Command, args []string, f *msgFilter) ([]string, error) {
	a := app.From(cmd.Context())
	var ids []string

	refs, err := shared.ResolvePrefixes(a, args)
	if err != nil {
		return nil, err
	}

	if len(refs) == 1 && mailsvc.LooksLikeID(refs[0]) {
		if err := a.Mail.AssertMessageKind(cmd.Context(), refs[0]); err != nil {
			var wte *mailsvc.WrongTableError
			if errors.As(err, &wte) {
				return nil, err
			}
			// non-WrongTable errors fall through; the bulk op below will retry
			// and return its own error.
		}
	}

	for _, arg := range refs {
		id, err := a.Mail.Resolve(cmd.Context(), arg)
		if err != nil {
			return nil, app.Exit(resolveExit(err), err)
		}
		ids = append(ids, id)
	}

	if f.set() {
		// --all without any actual filter needs at least a scope (folder) or
		// operates on everything — make the user be explicit.
		if f.all && !f.unread && f.from == "" && f.to == "" && f.subject == "" &&
			f.keyword == "" && f.folder == "" && f.olderThan == "" && f.newerThan == "" {
			a.R.Info("--all with no other filter will affect every message in the account. Add --folder to scope it.")
		}
		search, err := f.toSearch()
		if err != nil {
			return nil, err
		}
		msgs, _, err := a.Mail.Search(cmd.Context(), search)
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

	return shared.Dedupe(ids), nil
}

func msgTrashCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use: "trash [REF...]", Short: "Move messages to trash",
		RunE: bulkMessageAction(&f, "Moved %d message(s) to trash.", "trash", func(cmd *cobra.Command, ids []string) error {
			a := app.From(cmd.Context())
			return a.Mail.Trash(cmd.Context(), ids)
		}),
	}
	f.register(c)
	return c
}

func msgDeleteCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use: "delete [REF...]", Short: "Permanently delete messages",
		RunE: bulkMessageAction(&f, "Permanently deleted %d message(s).", "delete", func(cmd *cobra.Command, ids []string) error {
			a := app.From(cmd.Context())
			return a.Mail.Delete(cmd.Context(), ids)
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
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			ids, err := collectMessageIDs(cmd, args, &f)
			if err != nil {
				return handleWrongTable(err, "move")
			}
			if a.DryRun {
				a.R.Info(fmt.Sprintf("dry-run: would move %d message(s) to %s", len(ids), dest))
				return nil
			}
			if err := a.Mail.Move(cmd.Context(), ids, dest); err != nil {
				return err
			}
			a.R.Success(fmt.Sprintf("Moved %d message(s) to %s.", len(ids), dest))
			return nil
		},
	}
	// --dest keeps --folder available as a scope filter.
	c.Flags().StringVar(&dest, "dest", "", "Destination folder (inbox, sent, drafts, trash, spam, archive, starred, or a label ID)")
	_ = c.MarkFlagRequired("dest")
	f.register(c)
	return c
}

// msgMarkCmd accepts a positional ACTION (read|unread) so the --unread flag
// stays unambiguous as a filter.
func msgMarkCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use:       "mark ACTION [REF...]",
		Short:     "Mark messages (ACTION: read|unread)",
		Args:      cobra.MinimumNArgs(1),
		ValidArgs: []string{"read", "unread"},
		RunE: func(cmd *cobra.Command, args []string) error {
			action := strings.ToLower(args[0])
			rest := args[1:]
			if action != "read" && action != "unread" {
				return fmt.Errorf("unknown action %q (use: read, unread)", action)
			}
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			ids, err := collectMessageIDs(cmd, rest, &f)
			if err != nil {
				return handleWrongTable(err, "mark "+action)
			}
			if a.DryRun {
				a.R.Info(fmt.Sprintf("dry-run: would mark %d message(s) as %s", len(ids), action))
				return nil
			}
			if err := a.Mail.Mark(cmd.Context(), ids, action == "read", action == "unread", false, false); err != nil {
				return err
			}
			a.R.Success(fmt.Sprintf("Marked %d message(s) as %s.", len(ids), action))
			return nil
		},
	}
	f.register(c)
	return c
}

// msgStarCmd / msgUnstarCmd are separate commands so the star action never
// collides with the --starred filter on other commands.
func msgStarCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use: "star [REF...]", Short: "Add a star to messages",
		RunE: bulkMessageAction(&f, "Starred %d message(s).", "star", func(cmd *cobra.Command, ids []string) error {
			a := app.From(cmd.Context())
			return a.Mail.Mark(cmd.Context(), ids, false, false, true, false)
		}),
	}
	f.register(c)
	return c
}

func msgUnstarCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use: "unstar [REF...]", Short: "Remove a star from messages",
		RunE: bulkMessageAction(&f, "Unstarred %d message(s).", "unstar", func(cmd *cobra.Command, ids []string) error {
			a := app.From(cmd.Context())
			return a.Mail.Mark(cmd.Context(), ids, false, false, false, true)
		}),
	}
	f.register(c)
	return c
}

func bulkMessageAction(f *msgFilter, successFmt, otherVerb string, do func(cmd *cobra.Command, ids []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		a := app.From(cmd.Context())
		if err := a.Authenticate(cmd.Context()); err != nil {
			return err
		}
		ids, err := collectMessageIDs(cmd, args, f)
		if err != nil {
			return handleWrongTable(err, otherVerb)
		}
		if a.DryRun {
			a.R.Info(fmt.Sprintf("dry-run: would affect %d message(s)", len(ids)))
			for _, id := range ids {
				_, _ = fmt.Fprintln(a.R.Stderr, "  "+id)
			}
			return nil
		}
		if err := do(cmd, ids); err != nil {
			return err
		}
		a.R.Success(fmt.Sprintf(successFmt, len(ids)))
		return nil
	}
}

// renderListMessages emits a paginated listing of messages.
func renderListMessages(a *app.App, msgs []mailsvc.Message, total int, opts mailsvc.ListOptions) error {
	cacheMessageIDs(a, msgs)
	if a.R.Format != render.FormatText {
		hasMore := (opts.Page+1)*opts.PageSize < total
		return a.R.Object(struct {
			Total    int               `json:"total"`
			Page     int               `json:"page"`
			PageSize int               `json:"page_size"`
			HasMore  bool              `json:"has_more"`
			Messages []mailsvc.Message `json:"messages"`
		}{Total: total, Page: opts.Page, PageSize: opts.PageSize, HasMore: hasMore, Messages: msgs})
	}
	renderMessageTable(a, msgs)
	if footer := render.PaginationFooter("messages", total, opts.Page, opts.PageSize, len(msgs)); footer != "" {
		_, _ = fmt.Fprintln(a.R.Stderr, "\n"+footer)
	}
	return nil
}

// renderSearchMessages emits a search-result listing. No --page concept here;
// the footer reflects whether the limit was hit.
func renderSearchMessages(a *app.App, msgs []mailsvc.Message, limit int) error {
	cacheMessageIDs(a, msgs)
	if a.R.Format != render.FormatText {
		limited := limit > 0 && len(msgs) >= limit
		return a.R.Object(struct {
			Total    int               `json:"total"`
			Results  int               `json:"results"`
			Limited  bool              `json:"limited"`
			Messages []mailsvc.Message `json:"messages"`
		}{Total: len(msgs), Results: len(msgs), Limited: limited, Messages: msgs})
	}
	renderMessageTable(a, msgs)
	_, _ = fmt.Fprintln(a.R.Stderr, "\n"+render.SearchFooter(len(msgs), limit))
	return nil
}

// cacheMessageIDs is fire-and-forget: failures are ignored so the user-facing
// list command never breaks because of a cache hiccup.
func cacheMessageIDs(a *app.App, msgs []mailsvc.Message) {
	if a == nil || a.IDCache == nil || len(msgs) == 0 {
		return
	}
	ids := make([]string, 0, len(msgs))
	for _, m := range msgs {
		ids = append(ids, m.ID)
	}
	_ = a.IDCache.Save(ids...)
}

func renderMessageTable(a *app.App, msgs []mailsvc.Message) {
	short := a.R.IsTTY() && !a.FullIDs
	headers := []string{"ID", "FROM", "SUBJECT", "DATE", "⚑"}
	rows := make([][]string, 0, len(msgs))
	for _, m := range msgs {
		from := m.FromAddress
		if m.FromName != "" {
			from = m.FromName
		}
		flags := ""
		if m.Unread == 1 {
			flags += "●"
		}
		if m.NumAttachments > 0 {
			flags += "📎"
		}
		rows = append(rows, []string{render.ShortID(m.ID, short), from, m.Subject, time.Unix(m.Time, 0).Local().Format("2006-01-02 15:04"), flags})
	}
	render.Table(a.R.Stdout, headers, rows)
}

// ── mail attachments ──

func attachmentsCmd() *cobra.Command {
	c := &cobra.Command{Use: "attachments", Short: "Manage message attachments"}
	c.AddCommand(attachmentsListCmd())
	c.AddCommand(attachmentDownloadCmd())
	return c
}

func attachmentsListCmd() *cobra.Command {
	var includeInline bool
	c := &cobra.Command{
		Use: "list MESSAGE_ID", Short: "List attachments of a message",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			msgID, err := shared.ResolvePrefix(a, args[0])
			if err != nil {
				return err
			}
			atts, err := a.Mail.AttachmentsList(cmd.Context(), msgID, includeInline)
			if err != nil {
				return err
			}
			if a.IDCache != nil && len(atts) > 0 {
				ids := make([]string, 0, len(atts))
				for _, at := range atts {
					ids = append(ids, at.ID)
				}
				_ = a.IDCache.Save(ids...)
			}
			if a.R.Format != render.FormatText {
				return a.R.Object(atts)
			}
			short := a.R.IsTTY() && !a.FullIDs
			headers := []string{"ID", "NAME", "SIZE", "TYPE"}
			if includeInline {
				headers = append(headers, "DISPOSITION")
			}
			var rows [][]string
			for _, at := range atts {
				row := []string{render.ShortID(at.ID, short), at.Name, render.Size(at.Size), at.MIMEType}
				if includeInline {
					disp := at.Disposition
					if disp == "" {
						disp = "attachment"
					}
					row = append(row, disp)
				}
				rows = append(rows, row)
			}
			render.Table(a.R.Stdout, headers, rows)
			return nil
		},
	}
	c.Flags().BoolVar(&includeInline, "include-inline", false,
		"Include inline attachments (e.g. signature graphics) hidden by default")
	return c
}

func attachmentDownloadCmd() *cobra.Command {
	var output, outputDir string
	var all, force, includeInline bool
	c := &cobra.Command{
		Use:   "download MESSAGE_ID [ATTACHMENT_ID] [OUTPUT_PATH]",
		Short: "Download and decrypt attachment(s) (- for stdout, --all for every attachment)",
		Args:  cobra.RangeArgs(1, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			msgID := args[0]
			var attID, posPath string
			switch len(args) {
			case 2:
				attID = args[1]
			case 3:
				attID, posPath = args[1], args[2]
			}

			if err := validateDownloadShape(attID, posPath, output, outputDir, all); err != nil {
				return err
			}
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			u, err := a.Unlock(cmd.Context())
			if err != nil {
				return err
			}
			if msgID, err = shared.ResolvePrefix(a, msgID); err != nil {
				return err
			}
			if attID, err = shared.ResolvePrefix(a, attID); err != nil {
				return err
			}

			if all {
				return downloadAllAttachments(cmd, a, u, msgID, outputDir, force, includeInline)
			}
			return downloadOneAttachment(cmd, a, u, msgID, attID, posPath, output, outputDir, force)
		},
	}
	c.Flags().StringVar(&output, "output", "", "Explicit output path (- for stdout); errors on existing file")
	c.Flags().StringVar(&outputDir, "output-dir", "", "Output directory; uses the attachment's own name (auto-suffix on collision)")
	c.Flags().BoolVar(&all, "all", false, "Download every attachment on the message (requires --output-dir)")
	c.Flags().BoolVar(&force, "force", false, "Overwrite existing destination files")
	c.Flags().BoolVar(&includeInline, "include-inline", false,
		"Include inline attachments (e.g. signature graphics) when --all")
	return c
}

// validateDownloadShape checks the flag/positional combination matrix shared
// by `mail attachments download` and `mail conversations attachments
// download`. idArg is the ATTACHMENT_ID positional (empty when --all).
func validateDownloadShape(idArg, posPath, output, outputDir string, all bool) error {
	if posPath != "" && output != "" {
		return fmt.Errorf("specify either positional path or --output, not both")
	}
	if posPath != "" && outputDir != "" {
		return fmt.Errorf("positional path is incompatible with --output-dir")
	}
	if output != "" && outputDir != "" {
		return fmt.Errorf("specify either --output or --output-dir, not both")
	}
	if all {
		if idArg != "" {
			return fmt.Errorf("--all does not take an ATTACHMENT_ID")
		}
		if posPath != "" {
			return fmt.Errorf("--all requires --output-dir, not a positional path")
		}
		if output != "" {
			if output == "-" {
				return fmt.Errorf("--all cannot write to stdout")
			}
			return fmt.Errorf("--all requires --output-dir, not --output")
		}
		if outputDir == "" {
			return fmt.Errorf("--all requires --output-dir")
		}
	} else if idArg == "" {
		return fmt.Errorf("ATTACHMENT_ID is required (or use --all)")
	}
	return nil
}

// downloadOneAttachment handles the single-attachment path. The destination
// is resolved (in priority order) from positional path, --output, --output-dir,
// or the attachment's own name in the current working directory.
func downloadOneAttachment(cmd *cobra.Command, a *app.App, u *keys.Unlocked, msgID, attID, posPath, output, outputDir string, force bool) error {
	bin, name, err := a.Mail.AttachmentDownload(cmd.Context(), u, msgID, attID)
	if err != nil {
		return err
	}

	switch {
	case posPath == "-" || output == "-":
		_, err := a.R.Stdout.Write(bin)
		return err
	case posPath != "":
		return writeAttachment(a, bin, posPath, shared.WriteError, force)
	case output != "":
		return writeAttachment(a, bin, output, shared.WriteError, force)
	case outputDir != "":
		if err := ensureDir(outputDir); err != nil {
			return err
		}
		return writeAttachment(a, bin, filepath.Join(outputDir, name), shared.WriteAutoSuffix, force)
	default:
		// Implicit destination: attachment's own name in CWD.
		return writeAttachment(a, bin, name, shared.WriteAutoSuffix, force)
	}
}

func downloadAllAttachments(cmd *cobra.Command, a *app.App, u *keys.Unlocked, msgID, outputDir string, force, includeInline bool) error {
	atts, err := a.Mail.AttachmentsList(cmd.Context(), msgID, includeInline)
	if err != nil {
		return err
	}
	if len(atts) == 0 {
		a.R.Info("no attachments to download")
		return nil
	}
	if err := ensureDir(outputDir); err != nil {
		return err
	}
	for _, at := range atts {
		bin, _, err := a.Mail.AttachmentDownload(cmd.Context(), u, msgID, at.ID)
		if err != nil {
			return fmt.Errorf("download %s (%s): %w", at.Name, at.ID, err)
		}
		if err := writeAttachment(a, bin, filepath.Join(outputDir, at.Name), shared.WriteAutoSuffix, force); err != nil {
			return err
		}
	}
	a.R.Success(fmt.Sprintf("Downloaded %d attachment(s) to %s", len(atts), outputDir))
	return nil
}

// writeAttachment writes data via shared.WriteFileSafe, picking the mode from
// the caller's intent: implicit names auto-suffix, explicit names error on
// collision, --force overrides everything.
func writeAttachment(a *app.App, data []byte, path string, mode shared.WriteMode, force bool) error {
	if force {
		mode = shared.WriteForce
	}
	written, err := shared.WriteFileSafe(path, data, 0644, mode)
	if err != nil {
		return err
	}
	a.R.Success(fmt.Sprintf("Downloaded %s (%d bytes)", written, len(data)))
	return nil
}

// ensureDir creates dir (and any missing parents) when it is missing, and
// errors if the path exists but is not a directory.
func ensureDir(dir string) error {
	if fi, err := os.Stat(dir); err == nil {
		if !fi.IsDir() {
			return fmt.Errorf("%s exists and is not a directory", dir)
		}
		return nil
	}
	return os.MkdirAll(dir, 0755)
}

// ── mail conversations ──

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

// ── mail conversations attachments ──

// convAttachmentsCmd returns the `mail conversations attachments` subtree:
// the conversation-level twin of `mail attachments`. Lists span all messages
// in the thread; download resolves ATTACHMENT_ID to its parent message
// internally so callers don't have to.
func convAttachmentsCmd() *cobra.Command {
	c := &cobra.Command{Use: "attachments", Short: "Manage attachments across a conversation"}
	c.AddCommand(convAttachmentsListCmd())
	c.AddCommand(convAttachmentDownloadCmd())
	return c
}

func convAttachmentsListCmd() *cobra.Command {
	var includeInline bool
	c := &cobra.Command{
		Use:   "list CONVERSATION_ID",
		Short: "List attachments across all messages in a conversation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			convID, err := shared.ResolvePrefix(a, args[0])
			if err != nil {
				return err
			}
			atts, err := a.Mail.ConversationAttachmentsList(cmd.Context(), convID, includeInline)
			if err != nil {
				return handleWrongTable(err, "attachments list")
			}
			if a.IDCache != nil && len(atts) > 0 {
				ids := make([]string, 0, len(atts)*2)
				for _, at := range atts {
					ids = append(ids, at.ID, at.MessageID)
				}
				_ = a.IDCache.Save(ids...)
			}
			if a.R.Format != render.FormatText {
				return a.R.Object(atts)
			}
			short := a.R.IsTTY() && !a.FullIDs
			headers := []string{"ID", "NAME", "SIZE", "TYPE", "MESSAGE_ID"}
			if includeInline {
				headers = append(headers, "DISPOSITION")
			}
			var rows [][]string
			for _, at := range atts {
				row := []string{render.ShortID(at.ID, short), at.Name, render.Size(at.Size), at.MIMEType, render.ShortID(at.MessageID, short)}
				if includeInline {
					disp := at.Disposition
					if disp == "" {
						disp = "attachment"
					}
					row = append(row, disp)
				}
				rows = append(rows, row)
			}
			render.Table(a.R.Stdout, headers, rows)
			return nil
		},
	}
	c.Flags().BoolVar(&includeInline, "include-inline", false,
		"Include inline attachments (e.g. signature graphics) hidden by default")
	return c
}

func convAttachmentDownloadCmd() *cobra.Command {
	var output, outputDir string
	var all, force, includeInline bool
	c := &cobra.Command{
		Use:   "download CONVERSATION_ID [ATTACHMENT_ID] [OUTPUT_PATH]",
		Short: "Download and decrypt attachment(s) from a conversation",
		Args:  cobra.RangeArgs(1, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			convID := args[0]
			var attID, posPath string
			switch len(args) {
			case 2:
				attID = args[1]
			case 3:
				attID, posPath = args[1], args[2]
			}
			if err := validateDownloadShape(attID, posPath, output, outputDir, all); err != nil {
				return err
			}
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			u, err := a.Unlock(cmd.Context())
			if err != nil {
				return err
			}
			if convID, err = shared.ResolvePrefix(a, convID); err != nil {
				return err
			}
			if attID, err = shared.ResolvePrefix(a, attID); err != nil {
				return err
			}
			if all {
				return downloadAllConvAttachments(cmd, a, u, convID, outputDir, force, includeInline)
			}
			return downloadOneConvAttachment(cmd, a, u, convID, attID, posPath, output, outputDir, force)
		},
	}
	c.Flags().StringVar(&output, "output", "", "Explicit output path (- for stdout); errors on existing file")
	c.Flags().StringVar(&outputDir, "output-dir", "", "Output directory; uses the attachment's own name (auto-suffix on collision)")
	c.Flags().BoolVar(&all, "all", false, "Download every attachment in the conversation (requires --output-dir)")
	c.Flags().BoolVar(&force, "force", false, "Overwrite existing destination files")
	c.Flags().BoolVar(&includeInline, "include-inline", false,
		"Include inline attachments (e.g. signature graphics) when --all")
	return c
}

// downloadOneConvAttachment resolves ATTACHMENT_ID to its parent message ID
// via a conversation-wide listing, then dispatches to AttachmentDownload.
// Inline attachments are reachable here by ID (we always look them up with
// includeInline=true) so the user isn't blocked from downloading a known
// inline asset.
func downloadOneConvAttachment(cmd *cobra.Command, a *app.App, u *keys.Unlocked, convID, attID, posPath, output, outputDir string, force bool) error {
	list, err := a.Mail.ConversationAttachmentsList(cmd.Context(), convID, true)
	if err != nil {
		return handleWrongTable(err, "attachments download")
	}
	var msgID, name string
	for _, at := range list {
		if at.ID == attID {
			msgID, name = at.MessageID, at.Name
			break
		}
	}
	if msgID == "" {
		return app.Exit(3, fmt.Errorf("attachment %s not found in conversation %s", attID, convID))
	}
	bin, _, err := a.Mail.AttachmentDownload(cmd.Context(), u, msgID, attID)
	if err != nil {
		return err
	}
	switch {
	case posPath == "-" || output == "-":
		_, err := a.R.Stdout.Write(bin)
		return err
	case posPath != "":
		return writeAttachment(a, bin, posPath, shared.WriteError, force)
	case output != "":
		return writeAttachment(a, bin, output, shared.WriteError, force)
	case outputDir != "":
		if err := ensureDir(outputDir); err != nil {
			return err
		}
		return writeAttachment(a, bin, filepath.Join(outputDir, name), shared.WriteAutoSuffix, force)
	default:
		return writeAttachment(a, bin, name, shared.WriteAutoSuffix, force)
	}
}

func downloadAllConvAttachments(cmd *cobra.Command, a *app.App, u *keys.Unlocked, convID, outputDir string, force, includeInline bool) error {
	list, err := a.Mail.ConversationAttachmentsList(cmd.Context(), convID, includeInline)
	if err != nil {
		return handleWrongTable(err, "attachments download")
	}
	if len(list) == 0 {
		a.R.Info("no attachments to download")
		return nil
	}
	if err := ensureDir(outputDir); err != nil {
		return err
	}
	for _, at := range list {
		bin, _, err := a.Mail.AttachmentDownload(cmd.Context(), u, at.MessageID, at.ID)
		if err != nil {
			return fmt.Errorf("download %s (%s): %w", at.Name, at.ID, err)
		}
		if err := writeAttachment(a, bin, filepath.Join(outputDir, at.Name), shared.WriteAutoSuffix, force); err != nil {
			return err
		}
	}
	a.R.Success(fmt.Sprintf("Downloaded %d attachment(s) to %s", len(list), outputDir))
	return nil
}

func convListCmd() *cobra.Command {
	var opts mailsvc.ListOptions
	c := &cobra.Command{
		Use: "list", Short: "List conversations",
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			convs, total, err := a.Mail.ConversationsList(cmd.Context(), opts)
			if err != nil {
				return err
			}
			return renderListConversations(a, convs, total, opts)
		},
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
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			convs, _, err := a.Mail.ConversationsSearch(cmd.Context(), opts)
			if err != nil {
				return err
			}
			if len(convs) == 0 {
				hintFromTo(a, "conversations", opts)
			}
			return renderSearchConversations(a, convs, opts.Limit)
		},
	}
	c.Flags().StringVar(&opts.Keyword, "keyword", "", "Search keyword")
	c.Flags().StringVar(&opts.From, "from", "", "Filter by sender email address (use --keyword to also match display names)")
	c.Flags().StringVar(&opts.To, "to", "", "Filter by recipient email address (use --keyword to also match display names)")
	c.Flags().StringVar(&opts.Subject, "subject", "", "Filter by subject")
	c.Flags().StringVar(&opts.After, "after", "", "After date (YYYY-MM-DD)")
	c.Flags().StringVar(&opts.Before, "before", "", "Before date (YYYY-MM-DD)")
	c.Flags().StringVar(&opts.Folder, "folder", "all", "Folder to search in")
	c.Flags().IntVar(&opts.Limit, "limit", 25, "Max results")
	return c
}

func convReadCmd() *cobra.Command {
	var format string
	var includeInline, bodyOnly, stripQuotes, summary bool
	c := &cobra.Command{
		Use: "read REF", Short: "Read a conversation (full thread, decrypted)",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			ref, err := shared.ResolvePrefix(a, args[0])
			if err != nil {
				return err
			}
			id, err := a.Mail.ResolveConversation(cmd.Context(), ref)
			if err != nil {
				return app.Exit(resolveExit(err), err)
			}
			u, err := a.Unlock(cmd.Context())
			if err != nil {
				return err
			}
			conv, err := a.Mail.ConversationRead(cmd.Context(), u, id)
			if err != nil {
				return handleWrongTable(err, "read")
			}
			if a.R.Format != render.FormatText {
				return a.R.Object(conv)
			}
			if summary {
				return renderConversationSummary(a, conv)
			}
			return renderConversationText(a, conv, format, includeInline, bodyOnly, stripQuotes)
		},
	}
	c.Flags().StringVar(&format, "format", "text", "Body format: text, html, raw")
	c.Flags().BoolVar(&includeInline, "include-inline", false,
		"Include inline attachments (e.g. signature graphics) in per-message footers")
	c.Flags().BoolVar(&bodyOnly, "body-only", false,
		"Suppress envelope, dividers, headers, and per-message footers; concatenate bodies separated by blank lines (default for --format html|raw)")
	c.Flags().BoolVar(&stripQuotes, "strip-quotes", false,
		"Remove quoted reply blocks from each message body (heuristic)")
	c.Flags().BoolVar(&summary, "summary", false,
		"One-line preview per message; implies --strip-quotes")
	return c
}

func convTrashCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use: "trash [REF...]", Short: "Move conversations to trash",
		RunE: bulkConversationAction(&f, "Moved %d conversation(s) to trash.", "trash", func(cmd *cobra.Command, ids []string) error {
			a := app.From(cmd.Context())
			return a.Mail.ConversationsTrash(cmd.Context(), ids)
		}),
	}
	f.register(c)
	return c
}

func convDeleteCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use: "delete [REF...]", Short: "Permanently delete conversations",
		RunE: bulkConversationAction(&f, "Permanently deleted %d conversation(s).", "delete", func(cmd *cobra.Command, ids []string) error {
			a := app.From(cmd.Context())
			return a.Mail.ConversationsDelete(cmd.Context(), ids, f.folder)
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
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			ids, err := collectConversationIDs(cmd, args, &f)
			if err != nil {
				return handleWrongTable(err, "move")
			}
			if a.DryRun {
				a.R.Info(fmt.Sprintf("dry-run: would move %d conversation(s) to %s", len(ids), dest))
				return nil
			}
			if err := a.Mail.ConversationsMove(cmd.Context(), ids, dest); err != nil {
				return err
			}
			a.R.Success(fmt.Sprintf("Moved %d conversation(s) to %s.", len(ids), dest))
			return nil
		},
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
		RunE: func(cmd *cobra.Command, args []string) error {
			action := strings.ToLower(args[0])
			rest := args[1:]
			if action != "read" && action != "unread" {
				return fmt.Errorf("unknown action %q (use: read, unread)", action)
			}
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			ids, err := collectConversationIDs(cmd, rest, &f)
			if err != nil {
				return handleWrongTable(err, "mark "+action)
			}
			if a.DryRun {
				a.R.Info(fmt.Sprintf("dry-run: would mark %d conversation(s) as %s", len(ids), action))
				return nil
			}
			if err := a.Mail.ConversationsMark(cmd.Context(), ids, action == "read", action == "unread", false, false, f.folder); err != nil {
				return err
			}
			a.R.Success(fmt.Sprintf("Marked %d conversation(s) as %s.", len(ids), action))
			return nil
		},
	}
	f.register(c)
	return c
}

func convStarCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use: "star [REF...]", Short: "Add a star to conversations",
		RunE: bulkConversationAction(&f, "Starred %d conversation(s).", "star", func(cmd *cobra.Command, ids []string) error {
			a := app.From(cmd.Context())
			return a.Mail.ConversationsMark(cmd.Context(), ids, false, false, true, false, f.folder)
		}),
	}
	f.register(c)
	return c
}

func convUnstarCmd() *cobra.Command {
	var f msgFilter
	c := &cobra.Command{
		Use: "unstar [REF...]", Short: "Remove a star from conversations",
		RunE: bulkConversationAction(&f, "Unstarred %d conversation(s).", "unstar", func(cmd *cobra.Command, ids []string) error {
			a := app.From(cmd.Context())
			return a.Mail.ConversationsMark(cmd.Context(), ids, false, false, false, true, f.folder)
		}),
	}
	f.register(c)
	return c
}

func bulkConversationAction(f *msgFilter, successFmt, otherVerb string, do func(cmd *cobra.Command, ids []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		a := app.From(cmd.Context())
		if err := a.Authenticate(cmd.Context()); err != nil {
			return err
		}
		ids, err := collectConversationIDs(cmd, args, f)
		if err != nil {
			return handleWrongTable(err, otherVerb)
		}
		if a.DryRun {
			a.R.Info(fmt.Sprintf("dry-run: would affect %d conversation(s)", len(ids)))
			for _, id := range ids {
				_, _ = fmt.Fprintln(a.R.Stderr, "  "+id)
			}
			return nil
		}
		if err := do(cmd, ids); err != nil {
			return err
		}
		a.R.Success(fmt.Sprintf(successFmt, len(ids)))
		return nil
	}
}

// collectConversationIDs is the conversations-side counterpart of
// collectMessageIDs.
func collectConversationIDs(cmd *cobra.Command, args []string, f *msgFilter) ([]string, error) {
	a := app.From(cmd.Context())
	var ids []string

	refs, err := shared.ResolvePrefixes(a, args)
	if err != nil {
		return nil, err
	}

	if len(refs) == 1 && mailsvc.LooksLikeID(refs[0]) {
		if err := a.Mail.AssertConversationKind(cmd.Context(), refs[0]); err != nil {
			var wte *mailsvc.WrongTableError
			if errors.As(err, &wte) {
				return nil, err
			}
		}
	}

	for _, arg := range refs {
		id, err := a.Mail.ResolveConversation(cmd.Context(), arg)
		if err != nil {
			return nil, app.Exit(resolveExit(err), err)
		}
		ids = append(ids, id)
	}

	if f.set() {
		if f.all && !f.unread && f.from == "" && f.to == "" && f.subject == "" &&
			f.keyword == "" && f.folder == "" && f.olderThan == "" && f.newerThan == "" {
			a.R.Info("--all with no other filter will affect every conversation in the account. Add --folder to scope it.")
		}
		search, err := f.toSearch()
		if err != nil {
			return nil, err
		}
		convs, _, err := a.Mail.ConversationsSearch(cmd.Context(), search)
		if err != nil {
			return nil, err
		}
		for _, c := range convs {
			ids = append(ids, c.ID)
		}
	}

	if len(args) == 0 && !f.set() {
		return nil, fmt.Errorf("no conversations selected: pass REF(s) or a filter (e.g. --unread, --from, --older-than); use --all to target an entire folder")
	}

	return shared.Dedupe(ids), nil
}

// renderListConversations emits a paginated listing of conversations.
func renderListConversations(a *app.App, convs []mailsvc.Conversation, total int, opts mailsvc.ListOptions) error {
	cacheConversationIDs(a, convs)
	if a.R.Format != render.FormatText {
		hasMore := (opts.Page+1)*opts.PageSize < total
		return a.R.Object(struct {
			Total         int                    `json:"total"`
			Page          int                    `json:"page"`
			PageSize      int                    `json:"page_size"`
			HasMore       bool                   `json:"has_more"`
			Conversations []mailsvc.Conversation `json:"conversations"`
		}{Total: total, Page: opts.Page, PageSize: opts.PageSize, HasMore: hasMore, Conversations: convs})
	}
	renderConversationTable(a, convs)
	if footer := render.PaginationFooter("conversations", total, opts.Page, opts.PageSize, len(convs)); footer != "" {
		_, _ = fmt.Fprintln(a.R.Stderr, "\n"+footer)
	}
	return nil
}

// renderSearchConversations emits a search-result listing of conversations.
func renderSearchConversations(a *app.App, convs []mailsvc.Conversation, limit int) error {
	cacheConversationIDs(a, convs)
	if a.R.Format != render.FormatText {
		limited := limit > 0 && len(convs) >= limit
		return a.R.Object(struct {
			Total         int                    `json:"total"`
			Results       int                    `json:"results"`
			Limited       bool                   `json:"limited"`
			Conversations []mailsvc.Conversation `json:"conversations"`
		}{Total: len(convs), Results: len(convs), Limited: limited, Conversations: convs})
	}
	renderConversationTable(a, convs)
	_, _ = fmt.Fprintln(a.R.Stderr, "\n"+render.SearchFooter(len(convs), limit))
	return nil
}

func cacheConversationIDs(a *app.App, convs []mailsvc.Conversation) {
	if a == nil || a.IDCache == nil || len(convs) == 0 {
		return
	}
	ids := make([]string, 0, len(convs))
	for _, c := range convs {
		ids = append(ids, c.ID)
	}
	_ = a.IDCache.Save(ids...)
}

func renderConversationTable(a *app.App, convs []mailsvc.Conversation) {
	short := a.R.IsTTY() && !a.FullIDs
	headers := []string{"ID", "FROM", "SUBJECT", "#", "DATE", "⚑"}
	rows := make([][]string, 0, len(convs))
	for _, c := range convs {
		from := ""
		if len(c.Senders) > 0 {
			if n, ok := c.Senders[0]["Name"].(string); ok && n != "" {
				from = n
			} else if a, ok := c.Senders[0]["Address"].(string); ok {
				from = a
			}
		}
		flags := ""
		if c.NumUnread > 0 {
			flags += "●"
		}
		if c.NumAttachments > 0 {
			flags += "📎"
		}
		rows = append(rows, []string{
			render.ShortID(c.ID, short), from, c.Subject,
			fmt.Sprintf("%d", c.NumMessages),
			time.Unix(c.Time, 0).Local().Format("2006-01-02 15:04"),
			flags,
		})
	}
	render.Table(a.R.Stdout, headers, rows)
}

func renderConversationText(a *app.App, conv *mailsvc.ConversationFull, format string, includeInline, bodyOnly, stripQuotes bool) error {
	if format != "text" && format != "html" && format != "raw" {
		return fmt.Errorf("unknown --format %q (use text, html, raw)", format)
	}
	n := len(conv.Messages)
	// Envelope, per-message dividers/headers, and per-message attachments
	// footer only in text mode without --body-only. --format html and
	// --format raw skip them so captured output is valid for downstream
	// parsers.
	showMetadata := format == "text" && !bodyOnly
	if showMetadata {
		_, _ = fmt.Fprintf(a.R.Stdout, "Subject:      %s\n", conv.Conversation.Subject)
		_, _ = fmt.Fprintf(a.R.Stdout, "Conversation: %s\n", conv.Conversation.ID)
		_, _ = fmt.Fprintf(a.R.Stdout, "Messages:     %d\n\n", n)
	}
	divider := strings.Repeat("─", 56)
	for i, m := range conv.Messages {
		if showMetadata {
			_, _ = fmt.Fprintf(a.R.Stdout, "─── %d/%d %s\n", i+1, n, divider)
			if s, ok := m.Sender["Address"].(string); ok {
				name, _ := m.Sender["Name"].(string)
				if name != "" {
					_, _ = fmt.Fprintf(a.R.Stdout, "From: %s <%s>\n", name, s)
				} else {
					_, _ = fmt.Fprintf(a.R.Stdout, "From: %s\n", s)
				}
			}
			for _, t := range m.ToList {
				if s, ok := t["Address"].(string); ok {
					_, _ = fmt.Fprintf(a.R.Stdout, "To:   %s\n", s)
				}
			}
			if m.Time > 0 {
				_, _ = fmt.Fprintf(a.R.Stdout, "Date: %s\n", time.Unix(m.Time, 0).Local().Format("2006-01-02 15:04"))
			}
			_, _ = fmt.Fprintf(a.R.Stdout, "ID:   %s\n\n", m.ID)
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
		_, _ = fmt.Fprintln(a.R.Stdout, body)
		if showMetadata {
			if footer := renderAttachmentsFooter(m.Attachments, includeInline); footer != "" {
				_, _ = io.WriteString(a.R.Stdout, footer)
			}
		}
		if i < n-1 {
			_, _ = fmt.Fprintln(a.R.Stdout)
		}
	}
	return nil
}

// renderConversationSummary emits one preview line per message in a thread:
//
//	N/M  YYYY-MM-DD HH:MM  sender@addr   <preview>  [(K attachments)]
//
// Bodies are quote-stripped (so the preview is drawn from new content,
// not the trailing quote chain). Attachment count covers non-inline
// attachments only — inline graphics aren't user-facing in triage.
func renderConversationSummary(a *app.App, conv *mailsvc.ConversationFull) error {
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
				// Body collapsed to nothing but there are attachments —
				// show the attachment count as the preview cell, no separate tag.
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
		_, _ = fmt.Fprintln(a.R.Stdout, line)
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

// ── mail labels ──

func labelsCmd() *cobra.Command {
	c := &cobra.Command{Use: "labels", Short: "Manage labels and folders"}
	c.AddCommand(&cobra.Command{
		Use: "list", Short: "List labels and folders",
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			labels, folders, err := a.Mail.LabelsList(cmd.Context())
			if err != nil {
				return err
			}
			if a.IDCache != nil {
				ids := make([]string, 0, len(folders)+len(labels))
				for _, l := range folders {
					ids = append(ids, l.ID)
				}
				for _, l := range labels {
					ids = append(ids, l.ID)
				}
				_ = a.IDCache.Save(ids...)
			}
			if a.R.Format != render.FormatText {
				return a.R.Object(map[string]any{"Labels": labels, "Folders": folders})
			}
			short := a.R.IsTTY() && !a.FullIDs
			headers := []string{"ID", "TYPE", "NAME", "COLOR", "PATH"}
			var rows [][]string
			for _, l := range folders {
				rows = append(rows, []string{render.ShortID(l.ID, short), "FOLDER", l.Name, l.Color, l.Path})
			}
			for _, l := range labels {
				rows = append(rows, []string{render.ShortID(l.ID, short), "LABEL", l.Name, l.Color, ""})
			}
			render.Table(a.R.Stdout, headers, rows)
			return nil
		},
	})
	var createName, createColor string
	var createFolder bool
	createCmd := &cobra.Command{
		Use: "create", Short: "Create a label or folder",
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			if createName == "" {
				return fmt.Errorf("--name is required")
			}
			if a.DryRun {
				a.R.Info(fmt.Sprintf("dry-run: would create label %q", createName))
				return nil
			}
			body, err := a.Mail.LabelCreate(cmd.Context(), createName, createColor, createFolder)
			if err != nil {
				return err
			}
			id := pickID(body, "Label", "ID")
			kind := "Label"
			if createFolder {
				kind = "Folder"
			}
			a.R.ID(id, fmt.Sprintf("Created %s %q", kind, createName))
			return nil
		},
	}
	createCmd.Flags().StringVar(&createName, "name", "", "Label name")
	createCmd.Flags().StringVar(&createColor, "color", "#7272a7", "Label color (hex)")
	createCmd.Flags().BoolVar(&createFolder, "folder", false, "Create a folder instead of a label")
	c.AddCommand(createCmd)

	c.AddCommand(&cobra.Command{
		Use: "delete LABEL_ID...", Short: "Delete labels or folders",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			ids, err := shared.ResolvePrefixes(a, args)
			if err != nil {
				return err
			}
			if a.DryRun {
				a.R.Info(fmt.Sprintf("dry-run: would delete %d label(s)", len(ids)))
				return nil
			}
			if err := a.Mail.LabelDelete(cmd.Context(), ids); err != nil {
				return err
			}
			a.R.Success(fmt.Sprintf("Deleted %d label(s)/folder(s).", len(ids)))
			return nil
		},
	})
	return c
}

// ── mail filters ──

func filtersCmd() *cobra.Command {
	c := &cobra.Command{Use: "filters", Short: "Manage Sieve filters"}
	c.AddCommand(&cobra.Command{
		Use: "list", Short: "List filters",
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			body, err := a.Mail.FiltersList(cmd.Context())
			if err != nil {
				return err
			}
			if a.R.Format != render.FormatText {
				return a.R.JSON(body)
			}
			var r struct {
				Filters []struct {
					ID, Name string
					Status   int
					Version  int
				}
			}
			if err := json.Unmarshal(body, &r); err != nil {
				return err
			}
			if a.IDCache != nil && len(r.Filters) > 0 {
				ids := make([]string, 0, len(r.Filters))
				for _, f := range r.Filters {
					ids = append(ids, f.ID)
				}
				_ = a.IDCache.Save(ids...)
			}
			short := a.R.IsTTY() && !a.FullIDs
			headers := []string{"ID", "STATUS", "NAME", "VERSION"}
			var rows [][]string
			for _, f := range r.Filters {
				st := "disabled"
				if f.Status == 1 {
					st = "enabled"
				}
				rows = append(rows, []string{render.ShortID(f.ID, short), st, f.Name, fmt.Sprintf("%d", f.Version)})
			}
			render.Table(a.R.Stdout, headers, rows)
			return nil
		},
	})
	var fName, fSieve string
	var fStatus int
	createCmd := &cobra.Command{
		Use: "create", Short: "Create a sieve filter",
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			if fName == "" || fSieve == "" {
				return fmt.Errorf("--name and --sieve are required")
			}
			if a.DryRun {
				a.R.Info(fmt.Sprintf("dry-run: would create filter %q", fName))
				return nil
			}
			body, err := a.Mail.FilterCreate(cmd.Context(), fName, fSieve, fStatus)
			if err != nil {
				return err
			}
			id := pickID(body, "Filter", "ID")
			a.R.ID(id, fmt.Sprintf("Created filter %q", fName))
			return nil
		},
	}
	createCmd.Flags().StringVar(&fName, "name", "", "Filter name")
	createCmd.Flags().StringVar(&fSieve, "sieve", "", "Sieve script")
	createCmd.Flags().IntVar(&fStatus, "status", 1, "Status (1=enabled, 0=disabled)")
	c.AddCommand(createCmd)

	c.AddCommand(filterSingleArg("delete", "Delete a filter", func(a *app.App, ctx context.Context, id string) error { return a.Mail.FilterDelete(ctx, id) }, "Deleted filter %s."))
	c.AddCommand(filterSingleArg("enable", "Enable a filter", func(a *app.App, ctx context.Context, id string) error { return a.Mail.FilterEnable(ctx, id) }, "Enabled filter %s."))
	c.AddCommand(filterSingleArg("disable", "Disable a filter", func(a *app.App, ctx context.Context, id string) error { return a.Mail.FilterDisable(ctx, id) }, "Disabled filter %s."))
	return c
}

// ── mail addresses ──

func addressesCmd() *cobra.Command {
	c := &cobra.Command{Use: "addresses", Short: "Manage email addresses"}
	c.AddCommand(&cobra.Command{
		Use: "list", Short: "List email addresses on the account",
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			body, err := a.Mail.AddressesList(cmd.Context())
			if err != nil {
				return err
			}
			if a.R.Format != render.FormatText {
				return a.R.JSON(body)
			}
			var r struct {
				Addresses []struct {
					ID, Email, DisplayName string
					Type, Status           int
				}
			}
			if err := json.Unmarshal(body, &r); err != nil {
				return err
			}
			if a.IDCache != nil && len(r.Addresses) > 0 {
				ids := make([]string, 0, len(r.Addresses))
				for _, ad := range r.Addresses {
					ids = append(ids, ad.ID)
				}
				_ = a.IDCache.Save(ids...)
			}
			short := a.R.IsTTY() && !a.FullIDs
			headers := []string{"ID", "EMAIL", "DISPLAY_NAME", "STATUS", "TYPE"}
			var rows [][]string
			for _, ad := range r.Addresses {
				ad.ID = render.ShortID(ad.ID, short)
				st := "disabled"
				if ad.Status == 1 {
					st = "active"
				}
				rows = append(rows, []string{ad.ID, ad.Email, ad.DisplayName, st, addressType(ad.Type)})
			}
			render.Table(a.R.Stdout, headers, rows)
			return nil
		},
	})
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

// Helpers ─────────────────────────────────────────────────────────────

func filterSingleArg(use, short string, fn func(a *app.App, ctx context.Context, id string) error, successFmt string) *cobra.Command {
	return &cobra.Command{
		Use: use + " FILTER_ID", Short: short,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.From(cmd.Context())
			if err := a.Authenticate(cmd.Context()); err != nil {
				return err
			}
			id, err := shared.ResolvePrefix(a, args[0])
			if err != nil {
				return err
			}
			if a.DryRun {
				a.R.Info(fmt.Sprintf("dry-run: would %s filter %s", use, id))
				return nil
			}
			if err := fn(a, cmd.Context(), id); err != nil {
				return err
			}
			a.R.Success(fmt.Sprintf(successFmt, id))
			return nil
		},
	}
}

func resolveExit(err error) int {
	if err == nil {
		return 0
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "ambiguous"):
		return 4
	case strings.Contains(s, "not found"), strings.Contains(s, "no "):
		return 3
	}
	return 1
}

func pickID(body []byte, keys ...string) string {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return ""
	}
	cur := v
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[k]
	}
	if s, ok := cur.(string); ok {
		return s
	}
	return ""
}
