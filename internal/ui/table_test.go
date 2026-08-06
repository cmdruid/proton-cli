package ui

import (
	"strings"
	"testing"
)

// message is the shape a mail list renders, reduced to what the table needs.
type message struct {
	id      string
	from    string
	subject string
	date    string
	flags   string
}

func messageColumns() []Column[message] {
	return []Column[message]{
		{Header: "ID", ID: true, Cell: func(m message) string { return m.id }},
		{Header: "FROM", Flex: true, Cell: func(m message) string { return m.from }},
		{Header: "SUBJECT", Flex: true, Cell: func(m message) string { return m.subject }},
		{Header: "DATE", Cell: func(m message) string { return m.date }},
		{Header: "FLAGS", Accent: true, Cell: func(m message) string { return m.flags }},
	}
}

func messages() []message {
	return []message{
		{"5bH2mQxKT9wLpN4vRs8kZc1yXd7fGh3jAe6bUi0oQm2nWr5tYv==", "Fastmail Billing", "Invoice #2291 is ready", "2026-04-15 14:32", GlyphUnread + GlyphStarred + GlyphAttachment},
		{"9xL4pQrTz2mKd8vBn6cXs1wYf5hJ3gAe7bUi0oQm4nWr2tYv==", "Trailhead Weekly", "The north trail is open again", "2026-04-15 09:02", GlyphUnread},
		{"2mNp7RsVx8kLd4vZn1cQs6wYf9hJ5gAe3bUi0oQm7nWr4tYv==", "Jane Roe", "Re: Quarterly numbers", "2026-04-14 17:48", ""},
	}
}

func TestTablePaginated(t *testing.T) {
	u, out, errb := fixture(t, Options{})
	spec := TableSpec[message]{
		Noun: "messages", Columns: messageColumns(),
		Total: 312, Page: 0, PageSize: 3,
	}
	if err := Table(u, spec, messages()); err != nil {
		t.Fatal(err)
	}
	check(t, "table_paginated", out, errb)
}

func TestTableLastPage(t *testing.T) {
	u, out, errb := fixture(t, Options{})
	spec := TableSpec[message]{
		Noun: "messages", Columns: messageColumns(),
		Total: 6, Page: 1, PageSize: 3,
	}
	if err := Table(u, spec, messages()); err != nil {
		t.Fatal(err)
	}
	check(t, "table_last_page", out, errb)
}

// A short ID is the interactive form: the same table, shortened, is what a user
// copies from.
func TestTableShortIDs(t *testing.T) {
	t.Setenv("PROTON_CLI_FORCE_TTY", "1")
	u, out, errb := fixture(t, Options{})
	spec := TableSpec[message]{Noun: "messages", Columns: messageColumns(), Total: Unknown, Page: Unpaged}
	if err := Table(u, spec, messages()); err != nil {
		t.Fatal(err)
	}
	check(t, "table_short_ids", out, errb)
}

// An empty collection writes nothing at all on stdout, so a redirect yields an
// empty file rather than a stray header.
func TestTableEmptyWritesNothingToStdout(t *testing.T) {
	u, out, errb := fixture(t, Options{})
	spec := TableSpec[message]{Noun: "messages", Columns: messageColumns(), Total: 0, Page: 0, PageSize: 25}
	if err := Table(u, spec, nil); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout should be empty, got %q", out.String())
	}
	check(t, "table_empty", out, errb)
}

// When the table is too wide, the widest flexible column gives up room. Columns
// without Flex keep their natural width, so a date or an ID is never mangled.
//
// A narrow terminal is also where IDs are shortened, so that is the combination
// tested: full-length IDs plus a narrow budget is not a case that occurs.
func TestTableNarrowShrinksWidestFlexColumn(t *testing.T) {
	t.Setenv("PROTON_CLI_FORCE_TTY", "1")
	u, out, errb := fixture(t, Options{Width: 62})
	spec := TableSpec[message]{Noun: "messages", Columns: messageColumns(), Total: Unknown, Page: Unpaged}
	if err := Table(u, spec, messages()); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
		if n := len([]rune(line)); n > 62 {
			t.Errorf("line exceeds the budget (%d > 62): %q", n, line)
		}
	}
	// The DATE column is not flexible, so every row keeps its full timestamp.
	if !strings.Contains(out.String(), "2026-04-15 14:32") {
		t.Error("a non-flex column was truncated")
	}
	check(t, "table_narrow", out, errb)
}

func TestTableRightAlign(t *testing.T) {
	type row struct{ name, size string }
	u, out, errb := fixture(t, Options{})
	spec := TableSpec[row]{
		Noun: "items", Total: Unknown, Page: Unpaged,
		Columns: []Column[row]{
			{Header: "NAME", Flex: true, Cell: func(r row) string { return r.name }},
			{Header: "SIZE", Right: true, Cell: func(r row) string { return r.size }},
		},
	}
	rows := []row{{"report.pdf", "2.4 MB"}, {"notes.txt", "812 B"}, {"archive.tar.gz", "1.1 GB"}}
	if err := Table(u, spec, rows); err != nil {
		t.Fatal(err)
	}
	check(t, "table_right_align", out, errb)
}

// Every collection has the same envelope, so one consumer can read any list.
func TestTableEnvelope(t *testing.T) {
	u, out, errb := fixture(t, Options{Format: FormatJSON})
	spec := TableSpec[message]{
		Noun: "messages", Columns: messageColumns(),
		Total: 312, Page: 0, PageSize: 3,
		Rows: []map[string]any{{"id": "5bH2mQxK", "subject": "Invoice #2291 is ready"}},
	}
	if err := Table(u, spec, messages()); err != nil {
		t.Fatal(err)
	}
	check(t, "table_envelope_json", out, errb)
}

// An unpaginated collection omits the pagination fields rather than reporting
// them as zero, so a consumer can tell "page 0" from "not paginated".
func TestTableEnvelopeUnpaginated(t *testing.T) {
	u, out, errb := fixture(t, Options{Format: FormatJSON})
	spec := TableSpec[message]{Noun: "messages", Columns: messageColumns(), Total: Unknown, Page: Unpaged}
	if err := Table(u, spec, messages()[:1]); err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"page", "page_size", "has_more", "total", "limited"} {
		if strings.Contains(out.String(), `"`+absent+`"`) {
			t.Errorf("unpaginated envelope should omit %q: %s", absent, out.String())
		}
	}
	check(t, "table_envelope_unpaginated_json", out, errb)
}

func TestTableEnvelopeSearchLimited(t *testing.T) {
	u, out, errb := fixture(t, Options{Format: FormatJSON})
	spec := TableSpec[message]{
		Noun: "messages", Columns: messageColumns(),
		Total: Unknown, Page: Unpaged, Limit: 3,
	}
	if err := Table(u, spec, messages()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"limited": true`) {
		t.Errorf("hitting the limit must be reported: %s", out.String())
	}
	check(t, "table_envelope_limited_json", out, errb)
}

func TestTableEnvelopeYAML(t *testing.T) {
	u, out, errb := fixture(t, Options{Format: FormatYAML})
	spec := TableSpec[message]{Noun: "messages", Columns: messageColumns(), Total: 3, Page: 0, PageSize: 25}
	if err := Table(u, spec, messages()[:1]); err != nil {
		t.Fatal(err)
	}
	check(t, "table_envelope_yaml", out, errb)
}

// Machine output is never shortened and never coloured, whatever the terminal is
// doing, because a program is reading it.
func TestTableMachineOutputIgnoresTTY(t *testing.T) {
	t.Setenv("PROTON_CLI_FORCE_TTY", "1")
	u, out, _ := fixture(t, Options{Format: FormatJSON})
	spec := TableSpec[message]{Noun: "messages", Columns: messageColumns(), Total: Unknown, Page: Unpaged}
	if err := Table(u, spec, messages()[:1]); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "\x1b[") {
		t.Error("machine output must carry no escape sequences")
	}
}

// Colour is applied after layout, so enabling it must not move a single column.
func TestTableColourDoesNotAffectLayout(t *testing.T) {
	plain, _, _ := fixture(t, Options{})
	u, out, _ := fixture(t, Options{})
	u.theme = Theme{enabled: true, wide: true}
	spec := TableSpec[message]{Noun: "messages", Columns: messageColumns(), Total: Unknown, Page: Unpaged}
	if err := Table(u, spec, messages()); err != nil {
		t.Fatal(err)
	}
	coloured := out.String()

	plainOut := &strings.Builder{}
	plain.Out = plainOut
	if err := Table(plain, spec, messages()); err != nil {
		t.Fatal(err)
	}
	if stripANSI(coloured) != plainOut.String() {
		t.Errorf("colour changed the layout\ncoloured (stripped):\n%s\nplain:\n%s",
			stripANSI(coloured), plainOut.String())
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
