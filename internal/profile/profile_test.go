package profile

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAcceptsOrdinaryNames(t *testing.T) {
	for _, in := range []string{"default", "work", "my-work.2", "a", "A1_b-c.d", "personal2"} {
		n, err := Parse(in)
		if err != nil {
			t.Errorf("Parse(%q) = %v", in, err)
			continue
		}
		if n.String() != in {
			t.Errorf("Parse(%q).String() = %q", in, n.String())
		}
	}
}

func TestParseEmptySelectsTheDefault(t *testing.T) {
	n, err := Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if n.String() != Default || !n.IsDefault() {
		t.Errorf("Parse(\"\") = %q", n.String())
	}
}

// A profile names a file. Every one of these would put that file somewhere other
// than the directory it belongs in, or make a name no filesystem accepts.
func TestParseRefusesAnythingThatCouldEscapeItsDirectory(t *testing.T) {
	for _, in := range []string{
		"..",
		".",
		"../../etc/passwd",
		"..\\..\\windows",
		"work/../../elsewhere",
		"/absolute",
		"C:\\absolute",
		"has/separator",
		"has\\separator",
		".hidden",
		"-leading-dash",
		"_leading-underscore",
		"with space",
		"with\x00null",
		"with\nnewline",
		"tab\there",
		"emoji🎉",
		strings.Repeat("a", maxBytes+1),
	} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) was accepted", in)
		}
	}
}

func TestFileNameIsAlwaysASinglePathElement(t *testing.T) {
	n, err := Parse("my-work.2")
	if err != nil {
		t.Fatal(err)
	}
	file := n.FileName(".json")
	if dir, base := filepath.Split(file); dir != "" || base != file {
		t.Errorf("FileName = %q, which is not a single element", file)
	}
}

func TestNamesReportsTheFirstNameThatIsNotOne(t *testing.T) {
	if _, err := Names([]string{"work", "../escape", "other"}); err == nil {
		t.Fatal("Names accepted a name that escapes its directory")
	}
	got, err := Names([]string{" work ", "other"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].String() != "work" {
		t.Errorf("Names = %v", got)
	}
}

func TestErrorMessagesDoNotEchoAControlCharacter(t *testing.T) {
	_, err := Parse("a\x07b")
	if err == nil {
		t.Fatal("Parse accepted a control character")
	}
	if strings.ContainsRune(err.Error(), 0x07) {
		t.Errorf("the error puts a control character on the terminal: %q", err.Error())
	}
}

func TestEqualComparesTheSelectedProfile(t *testing.T) {
	empty, _ := Parse("")
	named, _ := Parse("default")
	if !empty.Equal(named) {
		t.Error("the empty name and \"default\" select different profiles")
	}
	other, _ := Parse("work")
	if empty.Equal(other) {
		t.Error("different names compare equal")
	}
}
