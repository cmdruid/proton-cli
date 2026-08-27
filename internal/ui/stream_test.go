package ui

import "testing"

// The stream fixture is the mail watch's own shape: a time, a reference, a
// sender held to a fixed width, and a subject that runs on.
func arrivalColumns() []StreamColumn[message] {
	return []StreamColumn[message]{
		{Width: 5, Cell: func(m message) string { return m.date[11:] }},
		{ID: true, Cell: func(m message) string { return m.id }},
		{Width: 20, Cell: func(m message) string { return m.from }},
		{Cell: func(m message) string { return m.subject }},
	}
}

func arrivalSpec() StreamSpec[message] {
	return StreamSpec[message]{
		Columns: arrivalColumns(),
		Opening: "Watching the inbox and Receipts. Ctrl+C to stop.",
	}
}

func emitAll(t *testing.T, s *Stream[message], items []message) {
	t.Helper()
	for _, m := range items {
		if err := s.Emit(m); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStreamDrawsALinePerThing(t *testing.T) {
	u, out, errb := fixture(t, Options{})
	emitAll(t, Open(u, arrivalSpec()), messages())
	check(t, "stream_text", out, errb)
}

// A stream has no set of rows to measure, so a cell wider than its column is
// cut - except an ID, which is shortened by its own rule and never truncated,
// since a reference that cannot be pasted back is worse than a ragged line.
func TestStreamHoldsItsDeclaredWidths(t *testing.T) {
	u, out, errb := fixture(t, Options{})
	long := []message{{
		id:      "5bH2mQxKT9wLpN4vRs8kZc1yXd7fGh3jAe6bUi0oQm2nWr5tYv==",
		from:    "A Sender Whose Name Runs Well Past The Column",
		subject: "Short",
		date:    "2026-04-15 14:32",
	}}
	emitAll(t, Open(u, arrivalSpec()), long)
	check(t, "stream_widths", out, errb)
}

// --quiet silences the opening the way it silences a footer, and never the
// answer.
func TestStreamQuietKeepsTheLines(t *testing.T) {
	u, out, errb := fixture(t, Options{Quiet: true})
	emitAll(t, Open(u, arrivalSpec()), messages()[:1])
	check(t, "stream_quiet", out, errb)
}

// A stream cannot be an envelope, because an envelope has to be closed. One
// object per line is what jq reads without --slurp.
func TestStreamJSONIsOneObjectPerLine(t *testing.T) {
	u, out, errb := fixture(t, Options{Format: FormatJSON})
	spec := arrivalSpec()
	spec.Object = func(m message) any {
		return map[string]any{"id": m.id, "from_name": m.from, "subject": m.subject}
	}
	emitAll(t, Open(u, spec), messages())
	check(t, "stream_json", out, errb)
}

func TestStreamYAMLIsOneDocumentPerThing(t *testing.T) {
	u, out, errb := fixture(t, Options{Format: FormatYAML})
	spec := arrivalSpec()
	spec.Object = func(m message) any {
		return map[string]any{"id": m.id, "from_name": m.from, "subject": m.subject}
	}
	emitAll(t, Open(u, spec), messages()[:2])
	check(t, "stream_yaml", out, errb)
}
