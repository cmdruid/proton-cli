package ui

import (
	"strings"
	"testing"
)

type attachment struct {
	id, name, size string
}

func attachmentTable(u *UI) error {
	return Table(u, TableSpec[attachment]{
		Noun:  "attachments",
		Total: Unknown, Page: Unpaged,
		Columns: []Column[attachment]{
			{Header: "ID", ID: true, Cell: func(a attachment) string { return a.id }},
			{Header: "NAME", Flex: true, Cell: func(a attachment) string { return a.name }},
			{Header: "SIZE", Right: true, Cell: func(a attachment) string { return a.size }},
		},
	}, []attachment{{"kQ81mDx4", "invoice-2291.pdf", "84.2 KB"}})
}

func messageHeader() []Field {
	return []Field{
		{Label: "Subject", Value: "Invoice #2291 is ready"},
		{Label: "From", Value: "Fastmail Billing <billing@fastmail.com>"},
		{Label: "To", Value: "me@proton.me"},
		{Label: "Date", Value: "2026-04-15 14:32"},
		{Label: "Signature", Value: "verified"},
		{Label: "ID", Value: "5bH2mQxK", ID: true},
	}
}

func TestDocumentSingleMessage(t *testing.T) {
	u, out, errb := fixture(t, Options{})
	spec := DocumentSpec{Parts: []Part{{
		Header:       messageHeader(),
		Body:         "Hi Roman, your invoice is attached.",
		TrailerTitle: "Attachments",
		Trailer:      attachmentTable,
	}}}
	if err := Document(u, spec); err != nil {
		t.Fatal(err)
	}
	check(t, "document_message", out, errb)
}

// --body-only is what a redirect into a file wants: no headers, no dividers, no
// trailers, just the text.
func TestDocumentBodyOnlyIsExactlyTheBody(t *testing.T) {
	u, out, errb := fixture(t, Options{})
	body := "Hi Roman, your invoice is attached."
	spec := DocumentSpec{
		Header:   []Field{{Label: "Subject", Value: "Invoice #2291 is ready"}},
		Parts:    []Part{{Header: messageHeader(), Body: body, TrailerTitle: "Attachments", Trailer: attachmentTable}},
		BodyOnly: true,
	}
	if err := Document(u, spec); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != body+"\n" {
		t.Errorf("body-only should emit exactly the body, got %q", got)
	}
	if errb.Len() != 0 {
		t.Errorf("unexpected stderr: %q", errb.String())
	}
}

func TestDocumentThread(t *testing.T) {
	u, out, errb := fixture(t, Options{})
	spec := DocumentSpec{
		Header: []Field{
			{Label: "Subject", Value: "Quarterly numbers"},
			{Label: "Messages", Value: "2"},
			{Label: "ID", Value: "8Tr4nVx2", ID: true},
		},
		Parts: []Part{{
			Divider: "1/2",
			Header: []Field{
				{Label: "From", Value: "Jane Roe <jane@example.com>"},
				{Label: "Date", Value: "2026-04-14 17:48"},
				{Label: "ID", Value: "2mNp7RsV", ID: true},
			},
			Body: "Numbers attached. Let me know.",
		}, {
			Divider: "2/2",
			Header: []Field{
				{Label: "From", Value: "me@proton.me"},
				{Label: "Date", Value: "2026-04-14 18:10"},
				{Label: "ID", Value: "4Wq9zLm1", ID: true},
			},
			Body: "Got them, thanks.",
		}},
	}
	if err := Document(u, spec); err != nil {
		t.Fatal(err)
	}
	check(t, "document_thread", out, errb)
}

// A document's machine form carries the body as a field, so a consumer never has
// to parse the header block back out of loose text.
func TestDocumentMachineUsesObject(t *testing.T) {
	u, out, _ := fixture(t, Options{Format: FormatJSON})
	spec := DocumentSpec{
		Parts: []Part{{Header: messageHeader(), Body: "Hi."}},
		Object: map[string]any{
			"id":      "5bH2mQxK",
			"subject": "Invoice #2291 is ready",
			"body":    "Hi.",
		},
	}
	if err := Document(u, spec); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{`"body"`, `"subject"`, `"id"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	if strings.Contains(got, "Signature:") {
		t.Errorf("machine output leaked a display label: %s", got)
	}
}
