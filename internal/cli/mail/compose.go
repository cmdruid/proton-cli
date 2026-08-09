package mail

import (
	"os"
	"time"

	"github.com/roman-16/proton-cli/internal/account/keys"
	"github.com/roman-16/proton-cli/internal/cli/kit"
	"github.com/roman-16/proton-cli/internal/ical"
	mailsvc "github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/roman-16/proton-cli/internal/units"
	"github.com/spf13/cobra"
)

// Every command that builds an outgoing message - send, reply, forward, and draft
// create and update - shares these flags and this assembly, so the same body
// conventions, attachment handling and identity rules apply to all of them.

type composeFlags struct {
	to, cc, bcc  []string
	subject      string
	body         string
	html         bool
	plain        bool
	attach       []string
	attachInline []string
	detach       []string
	from         string
	noSignature  bool
	eml          string
}

func (f *composeFlags) registerRecipients(c *cobra.Command) {
	fl := c.Flags()
	fl.StringArrayVar(&f.to, "to", nil, `Recipient (repeatable; accepts "Name <addr>")`)
	fl.StringArrayVar(&f.cc, "cc", nil, "Carbon-copy recipient (repeatable)")
	fl.StringArrayVar(&f.bcc, "bcc", nil, "Blind-carbon-copy recipient (repeatable)")
}

func (f *composeFlags) registerBody(c *cobra.Command) {
	fl := c.Flags()
	fl.StringVar(&f.subject, "subject", "", "Subject line")
	fl.StringVar(&f.body, "body", "", "Message body (- reads stdin)")
	fl.BoolVar(&f.html, "html", false, "Treat the body as HTML rather than plain text")
}

func (f *composeFlags) registerAttachments(c *cobra.Command) {
	fl := c.Flags()
	fl.StringArrayVar(&f.attach, "attach", nil, "File to attach (repeatable)")
	fl.StringArrayVar(&f.attachInline, "attach-inline", nil,
		"Image to embed in the HTML body by Content-ID (repeatable; needs --html)")
}

func (f *composeFlags) registerIdentity(c *cobra.Command) {
	fl := c.Flags()
	fl.StringVar(&f.from, "from", "", "Address to send from, by email or ID (default: your primary)")
	fl.BoolVar(&f.noSignature, "no-signature", false,
		"Leave out this address's signature and Proton's footer")
}

func (f *composeFlags) registerEML(c *cobra.Command) {
	c.Flags().StringVar(&f.eml, "eml", "",
		"Build the message from an RFC 822 file; other flags override what it says")
}

// localAttachments reads every --attach and --attach-inline path from disk.
func (f *composeFlags) localAttachments() ([]mailsvc.LocalAttachment, error) {
	out := make([]mailsvc.LocalAttachment, 0, len(f.attach)+len(f.attachInline))
	for _, spec := range []struct {
		paths  []string
		inline bool
		label  string
	}{{f.attach, false, "attachment"}, {f.attachInline, true, "inline attachment"}} {
		for _, path := range spec.paths {
			a, err := mailsvc.ReadLocalAttachment(path, spec.inline)
			if err != nil {
				return nil, kit.Fail("%s %s: %v", spec.label, path, err)
			}
			out = append(out, a)
		}
	}
	return out, nil
}

// resolvedBody reads the body, honouring "-" for stdin.
func (f *composeFlags) resolvedBody(c *kit.Invocation) (string, error) {
	return kit.ReadTextArg(c, f.body, "--body")
}

// content assembles a fresh message: recipients, subject and body from the flags
// (or from --eml, which the flags then override), the resolved sending address,
// and the signature unless suppressed.
func (f *composeFlags) content(c *kit.Invocation, u *keys.Unlocked) (mailsvc.Content, error) {
	body, err := f.resolvedBody(c)
	if err != nil {
		return mailsvc.Content{}, err
	}
	atts, err := f.localAttachments()
	if err != nil {
		return mailsvc.Content{}, err
	}
	out := mailsvc.Content{
		To:      mailsvc.ParseRecipients(f.to),
		CC:      mailsvc.ParseRecipients(f.cc),
		BCC:     mailsvc.ParseRecipients(f.bcc),
		Subject: f.subject,
		Body:    body,
		HTML:    f.html,
		Attach:  atts,
	}

	if f.eml != "" {
		parsed, err := parseEML(f.eml)
		if err != nil {
			return mailsvc.Content{}, err
		}
		f.mergeEML(&out, parsed, c)
	}

	sender, err := mailsvc.ResolveSender(u, mailsvc.SenderRequest{Explicit: f.from})
	if err != nil {
		return mailsvc.Content{}, err
	}
	out.From = sender

	// An .eml is already a finished message; appending to it would corrupt what
	// the caller handed over.
	if f.eml == "" && !f.noSignature {
		sig, err := c.App.Mail.SignatureBlock(c.Ctx, sender)
		if err != nil {
			return mailsvc.Content{}, err
		}
		out.AppendSignature(sig, "")
	}
	return out, nil
}

// applyTo overlays the flags the user actually passed onto a loaded draft, leaving
// everything unmentioned untouched.
func (f *composeFlags) applyTo(c *kit.Invocation, u *keys.Unlocked, draft *mailsvc.Draft) (mailsvc.Content, error) {
	out := draft.Content
	if c.Changed("to") {
		out.To = mailsvc.ParseRecipients(f.to)
	}
	if c.Changed("cc") {
		out.CC = mailsvc.ParseRecipients(f.cc)
	}
	if c.Changed("bcc") {
		out.BCC = mailsvc.ParseRecipients(f.bcc)
	}
	if c.Changed("subject") {
		out.Subject = f.subject
	}
	if c.Changed("body") {
		body, err := f.resolvedBody(c)
		if err != nil {
			return out, err
		}
		out.Body = body
	}
	switch {
	case c.Changed("html"):
		out.HTML = f.html
	case c.Changed("plain"):
		out.HTML = !f.plain
	}
	if c.Changed("from") {
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

func parseEML(path string) (*mailsvc.ParsedEML, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = fh.Close() }()
	parsed, err := mailsvc.ParseEML(fh)
	if err != nil {
		return nil, kit.Fail("%s: %v", path, err)
	}
	return parsed, nil
}

// mergeEML folds a parsed .eml into Content, letting any flag the user actually
// passed win over what the file says.
func (f *composeFlags) mergeEML(out *mailsvc.Content, parsed *mailsvc.ParsedEML, c *kit.Invocation) {
	if !c.Changed("to") {
		out.To = parsed.To
	}
	if !c.Changed("cc") {
		out.CC = parsed.CC
	}
	if !c.Changed("bcc") {
		out.BCC = parsed.BCC
	}
	if !c.Changed("subject") {
		out.Subject = parsed.Subject
	}
	if !c.Changed("body") {
		out.Body = parsed.Body
		if !c.Changed("html") {
			out.HTML = parsed.HTML
		}
	}
	out.Attach = append(out.Attach, parsed.Attachments...)
}

// ── delivery ──

// deliveryFlags are the send-time options: when a message goes out, when it
// self-destructs, and how recipients outside Proton receive it.
type deliveryFlags struct {
	sendAt         string
	expires        string
	eoPassword     string
	eoPasswordHint string
}

func (f *deliveryFlags) register(c *cobra.Command) {
	fl := c.Flags()
	fl.StringVar(&f.sendAt, "send-at", "",
		"Schedule delivery (RFC 3339, or YYYY-MM-DDTHH:MM in the system timezone)")
	fl.StringVar(&f.expires, "expires", "", "Self-destruct after DURATION (e.g. 7d, 24h)")
	fl.StringVar(&f.eoPassword, "eo-password", "",
		"Password-protect the message for recipients outside Proton (expires after 28 days)")
	fl.StringVar(&f.eoPasswordHint, "eo-password-hint", "",
		"Hint shown to password-protected recipients")
}

// delivery parses the flags, also returning the resolved schedule so a caller can
// echo the time back.
func (f *deliveryFlags) delivery() (mailsvc.Delivery, time.Time, error) {
	var del mailsvc.Delivery
	var at time.Time
	if f.sendAt != "" {
		t, err := ical.ParseTime(f.sendAt)
		if err != nil {
			return del, at, kit.Fail("--send-at: %v", err)
		}
		at, del.At = t, t.Unix()
	}
	if f.expires != "" {
		d, err := units.ParseDuration(f.expires)
		if err != nil {
			return del, at, kit.Fail("--expires: %v", err)
		}
		del.ExpiresInSeconds = int(d.Seconds())
	}
	del.EOPassword, del.EOPasswordHint = f.eoPassword, f.eoPasswordHint
	return del, at, nil
}

// withPinnedKeys consults Contacts for each recipient's pinned keys. A pinned key
// means the message is encrypted to the key the user trusts rather than to
// whatever the server hands back.
func withPinnedKeys(c *kit.Invocation, u *keys.Unlocked, del *mailsvc.Delivery, content mailsvc.Content) error {
	for _, email := range content.RecipientAddresses() {
		pin, err := c.App.Contacts.PinnedKeysFor(c.Ctx, u, email)
		if err != nil {
			return err
		}
		if pin == nil {
			continue
		}
		if del.PinnedKeys == nil {
			del.PinnedKeys = map[string]*mailsvc.PinnedRecipient{}
		}
		del.PinnedKeys[email] = &mailsvc.PinnedRecipient{
			ArmoredKeys:       pin.ArmoredKeys,
			Encrypt:           pin.Encrypt,
			Sign:              pin.Sign,
			Scheme:            pin.Scheme,
			SignatureVerified: pin.SignatureVerified,
		}
	}
	return nil
}

// deliver sends content, reporting the schedule when one was set. It is the shared
// tail of send, reply and forward.
func deliver(c *kit.Invocation, u *keys.Unlocked, content mailsvc.Content, del mailsvc.Delivery, at time.Time) error {
	action, detail := ui.Sent, ""
	if !at.IsZero() {
		action = ui.Scheduled
		detail = "for " + at.Format("2006-01-02 15:04 -07:00")
	}
	return kit.Create(c, ui.ResultSpec{
		Action: action, Kind: "messages",
		Name: content.Subject, Detail: detail,
	}, func() (string, error) {
		if err := withPinnedKeys(c, u, &del, content); err != nil {
			return "", err
		}
		return c.App.Mail.Send(c.Ctx, u, content, del)
	})
}

// saveDraft stores content without sending, which is what --draft does on reply
// and forward.
func saveDraft(c *kit.Invocation, u *keys.Unlocked, content mailsvc.Content) error {
	return kit.Create(c, ui.ResultSpec{
		Action: ui.Saved, Kind: "drafts", Name: content.Subject, Detail: "as a draft",
	}, func() (string, error) {
		d, err := c.App.Mail.DraftCreate(c.Ctx, u, content)
		if err != nil {
			return "", err
		}
		return d.ID, nil
	})
}

// ── send, reply, forward ──

func sendCmd() *cobra.Command {
	var f composeFlags
	var d deliveryFlags
	c := &cobra.Command{
		Use:   "send",
		Short: "Compose and send a message",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepAuth}, func(c *kit.Invocation) error {
			if f.eml == "" && f.subject == "" {
				return kit.Fail("A subject is required.").Hint("--subject \"Quarterly numbers\"")
			}
			if f.eml == "" && f.body == "" {
				return kit.Fail("A body is required.").Hint("--body \"…\", or --body - to read stdin.")
			}
			del, at, err := d.delivery()
			if err != nil {
				return err
			}
			u, err := c.App.Unlock(c.Ctx)
			if err != nil {
				return err
			}
			content, err := f.content(c, u)
			if err != nil {
				return err
			}
			if !content.HasRecipients() {
				return kit.Fail("At least one recipient is required.").Hint("--to, --cc or --bcc")
			}
			return deliver(c, u, content, del, at)
		}),
	}
	f.registerRecipients(c)
	f.registerBody(c)
	f.registerAttachments(c)
	f.registerIdentity(c)
	f.registerEML(c)
	d.register(c)
	return c
}

func replyCmd() *cobra.Command {
	return answerCmd("reply", "Reply to a message",
		"Reply to a message.\n\n"+
			"The original is quoted below your text, the subject gains \"Re:\", and the reply\n"+
			"leaves from the address the original arrived on.\n\n"+
			"--all includes everyone who was on the message. --draft stops before sending,\n"+
			"so you can edit it with `mail drafts update`.", false)
}

func forwardCmd() *cobra.Command {
	return answerCmd("forward", "Forward a message",
		"Forward a message.\n\n"+
			"The original is quoted below your text with its own headers, the subject gains\n"+
			"\"Fw:\", and its attachments come along without being re-uploaded.", true)
}

// answerCmd builds reply and forward, which differ only in how they derive
// recipients and whether the original's attachments come along.
func answerCmd(use, short, long string, forward bool) *cobra.Command {
	var f composeFlags
	var d deliveryFlags
	var replyAll, noQuote, noAttachments, asDraft bool
	c := &cobra.Command{
		Use:   use + " REF",
		Short: short,
		Long:  long,
		Args:  cobra.ExactArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand}, func(c *kit.Invocation) error {
			del, at, err := d.delivery()
			if err != nil {
				return err
			}
			u, err := c.App.Unlock(c.Ctx)
			if err != nil {
				return err
			}
			id, err := c.App.Mail.Resolve(c.Ctx, c.Args[0])
			if err != nil {
				return wrongTable(err, use)
			}
			content, err := buildAnswer(c, u, id, &f, answerAction(forward, replyAll), noQuote, noAttachments)
			if err != nil {
				return wrongTable(err, use)
			}
			if asDraft {
				return saveDraft(c, u, content)
			}
			return deliver(c, u, content, del, at)
		}),
	}
	f.registerRecipients(c)
	c.Flags().StringVar(&f.body, "body", "", "Your text, placed above the quoted original (- reads stdin)")
	c.Flags().BoolVar(&f.html, "html", false, "Compose in HTML (default: match the original)")
	f.registerAttachments(c)
	f.registerIdentity(c)
	d.register(c)
	c.Flags().BoolVar(&noQuote, "no-quote", false, "Do not quote the original message")
	c.Flags().BoolVar(&asDraft, "draft", false, "Save as a draft instead of sending")
	if forward {
		c.Flags().BoolVar(&noAttachments, "no-attachments", false, "Leave the original's attachments behind")
	} else {
		c.Flags().BoolVar(&replyAll, "all", false, "Reply to everyone who was on the message")
	}
	return c
}

func buildAnswer(c *kit.Invocation, u *keys.Unlocked, id string, f *composeFlags,
	action int, noQuote, noAttachments bool) (mailsvc.Content, error) {
	body, err := f.resolvedBody(c)
	if err != nil {
		return mailsvc.Content{}, err
	}
	atts, err := f.localAttachments()
	if err != nil {
		return mailsvc.Content{}, err
	}
	spec := mailsvc.AnswerSpec{
		Action: action, Body: body,
		To:            mailsvc.ParseRecipients(f.to),
		CC:            mailsvc.ParseRecipients(f.cc),
		BCC:           mailsvc.ParseRecipients(f.bcc),
		From:          f.from,
		Attach:        atts,
		NoQuote:       noQuote,
		NoAttachments: noAttachments,
		NoSignature:   f.noSignature,
	}
	if c.Changed("html") {
		spec.HTML = &f.html
	}
	return c.App.Mail.Answer(c.Ctx, u, id, spec)
}

func answerAction(forward, replyAll bool) int {
	switch {
	case forward:
		return mailsvc.ActionForward
	case replyAll:
		return mailsvc.ActionReplyAll
	}
	return mailsvc.ActionReply
}
