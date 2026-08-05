package mail

import (
	"strings"
	"testing"
)

// buildDocument assembles what Export writes: a header block, a blank line, and
// the MIME body.
func buildDocument(t *testing.T, base, plain, html string, atts []mimePart) string {
	t.Helper()
	h, body, err := buildExportMIME(plain, html, atts)
	if err != nil {
		t.Fatalf("buildExportMIME: %v", err)
	}
	return mergeHeaders(base, h) + "\r\n" + string(body) + "\r\n"
}

const originalHeaders = "Received: from mx.example.com\r\n" +
	"Date: Tue, 14 Nov 2023 22:13:20 +0000\r\n" +
	"From: Alice <alice@example.com>\r\n" +
	"To: me@proton.me\r\n" +
	"Subject: Invoice\r\n" +
	"Message-ID: <abc@example.com>\r\n" +
	"Content-Type: text/plain; charset=iso-8859-1\r\n" +
	"Content-Transfer-Encoding: 7bit\r\n"

func TestMergeHeadersKeepsTheOriginalAndOverwritesMIME(t *testing.T) {
	doc := buildDocument(t, originalHeaders, "hello", "", nil)
	head, _, _ := strings.Cut(doc, "\r\n\r\n")

	for _, want := range []string{
		"Received: from mx.example.com",
		"From: Alice <alice@example.com>",
		"Subject: Invoice",
		"Message-ID: <abc@example.com>",
	} {
		if !strings.Contains(head, want) {
			t.Errorf("merged headers lost %q:\n%s", want, head)
		}
	}
	// The transport headers describing the old body must be replaced, not kept
	// alongside the new ones.
	if strings.Contains(head, "iso-8859-1") {
		t.Errorf("the original Content-Type survived:\n%s", head)
	}
	if strings.Contains(head, "7bit") {
		t.Errorf("the original Content-Transfer-Encoding survived:\n%s", head)
	}
	if n := strings.Count(head, "Content-Type:"); n != 1 {
		t.Errorf("expected exactly one Content-Type, got %d:\n%s", n, head)
	}
}

func TestMergeHeadersAppendsWhatTheOriginalLacked(t *testing.T) {
	base := "From: alice@example.com\r\nSubject: Hi\r\n"
	doc := buildDocument(t, base, "hello", "", nil)
	head, _, _ := strings.Cut(doc, "\r\n\r\n")
	if !strings.Contains(head, "Content-Type:") {
		t.Errorf("a header the original lacked was not appended:\n%s", head)
	}
}

func TestMergeHeadersUnfoldsContinuationLines(t *testing.T) {
	base := "Subject: a very long subject\r\n that folded over two lines\r\nFrom: a@b.c\r\n"
	doc := buildDocument(t, base, "hi", "", nil)
	head, _, _ := strings.Cut(doc, "\r\n\r\n")
	if !strings.Contains(head, "that folded over two lines") {
		t.Errorf("a folded header lost its continuation:\n%s", head)
	}
	if strings.Contains(head, "folded over two lines: ") {
		t.Errorf("a continuation line was mistaken for a header:\n%s", head)
	}
}

func TestSynthesizeHeadersForAMessageWithoutAny(t *testing.T) {
	raw := &rawMessage{
		Subject:    "Draft subject",
		Sender:     map[string]any{"Address": "me@proton.me", "Name": "Me"},
		ToList:     []map[string]any{{"Address": "bob@example.com"}},
		CCList:     []map[string]any{{"Address": "carol@example.com"}},
		ExternalID: "generated@proton.me",
		Time:       1_700_000_000,
	}
	got := synthesizeHeaders(raw)
	for _, want := range []string{
		"Subject: Draft subject",
		"From: Me <me@proton.me>",
		"To: bob@example.com",
		"Cc: carol@example.com",
		"Message-ID: <generated@proton.me>",
		"MIME-Version: 1.0",
		"Date: ",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("synthesized headers are missing %q:\n%s", want, got)
		}
	}
}

func TestExportMIMEStructurePerBodyCombination(t *testing.T) {
	inline := mimePart{Filename: "logo.png", MIMEType: "image/png", Data: []byte("png"), ContentID: "cid1"}
	regular := mimePart{Filename: "report.pdf", MIMEType: "application/pdf", Data: []byte("pdf")}

	tests := []struct {
		name        string
		plain, html string
		atts        []mimePart
		wantType    string
		wantNested  []string
	}{
		{"plain only", "hello", "", nil, "text/plain", nil},
		{"html only", "", "<p>hi</p>", nil, "text/html", nil},
		{"both bodies", "hello", "<p>hi</p>", nil, "multipart/alternative", []string{"text/plain", "text/html"}},
		{"html with inline image", "", "<p>hi</p>", []mimePart{inline},
			"multipart/related", []string{"text/html", "image/png"}},
		{"plain with attachment", "hello", "", []mimePart{regular},
			"multipart/mixed", []string{"text/plain", "application/pdf"}},
		{"everything", "hello", "<p>hi</p>", []mimePart{inline, regular},
			"multipart/mixed", []string{"multipart/alternative", "multipart/related", "application/pdf"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, body, err := buildExportMIME(tt.plain, tt.html, tt.atts)
			if err != nil {
				t.Fatalf("buildExportMIME: %v", err)
			}
			if got := h.Get("Content-Type"); !strings.HasPrefix(got, tt.wantType) {
				t.Errorf("top-level Content-Type = %q, want %s", got, tt.wantType)
			}
			for _, want := range tt.wantNested {
				if !strings.Contains(string(body), want) {
					t.Errorf("body is missing a %s part:\n%s", want, body)
				}
			}
		})
	}
}

func TestExportRoundTripsThroughParseEML(t *testing.T) {
	atts := []mimePart{
		{Filename: "logo.png", MIMEType: "image/png", Data: []byte("\x89PNG binary\x00bytes"), ContentID: "cid1"},
		{Filename: "report.pdf", MIMEType: "application/pdf", Data: []byte("%PDF-1.4 binary\x00")},
	}
	doc := buildDocument(t, originalHeaders, "plain body", "<p>html body</p>", atts)

	got, err := ParseEML(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("ParseEML: %v", err)
	}
	if got.Subject != "Invoice" {
		t.Errorf("subject = %q, want Invoice", got.Subject)
	}
	if len(got.To) != 1 || got.To[0].Address != "me@proton.me" {
		t.Errorf("to = %v", got.To)
	}
	// Both bodies survive; the HTML one wins, as a mail client would prefer.
	if !got.HTML || !strings.Contains(got.Body, "html body") {
		t.Errorf("body = %q (html=%v), want the HTML alternative", got.Body, got.HTML)
	}
	if len(got.Attachments) != 2 {
		t.Fatalf("got %d attachments, want 2: %v", len(got.Attachments), got.Attachments)
	}
	byName := map[string]LocalAttachment{}
	for _, a := range got.Attachments {
		byName[a.Filename] = a
	}
	logo, ok := byName["logo.png"]
	if !ok {
		t.Fatalf("logo.png missing from %v", byName)
	}
	if !logo.Inline || logo.ContentID != "cid1" {
		t.Errorf("logo.png should come back inline with its cid, got inline=%v cid=%q", logo.Inline, logo.ContentID)
	}
	if string(logo.Data) != string(atts[0].Data) {
		t.Errorf("binary attachment did not round-trip byte for byte")
	}
	report := byName["report.pdf"]
	if report.Inline {
		t.Error("report.pdf should not be inline")
	}
	if string(report.Data) != string(atts[1].Data) {
		t.Errorf("binary attachment did not round-trip byte for byte")
	}
}

func TestParseEMLDecodesTransferEncodingsAndCharsets(t *testing.T) {
	// A quoted-printable latin-1 body with an RFC 2047 encoded subject: the
	// shapes real mail actually arrives in.
	doc := "From: alice@example.com\r\n" +
		"To: me@proton.me\r\n" +
		"Subject: =?utf-8?q?Gr=C3=BC=C3=9Fe?=\r\n" +
		"Content-Type: text/plain; charset=iso-8859-1\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n" +
		"\r\n" +
		"Sch=F6ne Gr=FC=DFe\r\n"

	got, err := ParseEML(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("ParseEML: %v", err)
	}
	if got.Subject != "Grüße" {
		t.Errorf("subject = %q, want Grüße", got.Subject)
	}
	if !strings.Contains(got.Body, "Schöne Grüße") {
		t.Errorf("body = %q, want the latin-1 text decoded", got.Body)
	}
}

func TestParseEMLReadsAddressDisplayNames(t *testing.T) {
	doc := "From: a@b.c\r\n" +
		`To: "Alice Smith" <alice@example.com>, bob@example.com` + "\r\n" +
		"Cc: Carol <carol@example.com>\r\n" +
		"Subject: Hi\r\n\r\nbody\r\n"
	got, err := ParseEML(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("ParseEML: %v", err)
	}
	if len(got.To) != 2 || got.To[0].Name != "Alice Smith" || got.To[1].Address != "bob@example.com" {
		t.Errorf("to = %+v", got.To)
	}
	if len(got.CC) != 1 || got.CC[0].Name != "Carol" {
		t.Errorf("cc = %+v", got.CC)
	}
}

func TestParseEMLRejectsAnEmptyDocument(t *testing.T) {
	doc := "From: a@b.c\r\nSubject: nothing\r\n\r\n"
	if _, err := ParseEML(strings.NewReader(doc)); err == nil {
		t.Error("expected an error for a document with no body and no attachments")
	}
}

func TestMboxEntryFramesAndEscapes(t *testing.T) {
	doc := "From: alice@example.com\nSubject: Hi\n\nFrom the top.\n>From already escaped\n"
	got := string(MboxEntry([]byte(doc), &ExportMeta{From: "alice@example.com", Time: 1_700_000_000}))

	if !strings.HasPrefix(got, "From alice@example.com ") {
		t.Errorf("mbox entry must start with a From_ separator, got:\n%s", got)
	}
	// The header line is a real header, not a separator, so it gets escaped too;
	// what matters is that no body line can be mistaken for a new message.
	for _, line := range strings.Split(got, "\n")[1:] {
		if strings.HasPrefix(line, "From ") {
			t.Errorf("unescaped %q would start a new mbox message:\n%s", line, got)
		}
	}
	if !strings.Contains(got, ">>From already escaped") {
		t.Errorf("an already-escaped line must gain another '>', got:\n%s", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Error("mbox entries must be separated by a blank line")
	}
}

func TestMboxEntryFallsBackWhenThereIsNoSender(t *testing.T) {
	got := string(MboxEntry([]byte("Subject: x\n\nbody\n"), &ExportMeta{}))
	if !strings.HasPrefix(got, "From MAILER-DAEMON ") {
		t.Errorf("expected a MAILER-DAEMON envelope sender, got:\n%s", got)
	}
}

// Some clients label a part us-ascii and then put a UTF-8 character in it.
// Decoding as declared would produce mojibake, and US-ASCII provably cannot
// contain the bytes in question, so the declaration is ignored.
func TestParseEMLRecoversMisdeclaredUTF8(t *testing.T) {
	doc := "From: bob@outlook.example\r\n" +
		"To: me@proton.me\r\n" +
		"Subject: Dash\r\n" +
		"Content-Type: text/plain; charset=\"us-ascii\"\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n" +
		"\r\n" +
		"Hello =E2=80=94 dash\r\n"

	got, err := ParseEML(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("ParseEML: %v", err)
	}
	if !strings.Contains(got.Body, "Hello — dash") {
		t.Errorf("body = %q, want the em-dash intact", got.Body)
	}
}

// A part that really is latin-1 must still be transcoded, so the recovery above
// cannot swallow genuine single-byte encodings.
func TestParseEMLStillTranscodesRealLatin1(t *testing.T) {
	doc := "From: a@b.c\r\n" +
		"To: me@proton.me\r\n" +
		"Subject: Umlaut\r\n" +
		"Content-Type: text/plain; charset=\"iso-8859-1\"\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n" +
		"\r\n" +
		"Sch=F6n\r\n"

	got, err := ParseEML(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("ParseEML: %v", err)
	}
	if !strings.Contains(got.Body, "Schön") {
		t.Errorf("body = %q, want Schön", got.Body)
	}
}
