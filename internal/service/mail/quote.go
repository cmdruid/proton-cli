package mail

import (
	"fmt"
	"strings"
	"time"

	"github.com/roman-16/proton-cli/internal/mailtext"
)

// Replying and forwarding derive a new Content from the message being answered:
// the subject gains a prefix, the recipients come from the parent, and the
// parent's body is quoted below the new text.
//
// The markers below are the ones the web client writes and the ones
// mailtext.StripHTMLQuotes / mailtext.StripPlaintextQuotes look for, so a quote
// proton produces is one proton's own --strip-quotes removes.

const (
	rePrefix = "Re:"
	fwPrefix = "Fw:"

	forwardedMessageMarker = "------- Forwarded Message -------"

	// classQuote wraps a quoted original. It is the first selector the
	// HTML quote stripper matches.
	classQuote = "protonmail_quote"

	// quoteDateFormat is deliberately locale-neutral: the CLI has no locale
	// machinery, and a stable format keeps quotes diffable and testable.
	quoteDateFormat = "Monday, 2 January 2006 at 15:04"
)

// formatSubject prepends prefix unless the subject already carries it, so
// answering a reply cannot produce "Re: Re: …".
func formatSubject(subject, prefix string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(subject)), strings.ToLower(prefix)) {
		return subject
	}
	if subject == "" {
		return prefix
	}
	return prefix + " " + subject
}

// subjectFor returns the subject a reply or forward of parent carries.
func subjectFor(action int, subject string) string {
	if action == ActionForward {
		return formatSubject(subject, fwPrefix)
	}
	return formatSubject(subject, rePrefix)
}

// replyRecipients derives who a reply is addressed to, mirroring the web
// client's reply/replyAll:
//
//   - a received message is answered to its Reply-To addresses
//   - a message we sent is answered to its own recipients, so replying to your
//     own mail continues the thread rather than mailing yourself
//   - reply-all additionally CCs everyone who was on it, minus your own address
//
// own is the address the parent arrived on, excluded from the CC list.
func replyRecipients(action int, parent replyContext, own string) (to, cc, bcc []Recipient) {
	if parent.Sent {
		to = parent.To
		if action == ActionReplyAll {
			cc, bcc = parent.CC, parent.BCC
		}
		return to, cc, bcc
	}
	to = parent.ReplyTos
	if len(to) == 0 {
		to = []Recipient{parent.Sender}
	}
	if action != ActionReplyAll {
		return to, nil, nil
	}
	seen := map[string]bool{canonicalAddress(own): true}
	for _, r := range to {
		seen[canonicalAddress(r.Address)] = true
	}
	for _, r := range append(append([]Recipient{}, parent.To...), parent.CC...) {
		key := canonicalAddress(r.Address)
		if seen[key] {
			continue
		}
		seen[key] = true
		cc = append(cc, r)
	}
	return to, cc, nil
}

// canonicalAddress normalises an address for comparison. Proton treats the local
// part of its own addresses case-insensitively and ignores dots and +tags, which
// is what stops reply-all from CCing you back.
func canonicalAddress(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return email
	}
	local, domain := email[:at], email[at:]
	if i := strings.Index(local, "+"); i >= 0 {
		local = local[:i]
	}
	local = strings.ReplaceAll(local, ".", "")
	return local + domain
}

// replyContext is everything about the parent message the derivation needs.
type replyContext struct {
	Sender      Recipient
	To, CC, BCC []Recipient
	ReplyTos    []Recipient
	Subject     string
	Body        string
	HTML        bool
	Time        int64
	Sent        bool
}

// quoteIntro renders the line (or block, for a forward) that introduces the
// quoted original. htmlEscaped switches the angle brackets around addresses to
// entities, as the HTML variant needs.
func quoteIntro(action int, p replyContext, html bool) string {
	date := time.Unix(p.Time, 0).Local().Format(quoteDateFormat)
	sender := formatQuoteRecipients([]Recipient{p.Sender}, html)
	if action != ActionForward {
		return fmt.Sprintf("On %s, %s wrote:", date, sender)
	}
	lines := []string{
		forwardedMessageMarker,
		"From: " + sender,
		"Date: On " + date,
		"Subject: " + p.Subject,
		"To: " + formatQuoteRecipients(p.To, html),
	}
	if len(p.CC) > 0 {
		lines = append(lines, "CC: "+formatQuoteRecipients(p.CC, html))
	}
	if html {
		return strings.Join(lines, "<br>")
	}
	return strings.Join(lines, "\n")
}

// formatQuoteRecipients renders `Name <addr>` per recipient, entity-escaping the
// brackets for HTML output the way the web client does.
//
// The angle brackets are never omitted, even for a recipient with no display
// name: the plaintext reply introducer is detected by looking for an address in
// brackets on a line ending in a colon, so dropping them would make our own
// quotes invisible to every client's quote stripper, including ours.
func formatQuoteRecipients(rs []Recipient, html bool) string {
	openBracket, closeBracket := "<", ">"
	if html {
		openBracket, closeBracket = "&lt;", "&gt;"
	}
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		bracketed := openBracket + r.Address + closeBracket
		if r.Name == "" {
			out = append(out, bracketed)
			continue
		}
		out = append(out, r.Name+" "+bracketed)
	}
	return strings.Join(out, ", ")
}

// quoteBlock renders the parent message as a quote in the target format. asHTML
// selects the output format, which may differ from the parent's: a plaintext
// reply to an HTML message flattens it, and the reverse escalates it.
func quoteBlock(action int, p replyContext, asHTML bool) string {
	if asHTML {
		body := p.Body
		if !p.HTML {
			body = mailtext.TextToHTML(body)
		}
		return fmt.Sprintf(
			"<div class=%q>%s<br><blockquote class=%q type=\"cite\">%s</blockquote><br></div>",
			classQuote, quoteIntro(action, p, true), classQuote, body)
	}
	body := p.Body
	if p.HTML {
		body = mailtext.HTMLToText(body)
	}
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for i, line := range lines {
		lines[i] = "> " + line
	}
	return quoteIntro(action, p, false) + "\n\n" + strings.Join(lines, "\n")
}
