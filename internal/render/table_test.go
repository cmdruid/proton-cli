package render

import (
	"bytes"
	"strings"
	"testing"
)

func TestTableLayout(t *testing.T) {
	var buf bytes.Buffer
	Table(&buf, []string{"ID", "NAME"}, [][]string{
		{"abc", "long value here"},
		{"de", "x"},
	}, TableStyle{})

	want := strings.Join([]string{
		"ID   NAME           ",
		"───  ───────────────",
		"abc  long value here",
		"de   x              ",
		"",
	}, "\n")
	if buf.String() != want {
		t.Errorf("Table output =\n%q\nwant\n%q", buf.String(), want)
	}
}

func TestTableColumnGrowsToFitContent(t *testing.T) {
	var buf bytes.Buffer
	Table(&buf, []string{"N"}, [][]string{{"xxxxx"}}, TableStyle{})
	lines := strings.Split(buf.String(), "\n")
	if lines[0] != "N    " || lines[2] != "xxxxx" {
		t.Errorf("header/cell = %q/%q, want %q/%q", lines[0], lines[2], "N    ", "xxxxx")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		in   string
		max  int
		want string
	}{
		{"abc", 5, "abc"},
		{"abcde", 5, "abcde"},
		{"abcdef", 5, "abcd…"},
		{"abcdef", 3, "abc"},
		{"日本語テキスト", 4, "日本語…"},
	}
	for _, tc := range tests {
		if got := truncate(tc.in, tc.max); got != tc.want {
			t.Errorf("truncate(%q,%d) = %q, want %q", tc.in, tc.max, got, tc.want)
		}
	}
}

func TestTableMissingCellsArePadded(t *testing.T) {
	var buf bytes.Buffer
	Table(&buf, []string{"A", "B"}, [][]string{{"1"}}, TableStyle{})
	if got := strings.Split(buf.String(), "\n")[2]; got != "1   " {
		t.Errorf("row with missing cell = %q, want %q", got, "1   ")
	}
}

// Styling is applied after layout, so removing the escapes must reproduce the
// unstyled table byte for byte.
func TestTableStyleDoesNotAffectLayout(t *testing.T) {
	headers := []string{"ID", "FROM", "⚑"}
	rows := [][]string{
		{"5bH2mQxK", "Fastmail Billing", "●"},
		{"Kd9rTp1a", "Proton", ""},
	}
	c := Colors{enabled: true, wide: true}

	var plain, styled bytes.Buffer
	Table(&plain, headers, rows, TableStyle{})
	Table(&styled, headers, rows, TableStyle{
		Header: c.Hint,
		Rule:   c.Rule,
		Cells:  []func(string) string{c.ID, nil, c.Accent},
	})

	if !strings.Contains(styled.String(), "\x1b[") {
		t.Fatal("styled table contains no escape sequences")
	}
	if got := stripANSI(styled.String()); got != plain.String() {
		t.Errorf("stripped styled table =\n%q\nwant\n%q", got, plain.String())
	}
}

// An empty cell is whitespace only, so wrapping it in escape sequences would
// add bytes with nothing to show.
func TestTableStyleSkipsEmptyCells(t *testing.T) {
	c := Colors{enabled: true, wide: true}
	var buf bytes.Buffer
	Table(&buf, []string{"ID", "⚑"}, [][]string{{"abc", ""}}, TableStyle{
		Cells: []func(string) string{c.ID, c.Accent},
	})
	row := strings.Split(buf.String(), "\n")[2]
	if strings.Count(row, "\x1b[") != 2 {
		t.Errorf("row = %q, want escapes only around the ID cell", row)
	}
}

func TestTableEmptyRowsPrintsNothing(t *testing.T) {
	var buf bytes.Buffer
	Table(&buf, []string{"ID"}, nil, TableStyle{})
	if buf.Len() != 0 {
		t.Errorf("Table with no rows wrote %q to the table writer", buf.String())
	}
}
