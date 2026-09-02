package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/cmdruid/proton-cli/internal/progress"
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

// bar returns a Progress drawing into a buffer, redrawing on every call so the
// frames are deterministic, and laid out for a terminal of the given width.
func bar(width int) (*Progress, *bytes.Buffer) {
	var buf bytes.Buffer
	return &Progress{w: &buf, active: true, width: func() int { return width }}, &buf
}

// frames splits what was drawn into the successive states of the line.
func frames(buf *bytes.Buffer) []string {
	out := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\r")
	for i, f := range out {
		out[i] = strings.ReplaceAll(f, clearToEOL, "")
	}
	return out[1:] // the leading field before the first carriage return is empty
}

func TestProgressFrames(t *testing.T) {
	p, buf := bar(80)
	p.Start(1000, "Uploading report.pdf")
	p.Add(500)
	p.Add(500)
	p.Done()

	got := frames(buf)
	if len(got) != 4 { // start, two additions, and the final frame Done draws
		t.Fatalf("want 4 frames, got %d: %q", len(got), buf.String())
	}
	for i, want := range []string{
		"  0%  0 B / 1000 B",
		" 50%  500 B / 1000 B",
		"100%  1000 B / 1000 B",
		"100%  1000 B / 1000 B",
	} {
		if !strings.Contains(got[i], want) {
			t.Errorf("frame %d: want %q in %q", i, want, got[i])
		}
		if !strings.HasPrefix(got[i], "Uploading report.pdf") {
			t.Errorf("frame %d: the label should lead the line: %q", i, got[i])
		}
	}
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Error("Done must close the line so the next output starts fresh")
	}
}

// A shorter line has to erase the longer one it replaces, or the tail of the old
// frame stays on screen.
func TestProgressErasesTheRestOfTheLine(t *testing.T) {
	p, buf := bar(80)
	p.Start(1000, "Uploading")
	if !strings.Contains(buf.String(), clearToEOL) {
		t.Errorf("a redraw must clear to end of line: %q", buf.String())
	}
}

// Encryption adds per-block overhead, so the byte counter can overrun the source
// size. The bar reports the size the user asked about rather than going past
// 100%.
func TestProgressClampsOverrun(t *testing.T) {
	p, buf := bar(80)
	p.Start(100, "Uploading")
	p.Add(140)

	last := frames(buf)[1]
	if !strings.Contains(last, "100%  100 B / 100 B") {
		t.Errorf("overrun not clamped: %q", last)
	}
	if strings.Contains(last, GlyphBarPending) {
		t.Errorf("a complete bar should have no pending segment: %q", last)
	}
	if strings.Count(last, GlyphBarFilled) != barWidth {
		t.Errorf("a complete bar should be entirely filled: %q", last)
	}
}

// An unknown total still has to draw without dividing by zero, and says how much
// has arrived rather than pretending to know how much is coming.
func TestProgressUnknownTotal(t *testing.T) {
	p, buf := bar(80)
	p.Start(0, "Uploading")
	p.Add(4096)

	last := frames(buf)[1]
	if !strings.Contains(last, "  0%") || !strings.Contains(last, "4.0 KB") {
		t.Errorf("unknown total should report 0%% and the bytes so far: %q", last)
	}
	if strings.Contains(last, " / ") {
		t.Errorf("unknown total must not claim a size: %q", last)
	}
}

// The line adapts to the terminal rather than wrapping, giving up the estimate
// first and the label last.
func TestProgressFitsTheTerminal(t *testing.T) {
	for _, width := range []int{120, 80, 60, 40, 30, 20, 12} {
		p, buf := bar(width)
		p.Start(2_400_000_000, "Uploading a-rather-long-file-name.tar.gz")
		p.Add(500_000_000)
		for i, f := range frames(buf) {
			if n := Cells(stripANSI(f)); n >= width {
				t.Errorf("width %d, frame %d: line is %d cells: %q", width, i, n, f)
			}
		}
	}
}

// A rate needs enough history to be a measurement rather than an extrapolation
// from two readings a microsecond apart.
func TestProgressWithholdsARateUntilItMeansSomething(t *testing.T) {
	p, buf := bar(120)
	p.Start(1000, "Uploading")
	p.Add(100)
	if strings.Contains(buf.String(), "/s") {
		t.Errorf("a rate claimed too early: %q", buf.String())
	}

	p.samples = []sample{{time.Now().Add(-2 * time.Second), 0}, {time.Now(), 200}}
	if r := p.rate(); r < 90 || r > 110 {
		t.Errorf("rate over a real window = %v, want about 100", r)
	}
}

func TestProgressDoneIsIdempotent(t *testing.T) {
	p, buf := bar(80)
	p.Start(10, "x")
	p.Done()
	before := buf.Len()
	p.Done()
	p.Add(5)
	if buf.Len() != before {
		t.Errorf("writing after Done: %q", buf.String()[before:])
	}
}
