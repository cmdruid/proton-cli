package mail

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewContentID(t *testing.T) {
	cid, err := newContentID("alice@proton.me")
	if err != nil {
		t.Fatalf("newContentID: %v", err)
	}
	local, domain, ok := strings.Cut(cid, "@")
	if !ok {
		t.Fatalf("cid %q has no @", cid)
	}
	if domain != "proton.me" {
		t.Errorf("domain = %q, want proton.me", domain)
	}
	// 8 random bytes -> 16 hex chars.
	if len(local) != 16 {
		t.Errorf("local part = %q (len %d), want 16 hex chars", local, len(local))
	}
	if _, err := hex.DecodeString(local); err != nil {
		t.Errorf("local part %q is not hex: %v", local, err)
	}

	custom, _ := newContentID("bob@company.example")
	if !strings.HasSuffix(custom, "@company.example") {
		t.Errorf("cid = %q, want @company.example suffix", custom)
	}

	// A sender without a parseable domain falls back to proton.me.
	fallback, _ := newContentID("no-at-sign")
	if !strings.HasSuffix(fallback, "@proton.me") {
		t.Errorf("fallback cid = %q, want @proton.me suffix", fallback)
	}

	a, _ := newContentID("x@y.z")
	b, _ := newContentID("x@y.z")
	if a == b {
		t.Error("two content IDs came out identical; they must be random")
	}
}

func TestPrepareInlineImagesNoInlineIsNoOp(t *testing.T) {
	opts := &SendOptions{HTML: true, Body: "<p>x</p>"}
	got, err := prepareInlineImages(opts, "a@b.c")
	if err != nil {
		t.Fatalf("prepareInlineImages: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	if opts.Body != "<p>x</p>" {
		t.Errorf("body must be untouched, got %q", opts.Body)
	}
}

func TestPrepareInlineImagesRequiresHTML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.png")
	if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	opts := &SendOptions{HTML: false, Body: "plain", InlineAttach: []string{p}}
	if _, err := prepareInlineImages(opts, "a@b.c"); err == nil {
		t.Error("expected an error when --attach-inline is used without --html")
	}
}

func TestPrepareInlineImagesEmbedsAndSetsContentID(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "logo.png")
	data := []byte("PNGBYTES")
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
	opts := &SendOptions{HTML: true, Body: "<p>hi</p>", InlineAttach: []string{p}}
	got, err := prepareInlineImages(opts, "sender@proton.me")
	if err != nil {
		t.Fatalf("prepareInlineImages: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d prepared, want 1", len(got))
	}
	a := got[0]
	if a.Filename != "logo.png" {
		t.Errorf("filename = %q, want logo.png", a.Filename)
	}
	if a.ContentID == "" {
		t.Error("ContentID must be set for an inline attachment")
	}
	if !strings.HasPrefix(a.MIMEType, "image/png") {
		t.Errorf("mime = %q, want image/png", a.MIMEType)
	}
	if string(a.Data) != string(data) {
		t.Errorf("data = %q, want %q", a.Data, data)
	}
	// The body keeps its original content and gains a cid: reference so the
	// image renders in place.
	if !strings.HasPrefix(opts.Body, "<p>hi</p>") {
		t.Errorf("original body should be preserved as a prefix, got %q", opts.Body)
	}
	if !strings.Contains(opts.Body, `src="cid:`+a.ContentID+`"`) {
		t.Errorf("body should reference the cid, got %q", opts.Body)
	}
}
