package mailtext

import (
	"strings"
	"testing"
)

func TestStripHTMLQuotesProtonmailQuote(t *testing.T) {
	in := `<p>Hi Bob, here's the proposal.</p><blockquote class="protonmail_quote">old reply chain</blockquote>`
	got := StripHTMLQuotes(in)
	if strings.Contains(got, "old reply chain") {
		t.Errorf("protonmail_quote should be stripped, got: %q", got)
	}
	if !strings.Contains(got, "Hi Bob") {
		t.Errorf("new content should be preserved, got: %q", got)
	}
}

func TestStripHTMLQuotesGmailQuote(t *testing.T) {
	in := `<div>Hi.</div><div class="gmail_quote">old</div>`
	got := StripHTMLQuotes(in)
	if strings.Contains(got, ">old<") {
		t.Errorf("gmail_quote should be stripped, got: %q", got)
	}
}

func TestStripHTMLQuotesBlockquoteCite(t *testing.T) {
	in := `<p>Reply.</p><blockquote type="cite">previous email</blockquote>`
	got := StripHTMLQuotes(in)
	if strings.Contains(got, "previous email") {
		t.Errorf("blockquote[type=cite] should be stripped, got: %q", got)
	}
}

func TestStripHTMLQuotesIDSelectors(t *testing.T) {
	cases := []struct {
		name string
		body string
		drop string
	}{
		{"divRplyFwdMsg", `<p>Reply.</p><div id="divRplyFwdMsg">forwarded thing</div>`, "forwarded thing"},
		{"isReplyContent", `<p>Reply.</p><blockquote id="isReplyContent">stuff</blockquote>`, "stuff"},
		{"oriMsgHtmlSeperator", `<p>Reply.</p><blockquote id="oriMsgHtmlSeperator">old</blockquote>`, "old"},
		{"moz-cite-prefix", `<p>Reply.</p><div class="moz-cite-prefix">moz</div>`, "moz"},
		{"name=quote", `<p>Reply.</p><div name="quote">gmx old</div>`, "gmx old"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := StripHTMLQuotes(tc.body)
			if strings.Contains(got, tc.drop) {
				t.Errorf("%s should be stripped, got: %q", tc.name, got)
			}
			if !strings.Contains(got, "Reply.") {
				t.Errorf("new content should be preserved, got: %q", got)
			}
		})
	}
}

func TestStripHTMLQuotesAfterContentDisqualifies(t *testing.T) {
	// gmail_quote is followed by significant text - must NOT be stripped.
	in := `<p>Hi.</p><div class="gmail_quote">old</div><p>real content after the supposed quote</p>`
	got := StripHTMLQuotes(in)
	if !strings.Contains(got, "old") {
		t.Errorf("quote with significant content after should NOT be stripped, got: %q", got)
	}
	if !strings.Contains(got, "real content") {
		t.Errorf("after-quote content should be preserved, got: %q", got)
	}
}

func TestStripHTMLQuotesUnchangedNoQuote(t *testing.T) {
	in := `<p>Just a reply, no quote at all.</p>`
	got := StripHTMLQuotes(in)
	if !strings.Contains(got, "Just a reply") {
		t.Errorf("body without quote should be returned: got %q", got)
	}
}

func TestStripHTMLQuotesOriginalMessageMarker(t *testing.T) {
	// Text-content fallback when no selector matches. The marker text must
	// be inside an element that ALSO wraps the rest of the quote, since
	// the parent of the marker is what gets stripped.
	in := `<div>New content here.</div><div>------- Original Message -------<p>previous body</p></div>`
	got := StripHTMLQuotes(in)
	if strings.Contains(got, "previous body") {
		t.Errorf("text after Original Message marker should be stripped, got: %q", got)
	}
	if !strings.Contains(got, "New content here.") {
		t.Errorf("new content should be preserved, got: %q", got)
	}
}

func TestStripHTMLQuotesEmpty(t *testing.T) {
	if got := StripHTMLQuotes(""); got != "" {
		t.Errorf("empty input → %q, want empty", got)
	}
}

func TestStripPlaintextQuotesForward(t *testing.T) {
	in := "My new note.\n\n------- Forwarded Message -------\nFrom: alice\nold body\n"
	got := StripPlaintextQuotes(in)
	if strings.Contains(got, "Forwarded Message") {
		t.Errorf("forward marker should be stripped, got: %q", got)
	}
	if !strings.Contains(got, "My new note") {
		t.Errorf("new content should be preserved, got: %q", got)
	}
}

func TestStripPlaintextQuotesReply(t *testing.T) {
	in := "Sounds good.\n\nOn Tuesday, 24 September 2024 at 4:00 PM, Sender <sender@address.com> wrote:\n\n> original text\n> spread over\n> several lines\n"
	got := StripPlaintextQuotes(in)
	if strings.Contains(got, "original text") {
		t.Errorf("quoted block should be stripped, got: %q", got)
	}
	if !strings.Contains(got, "Sounds good") {
		t.Errorf("new content should be preserved, got: %q", got)
	}
}

func TestStripPlaintextQuotesReplyNoEmailInIntroducer(t *testing.T) {
	// Has > lines but no canonical "On … <a@b.com> wrote:" introducer.
	in := "Hi.\n\n> some quoted line\n> another quoted line\n"
	got := StripPlaintextQuotes(in)
	if got != in {
		t.Errorf("uncanonical reply should NOT be stripped, got: %q", got)
	}
}

func TestStripPlaintextQuotesUnchangedNoMarker(t *testing.T) {
	in := "Just a plain reply without any marker."
	got := StripPlaintextQuotes(in)
	if got != in {
		t.Errorf("body without markers should be returned, got: %q", got)
	}
}

func TestStripPlaintextQuotesEmpty(t *testing.T) {
	if got := StripPlaintextQuotes(""); got != "" {
		t.Errorf("empty input → %q, want empty", got)
	}
}

func TestMessagePreviewHTMLStripsAndReturnsFirstLine(t *testing.T) {
	body := `<p>Hi Bob, here's the latest draft.</p><blockquote class="protonmail_quote">old reply</blockquote>`
	got := MessagePreview(body, "text/html")
	want := "Hi Bob, here's the latest draft."
	if got != want {
		t.Errorf("preview = %q, want %q", got, want)
	}
}

func TestMessagePreviewPlaintext(t *testing.T) {
	body := "Sounds good.\n\nOn Tue, 24 Sep, Sender <a@b.com> wrote:\n\n> old"
	got := MessagePreview(body, "text/plain")
	if got != "Sounds good." {
		t.Errorf("preview = %q, want %q", got, "Sounds good.")
	}
}

func TestMessagePreviewEmptyAfterStrip(t *testing.T) {
	body := `<blockquote class="protonmail_quote">whole body is a quote</blockquote>`
	got := MessagePreview(body, "text/html")
	if got != "" {
		t.Errorf("preview should be empty when body is entirely a quote, got %q", got)
	}
}

func TestMessagePreviewEmptyBody(t *testing.T) {
	if got := MessagePreview("", "text/html"); got != "" {
		t.Errorf("empty body → %q, want empty", got)
	}
}
