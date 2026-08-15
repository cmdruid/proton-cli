package ui

import (
	"strings"
	"testing"
)

func TestCells(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"ascii", "Invoice #2291", 13},
		{"latin-1 accents", "Rechnung für Grüße", 18},
		{"the glyph vocabulary is narrow", GlyphSuccess + GlyphUnread + GlyphStarred, 3},
		{"box drawing is narrow", strings.Repeat(GlyphRule, 8), 8},
		{"japanese is two cells a rune", "請求書", 6},
		{"chinese is two cells a rune", "报告", 4},
		{"korean is two cells a rune", "청구서", 6},
		{"mixed script", "請求書 2291", 11},
		{"fullwidth forms", "ＡＢＣ", 6},
		{"emoji is two cells", "🎉", 2},
		{"emoji among text", "🎉 Party", 8},
		{"combining marks add nothing", "e\u0301", 1},
		{"zero width space adds nothing", "a\u200Bb", 2},
		{"presentation selector widens", "\u2764\uFE0F", 2},
		{"presentation selector never doubles a wide rune", "🎉\uFE0F", 2},
		{"a joined sequence is one glyph", "👨\u200D👩\u200D👧", 2},
		{"text after a joined sequence still counts", "👨\u200D👩 ok", 5},
		{"regional indicators pair up", "🇦🇹", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Cells(tc.in); got != tc.want {
				t.Errorf("Cells(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// Padding is what makes a column a column, so every padded string has to come
// out at exactly the requested width whatever script it is written in.
func TestPadCellsReachesTheRequestedWidth(t *testing.T) {
	for _, s := range []string{"", "a", "Invoice", "請求書", "🎉 Party", "e\u0301", "ＡＢ"} {
		for _, w := range []int{0, 1, 4, 12, 30} {
			for _, right := range []bool{false, true} {
				got := padCells(s, w, right)
				want := w
				if n := Cells(s); n > w {
					want = n
				}
				if n := Cells(got); n != want {
					t.Errorf("padCells(%q, %d, %v) is %d cells, want %d", s, w, right, n, want)
				}
			}
		}
	}
}

// Truncation has to land on the budget exactly. A two-cell rune straddling the
// limit is dropped whole, and the cell it vacates is padded, so the column keeps
// its width rather than coming up one short.
func TestTruncateCellsNeverExceedsOrUndershootsTheBudget(t *testing.T) {
	for _, s := range []string{"Invoice #2291 is ready", "請求書のご送付について", "🎉 Party time", "ＡＢＣＤＥ"} {
		for max := 0; max <= Cells(s)+2; max++ {
			got := truncateCells(s, max)
			n := Cells(got)
			if n > max {
				t.Errorf("truncateCells(%q, %d) = %q, which is %d cells", s, max, got, n)
			}
			if max < Cells(s) && n != max {
				t.Errorf("truncateCells(%q, %d) = %q, which is %d cells; a truncated cell should fill its column", s, max, got, n)
			}
		}
	}
}

func TestTruncateCellsLeavesShortTextAlone(t *testing.T) {
	for _, s := range []string{"short", "請求書"} {
		if got := truncateCells(s, 40); got != s {
			t.Errorf("truncateCells(%q, 40) = %q, want it untouched", s, got)
		}
	}
}

// A cut must never split a character, which would emit invalid UTF-8 into the
// terminal.
func TestTruncateCellsNeverSplitsARune(t *testing.T) {
	s := "請求書のご送付について"
	for max := 1; max <= Cells(s); max++ {
		got := strings.TrimSuffix(truncateCells(s, max), "…")
		if !strings.HasPrefix(s, strings.TrimRight(got, " ")) {
			t.Errorf("truncateCells(%q, %d) = %q, which is not a prefix", s, max, got)
		}
	}
}
