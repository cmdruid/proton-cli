package mail

import (
	"strings"
	"testing"

	mailsvc "github.com/roman-16/proton-cli/internal/services/mail"
)

func TestRenderAttachmentsFooterEmpty(t *testing.T) {
	if got := renderAttachmentsFooter(nil, false); got != "" {
		t.Errorf("nil slice → %q, want empty", got)
	}
	if got := renderAttachmentsFooter([]mailsvc.Attachment{}, false); got != "" {
		t.Errorf("empty slice → %q, want empty", got)
	}
}

func TestRenderAttachmentsFooterAllNonInline(t *testing.T) {
	atts := []mailsvc.Attachment{
		{ID: "a1", Name: "Abrechnung.pdf", Size: 237 * 1024, Disposition: "attachment"},
		{ID: "a2", Name: "Scan.png", Size: 1258291, Disposition: "attachment"}, // ~1.2 MB
	}
	got := renderAttachmentsFooter(atts, false)
	want := "\n---\n" +
		"Attachments (2):\n" +
		"  - Abrechnung.pdf  (237.0 KB)  ID: a1\n" +
		"  - Scan.png        (1.2 MB)    ID: a2\n"
	if got != want {
		t.Errorf("output mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestRenderAttachmentsFooterFiltersInline(t *testing.T) {
	atts := []mailsvc.Attachment{
		{ID: "i1", Name: "logo.png", Size: 4096, Disposition: "inline"},
		{ID: "a1", Name: "doc.pdf", Size: 50000, Disposition: "attachment"},
		{ID: "i2", Name: "trail.gif", Size: 1024, Disposition: "inline"},
	}
	got := renderAttachmentsFooter(atts, false)
	if strings.Contains(got, "logo.png") || strings.Contains(got, "trail.gif") {
		t.Errorf("inline attachments leaked into default footer:\n%s", got)
	}
	if !strings.Contains(got, "doc.pdf") {
		t.Errorf("non-inline missing from default footer:\n%s", got)
	}
	if !strings.Contains(got, "Attachments (1):") {
		t.Errorf("count should reflect visible items only; got:\n%s", got)
	}
}

func TestRenderAttachmentsFooterIncludeInlineTags(t *testing.T) {
	atts := []mailsvc.Attachment{
		{ID: "i1", Name: "logo.png", Size: 4096, Disposition: "inline"},
		{ID: "a1", Name: "doc.pdf", Size: 50000, Disposition: "attachment"},
		{ID: "i2", Name: "trail.gif", Size: 1024, Disposition: "inline"},
	}
	got := renderAttachmentsFooter(atts, true)
	if !strings.Contains(got, "Attachments (3):") {
		t.Errorf("count should be 3 with --include-inline; got:\n%s", got)
	}
	if !strings.Contains(got, "logo.png") || !strings.Contains(got, "trail.gif") {
		t.Errorf("inline attachments missing with --include-inline:\n%s", got)
	}
	// Inline rows carry the (inline) tag; the non-inline row does not.
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "logo.png") && !strings.HasSuffix(line, "  (inline)") {
			t.Errorf("inline row missing tag: %q", line)
		}
		if strings.Contains(line, "trail.gif") && !strings.HasSuffix(line, "  (inline)") {
			t.Errorf("inline row missing tag: %q", line)
		}
		if strings.Contains(line, "doc.pdf") && strings.HasSuffix(line, "(inline)") {
			t.Errorf("non-inline row has inline tag: %q", line)
		}
	}
}

func TestRenderAttachmentsFooterMissingDisposition(t *testing.T) {
	atts := []mailsvc.Attachment{
		{ID: "x", Name: "old.bin", Size: 100, Disposition: ""},
		{ID: "i1", Name: "logo.png", Size: 4096, Disposition: "inline"},
	}
	// Default: empty disposition treated as visible (attachment).
	got := renderAttachmentsFooter(atts, false)
	if !strings.Contains(got, "old.bin") {
		t.Errorf("missing-disposition entry should be visible by default: %s", got)
	}
	if strings.Contains(got, "logo.png") {
		t.Errorf("inline still hidden by default: %s", got)
	}
	if !strings.Contains(got, "Attachments (1):") {
		t.Errorf("count should be 1: %s", got)
	}
}

func TestRenderAttachmentsFooterStartsWithSeparator(t *testing.T) {
	atts := []mailsvc.Attachment{{ID: "a", Name: "x", Size: 1, Disposition: "attachment"}}
	got := renderAttachmentsFooter(atts, false)
	if !strings.HasPrefix(got, "\n---\n") {
		t.Errorf("footer must start with blank-line + ---; got %q", got[:min(len(got), 20)])
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("footer must end with newline; got tail %q", got[max(0, len(got)-10):])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
