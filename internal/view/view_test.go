package view

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/roman-16/proton-cli/internal/render"
)

type row struct {
	id    string
	name  string
	flags string
}

func columns() []Column[row] {
	return []Column[row]{
		{Header: "ID", ID: true, Cell: func(r row) string { return r.id }},
		{Header: "NAME", Cell: func(r row) string { return r.name }},
		{Header: "⚑", Accent: true, Cell: func(r row) string { return r.flags }},
	}
}

var items = []row{
	{"abcdefghijklmnop", "Fastmail Billing", "●"},
	{"qrstuvwxyz012345", "Proton", ""},
}

func newRenderer(t *testing.T, format render.Format) (*render.Renderer, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	r := render.New(render.Options{Format: format, Stdout: &stdout, Stderr: &stderr, LogLevel: slog.LevelError})
	return r, &stdout, &stderr
}

func TestRenderTextShortensIDsWhenAsked(t *testing.T) {
	r, stdout, _ := newRenderer(t, render.FormatText)
	if err := Render(r, true, nil, List[row]{Columns: columns()}, items); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(stdout.String(), "abcdefgh  ") {
		t.Errorf("short ID missing from output:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "abcdefghijklmnop") {
		t.Errorf("full ID leaked into shortened output:\n%s", stdout.String())
	}
}

func TestRenderTextKeepsFullIDs(t *testing.T) {
	r, stdout, _ := newRenderer(t, render.FormatText)
	if err := Render(r, false, nil, List[row]{Columns: columns()}, items); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(stdout.String(), "abcdefghijklmnop") {
		t.Errorf("full ID missing from output:\n%s", stdout.String())
	}
}

func TestRenderFooterGoesToStderr(t *testing.T) {
	r, stdout, stderr := newRenderer(t, render.FormatText)
	list := List[row]{Columns: columns(), Footer: func(n int) string { return "2 rows total (single page)." }}
	if err := Render(r, true, nil, list, items); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(stdout.String(), "rows total") {
		t.Error("footer leaked into stdout")
	}
	if got := stderr.String(); got != "\n2 rows total (single page).\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestRenderJSONIgnoresColumnsAndFooter(t *testing.T) {
	r, stdout, stderr := newRenderer(t, render.FormatJSON)
	list := List[row]{Columns: columns(), Footer: func(n int) string { return "footer" }}
	if err := Render(r, true, nil, list, []row{{"abcdefghijklmnop", "x", ""}}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("JSON mode wrote %q to stderr", stderr.String())
	}
	if strings.Contains(stdout.String(), "NAME") {
		t.Errorf("JSON mode emitted a text table:\n%s", stdout.String())
	}
}

func TestRenderJSONOverrideIsUsed(t *testing.T) {
	r, stdout, _ := newRenderer(t, render.FormatJSON)
	list := List[row]{Columns: columns(), JSON: map[string]int{"total": 7}}
	if err := Render(r, true, nil, list, items); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(stdout.String(), `"total": 7`) {
		t.Errorf("JSON override not used:\n%s", stdout.String())
	}
}

// A non-terminal destination must receive exactly the same bytes as before
// color existed, so pipelines and golden output stay stable.
func TestRenderIsPlainWhenNotATerminal(t *testing.T) {
	r, stdout, stderr := newRenderer(t, render.FormatText)
	list := List[row]{Columns: columns(), Footer: func(n int) string { return "2 rows" }}
	if err := Render(r, true, nil, list, items); err != nil {
		t.Fatalf("Render: %v", err)
	}
	for name, out := range map[string]string{"stdout": stdout.String(), "stderr": stderr.String()} {
		if strings.Contains(out, "\x1b[") {
			t.Errorf("%s contains escape sequences: %q", name, out)
		}
	}
}
