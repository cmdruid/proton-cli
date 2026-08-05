package render

import (
	"bytes"
	"strings"
	"testing"
)

// drawTo renders one frame to a buffer, bypassing the TTY check that Start
// would otherwise fail in tests.
func drawTo(total, current int64, label string) string {
	var buf bytes.Buffer
	p := &Progress{Total: total, Label: label, Writer: &buf, current: current, active: true}
	p.draw()
	return buf.String()
}

func TestProgressFrame(t *testing.T) {
	got := drawTo(100, 50, "Uploading report.pdf")
	want := "\rUploading report.pdf [===============>              ] 50 B / 100 B (50%)"
	if got != want {
		t.Errorf("frame =\n%q\nwant\n%q", got, want)
	}
}

// The byte counter must never claim more than the total: encryption adds
// per-block overhead, so the transferred count can exceed the source size.
func TestProgressClampsOvershoot(t *testing.T) {
	got := drawTo(31, 82, "Uploading trail-map.txt")
	if !strings.Contains(got, "31 B / 31 B (100%)") {
		t.Errorf("frame = %q, want the counter clamped to the total", got)
	}
	if strings.Contains(got, "82 B") {
		t.Errorf("frame = %q, leaked the encrypted byte count", got)
	}
}

func TestProgressUnknownTotalDrawsEmptyBar(t *testing.T) {
	got := drawTo(0, 500, "Uploading -")
	if !strings.Contains(got, "500 B / 0 B (0%)") {
		t.Errorf("frame = %q, want the raw counter when the total is unknown", got)
	}
}

func TestProgressInactiveWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	p := &Progress{Total: 10, Writer: &buf, current: 5}
	p.draw()
	p.Add(5)
	p.Finish()
	if buf.Len() != 0 {
		t.Errorf("inactive progress wrote %q", buf.String())
	}
}

func TestProgressQuietStaysInactiveOnATerminal(t *testing.T) {
	var buf bytes.Buffer
	p := &Progress{Total: 10, Writer: &buf, Quiet: true}
	p.Start()
	p.Set(5)
	if buf.Len() != 0 {
		t.Errorf("quiet progress wrote %q", buf.String())
	}
}

func TestProgressAddAccumulates(t *testing.T) {
	var buf bytes.Buffer
	p := &Progress{Total: 100, Label: "x", Writer: &buf, active: true}
	p.Add(30)
	p.Add(20)
	if !strings.Contains(buf.String(), "50 B / 100 B (50%)") {
		t.Errorf("frames = %q, want a cumulative counter", buf.String())
	}
}

func TestProgressFinishEndsTheLine(t *testing.T) {
	var buf bytes.Buffer
	p := &Progress{Total: 10, Writer: &buf, active: true}
	p.Finish()
	if buf.String() != "\n" {
		t.Errorf("Finish wrote %q, want a newline", buf.String())
	}
}
