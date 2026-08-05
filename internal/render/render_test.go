package render

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestParseFormat(t *testing.T) {
	tests := []struct {
		in      string
		want    Format
		wantErr bool
	}{
		{"", FormatText, false},
		{"text", FormatText, false},
		{"json", FormatJSON, false},
		{"yaml", FormatYAML, false},
		{"yml", FormatYAML, false},
		{"toml", "", true},
	}
	for _, tc := range tests {
		got, err := ParseFormat(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseFormat(%q) error = %v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseFormat(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func newTestRenderer(t *testing.T, opts Options) (*Renderer, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	opts.Stdout, opts.Stderr = &stdout, &stderr
	opts.LogLevel = slog.LevelError
	return New(opts), &stdout, &stderr
}

func TestSuccessGoesToStderrPlainWhenNotATerminal(t *testing.T) {
	r, stdout, stderr := newTestRenderer(t, Options{Format: FormatText})
	r.Success("Message sent.")
	if stdout.Len() != 0 {
		t.Errorf("Success wrote %q to stdout, want nothing", stdout.String())
	}
	if got := stderr.String(); got != "✓ Message sent.\n" {
		t.Errorf("stderr = %q, want %q", got, "✓ Message sent.\n")
	}
}

func TestSuccessQuietWritesNothing(t *testing.T) {
	r, _, stderr := newTestRenderer(t, Options{Format: FormatText, Quiet: true})
	r.Success("Message sent.")
	if stderr.Len() != 0 {
		t.Errorf("quiet Success wrote %q", stderr.String())
	}
}

// The new ID belongs on stdout so `ID=$(proton-cli ... create ...)` works; the
// confirmation must stay on stderr.
func TestIDSplitsStreams(t *testing.T) {
	r, stdout, stderr := newTestRenderer(t, Options{Format: FormatText})
	r.ID("abc123", "Created event")
	if got := stdout.String(); got != "abc123\n" {
		t.Errorf("stdout = %q, want %q", got, "abc123\n")
	}
	if got := stderr.String(); got != "✓ Created event\n" {
		t.Errorf("stderr = %q, want %q", got, "✓ Created event\n")
	}
}

func TestIDQuietStillPrintsTheID(t *testing.T) {
	r, stdout, stderr := newTestRenderer(t, Options{Format: FormatText, Quiet: true})
	r.ID("abc123", "Created event")
	if got := stdout.String(); got != "abc123\n" {
		t.Errorf("stdout = %q, want %q", got, "abc123\n")
	}
	if stderr.Len() != 0 {
		t.Errorf("quiet ID wrote %q to stderr", stderr.String())
	}
}

func TestSuccessColorsTheMarkOnly(t *testing.T) {
	r, _, stderr := newTestRenderer(t, Options{Format: FormatText})
	r.Err = Colors{enabled: true, wide: true}
	r.Success("Uploaded report.pdf")
	want := "\x1b[38;2;53;177;145m✓\x1b[0m Uploaded report.pdf\n"
	if got := stderr.String(); got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

// Machine-readable output must never be colored, whatever the destination is.
func TestNewDisablesColorForNonTextFormats(t *testing.T) {
	for _, f := range []Format{FormatJSON, FormatYAML} {
		r, _, _ := newTestRenderer(t, Options{Format: f})
		if r.Out.Enabled() || r.Err.Enabled() {
			t.Errorf("format %q enabled color", f)
		}
	}
}

func TestNewDisablesColorWhenAsked(t *testing.T) {
	r, _, _ := newTestRenderer(t, Options{Format: FormatText, NoColor: true})
	if r.Out.Enabled() || r.Err.Enabled() {
		t.Error("NoColor did not disable color")
	}
}

func TestObjectJSONUsesConfiguredStream(t *testing.T) {
	r, stdout, _ := newTestRenderer(t, Options{Format: FormatJSON})
	if err := r.Object(map[string]string{"id": "x"}); err != nil {
		t.Fatalf("Object: %v", err)
	}
	if got := stdout.String(); got != "{\n  \"id\": \"x\"\n}\n" {
		t.Errorf("stdout = %q", got)
	}
}
