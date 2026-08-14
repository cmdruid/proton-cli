package mail

import (
	"strconv"

	"github.com/roman-16/proton-cli/internal/cli/kit"
	"github.com/roman-16/proton-cli/internal/mailtext"
	mailsvc "github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/roman-16/proton-cli/internal/units"
	"github.com/spf13/cobra"
)

func messagesCmd() *cobra.Command {
	c := &cobra.Command{Use: "messages", Short: "Individual messages"}
	c.AddCommand(
		listCmd(), searchCmd(), getCmd(), sendCmd(), replyCmd(), forwardCmd(), exportCmd(),
		moveCmd(), labelCmd(), unlabelCmd(), starCmd(), unstarCmd(), markCmd(),
		trashCmd(), deleteCmd(), unscheduleCmd(), attachmentsCmd(),
	)
	return c
}

// ── reading ──

func listCmd() *cobra.Command {
	var opts mailsvc.ListOptions
	var starred bool
	c := &cobra.Command{
		Use:   "list",
		Short: "List messages in a folder",
		Args:  cobra.NoArgs,
		RunE: kit.Run(nil, func(c *kit.Invocation) error {
			msgs, total, err := c.App.Mail.List(c.Ctx, opts)
			if err != nil {
				return err
			}
			if starred {
				msgs = applyLocalFilters(msgs, &filters{starred: true})
			}
			return kit.List(c, ui.TableSpec[mailsvc.Message]{
				Noun: "messages", Columns: messageColumns(),
				Total: total, Page: opts.Page, PageSize: opts.PageSize,
			}, msgs, func(m mailsvc.Message) []string { return []string{m.ID} })
		}),
	}
	registerFolder(c, &opts.Folder, "inbox")
	c.Flags().IntVar(&opts.Page, "page", 0, "Which page of results, counting from zero")
	c.Flags().IntVar(&opts.PageSize, "page-size", 25, "How many messages per page")
	c.Flags().BoolVar(&opts.Unread, "unread", false, "Show only unread messages")
	c.Flags().BoolVar(&starred, "starred", false, "Show only starred messages")
	return c
}

func searchCmd() *cobra.Command {
	var opts mailsvc.SearchOptions
	c := &cobra.Command{
		Use:   "search",
		Short: "Search messages through Proton's index",
		Args:  cobra.NoArgs,
		RunE: kit.Run(nil, func(c *kit.Invocation) error {
			msgs, _, err := c.App.Mail.Search(c.Ctx, opts)
			if err != nil {
				return err
			}
			if len(msgs) == 0 {
				addressOnlyHint(c, opts.Keyword, opts.From, opts.To)
			}
			return kit.List(c, ui.TableSpec[mailsvc.Message]{
				Noun: "messages", Columns: messageColumns(),
				Total: ui.Unknown, Page: ui.Unpaged, Limit: opts.Limit,
			}, msgs, func(m mailsvc.Message) []string { return []string{m.ID} })
		}),
	}
	registerSearchFlags(c, &opts)
	return c
}

func getCmd() *cobra.Command {
	var bodyOnly, stripQuotes, includeInline bool
	format := bodyFormat()
	c := &cobra.Command{
		Use:   "get REF",
		Short: "Show one message, decrypted",
		Args:  cobra.ExactArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			shape, err := format.Value()
			if err != nil {
				return err
			}
			id, err := c.App.Mail.Resolve(c.Ctx, c.Args[0])
			if err != nil {
				return wrongTable(err, "get")
			}
			msg, err := c.App.Mail.Read(c.Ctx, id)
			if err != nil {
				return wrongTable(err, "get")
			}
			return kit.Read(c, ui.DocumentSpec{
				Object: msg,
				// Asking for html or raw means asking for the body as it is, so the
				// header block and attachment list would only get in the way.
				BodyOnly: bodyOnly || shape != "text",
				Parts:    []ui.Part{messagePart(msg, shape, stripQuotes, includeInline)},
			})
		}),
	}
	format.Register(c)
	c.Flags().BoolVar(&bodyOnly, "body-only", false, "Emit only the body, with no headers or attachment list")
	c.Flags().BoolVar(&stripQuotes, "strip-quotes", false, "Drop quoted reply blocks from the body")
	c.Flags().BoolVar(&includeInline, "include-inline", false, "List inline attachments too, such as signature graphics")
	return c
}

// messagePart turns a decrypted message into the header block, body and
// attachment table that make up one part of a document. A single message is one
// part; a thread is several.
func messagePart(msg *mailsvc.Full, shape string, stripQuotes, includeInline bool) ui.Part {
	body := msg.Body
	if stripQuotes {
		if mailtext.IsHTML(msg.MIMEType) {
			body = mailtext.StripHTMLQuotes(body)
		} else {
			body = mailtext.StripPlaintextQuotes(body)
		}
	}
	if shape == "text" && mailtext.IsHTML(msg.MIMEType) {
		body = mailtext.HTMLToText(body)
	}

	part := ui.Part{Header: messageHeader(msg), Body: body}
	visible := msg.Attachments
	if !includeInline {
		visible = mailsvc.FilterInline(visible)
	}
	if len(visible) > 0 {
		part.TrailerTitle = "Attachments"
		part.Trailer = func(u *ui.UI) error {
			return ui.Table(u, attachmentTableSpec(includeInline), visible)
		}
	}
	return part
}

// messageHeader is the block above a message body: the date and the full
// recipient lists.
func messageHeader(msg *mailsvc.Full) []ui.Field {
	fields := []ui.Field{
		{Label: "Subject", Value: msg.Subject},
		{Label: "From", Value: addressLine(msg.Sender)},
	}
	for _, group := range []struct {
		label string
		list  []map[string]any
	}{{"To", msg.ToList}, {"Cc", msg.CCList}, {"Bcc", msg.BCCList}} {
		for _, a := range group.list {
			fields = append(fields, ui.Field{Label: group.label, Value: addressLine(a)})
		}
	}
	return append(fields,
		ui.Field{Label: "Date", Value: units.Time(msg.Time)},
		ui.Field{Label: "Signature", Value: string(msg.Signature), Always: true},
		ui.Field{Label: "ID", Value: msg.ID, ID: true},
	)
}

// addressLine renders one recipient the way mail does: a display name with the
// address in angle brackets, or the bare address when there is no name.
func addressLine(a map[string]any) string {
	addr, _ := a["Address"].(string)
	name, _ := a["Name"].(string)
	if name == "" || name == addr {
		return addr
	}
	return name + " <" + addr + ">"
}

// ── organising ──

func moveCmd() *cobra.Command {
	var f filters
	var into string
	c := &cobra.Command{
		Use:   "move [REF...]",
		Short: "Move messages to a folder",
		Long: "Move messages to a folder.\n\n" +
			"A folder is somewhere a message lives, so moving takes it out of wherever it\n" +
			"was. To add a tag while leaving it in place, use `label` instead.",
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			dest, err := c.App.Mail.ResolveFolderTarget(c.Ctx, into)
			if err != nil {
				return err
			}
			sel, err := selectMessages(c, &f)
			if err != nil {
				return wrongTable(err, "move")
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Moved, Kind: "messages", Count: sel.Len(), IDs: sel.IDs,
				Detail: "to " + dest.Name, Preview: sel.Preview(),
			}, func() error {
				return c.App.Mail.Label(c.Ctx, sel.IDs, dest.ID)
			})
		}),
	}
	c.Flags().StringVar(&into, "into", "", "Destination folder, by name or ID")
	_ = c.MarkFlagRequired("into")
	registerFolderCompletion(c, "into")
	f.register(c)
	return c
}

func labelCmd() *cobra.Command {
	return labelVerb("label", "Attach a label to messages", ui.Labelled,
		func(c *kit.Invocation, ids []string, labelID string) error {
			return c.App.Mail.Label(c.Ctx, ids, labelID)
		})
}

func unlabelCmd() *cobra.Command {
	return labelVerb("unlabel", "Detach a label from messages", ui.Unlabelled,
		func(c *kit.Invocation, ids []string, labelID string) error {
			return c.App.Mail.Unlabel(c.Ctx, ids, labelID)
		})
}

// labelVerb builds `label` and `unlabel`.
//
// Attaching a label and moving a message are separate actions, as they are in the
// web client and in the API's own /label and /unlabel endpoints. Folding them
// together would mean `move` could take a label and report a move that never
// happened.
func labelVerb(use, short string, action ui.Action, apply func(*kit.Invocation, []string, string) error) *cobra.Command {
	var f filters
	var label string
	c := &cobra.Command{
		Use:   use + " [REF...]",
		Short: short,
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			target, err := c.App.Mail.ResolveLabelTarget(c.Ctx, label)
			if err != nil {
				return err
			}
			sel, err := selectMessages(c, &f)
			if err != nil {
				return wrongTable(err, use)
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: action, Kind: "messages", Count: sel.Len(), IDs: sel.IDs,
				Detail: quoted(target.Name), Preview: sel.Preview(),
			}, func() error {
				return apply(c, sel.IDs, target.ID)
			})
		}),
	}
	c.Flags().StringVar(&label, "label", "", "The label to attach or detach, by name or ID")
	_ = c.MarkFlagRequired("label")
	f.register(c)
	return c
}

func starCmd() *cobra.Command {
	return starVerb("star", "Star messages", ui.Starred,
		func(c *kit.Invocation, ids []string) error {
			return c.App.Mail.Label(c.Ctx, ids, mailsvc.StarredLabelID)
		})
}

func unstarCmd() *cobra.Command {
	return starVerb("unstar", "Remove the star from messages", ui.Unstarred,
		func(c *kit.Invocation, ids []string) error {
			return c.App.Mail.Unlabel(c.Ctx, ids, mailsvc.StarredLabelID)
		})
}

// starVerb builds `star` and `unstar`, which are `label` and `unlabel` with the
// one label Proton gives a button of its own.
func starVerb(use, short string, action ui.Action, apply func(*kit.Invocation, []string) error) *cobra.Command {
	var f filters
	c := &cobra.Command{
		Use:   use + " [REF...]",
		Short: short,
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			sel, err := selectMessages(c, &f)
			if err != nil {
				return wrongTable(err, use)
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: action, Kind: "messages", Count: sel.Len(), IDs: sel.IDs,
				Preview: sel.Preview(),
			}, func() error { return apply(c, sel.IDs) })
		}),
	}
	f.register(c)
	return c
}

// markCmd is a group with two real subcommands rather than a verb taking a verb
// as an argument. `mark read` therefore has its own help and completion, and no
// hand-written check that the word was one of two.
func markCmd() *cobra.Command {
	c := &cobra.Command{Use: "mark", Short: "Set whether messages count as read"}
	c.AddCommand(
		markVerb("read", "Mark messages as read", ui.MarkedRead,
			func(c *kit.Invocation, ids []string) error { return c.App.Mail.MarkRead(c.Ctx, ids) }),
		markVerb("unread", "Mark messages as unread", ui.MarkedUnread,
			func(c *kit.Invocation, ids []string) error { return c.App.Mail.MarkUnread(c.Ctx, ids) }),
	)
	return c
}

func markVerb(use, short string, action ui.Action, apply func(*kit.Invocation, []string) error) *cobra.Command {
	var f filters
	c := &cobra.Command{
		Use:   use + " [REF...]",
		Short: short,
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			sel, err := selectMessages(c, &f)
			if err != nil {
				return wrongTable(err, "mark "+use)
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: action, Kind: "messages", Count: sel.Len(), IDs: sel.IDs,
				Detail: "as " + use, Preview: sel.Preview(),
			}, func() error { return apply(c, sel.IDs) })
		}),
	}
	f.register(c)
	return c
}

func trashCmd() *cobra.Command {
	var f filters
	c := &cobra.Command{
		Use:   "trash [REF...]",
		Short: "Move messages to the trash",
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			sel, err := selectMessages(c, &f)
			if err != nil {
				return wrongTable(err, "trash")
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Trashed, Kind: "messages", Count: sel.Len(), IDs: sel.IDs,
				Detail: "to trash", Preview: sel.Preview(),
			}, func() error { return c.App.Mail.Trash(c.Ctx, sel.IDs) })
		}),
	}
	f.register(c)
	return c
}

func deleteCmd() *cobra.Command {
	var f filters
	c := &cobra.Command{
		Use:   "delete [REF...]",
		Short: "Delete messages permanently",
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			sel, err := selectMessages(c, &f)
			if err != nil {
				return wrongTable(err, "delete")
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Deleted, Kind: "messages", Count: sel.Len(), IDs: sel.IDs,
				Preview: sel.Preview(),
			}, func() error { return c.App.Mail.Delete(c.Ctx, sel.IDs) })
		}),
	}
	f.register(c)
	return c
}

func unscheduleCmd() *cobra.Command {
	var all bool
	c := &cobra.Command{
		Use:   "unschedule [REF...]",
		Short: "Cancel a scheduled send, returning the message to drafts",
		Long: "Cancel a scheduled send.\n\n" +
			"The message leaves the queue and returns to Drafts, keeping its ID - the same\n" +
			"thing the web client's \"Edit and reschedule\" does. To change the time, cancel\n" +
			"it and send again with --send-at.",
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			ids, rows, err := scheduled(c, all)
			if err != nil {
				return err
			}
			preview := func(u *ui.UI) error {
				return ui.Table(u, ui.TableSpec[mailsvc.Message]{
					Noun: "messages", Columns: messageColumns(),
					Total: ui.Unknown, Page: ui.Unpaged,
				}, rows)
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Unscheduled, Kind: "messages", Count: len(ids), IDs: ids,
				Detail: "and returned them to drafts", Preview: preview,
			}, func() error { return c.App.Mail.Unschedule(c.Ctx, ids) })
		}),
	}
	c.Flags().BoolVar(&all, "all", false, "Cancel every scheduled send")
	return c
}

// scheduled resolves what to unschedule. References resolve within the Scheduled
// folder only, so a subject can never reach something already sent.
func scheduled(c *kit.Invocation, all bool) ([]string, []mailsvc.Message, error) {
	if len(c.Args) == 0 && !all {
		return nil, nil, kit.Fail("Nothing selected.").
			Hint("pass a REF, or --all to cancel every scheduled send.")
	}
	var rows []mailsvc.Message
	for _, refArg := range c.Args {
		id, err := c.App.Mail.ResolveScheduled(c.Ctx, refArg)
		if err != nil {
			return nil, nil, err
		}
		rows = append(rows, mailsvc.Message{ID: id})
	}
	if all {
		msgs, _, err := c.App.Mail.List(c.Ctx, mailsvc.ListOptions{Folder: "scheduled", PageSize: 150})
		if err != nil {
			return nil, nil, err
		}
		rows = append(rows, msgs...)
	}
	ids := make([]string, 0, len(rows))
	for _, m := range rows {
		ids = append(ids, m.ID)
	}
	return kit.Dedupe(ids), rows, nil
}

// ── shared flag registration ──

// bodyFormat is how a message body should be rendered. Declaring it as an enum
// gives it one error wording and shell completion, which a hand-checked string
// never had.
func bodyFormat() *kit.Enum {
	return &kit.Enum{
		Name: "format", Usage: "How to render the body", Default: "text",
		Values: []string{"text", "html", "raw"},
	}
}

func registerFolder(c *cobra.Command, target *string, def string) {
	c.Flags().StringVar(target, "folder", def, "Folder or label to look in")
	registerFolderCompletion(c, "folder")
}

// registerFolderCompletion offers the built-in folder names. A custom folder is
// not offered because listing them would need a request, and completion that
// pauses to authenticate is worse than completion that is incomplete.
func registerFolderCompletion(c *cobra.Command, flag string) {
	_ = c.RegisterFlagCompletionFunc(flag,
		func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return mailsvc.SystemFolderNames(), cobra.ShellCompDirectiveNoFileComp
		})
}

func registerSearchFlags(c *cobra.Command, opts *mailsvc.SearchOptions) {
	fl := c.Flags()
	fl.StringVar(&opts.Keyword, "keyword", "", "Match text anywhere, including display names and bodies")
	fl.StringVar(&opts.From, "from", "", "Match the sender's address")
	fl.StringVar(&opts.To, "to", "", "Match a recipient's address")
	fl.StringVar(&opts.Subject, "subject", "", "Match text in the subject")
	fl.StringVar(&opts.After, "after", "", "Only messages after this date (YYYY-MM-DD)")
	fl.StringVar(&opts.Before, "before", "", "Only messages before this date (YYYY-MM-DD)")
	fl.IntVar(&opts.Limit, "limit", 25, "Most results to return")
	registerFolder(c, &opts.Folder, "all")
}

// addressOnlyHint explains an empty result that a different flag would have
// found, since --from matches the address alone.
func addressOnlyHint(c *kit.Invocation, keyword, from, to string) {
	if keyword != "" {
		return
	}
	term := from
	flag := "--from"
	if term == "" {
		term, flag = to, "--to"
	}
	if term == "" {
		return
	}
	c.UI().Hint(flag + " matches the address only. To search display names and bodies too, " +
		"use --keyword " + term + ".")
}

func quoted(s string) string { return strconv.Quote(s) }
