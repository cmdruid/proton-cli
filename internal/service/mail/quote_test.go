package mail

import (
	"strings"
	"testing"
	"time"

	"github.com/roman-16/proton-cli/internal/render"
)

func TestFormatSubjectAddsPrefixOnlyOnce(t *testing.T) {
	tests := []struct{ in, prefix, want string }{
		{"Invoice", rePrefix, "Re: Invoice"},
		{"Re: Invoice", rePrefix, "Re: Invoice"},
		{"re: Invoice", rePrefix, "re: Invoice"},
		{"RE: Invoice", rePrefix, "RE: Invoice"},
		{"Invoice", fwPrefix, "Fw: Invoice"},
		{"Fw: Invoice", fwPrefix, "Fw: Invoice"},
		{"", rePrefix, "Re:"},
		// A reply to a forward still gains its own prefix.
		{"Fw: Invoice", rePrefix, "Re: Fw: Invoice"},
	}
	for _, tt := range tests {
		if got := formatSubject(tt.in, tt.prefix); got != tt.want {
			t.Errorf("formatSubject(%q, %q) = %q, want %q", tt.in, tt.prefix, got, tt.want)
		}
	}
}

func TestSubjectForPicksThePrefixPerAction(t *testing.T) {
	if got := subjectFor(ActionForward, "Report"); got != "Fw: Report" {
		t.Errorf("forward subject = %q", got)
	}
	for _, action := range []int{ActionReply, ActionReplyAll} {
		if got := subjectFor(action, "Report"); got != "Re: Report" {
			t.Errorf("reply subject = %q", got)
		}
	}
}

func receivedParent() replyContext {
	return replyContext{
		Sender:   Recipient{Name: "Alice", Address: "alice@example.com"},
		To:       []Recipient{{Address: "me@proton.me"}, {Address: "carol@example.com"}},
		CC:       []Recipient{{Address: "dave@example.com"}},
		ReplyTos: []Recipient{{Address: "alice@example.com"}},
		Subject:  "Invoice",
		Body:     "the original body",
		Time:     1_700_000_000,
	}
}

func addresses(rs []Recipient) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Address)
	}
	return out
}

func TestReplyGoesToTheReplyToAddress(t *testing.T) {
	to, cc, bcc := replyRecipients(ActionReply, receivedParent(), "me@proton.me")
	if got := addresses(to); len(got) != 1 || got[0] != "alice@example.com" {
		t.Errorf("to = %v, want the Reply-To address only", got)
	}
	if len(cc) != 0 || len(bcc) != 0 {
		t.Errorf("a plain reply must not CC anyone; got cc=%v bcc=%v", cc, bcc)
	}
}

func TestReplyFallsBackToTheSenderWithoutReplyTo(t *testing.T) {
	p := receivedParent()
	p.ReplyTos = nil
	to, _, _ := replyRecipients(ActionReply, p, "me@proton.me")
	if got := addresses(to); len(got) != 1 || got[0] != "alice@example.com" {
		t.Errorf("to = %v, want the sender", got)
	}
}

func TestReplyAllCCsEveryoneButYou(t *testing.T) {
	to, cc, _ := replyRecipients(ActionReplyAll, receivedParent(), "me@proton.me")
	if got := addresses(to); len(got) != 1 || got[0] != "alice@example.com" {
		t.Errorf("to = %v", got)
	}
	got := addresses(cc)
	want := []string{"carol@example.com", "dave@example.com"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("cc = %v, want %v (you and the Reply-To must be excluded)", got, want)
	}
}

// Proton ignores dots and +tags in the local part of its own addresses, so
// reply-all must not CC you back through an alias of your own address.
func TestReplyAllExcludesAliasesOfYourOwnAddress(t *testing.T) {
	p := receivedParent()
	p.To = []Recipient{{Address: "M.E+newsletter@proton.me"}, {Address: "carol@example.com"}}
	_, cc, _ := replyRecipients(ActionReplyAll, p, "me@proton.me")
	for _, r := range cc {
		if strings.Contains(r.Address, "proton.me") {
			t.Errorf("reply-all CC'd your own address back: %v", addresses(cc))
		}
	}
}

func TestReplyToYourOwnSentMailAddressesItsRecipients(t *testing.T) {
	p := receivedParent()
	p.Sent = true
	to, cc, _ := replyRecipients(ActionReply, p, "me@proton.me")
	if got := addresses(to); strings.Join(got, ",") != "me@proton.me,carol@example.com" {
		t.Errorf("to = %v, want the original recipients", got)
	}
	if len(cc) != 0 {
		t.Errorf("a plain reply to sent mail must not CC; got %v", addresses(cc))
	}

	_, cc, _ = replyRecipients(ActionReplyAll, p, "me@proton.me")
	if got := addresses(cc); strings.Join(got, ",") != "dave@example.com" {
		t.Errorf("reply-all cc = %v, want the original CC list", got)
	}
}

func TestCanonicalAddress(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Me@Proton.me", "me@proton.me"},
		{"m.e@proton.me", "me@proton.me"},
		{"me+tag@proton.me", "me@proton.me"},
		{"M.E+tag@Proton.me", "me@proton.me"},
		{"nodomain", "nodomain"},
	}
	for _, tt := range tests {
		if got := canonicalAddress(tt.in); got != tt.want {
			t.Errorf("canonicalAddress(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPlaintextReplyQuoteIsStrippableByOurOwnStripper(t *testing.T) {
	p := receivedParent()
	quote := quoteBlock(ActionReply, p, false)

	if !strings.Contains(quote, "wrote:") {
		t.Errorf("quote is missing the reply introducer:\n%s", quote)
	}
	if !strings.Contains(quote, "<alice@example.com>") {
		t.Errorf("quote is missing the sender address in angle brackets:\n%s", quote)
	}
	if !strings.Contains(quote, "> the original body") {
		t.Errorf("quote does not prefix the original with '> ':\n%s", quote)
	}

	// The whole point of matching the web client's markers: a body we compose is
	// one our own --strip-quotes removes.
	body := "My answer.\n\n" + quote
	if got := render.StripPlaintextQuotes(body); strings.Contains(got, "the original body") {
		t.Errorf("StripPlaintextQuotes left our own quote behind:\n%s", got)
	} else if !strings.Contains(got, "My answer.") {
		t.Errorf("StripPlaintextQuotes removed the new text too:\n%s", got)
	}
}

func TestHTMLReplyQuoteIsStrippableByOurOwnStripper(t *testing.T) {
	p := receivedParent()
	p.HTML = true
	p.Body = "<p>the original body</p>"
	quote := quoteBlock(ActionReply, p, true)

	if !strings.Contains(quote, `class="`+classQuote+`"`) {
		t.Errorf("quote is missing the %s class:\n%s", classQuote, quote)
	}
	if !strings.Contains(quote, `type="cite"`) {
		t.Errorf("quote is missing the cite blockquote:\n%s", quote)
	}
	if !strings.Contains(quote, "&lt;alice@example.com&gt;") {
		t.Errorf("HTML quote must escape the address brackets:\n%s", quote)
	}

	body := "<p>My answer.</p>" + quote
	if got := render.StripHTMLQuotes(body); strings.Contains(got, "the original body") {
		t.Errorf("StripHTMLQuotes left our own quote behind:\n%s", got)
	} else if !strings.Contains(got, "My answer.") {
		t.Errorf("StripHTMLQuotes removed the new text too:\n%s", got)
	}
}

func TestForwardQuoteCarriesTheOriginalHeaders(t *testing.T) {
	p := receivedParent()
	quote := quoteBlock(ActionForward, p, false)
	for _, want := range []string{
		forwardedMessageMarker,
		"From: Alice <alice@example.com>",
		"Subject: Invoice",
		"To: <me@proton.me>, <carol@example.com>",
		"CC: <dave@example.com>",
	} {
		if !strings.Contains(quote, want) {
			t.Errorf("forward quote is missing %q:\n%s", want, quote)
		}
	}
	if got := render.StripPlaintextQuotes("FYI.\n\n" + quote); strings.Contains(got, "the original body") {
		t.Errorf("our own forward marker was not stripped:\n%s", got)
	}
}

func TestForwardQuoteOmitsAnEmptyCC(t *testing.T) {
	p := receivedParent()
	p.CC = nil
	if got := quoteBlock(ActionForward, p, false); strings.Contains(got, "CC:") {
		t.Errorf("forward quote should omit CC when there is none:\n%s", got)
	}
}

func TestQuoteConvertsBetweenFormats(t *testing.T) {
	// A plaintext reply to an HTML message flattens the original.
	html := receivedParent()
	html.HTML = true
	html.Body = "<p>hello <b>world</b></p>"
	if got := quoteBlock(ActionReply, html, false); strings.Contains(got, "<b>") {
		t.Errorf("plaintext quote of an HTML parent must be flattened:\n%s", got)
	}

	// An HTML reply to a plaintext message escalates it, escaping markup.
	plain := receivedParent()
	plain.Body = "a < b & c"
	got := quoteBlock(ActionReply, plain, true)
	if !strings.Contains(got, "a &lt; b &amp; c") {
		t.Errorf("HTML quote of a plaintext parent must escape it:\n%s", got)
	}
}

func TestQuoteIntroUsesTheMessageTime(t *testing.T) {
	p := receivedParent()
	want := time.Unix(p.Time, 0).Local().Format(quoteDateFormat)
	if got := quoteIntro(ActionReply, p, false); !strings.Contains(got, want) {
		t.Errorf("intro %q does not carry the message date %q", got, want)
	}
}
