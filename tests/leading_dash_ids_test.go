package tests

import (
	"strings"
	"testing"
)

// dashedSyntheticID is a syntactically valid Proton ID (88 chars, URL-safe
// base64, ends "==") starting with '-'. Real APIs reject it as not-found,
// which is fine - these tests only assert that argument parsing succeeds.
const dashedSyntheticID = "-bJxDLEMvt-Z6t4Yna7V8SYQ_FIHWT2_QbBr-whe-bIE8rbZunzr5RhXGaihvQ43z2qcxcqFgVRwi7A=="

// assertNotFlagParseError fails the test if stderr looks like cobra's
// "unknown shorthand flag" complaint - i.e. arg parsing rejected the ID.
func assertNotFlagParseError(t *testing.T, stderr string) {
	t.Helper()
	for _, marker := range []string{
		"unknown shorthand flag",
		"unknown flag",
	} {
		if strings.Contains(stderr, marker) {
			t.Errorf("expected no flag-parse error, but stderr contains %q:\n%s", marker, truncateOutput(stderr))
		}
	}
}

// TestLeadingDashIDIsAccepted: each affected command parses cleanly when
// given a synthetic leading-dash ID. The API call may then fail with
// not-found / invalid-id (exit 3 or 1) - that's fine; we only assert
// that cobra doesn't reject the argument.
func TestLeadingDashIDIsAccepted(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"mail messages read", []string{"mail", "messages", "read", dashedSyntheticID}},
		{"mail messages trash", []string{"mail", "messages", "trash", dashedSyntheticID}},
		{"mail messages delete", []string{"mail", "messages", "delete", dashedSyntheticID}},
		{"mail messages star", []string{"mail", "messages", "star", dashedSyntheticID}},
		{"mail messages unstar", []string{"mail", "messages", "unstar", dashedSyntheticID}},
		{"mail messages move", []string{"mail", "messages", "move", "--dest", "archive", dashedSyntheticID}},
		{"mail messages mark read", []string{"mail", "messages", "mark", "read", dashedSyntheticID}},
		{"mail conversations read", []string{"mail", "conversations", "read", dashedSyntheticID}},
		{"mail conversations trash", []string{"mail", "conversations", "trash", dashedSyntheticID}},
		{"mail conversations delete", []string{"mail", "conversations", "delete", dashedSyntheticID}},
		{"mail conversations mark read", []string{"mail", "conversations", "mark", "read", dashedSyntheticID}},
		{"mail labels delete", []string{"mail", "labels", "delete", dashedSyntheticID}},
		{"mail filters delete", []string{"mail", "filters", "delete", dashedSyntheticID}},
		{"mail filters enable", []string{"mail", "filters", "enable", dashedSyntheticID}},
		{"calendar calendars delete", []string{"calendar", "calendars", "delete", dashedSyntheticID}},
		{"calendar events get", []string{"calendar", "events", "get", dashedSyntheticID, dashedSyntheticID}},
		{"calendar events delete", []string{"calendar", "events", "delete", dashedSyntheticID, dashedSyntheticID}},
		{"contacts get", []string{"contacts", "get", dashedSyntheticID}},
		{"contacts delete", []string{"contacts", "delete", dashedSyntheticID}},
		{"pass vaults delete", []string{"pass", "vaults", "delete", dashedSyntheticID}},
		{"pass items get", []string{"pass", "items", "get", dashedSyntheticID, dashedSyntheticID}},
		{"pass items delete", []string{"pass", "items", "delete", dashedSyntheticID, dashedSyntheticID}},
		{"drive trash restore", []string{"drive", "trash", "restore", dashedSyntheticID}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, stderr, _ := run(t, tc.args...)
			assertNotFlagParseError(t, stderr)
		})
	}
}

// TestLeadingDashIDWithFlagsBeforeParsesCleanly: `--format raw <leading-dash-id>`
// works because preprocessArgs inserts `--` before the ID after flag parsing
// has consumed `--format raw`.
func TestLeadingDashIDWithFlagsBeforeParsesCleanly(t *testing.T) {
	_, stderr, _ := run(t, "mail", "messages", "read", "--format", "raw", dashedSyntheticID)
	assertNotFlagParseError(t, stderr)
}

// TestLeadingDashIDWithFlagsAfterErrors: putting flags AFTER a leading-dash
// ID produces the rewrapped Layer-C hint. preprocessArgs auto-injects `--`
// before the ID, which makes any subsequent flag tokens positional;
// rewrapFlagError catches cobra's "accepts N arg(s)" error and explains
// the cause.
func TestLeadingDashIDWithFlagsAfterErrors(t *testing.T) {
	_, stderr, code := run(t, "mail", "messages", "read", dashedSyntheticID, "--format", "raw")
	if code == 0 {
		t.Errorf("expected non-zero exit, got 0; stderr=%s", stderr)
	}
	if !strings.Contains(stderr, "insert -- before it") {
		t.Errorf("expected stderr to contain 'insert -- before it', got:\n%s", stderr)
	}
}
