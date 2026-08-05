package cli

import (
	"fmt"
	"strings"

	"github.com/roman-16/proton-cli/internal/account/keys"
	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/render"
	mailsvc "github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/roman-16/proton-cli/internal/units"
	"github.com/roman-16/proton-cli/internal/view"
	"github.com/spf13/cobra"
)

// A draft is a message, so `mail messages read` and the organising verbs already
// work on one. This tree holds only what is specific to a message that has not
// gone out - creating, editing and sending it - plus a list, because REF
// resolution here is scoped to the Drafts folder so editing "Report" can never
// reach a message that has already been sent.

func draftsCmd() *cobra.Command {
	c := &cobra.Command{Use: "drafts", Short: "Write, edit and send drafts"}
	c.AddCommand(draftsListCmd(), draftsCreateCmd(), draftsEditCmd(), draftsSendCmd(), draftsDeleteCmd())
	return c
}

func draftsListCmd() *cobra.Command {
	var page, pageSize int
	c := &cobra.Command{
		Use: "list", Short: "List drafts",
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			msgs, total, err := c.App.Mail.DraftsList(c.Ctx, page, pageSize)
			if err != nil {
				return err
			}
			wrap := struct {
				Total    int               `json:"total"`
				Page     int               `json:"page"`
				PageSize int               `json:"page_size"`
				HasMore  bool              `json:"has_more"`
				Drafts   []mailsvc.Message `json:"drafts"`
			}{total, page, pageSize, (page+1)*pageSize < total, msgs}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[mailsvc.Message]{
				Columns:  draftColumns(),
				CacheIDs: func(m mailsvc.Message) []string { return []string{m.ID} },
				Footer:   func(n int) string { return render.PaginationFooter("drafts", total, page, pageSize, n) },
				JSON:     wrap,
			}, msgs)
		}),
	}
	c.Flags().IntVar(&page, "page", 0, "Page number (0-based)")
	c.Flags().IntVar(&pageSize, "page-size", 25, "Drafts per page")
	return c
}

// draftColumns shows recipients rather than senders: every draft is from you.
func draftColumns() []view.Column[mailsvc.Message] {
	return []view.Column[mailsvc.Message]{
		{Header: "ID", ID: true, Cell: func(m mailsvc.Message) string { return m.ID }},
		{Header: "SUBJECT", Cell: func(m mailsvc.Message) string { return m.Subject }},
		{Header: "SAVED", Cell: func(m mailsvc.Message) string { return units.Time(m.Time) }},
		{Header: "📎", Accent: true, Cell: func(m mailsvc.Message) string {
			if m.NumAttachments > 0 {
				return fmt.Sprintf("%d", m.NumAttachments)
			}
			return ""
		}},
	}
}

func draftsCreateCmd() *cobra.Command {
	var f composeFlags
	c := &cobra.Command{
		Use:   "create",
		Short: "Save a draft without sending it",
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			u, err := c.App.Unlock(c.Ctx)
			if err != nil {
				return err
			}
			content, err := f.content(c, u)
			if err != nil {
				return err
			}
			if c.dryRun("create a draft with subject %q", content.Subject) {
				return nil
			}
			return saveDraft(c, u, content, "message")
		}),
	}
	f.registerRecipients(c)
	f.registerBody(c)
	f.registerAttachments(c)
	f.registerIdentity(c)
	f.registerEML(c)
	return c
}

func draftsEditCmd() *cobra.Command {
	var f composeFlags
	c := &cobra.Command{
		Use:   "edit REF",
		Short: "Change a draft's recipients, subject, body or attachments",
		Long: "Change a draft. Only what you pass is replaced; everything else is kept.\n\n" +
			"--to, --cc and --bcc replace the whole list. --attach adds files and\n" +
			"--detach removes one by name or attachment ID.",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			u, err := c.App.Unlock(c.Ctx)
			if err != nil {
				return err
			}
			id, err := c.App.Mail.ResolveDraft(c.Ctx, c.Args[0])
			if err != nil {
				return err
			}
			draft, err := c.App.Mail.DraftLoad(c.Ctx, u, id)
			if err != nil {
				return err
			}
			content, err := f.applyTo(c, u, draft)
			if err != nil {
				return err
			}
			if c.dryRun("update draft %s", id) {
				return nil
			}
			for _, spec := range f.detach {
				attID, err := matchDraftAttachment(draft, spec)
				if err != nil {
					return err
				}
				if err := c.App.Mail.DraftDetach(c.Ctx, id, attID); err != nil {
					return err
				}
			}
			if _, err := c.App.Mail.DraftUpdate(c.Ctx, u, id, content); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Updated draft %s.", id))
			return nil
		}),
	}
	f.registerRecipients(c)
	f.registerBody(c)
	c.Flags().Lookup("html").Usage = "Switch the draft to text/html"
	c.Flags().BoolVar(&f.plain, "plain", false, "Switch the draft to text/plain")
	f.registerAttachments(c)
	c.Flags().StringArrayVar(&f.detach, "detach", nil, "Remove an attachment by name or ID (repeatable)")
	f.registerIdentity(c)
	return c
}

// applyTo overlays the flags the user passed onto a loaded draft, leaving
// everything untouched that was not mentioned.
func (f *composeFlags) applyTo(c *Invocation, u *keys.Unlocked, draft *mailsvc.Draft) (mailsvc.Content, error) {
	out := draft.Content
	if c.changed("to") {
		out.To = mailsvc.ParseRecipients(f.to)
	}
	if c.changed("cc") {
		out.CC = mailsvc.ParseRecipients(f.cc)
	}
	if c.changed("bcc") {
		out.BCC = mailsvc.ParseRecipients(f.bcc)
	}
	if c.changed("subject") {
		out.Subject = f.subject
	}
	if c.changed("body") {
		body, err := f.resolvedBody()
		if err != nil {
			return out, err
		}
		out.Body = body
	}
	switch {
	case c.changed("html"):
		out.HTML = f.html
	case c.changed("plain"):
		out.HTML = !f.plain
	}
	if c.changed("from") {
		sender, err := mailsvc.ResolveSender(u, mailsvc.SenderRequest{Explicit: f.from})
		if err != nil {
			return out, err
		}
		out.From = sender
	}
	atts, err := f.localAttachments()
	if err != nil {
		return out, err
	}
	out.Attach = atts
	return out, nil
}

// matchDraftAttachment resolves a --detach value against the draft's own
// attachments, by ID or by file name.
func matchDraftAttachment(draft *mailsvc.Draft, spec string) (string, error) {
	var names []string
	for _, a := range draft.AttachmentList() {
		if a.ID == spec || strings.EqualFold(a.Name, spec) {
			return a.ID, nil
		}
		names = append(names, a.Name)
	}
	if len(names) == 0 {
		return "", &errs.NotFound{Kind: "attachment on this draft", Ref: spec}
	}
	return "", errs.WithExit(3, fmt.Errorf("no attachment %q on this draft; it has: %s",
		spec, strings.Join(names, ", ")))
}

func draftsSendCmd() *cobra.Command {
	var d deliveryFlags
	c := &cobra.Command{
		Use:   "send REF",
		Short: "Send an existing draft",
		Long: "Send a draft as it stands. Its body already contains whatever signature it\n" +
			"was created with, so nothing is appended.",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			del, at, err := d.delivery()
			if err != nil {
				return err
			}
			u, err := c.App.Unlock(c.Ctx)
			if err != nil {
				return err
			}
			id, err := c.App.Mail.ResolveDraft(c.Ctx, c.Args[0])
			if err != nil {
				return err
			}
			draft, err := c.App.Mail.DraftLoad(c.Ctx, u, id)
			if err != nil {
				return err
			}
			if !draft.Content.HasRecipients() {
				return fmt.Errorf("draft %s has no recipients; add one with `mail drafts edit --to`", id)
			}
			if msg, dry := composeDryRun(c, draft.Content, "send draft "+id, at); dry {
				c.R().Info(msg)
				return nil
			}
			if err := withPinnedKeys(c, u, &del, draft.Content); err != nil {
				return err
			}
			if err := c.App.Mail.SendDraft(c.Ctx, draft, del); err != nil {
				return err
			}
			if at.IsZero() {
				c.R().ID(id, "Message sent.")
			} else {
				c.R().ID(id, fmt.Sprintf("Scheduled for %s", at.Format("2006-01-02 15:04:05 -07:00")))
			}
			return nil
		}),
	}
	d.register(c)
	return c
}

func draftsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete REF...",
		Short: "Delete drafts",
		Args:  cobra.MinimumNArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			ids := make([]string, 0, len(c.Args))
			for _, arg := range c.Args {
				id, err := c.App.Mail.ResolveDraft(c.Ctx, arg)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			ids = dedupe(ids)
			if c.dryRun("delete %d draft(s)", len(ids)) {
				return nil
			}
			if err := c.App.Mail.Delete(c.Ctx, ids); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Deleted %d draft(s).", len(ids)))
			return nil
		}),
	}
}
