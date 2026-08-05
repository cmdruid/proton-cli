package mail

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseRecipientAcceptsBareAndNamedAddresses(t *testing.T) {
	tests := []struct {
		in         string
		name, addr string
	}{
		{"alice@proton.me", "", "alice@proton.me"},
		{" alice@proton.me ", "", "alice@proton.me"},
		{"Alice <alice@proton.me>", "Alice", "alice@proton.me"},
		{`"Roe, Jane" <jane@proton.me>`, "Roe, Jane", "jane@proton.me"},
		// Anything unparseable is passed through as an address rather than
		// rejected, so the API reports the problem rather than the CLI guessing.
		{"not an address", "", "not an address"},
	}
	for _, tt := range tests {
		got := ParseRecipient(tt.in)
		if got.Name != tt.name || got.Address != tt.addr {
			t.Errorf("ParseRecipient(%q) = %+v, want name=%q addr=%q", tt.in, got, tt.name, tt.addr)
		}
	}
}

func TestParseRecipientsDropsBlanks(t *testing.T) {
	got := ParseRecipients([]string{"a@b.c", "", "   ", "d@e.f"})
	if len(got) != 2 {
		t.Errorf("got %d recipients, want 2: %+v", len(got), got)
	}
}

func TestRecipientStringRendersForHeaders(t *testing.T) {
	if got := (Recipient{Address: "a@b.c"}).String(); got != "a@b.c" {
		t.Errorf("bare = %q", got)
	}
	if got := (Recipient{Name: "A", Address: "a@b.c"}).String(); got != "A <a@b.c>" {
		t.Errorf("named = %q", got)
	}
}

func TestRecipientAddressesFlattensAndDeduplicates(t *testing.T) {
	c := Content{
		To:  ParseRecipients([]string{"alice@proton.me", "Bob <Bob@Example.com>"}),
		CC:  ParseRecipients([]string{"alice@PROTON.me", "carol@proton.me"}),
		BCC: ParseRecipients([]string{"bob@example.com"}),
	}
	want := []string{"alice@proton.me", "Bob@Example.com", "carol@proton.me"}
	if got := c.RecipientAddresses(); !reflect.DeepEqual(got, want) {
		t.Errorf("RecipientAddresses() = %v, want %v", got, want)
	}
}

func TestHasRecipientsAndAttachmentCount(t *testing.T) {
	if (Content{}).HasRecipients() {
		t.Error("an empty Content is addressed to nobody")
	}
	if !(Content{BCC: ParseRecipients([]string{"a@b.c"})}).HasRecipients() {
		t.Error("a BCC-only message still has a recipient")
	}
	c := Content{
		Attach: []LocalAttachment{{Filename: "a"}, {Filename: "b"}},
		Carry:  []CarriedAttachment{{Name: "c"}},
	}
	if got := c.AttachmentCount(); got != 3 {
		t.Errorf("AttachmentCount() = %d, want 3", got)
	}
}

func TestContentMIMETypeAndPlainBody(t *testing.T) {
	plain := Content{Body: "hello"}
	if plain.mimeType() != mimeTypePlain {
		t.Errorf("mimeType = %q", plain.mimeType())
	}
	if plain.plainBody() != "hello" {
		t.Errorf("plainBody = %q", plain.plainBody())
	}

	html := Content{Body: "<p>hello <b>world</b></p>", HTML: true}
	if html.mimeType() != mimeTypeHTML {
		t.Errorf("mimeType = %q", html.mimeType())
	}
	if got := html.plainBody(); strings.Contains(got, "<b>") || !strings.Contains(got, "hello world") {
		t.Errorf("plainBody = %q, want the markup flattened", got)
	}
}

// A reply reads as message, signature, then what it answers - the order the web
// client composes in.
func TestAppendSignaturePlacesTheQuoteLast(t *testing.T) {
	c := Content{Body: "My answer."}
	c.AppendSignature("Roman | Vienna", "> the original")

	iSig := strings.Index(c.Body, "Roman | Vienna")
	iQuote := strings.Index(c.Body, "> the original")
	if iSig < 0 || iQuote < 0 {
		t.Fatalf("body lost a part: %q", c.Body)
	}
	if !strings.HasPrefix(c.Body, "My answer.") {
		t.Errorf("the new text must come first: %q", c.Body)
	}
	if iSig > iQuote {
		t.Errorf("the signature must sit above the quote: %q", c.Body)
	}
}

func TestAppendSignatureFlattensHTMLIntoAPlaintextBody(t *testing.T) {
	c := Content{Body: "Hi"}
	c.AppendSignature("Roman<br>Vienna", "")
	if strings.Contains(c.Body, "<br>") {
		t.Errorf("a plaintext body must not carry markup: %q", c.Body)
	}
	if !strings.Contains(c.Body, "Roman") || !strings.Contains(c.Body, "Vienna") {
		t.Errorf("signature text was lost: %q", c.Body)
	}
}

func TestAppendSignatureWrapsHTMLInASignatureBlock(t *testing.T) {
	c := Content{Body: "<p>Hi</p>", HTML: true}
	c.AppendSignature("Roman", "")
	if !strings.Contains(c.Body, `class="protonmail_signature_block"`) {
		t.Errorf("HTML signatures should be wrapped so clients can style them: %q", c.Body)
	}
}

func TestAppendSignatureWithNothingToAppendLeavesTheBodyAlone(t *testing.T) {
	c := Content{Body: "Hi"}
	c.AppendSignature("", "")
	if c.Body != "Hi" {
		t.Errorf("body = %q, want it untouched", c.Body)
	}
}

func TestRecipientListFallsBackToTheAddressForTheName(t *testing.T) {
	got := recipientList(ParseRecipients([]string{"jane@proton.me", "Bob <bob@example.com>"}))
	if len(got) != 2 {
		t.Fatalf("got %d entries", len(got))
	}
	if got[0]["Address"] != "jane@proton.me" || got[0]["Name"] != "jane@proton.me" {
		t.Errorf("nameless entry = %v", got[0])
	}
	if got[1]["Name"] != "Bob" {
		t.Errorf("named entry = %v", got[1])
	}
	if recipientList(nil) == nil {
		t.Error("recipientList(nil) should return a non-nil empty slice for JSON encoding")
	}
}

func TestRecipientsFromRawSkipsEntriesWithoutAnAddress(t *testing.T) {
	got := recipientsFromRaw([]map[string]any{
		{"Address": "a@b.c", "Name": "A"},
		{"Name": "no address"},
		{"Address": "d@e.f"},
	})
	if len(got) != 2 || got[0].Name != "A" || got[1].Address != "d@e.f" {
		t.Errorf("recipientsFromRaw = %+v", got)
	}
}

// Forwarding without a covering note must not open with blank lines.
func TestAppendSignatureDropsEmptyParts(t *testing.T) {
	c := Content{}
	c.AppendSignature("", "> forwarded")
	if c.Body != "> forwarded" {
		t.Errorf("body = %q, want just the quote", c.Body)
	}

	html := Content{HTML: true}
	html.AppendSignature("Roman", "<blockquote>x</blockquote>")
	if strings.HasPrefix(html.Body, "<br><br>") {
		t.Errorf("body = %q, want no leading separator", html.Body)
	}
}
