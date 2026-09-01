package kit

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/roman-16/proton-cli/internal/app"
	"github.com/roman-16/proton-cli/internal/confirm"
	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/spf13/cobra"
)

// The gate runs before a command does anything, so what it is checked on is
// whether the body ran at all.

// tree builds a command whose path and verb are what the policy reads.
func tree(verb string, body func() error) *cobra.Command {
	root := &cobra.Command{Use: Program}
	mail := &cobra.Command{Use: "mail"}
	messages := &cobra.Command{Use: "messages"}
	leaf := &cobra.Command{Use: verb, RunE: Run(nil, func(*Invocation) error { return body() })}
	messages.AddCommand(leaf)
	mail.AddCommand(messages)
	root.AddCommand(mail)
	return leaf
}

// gated runs a leaf under a policy and reports whether the body ran.
func gated(t *testing.T, policy, verb, answer string, yes bool) (ran bool, out string, err error) {
	t.Helper()
	source, parseErr := confirm.Parse(policy)
	if parseErr != nil {
		t.Fatalf("Parse(%q): %v", policy, parseErr)
	}
	var errb bytes.Buffer
	a := &app.App{
		Yes:     yes,
		Confirm: confirm.Policy{source},
		UI: ui.New(ui.Options{
			Out: &bytes.Buffer{}, Err: &errb,
			In:      strings.NewReader(answer),
			NoInput: answer == noTerminal,
		}),
	}
	leaf := tree(verb, func() error { ran = true; return nil })
	leaf.SetContext(app.WithApp(context.Background(), a))
	err = leaf.RunE(leaf, nil)
	return ran, errb.String(), err
}

// A denied command does not run, and says so with a code of its own.
func TestADeniedCommandNeverRuns(t *testing.T) {
	ran, _, err := gated(t, "reads=deny", "list", "y\n", false)
	if ran {
		t.Error("the body ran despite a deny")
	}
	var coder errs.ExitCoder
	if err == nil || !errors.As(err, &coder) || coder.ExitCode() != ExitRefused {
		t.Errorf("err = %v, want exit %d", err, ExitRefused)
	}
}

// --yes answers a question. A deny is not one, so it answers nothing.
func TestYesDoesNotUnlockADeny(t *testing.T) {
	if ran, _, _ := gated(t, "reads=deny", "list", "y\n", true); ran {
		t.Error("--yes let a denied command through")
	}
}

// The refusal names no way around itself: the reader is stopped, not stuck, and
// the sentence must not hand whatever ran the command the edit that lifts it.
func TestARefusalCarriesNoRemedy(t *testing.T) {
	_, _, err := gated(t, "reads=deny", "list", "", false)
	var hinter errs.Hinter
	if errors.As(err, &hinter) && len(hinter.Hints()) > 0 {
		t.Errorf("a refusal offered %v", hinter.Hints())
	}
}

// A command that changes nothing is stopped where it stands, because there is
// nothing to resolve first and nothing better to say about it than its name.
func TestAReadIsAskedAboutByName(t *testing.T) {
	ran, out, err := gated(t, "reads", "list", "y\n", false)
	if !ran || err != nil {
		t.Fatalf("a yes should let it run: ran=%v err=%v", ran, err)
	}
	if want := "Would run " + Program + " mail messages list. Continue?"; !strings.Contains(out, want) {
		t.Errorf("prompt was %q, want it to contain %q", out, want)
	}
}

// Anything but a plain yes means no, and the command does not run.
func TestAReadIsNotRunWithoutAYes(t *testing.T) {
	for _, answer := range []string{"\n", "n\n", "no\n", "maybe\n"} {
		if ran, _, _ := gated(t, "reads", "list", answer, false); ran {
			t.Errorf("%q let the command run", answer)
		}
	}
}

// With nobody to ask, the question becomes an error that says what to add.
func TestAReadWithNobodyToAskIsRefused(t *testing.T) {
	ran, _, err := gated(t, "all", "list", noTerminal, false)
	if ran {
		t.Error("the body ran with nobody to ask")
	}
	var hinter errs.Hinter
	if !errors.As(err, &hinter) || !strings.Contains(strings.Join(hinter.Hints(), " "), "--yes") {
		t.Errorf("err = %v, want a hint naming --yes", err)
	}
}

// --yes is the answer given in advance, which is what an unattended run uses.
func TestYesAnswersTheQuestionInAdvance(t *testing.T) {
	if ran, _, err := gated(t, "all", "list", noTerminal, true); !ran || err != nil {
		t.Errorf("--yes should have let it run: ran=%v err=%v", ran, err)
	}
}

// A mutation is let past the gate so that it can be asked about where the filter
// has run and the question can name what it would touch.
func TestAMutationIsNotStoppedAtTheGate(t *testing.T) {
	if ran, _, err := gated(t, "mutations", "delete", noTerminal, false); !ran || err != nil {
		t.Errorf("a mutation should reach its body: ran=%v err=%v", ran, err)
	}
}

// With no policy at all, nothing about a command changes.
func TestNoPolicyLetsEverythingThrough(t *testing.T) {
	for _, verb := range []string{"list", "delete"} {
		if ran, _, err := gated(t, "", verb, noTerminal, false); !ran || err != nil {
			t.Errorf("%s: ran=%v err=%v", verb, ran, err)
		}
	}
}

// The classification is read off the verb, which the grammar already declares.
func TestClassifyReadsTheVerb(t *testing.T) {
	for _, tc := range []struct {
		verb                   string
		mutating, irreversible bool
	}{
		{"list", false, false},
		{"send", true, false},
		{"delete", true, true},
		{"empty", true, true},
	} {
		got := classify(tree(tc.verb, nil))
		if got.Mutating != tc.mutating || got.Irreversible != tc.irreversible {
			t.Errorf("%s: %+v, want mutating=%v irreversible=%v",
				tc.verb, got, tc.mutating, tc.irreversible)
		}
		if want := []string{"mail", "messages", tc.verb}; strings.Join(got.Path, " ") != strings.Join(want, " ") {
			t.Errorf("%s: path %v, want %v", tc.verb, got.Path, want)
		}
	}
}
