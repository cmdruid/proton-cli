package contentline

import (
	"strings"
	"testing"
)

func TestUnfoldJoinsContinuations(t *testing.T) {
	got := Unfold("SUMMARY:a very\r\n  long line\r\nUID:x\r\n")
	want := []string{"SUMMARY:a very long line", "UID:x"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Unfold = %q, want %q", got, want)
	}
}

func TestUnfoldAcceptsBareNewlines(t *testing.T) {
	// Cards arrive CRLF-joined from Proton, but a value that came from a file or
	// another client may not.
	if got := Unfold("UID:x\nSUMMARY:y"); len(got) != 2 {
		t.Errorf("Unfold = %q, want two lines", got)
	}
}

func TestParseSplitsGroupNameParamsAndValue(t *testing.T) {
	l, ok := Parse("item1.EMAIL;PREF=1;TYPE=work:jane@example.test")
	if !ok {
		t.Fatal("Parse refused a well-formed line")
	}
	if l.Group != "item1" || l.Name != "EMAIL" || l.Value != "jane@example.test" {
		t.Errorf("Parse = %+v", l)
	}
	if l.Params.Get("pref") != "1" || l.Params.Get("TYPE") != "work" {
		t.Errorf("params = %+v", l.Params)
	}
}

func TestParseKeepsAColonInsideAQuotedParameter(t *testing.T) {
	l, ok := Parse(`DTSTART;TZID="Etc/GMT:0":20260416T090000`)
	if !ok {
		t.Fatal("Parse refused a quoted parameter")
	}
	if l.Params.Get("TZID") != "Etc/GMT:0" || l.Value != "20260416T090000" {
		t.Errorf("Parse = %+v", l)
	}
}

func TestParseAllSkipsTheComponentDelimiters(t *testing.T) {
	lines := ParseAll("BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:x\r\nEND:VEVENT\r\nEND:VCALENDAR")
	if len(lines) != 1 || lines[0].Name != "UID" {
		t.Errorf("ParseAll = %+v, want just UID", lines)
	}
}

func TestRenderFoldsLongLinesWithoutSplittingARune(t *testing.T) {
	// Every continuation has to stay a valid UTF-8 string, or the value a reader
	// reassembles is not the value that was written.
	value := strings.Repeat("ä", 100)
	out := Render([]Line{{Name: "DESCRIPTION", Value: value}})
	for _, line := range strings.Split(out, "\r\n") {
		if len(line) > maxOctets+1 {
			t.Errorf("line is %d octets: %q", len(line), line)
		}
	}
	unfolded := Unfold(out)
	if len(unfolded) != 1 || unfolded[0] != "DESCRIPTION:"+value {
		t.Errorf("folding did not survive a round trip")
	}
}

func TestEscapeTextEncodesTheFourEscapes(t *testing.T) {
	// A comma left raw turns one value into two, which is how a description
	// silently loses everything after the first comma.
	got := EscapeText("a,b;c\\d\r\ne")
	if got != `a\,b\;c\\d\ne` {
		t.Errorf("EscapeText = %q", got)
	}
	if back := UnescapeText(got); back != "a,b;c\\d\ne" {
		t.Errorf("UnescapeText = %q", back)
	}
}

func TestSplitListSplitsOnUnescapedCommasOnly(t *testing.T) {
	got := SplitList(`20260416T090000,20260423T090000`)
	if len(got) != 2 {
		t.Errorf("SplitList = %q, want two dates", got)
	}
	if got := SplitList(`a\,b`); len(got) != 1 {
		t.Errorf("SplitList split on an escaped comma: %q", got)
	}
}
