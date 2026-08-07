package tests

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"
)

// The response contract, across every app: the envelope shape, the split between
// the streams, the exit codes, and what --dry-run does and does not do. It is one
// file because it is one contract.

// ── --output json: field names use json tags (snake_case) ──

func TestOutputJSONMailMessages(t *testing.T) {
	data := runJSON(t, "mail", "messages", "list", "--page-size", "1")
	if _, ok := data["messages"]; !ok {
		t.Fatal("expected 'messages' key (json tag) in output")
	}
	if _, ok := data["total"]; !ok {
		t.Fatal("expected 'total' key")
	}
}

func TestOutputJSONContacts(t *testing.T) {
	contacts := runJSONArray(t, "contacts", "list")
	if len(contacts) == 0 {
		t.Skip("no contacts")
	}
	c := contacts[0].(map[string]interface{})
	// 'id' and 'name' are json-tagged, not 'ID'/'Name'
	if _, ok := c["id"]; !ok {
		t.Errorf("expected 'id' json key; got %v", keysOf(c))
	}
}

// ── --output yaml: respects json tags, uses snake_case ──

func TestOutputYAMLSnakeCase(t *testing.T) {
	stdout := runOK(t, "mail", "messages", "list", "--page-size", "1", "--output", "yaml")
	// Non-omitempty keys only; from_name drops out when the sender has no display name.
	for _, want := range []string{"from_address", "num_attachments"} {
		if !strings.Contains(stdout, want+":") {
			t.Errorf("expected YAML key %q, got:\n%s", want, truncateOutput(stdout))
		}
	}
	// And NOT the Go-field lowercased alternatives
	for _, bad := range []string{"fromaddress:", "fromname:", "numattachments:"} {
		if strings.Contains(stdout, bad) {
			t.Errorf("unexpected YAML key %q (indicates yaml lib ignored json tags)", bad)
		}
	}
}

// ── --output yaml: raw api path keeps integers as integers ──

func TestOutputYAMLRawAPIKeepsIntegers(t *testing.T) {
	stdout := runOK(t, "--output", "yaml", "api", "GET", "/core/v4/users")
	// Code: 1000 (int) rather than 1000.0
	intRe := regexp.MustCompile(`(?m)^Code:\s+\d+$`)
	floatRe := regexp.MustCompile(`(?m)^Code:\s+\d+\.\d+`)
	if !intRe.MatchString(stdout) {
		t.Errorf("expected integer Code in YAML output, got:\n%s", truncateOutput(stdout))
	}
	if floatRe.MatchString(stdout) {
		t.Error("Code rendered as float; json.Number conversion regressed")
	}
}

// ── --output text (default): human-readable ──

func TestOutputTextIsDefault(t *testing.T) {
	stdout := runOK(t, "mail", "messages", "list", "--page-size", "1")
	// Table output has a separator line with ─ chars
	if !strings.Contains(stdout, "─") {
		t.Error("expected table output by default")
	}
	// And NOT a JSON brace
	if strings.HasPrefix(strings.TrimSpace(stdout), "{") {
		t.Error("default output looks like JSON")
	}
}

// ── invalid --output is rejected ──

func TestOutputUnknownFormat(t *testing.T) {
	_, stderr, code := run(t, "--output", "xml", "mail", "messages", "list")
	if code == 0 {
		t.Error("expected non-zero exit for unknown --output")
	}
	_ = stderr
}

// ── JSON output parses as valid JSON across many commands ──

func TestOutputJSONParsesEverywhere(t *testing.T) {
	cases := [][]string{
		{"mail", "messages", "list", "--page-size", "1"},
		{"mail", "settings", "labels", "list"},
		{"mail", "settings", "addresses", "list"},
		{"contacts", "list"},
		{"calendar", "settings", "calendars", "list"},
		{"pass", "vaults", "list"},
	}
	for _, args := range cases {
		stdout := runOK(t, append(args, "--output", "json")...)
		var v any
		if err := json.Unmarshal([]byte(stdout), &v); err != nil {
			t.Errorf("%v: not valid JSON: %v", args, err)
		}
	}
}

// ── from stdout_id_test.go ──
// The stdout=ID convention: every create command writes just the new ID on
// stdout (one line, no JSON) and a "✓ …" message on stderr. This lets scripts
// do ID=$(proton-cli foo create ...).

func assertBareID(t *testing.T, stdout, where string) string {
	t.Helper()
	id := strings.TrimSpace(stdout)
	// Exactly one non-empty line
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 1 {
		t.Fatalf("%s: expected 1 line on stdout, got %d:\n%s", where, len(lines), stdout)
	}
	if !looksLikeID(id) {
		t.Fatalf("%s: stdout is not a Proton ID: %q", where, id)
	}
	return id
}

func TestStdoutIDMailLabelCreate(t *testing.T) {
	name := testID() + "-stid-label"
	stdout, stderr := runOKStderr(t, "mail", "settings", "labels", "create",
		"--name", name, "--color", "#8080FF")
	id := assertBareID(t, stdout, "labels create")
	cleanupRun(t, fmt.Sprintf("Delete label: proton-cli mail settings labels delete -- %s", id),
		"mail", "settings", "labels", "delete", "--", id)
	if !strings.Contains(stderr, "✓") {
		t.Errorf("expected ✓ on stderr, got: %q", stderr)
	}
}

func TestStdoutIDMailFilterCreate(t *testing.T) {
	name := testID() + "-stid-filter"
	stdout, _ := runOKStderr(t, "mail", "settings", "filters", "create",
		"--name", name,
		"--sieve", `require ["fileinto"]; if header :contains "Subject" "nope-`+testID()+`" { fileinto "Archive"; }`)
	id := assertBareID(t, stdout, "filters create")
	cleanupRun(t, fmt.Sprintf("Delete filter: proton-cli mail settings filters delete -- %s", id),
		"mail", "settings", "filters", "delete", "--", id)
}

func TestStdoutIDCalendarCreate(t *testing.T) {
	name := testID() + "-stid-cal"
	stdout, _ := runOKStderr(t, "calendar", "settings", "calendars", "create",
		"--name", name, "--color", "#8080FF")
	id := assertBareID(t, stdout, "calendars create")
	cleanupRun(t, fmt.Sprintf("Delete calendar: proton-cli calendar settings calendars delete -- %s", id),
		"calendar", "settings", "calendars", "delete", "--", id)
}

func TestStdoutIDContactCreate(t *testing.T) {
	name := testID() + "-stid-contact"
	stdout, _ := runOKStderr(t, "contacts", "create",
		"--name", name, "--email", "t@x.invalid")
	id := assertBareID(t, stdout, "contacts create")
	cleanupRun(t, fmt.Sprintf("Delete contact: proton-cli contacts delete -- %s", id),
		"contacts", "delete", "--", id)
}

func TestStdoutIDVaultCreate(t *testing.T) {
	name := testID() + "-stid-vault"
	stdout, _ := runOKStderr(t, "pass", "vaults", "create", "--name", name)
	id := assertBareID(t, stdout, "vaults create")
	cleanupRun(t, fmt.Sprintf("Delete vault: proton-cli pass vaults delete -- %s", id),
		"pass", "vaults", "delete", "--", id)
}

func TestStdoutIDPassItemCreate(t *testing.T) {
	name := testID() + "-stid-item"
	stdout, _ := runOKStderr(t, "pass", "items", "create",
		"--type", "note", "--name", name, "--note", "x")
	id := assertBareID(t, stdout, "pass items create")
	_ = id
	cleanupRun(t, fmt.Sprintf("Delete pass item: proton-cli pass items delete %s", name),
		"pass", "items", "delete", name)
}

func TestStdoutIDCalendarEventCreate(t *testing.T) {
	title := testID() + "-stid-event"
	start := time.Now().Add(48 * time.Hour).Format("2006-01-02T15:04")
	stdout, _ := runOKStderr(t, "calendar", "events", "create",
		"--calendar", "Default",
		"--title", title,
		"--start", start,
		"--duration", "30m")
	_ = assertBareID(t, stdout, "events create")
	cleanupRun(t, fmt.Sprintf("Delete event by title: proton-cli calendar events delete %q", title),
		"calendar", "events", "delete", title)
}

// ── from exit_codes_test.go ──

// Exit-code scheme:
//   0 = success
//   1 = user error (bad flag, missing arg, etc.)
//   3 = not-found
//   4 = ambiguous / conflict

func TestExit0Success(t *testing.T) {
	_, _, code := run(t, "mail", "messages", "list", "--page-size", "1")
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestExit3NotFoundMail(t *testing.T) {
	_, _, code := run(t, "mail", "messages", "get", "no-such-message-"+testID())
	if code != 3 {
		t.Errorf("expected exit 3, got %d", code)
	}
}

func TestExit3NotFoundContact(t *testing.T) {
	_, _, code := run(t, "contacts", "get", "no-such-contact-"+testID())
	if code != 3 {
		t.Errorf("expected exit 3, got %d", code)
	}
}

func TestExit3NotFoundCalendarEvent(t *testing.T) {
	_, _, code := run(t, "calendar", "events", "get", "no-such-event-"+testID())
	if code != 3 {
		t.Errorf("expected exit 3, got %d", code)
	}
}

func TestExit4AmbiguousMail(t *testing.T) {
	// "a" matches many messages in any real inbox
	stdout := runOK(t, "mail", "messages", "list", "--page-size", "2")
	if stdout == "" {
		t.Skip("empty mailbox; cannot test ambiguous")
	}
	_, _, code := run(t, "mail", "messages", "get", "a")
	// 3 (no match) or 4 (ambiguous) both acceptable; we specifically want 4
	// only when there are >=2 matches. Accept either but flag unexpected codes.
	if code != 3 && code != 4 {
		t.Errorf("expected exit 3 or 4 for generic 'a' REF, got %d", code)
	}
}

func TestExit1MissingRequiredFlag(t *testing.T) {
	_, _, code := run(t, "mail", "messages", "send")
	if code == 0 {
		t.Error("expected non-zero exit for missing required --to")
	}
	if code != 1 {
		t.Errorf("expected exit 1 for user error, got %d", code)
	}
}

func TestExit1BadArgCount(t *testing.T) {
	_, _, code := run(t, "api")
	if code == 0 {
		t.Error("expected non-zero exit for missing api args")
	}
}

// ── from dry_run_test.go ──
// --dry-run must never mutate state.

func TestDryRunLabelCreate(t *testing.T) {
	name := testID() + "-dryrun"
	_, stderr := runOKStderr(t, "--dry-run", "mail", "settings", "labels", "create",
		"--name", name, "--color", "#8080FF")
	assertContains(t, stderr, "Dry run")

	list := runOK(t, "mail", "settings", "labels", "list")
	if strings.Contains(list, name) {
		t.Errorf("dry-run created a label: %q appears in list", name)
	}
}

func TestDryRunFolderCreate(t *testing.T) {
	path := "/" + testID() + "-dryrun"
	_, stderr := runOKStderr(t, "--dry-run", "drive", "folders", "create", path)
	assertContains(t, stderr, "Dry run")

	list := runOK(t, "drive", "items", "list")
	name := strings.TrimPrefix(path, "/")
	if strings.Contains(list, name) {
		t.Errorf("dry-run created a folder: %q appears in listing", name)
	}
}

func TestDryRunMailTrashBatch(t *testing.T) {
	_, stderr := runOKStderr(t, "--dry-run", "mail", "messages", "trash",
		"--unread", "--limit", "3")
	assertContains(t, stderr, "Dry run")
}

func TestDryRunPassTrashBatch(t *testing.T) {
	_, stderr, code := run(t, "--dry-run", "pass", "items", "trash", "--type", "note")
	if code != 0 {
		t.Fatalf("dry-run should succeed, got exit %d", code)
	}
	assertContains(t, stderr, "Dry run")
}

func TestDryRunContactsCreate(t *testing.T) {
	name := testID() + "-dryrun-contact"
	_, stderr := runOKStderr(t, "--dry-run", "contacts", "create",
		"--name", name, "--email", "t@x.invalid")
	assertContains(t, stderr, "Dry run")

	_, _, code := run(t, "contacts", "get", name)
	if code != 3 {
		t.Error("dry-run should not create the contact")
	}
}

// ── consent ──
//
// `run` passes the command line through untouched, so these see what a cron job
// sees: no terminal, and therefore a question that has to become an error rather
// than a wait. The helpers that demand success add --yes, which is why the guard
// needs checking on purpose here rather than incidentally everywhere else.

// A permanent deletion refuses to happen unattended, and the thing it was asked
// to delete is still there afterwards.
func TestDeleteWithoutConsentRefusesAndChangesNothing(t *testing.T) {
	name := testID() + "-consent"
	runOK(t, "mail", "settings", "labels", "create", "--name", name, "--color", "#8080FF")
	cleanupRun(t, "Delete: proton-cli mail settings labels delete "+name,
		"mail", "settings", "labels", "delete", name)

	_, stderr, code := run(t, "mail", "settings", "labels", "delete", name)
	if code != 1 {
		t.Fatalf("want exit 1 for a refused deletion, got %d (stderr: %s)", code, stderr)
	}
	assertContains(t, stderr, "cannot be undone")
	assertContains(t, stderr, "--yes")
	assertContains(t, stderr, "--dry-run")
	// The refusal names the label: a question about "1 label" is one nobody can
	// actually answer.
	assertContains(t, stderr, name)

	assertContains(t, runOK(t, "mail", "settings", "labels", "list"), name)
}

// Trashing something named by hand is not worth a question: it is reversible,
// and the user typed the reference.
func TestTrashOfANamedReferenceNeedsNoConsent(t *testing.T) {
	path := "/" + testID() + "-consent-trash"
	runOK(t, "drive", "folders", "create", path)
	cleanupRun(t, "Delete: proton-cli drive items delete "+path,
		"drive", "items", "delete", path)
	// Taken before the trash, because a trashed item has no path any more - and
	// its name arrives encrypted, so the ID is the only way back to this exact
	// folder rather than to whatever else the suite has left in there.
	linkID, _ := runJSON(t, "drive", "items", "get", path)["link_id"].(string)
	if linkID == "" {
		t.Fatal("drive items get should report the folder's link ID")
	}

	if _, stderr, code := run(t, "drive", "items", "trash", path); code != 0 {
		t.Fatalf("trashing a named path should not ask, got exit %d: %s", code, stderr)
	}

	// Put it back, so the cleanup registered above can find it by path.
	runOK(t, "drive", "trash", "restore", "--", linkID)
}

// Trashing what a filter found is, because the filter chose them and nobody has
// read the list.
func TestTrashOfAFilteredSelectionNeedsConsent(t *testing.T) {
	_, stderr, code := run(t, "mail", "messages", "trash", "--unread", "--limit", "1")
	if code == 0 {
		// Nothing matched, so there was nothing to ask about.
		assertContains(t, stderr, "Nothing to move")
		return
	}
	if code != 1 {
		t.Fatalf("want exit 1 for a refused filtered trash, got %d (stderr: %s)", code, stderr)
	}
	assertContains(t, stderr, "--yes")
	// A trash is recoverable, so the refusal must not claim otherwise.
	assertNotContains(t, stderr, "cannot be undone")
}

// --dry-run answers the question in a safer form, so it never has to ask it.
func TestDryRunNeedsNoConsent(t *testing.T) {
	_, stderr, code := run(t, "--dry-run", "mail", "messages", "delete", "--unread", "--limit", "1")
	if code != 0 {
		t.Fatalf("a dry run should never need consent, got exit %d: %s", code, stderr)
	}
	assertContains(t, stderr, "Dry run")
}
