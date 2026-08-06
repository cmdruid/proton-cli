package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/roman-16/proton-cli/internal/progress"
)

// A progress bar is for a human watching a terminal. Everywhere else it must
// vanish, or it corrupts a log file and a JSON stream alike.
func TestNewProgressIsNopWhereItWouldBeNoise(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts Options
	}{
		{"stderr is not a terminal", Options{}},
		{"quiet", Options{Quiet: true}},
		{"machine output", Options{Format: FormatJSON}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u, _, errb := fixture(t, tc.opts)
			p := NewProgress(u)
			if _, isNop := p.(progress.Nop); !isNop {
				t.Errorf("want a no-op sink, got %T", p)
			}
			p.Start(100, "Uploading")
			p.Add(50)
			p.Done()
			if errb.Len() != 0 {
				t.Errorf("a no-op sink wrote %q", errb.String())
			}
		})
	}
}

// The bar itself, driven directly so the frames are deterministic.
func TestProgressFrames(t *testing.T) {
	var buf bytes.Buffer
	p := &Progress{w: &buf, active: true}

	p.Start(1000, "Uploading report.pdf")
	p.Add(500)
	p.Add(500)
	p.Done()

	frames := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\r")
	if len(frames) != 4 { // a leading empty field, then three redraws
		t.Fatalf("want 3 redraws, got %d: %q", len(frames)-1, buf.String())
	}
	for i, want := range []string{"0 B / 1000 B (0%)", "500 B / 1000 B (50%)", "1000 B / 1000 B (100%)"} {
		if !strings.HasSuffix(frames[i+1], want) {
			t.Errorf("frame %d: want suffix %q, got %q", i, want, frames[i+1])
		}
	}
	if !strings.HasPrefix(frames[1], "Uploading report.pdf ") {
		t.Errorf("the label should lead the bar: %q", frames[1])
	}
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Error("Done must close the line so the next output starts fresh")
	}
}

// Encryption adds per-block overhead, so the byte counter can overrun the source
// size. The bar reports the size the user asked about rather than going past
// 100%.
func TestProgressClampsOverrun(t *testing.T) {
	var buf bytes.Buffer
	p := &Progress{w: &buf, active: true}
	p.Start(100, "Uploading")
	p.Add(140)
	if !strings.HasSuffix(buf.String(), "100 B / 100 B (100%)") {
		t.Errorf("overrun not clamped: %q", buf.String())
	}
	// Only the final frame matters; the frame Start drew was legitimately empty.
	frames := strings.Split(buf.String(), "\r")
	last := frames[len(frames)-1]
	if strings.Contains(last, GlyphBarPending) {
		t.Errorf("a complete bar should have no pending segment: %q", last)
	}
	if strings.Count(last, GlyphBarFilled) != barWidth {
		t.Errorf("a complete bar should be entirely filled: %q", last)
	}
}

// An unknown total still has to draw without dividing by zero.
func TestProgressUnknownTotal(t *testing.T) {
	var buf bytes.Buffer
	p := &Progress{w: &buf, active: true}
	p.Start(0, "Uploading")
	p.Add(4096)
	if !strings.Contains(buf.String(), "(0%)") {
		t.Errorf("unknown total should report 0%%: %q", buf.String())
	}
}

func TestProgressDoneIsIdempotent(t *testing.T) {
	var buf bytes.Buffer
	p := &Progress{w: &buf, active: true}
	p.Start(10, "x")
	p.Done()
	before := buf.Len()
	p.Done()
	p.Add(5)
	if buf.Len() != before {
		t.Errorf("writing after Done: %q", buf.String()[before:])
	}
}
