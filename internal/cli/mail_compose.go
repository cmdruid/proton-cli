package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/roman-16/proton-cli/internal/account/keys"
	"github.com/roman-16/proton-cli/internal/ical"
	mailsvc "github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/roman-16/proton-cli/internal/units"
	"github.com/spf13/cobra"
)

// Every command that builds an outgoing message - send, reply, forward, and
// draft create/edit - shares these flags and this assembly, so the same body
// conventions, attachment handling and identity rules apply everywhere.

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
	c.Flags().StringArrayVar(&f.to, "to", nil, "Recipient (repeatable; accepts \"Name <addr>\")")
	c.Flags().StringArrayVar(&f.cc, "cc", nil, "CC recipient (repeatable)")
	c.Flags().StringArrayVar(&f.bcc, "bcc", nil, "BCC recipient (repeatable)")
}

func (f *composeFlags) registerBody(c *cobra.Command) {
	c.Flags().StringVar(&f.subject, "subject", "", "Subject")
	c.Flags().StringVar(&f.body, "body", "", "Message body (use - for stdin)")
	c.Flags().BoolVar(&f.html, "html", false, "Send the body as text/html instead of text/plain")
}

func (f *composeFlags) registerAttachments(c *cobra.Command) {
	c.Flags().StringArrayVar(&f.attach, "attach", nil, "File to attach (repeatable)")
	c.Flags().StringArrayVar(&f.attachInline, "attach-inline", nil,
		"Image embedded inline in the HTML body via Content-ID (repeatable; requires --html)")
}

func (f *composeFlags) registerIdentity(c *cobra.Command) {
	c.Flags().StringVar(&f.from, "from", "", "Address to send from (email or address ID; default: your primary)")
	c.Flags().BoolVar(&f.noSignature, "no-signature", false,
		"Do not append this address's signature or Proton's footer")
}

func (f *composeFlags) registerEML(c *cobra.Command) {
	c.Flags().StringVar(&f.eml, "eml", "",
		"Build the message from an RFC 822 (.eml) file; other flags override what it contains")
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
				return nil, fmt.Errorf("%s %s: %w", spec.label, path, err)
			}
			out = append(out, a)
		}
	}
	return out, nil
}

// resolvedBody reads the body, honouring "-" for stdin.
func (f *composeFlags) resolvedBody() (string, error) {
	return readTextArg(f.body, "--body")
}

// content assembles the Content for a fresh message: recipients, subject and
// body from the flags (or from --eml, which the flags then override), the
// resolved sending address, and the signature unless suppressed.
func (f *composeFlags) content(c *Invocation, u *keys.Unlocked) (mailsvc.Content, error) {
	body, err := f.resolvedBody()
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
		parsed, err := parseEMLFile(f.eml)
		if err != nil {
			return mailsvc.Content{}, err
		}
		mergeParsedEML(&out, parsed, c)
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

func parseEMLFile(path string) (*mailsvc.ParsedEML, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = fh.Close() }()
	parsed, err := mailsvc.ParseEML(fh)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return parsed, nil
}

// mergeParsedEML folds a parsed .eml into Content, letting any flag the user
// actually passed win over what the file says.
func mergeParsedEML(out *mailsvc.Content, parsed *mailsvc.ParsedEML, c *Invocation) {
	if !c.changed("to") {
		out.To = parsed.To
	}
	if !c.changed("cc") {
		out.CC = parsed.CC
	}
	if !c.changed("bcc") {
		out.BCC = parsed.BCC
	}
	if !c.changed("subject") {
		out.Subject = parsed.Subject
	}
	if !c.changed("body") {
		out.Body = parsed.Body
		if !c.changed("html") {
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
	c.Flags().StringVar(&f.sendAt, "send-at", "",
		"Schedule delivery (RFC3339, or YYYY-MM-DDTHH:MM in the local system timezone)")
	c.Flags().StringVar(&f.expires, "expires", "", "Self-destruct after DURATION (e.g. 7d, 24h)")
	c.Flags().StringVar(&f.eoPassword, "eo-password", "",
		"Password-protect the message for non-Proton recipients (Encrypted Outside; defaults to a 28-day expiry)")
	c.Flags().StringVar(&f.eoPasswordHint, "eo-password-hint", "",
		"Optional hint shown to Encrypted Outside recipients")
}

// delivery parses the flags into a Delivery, also returning the resolved
// schedule so callers can echo it back.
func (f *deliveryFlags) delivery() (mailsvc.Delivery, time.Time, error) {
	var del mailsvc.Delivery
	var at time.Time
	if f.sendAt != "" {
		t, err := ical.ParseTime(f.sendAt)
		if err != nil {
			return del, at, fmt.Errorf("invalid --send-at: %w", err)
		}
		at, del.At = t, t.Unix()
	}
	if f.expires != "" {
		d, err := units.ParseDuration(f.expires)
		if err != nil {
			return del, at, fmt.Errorf("invalid --expires: %w", err)
		}
		del.ExpiresInSeconds = int(d.Seconds())
	}
	del.EOPassword, del.EOPasswordHint = f.eoPassword, f.eoPasswordHint
	return del, at, nil
}

// withPinnedKeys consults Contacts for each recipient's pinned encryption keys.
// A pinned key means the message is encrypted to the key the user trusts rather
// than whatever the server hands back.
func withPinnedKeys(c *Invocation, u *keys.Unlocked, del *mailsvc.Delivery, content mailsvc.Content) error {
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

// deliver sends content, reporting the schedule when one was set. It is the
// shared tail of send, reply and forward.
func deliver(c *Invocation, u *keys.Unlocked, content mailsvc.Content, del mailsvc.Delivery, at time.Time) error {
	if err := withPinnedKeys(c, u, &del, content); err != nil {
		return err
	}
	id, err := c.App.Mail.Send(c.Ctx, u, content, del)
	if err != nil {
		return err
	}
	if at.IsZero() {
		c.R().ID(id, "Message sent.")
		return nil
	}
	c.R().ID(id, fmt.Sprintf("Scheduled for %s", at.Format("2006-01-02 15:04:05 -07:00")))
	return nil
}

// saveDraft stores content without sending, which is what --draft does on reply
// and forward.
func saveDraft(c *Invocation, u *keys.Unlocked, content mailsvc.Content, what string) error {
	d, err := c.App.Mail.DraftCreate(c.Ctx, u, content)
	if err != nil {
		return err
	}
	c.R().ID(d.ID, fmt.Sprintf("Saved %s as a draft.", what))
	return nil
}
