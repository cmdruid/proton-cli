package confirm

import (
	"slices"
	"strings"
	"testing"
)

func TestParseReadsEveryPartOfADirective(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Directive
	}{
		{"mutations", Directive{Class: Mutations, Outcome: Ask}},
		{"*:mutations", Directive{Class: Mutations, Outcome: Ask}},
		{"pass:all", Directive{Path: []string{"pass"}, Class: All, Outcome: Ask}},
		{"deletions=deny", Directive{Class: Deletions, Outcome: Deny}},
		{"drive:all=deny", Directive{Path: []string{"drive"}, Class: All, Outcome: Deny}},
		{"mail drafts send:all", Directive{Path: []string{"mail", "drafts", "send"}, Class: All, Outcome: Ask}},
		{"  pass : all  ", Directive{Path: []string{"pass"}, Class: All, Outcome: Ask}},
	} {
		got, err := Parse(tc.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", tc.in, err)
			continue
		}
		if len(got) != 1 {
			t.Errorf("Parse(%q) gave %d directives, want 1", tc.in, len(got))
			continue
		}
		if !slices.Equal(got[0].Path, tc.want.Path) || got[0].Class != tc.want.Class || got[0].Outcome != tc.want.Outcome {
			t.Errorf("Parse(%q) = %+v, want %+v", tc.in, got[0], tc.want)
		}
	}
}

func TestParseReadsAListInOrder(t *testing.T) {
	got, err := Parse("mutations, pass:all, deletions=deny")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d directives, want 3", len(got))
	}
	if got[2].Outcome != Deny || got[2].Class != Deletions {
		t.Errorf("last directive = %+v", got[2])
	}
}

// Nothing written is a policy that demands nothing, which is the state of every
// install that has not configured one.
func TestParseTakesAnEmptyStringAsNoPolicy(t *testing.T) {
	for _, in := range []string{"", "   ", ",", " , ,"} {
		got, err := Parse(in)
		if err != nil || len(got) != 0 {
			t.Errorf("Parse(%q) = (%v, %v), want no policy", in, got, err)
		}
	}
}

// A directive that cannot mean anything is refused where it was written, rather
// than becoming a guard that silently covers nothing.
func TestParseRefusesWhatItCannotMean(t *testing.T) {
	for _, in := range []string{"nonsense", "mail:nonsense", "pass:", "mutations=nope", "MUTATIONS"} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) was accepted", in)
		}
	}
}

// The error offers the choice, because a mistyped class is most often a wrong
// guess at the vocabulary rather than a wrong idea.
func TestABadClassNamesTheOnesThereAre(t *testing.T) {
	_, err := Parse("mail:everything")
	if err == nil {
		t.Fatal("want an error")
	}
	for _, class := range Classes {
		if !strings.Contains(err.Error(), class.String()) {
			t.Errorf("error %q omits %q", err, class)
		}
	}
}

// The file's two maps and the one-line form say the same thing, so a policy can
// move between a shell and a configuration file unchanged.
func TestDocumentSaysWhatTheOneLineFormSays(t *testing.T) {
	doc := Document{
		Ask:  map[string]string{Everywhere: "mutations", "pass": "all"},
		Deny: map[string]string{Everywhere: "deletions", "drive": "all"},
	}
	fromDoc, err := doc.Source()
	if err != nil {
		t.Fatalf("Document.Source: %v", err)
	}
	fromLine, err := Parse("mutations, pass:all, deletions=deny, drive:all=deny")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cmds := []Command{
		{Path: []string{"mail", "drafts", "send"}, Mutating: true},
		{Path: []string{"pass", "items", "list"}},
		{Path: []string{"mail", "messages", "delete"}, Mutating: true, Irreversible: true},
		{Path: []string{"drive", "items", "list"}},
	}
	for _, cmd := range cmds {
		if a, b := (Policy{fromDoc}).Require(cmd), (Policy{fromLine}).Require(cmd); a != b {
			t.Errorf("%v: document says %v, one line says %v", cmd.Path, a, b)
		}
	}
}

// A bad class in a file is reported with the key that holds it, because a file
// has somewhere to point and a reader has to find it.
func TestDocumentErrorNamesTheKey(t *testing.T) {
	_, err := Document{Deny: map[string]string{"drive": "everything"}}.Source()
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "confirm.deny.drive") {
		t.Errorf("error %q does not name the key", err)
	}
}
