package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cmdruid/proton-cli/internal/errs"
)

func TestWriteErrorShapes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		golden string
	}{{
		"a bare problem",
		errs.Problemf("--format accepts: text, html, raw"),
		"errs_enum",
	}, {
		"a problem with a remedy",
		errs.Problemf("Nothing selected.").
			Hint("pass a REF, or a filter such as --unread, --from, --older-than.",
				"Use --all to target a whole folder."),
		"errs_nothing_selected",
	}, {
		"a redirect to the right command",
		errs.Problemf("That ID is a conversation, not a message.").
			Hint("proton mail conversations get 5bH2mQxK").Exit(3),
		"errs_wrong_table",
	}, {
		"not found",
		&errs.NotFound{Kind: "message", Ref: "Invoice #9999"},
		"errs_not_found",
	}, {
		"ambiguous, with the candidates listed",
		&errs.Ambiguous{Kind: "contact", Ref: "jane", Candidates: twoContacts},
		"errs_ambiguous",
	}, {
		"a wrapped error from elsewhere keeps its chain",
		fmt.Errorf("upload report.pdf: %w", errors.New("open report.pdf: no such file or directory")),
		"errs_wrapped",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			u, out, errb := fixture(t, Options{})
			u.Fail(tc.err)
			if out.Len() != 0 {
				t.Errorf("an error belongs on stderr, got %q on stdout", out.String())
			}
			check(t, tc.golden, out, errb)
		})
	}
}

// The candidates a listing offers are IDs, so they are written the way a listing
// writes them - but only while that still tells them apart. A short ID that
// matched several is the case where it cannot: shortening them all would print
// the same token twice and answer nothing.
func TestAmbiguousShortensCandidatesWhileTheyStayDistinct(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    *errs.Ambiguous
		golden string
	}{{
		"distinct once shortened",
		&errs.Ambiguous{Kind: "contact", Ref: "jane", Candidates: twoContacts},
		"errs_ambiguous_short",
	}, {
		"matched by a short ID they share",
		&errs.Ambiguous{Kind: "cached ID", Ref: "abcd1234", Candidates: []errs.Candidate{
			{ID: "abcd1234FIRSTabc" + idTail + "=="},
			{ID: "abcd1234SECONDab" + idTail + "=="},
		}},
		"errs_ambiguous_collides",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			u, out, errb := fixture(t, Options{})
			WriteError(u.Err, tc.err, u.ErrStyle(), true)
			check(t, tc.golden, out, errb)
		})
	}
}

const idTail = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

var twoContacts = []errs.Candidate{
	{ID: "7Kd91mQxT9wLpN4v" + idTail + "==", Label: "jane@example.com"},
	{ID: "3Ns8pT2vSJf2oHzH" + idTail + "==", Label: "jane.roe@work.example"},
}

// Messages are sentences wherever they are printed: capitalised, and closed.
func TestProblemNormalisesToASentence(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"nothing selected", "Nothing selected."},
		{"Nothing selected.", "Nothing selected."},
		// Anything the user typed keeps its case: capitalising it would misquote
		// the flag, key or path being complained about.
		{"--format accepts: text, html, raw", "--format accepts: text, html, raw."},
		{"week-start accepts: locale, monday", "week-start accepts: locale, monday."},
		{"page_size must be positive", "page_size must be positive."},
		{"/Documents/report.pdf already exists", "/Documents/report.pdf already exists."},
		{"is this right?", "Is this right?"},
	} {
		if got := errs.Problemf("%s", tc.in).Error(); got != tc.want {
			t.Errorf("Problemf(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The exit code travels with the error, so the shell sees the same
// classification the message describes.
func TestErrorExitCodes(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{errs.Problemf("bad flag"), 1},
		{errs.Problemf("locked").Exit(2), 2},
		{&errs.NotFound{Kind: "message", Ref: "x"}, 3},
		{&errs.Ambiguous{Kind: "contact", Ref: "j"}, 4},
		{errs.WithExit(5, errors.New("timeout")), 5},
	} {
		var coder errs.ExitCoder
		if !errors.As(tc.err, &coder) {
			t.Fatalf("%v carries no exit code", tc.err)
		}
		if got := coder.ExitCode(); got != tc.want {
			t.Errorf("%v: exit %d, want %d", tc.err, got, tc.want)
		}
	}
}

// A remedy has to be reachable through wrapping, or adding context to an error
// silently discards the advice.
func TestHintsSurviveWrapping(t *testing.T) {
	inner := errs.Problemf("Nothing selected.").Hint("pass a REF")
	wrapped := fmt.Errorf("trash messages: %w", inner)
	var hinter errs.Hinter
	if !errors.As(wrapped, &hinter) {
		t.Fatal("hints lost through wrapping")
	}
	if len(hinter.Hints()) != 1 || !strings.Contains(hinter.Hints()[0], "pass a REF") {
		t.Errorf("unexpected hints: %v", hinter.Hints())
	}
}
