package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var binaryPath string

// TestMain builds the CLI binary once before any integration test
// runs. The binary is built release-shaped (with -tags=embed_hv) so
// any test that triggers Proton's CAPTCHA HV flow can solve it via
// the embedded webview helper instead of skipping. This requires
// libwebkit2gtk-4.1-dev + pkg-config in the environment, which
// `devbox shell` provides.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "proton-cli-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	// Build the per-platform proton-cli-hv helper into
	// internal/hv/assets/ so the main build's //go:embed picks it up.
	helpers := exec.Command("bash", "../scripts/build-hv-helpers.sh")
	helpers.Stderr = os.Stderr
	if err := helpers.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build hv helper: %v\n", err)
		os.Exit(1)
	}

	binaryPath = filepath.Join(dir, "proton-cli")
	cmd := exec.Command("go", "build", "-tags=embed_hv", "-o", binaryPath, "..")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(dir)
		fmt.Fprintf(os.Stderr, "failed to build binary: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	// Shared fixtures outlive individual tests, so their teardown runs here
	// while binaryPath is still valid. os.Exit skips deferred funcs, so the
	// temp-dir removal is explicit too.
	flushSuiteCleanup()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// ── Credential gate ──

func skipIfNoCredentials(t *testing.T) {
	t.Helper()
	if os.Getenv("PROTON_USER") == "" || os.Getenv("PROTON_PASSWORD") == "" {
		t.Skip("PROTON_USER and PROTON_PASSWORD not set")
	}
}

// ── Running the binary ──

// run executes the CLI with args and returns stdout, stderr, exit code.
func run(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	return runWithStdin(t, nil, args...)
}

// runArgs executes the CLI without a *testing.T, so suite-level fixtures and
// cleanup (which run outside a test) can invoke it. It returns stdout, stderr,
// the exit code, and a non-nil error only when the process failed to start.
func runArgs(stdin io.Reader, args ...string) (stdout, stderr string, exitCode int, err error) {
	var outBuf, errBuf bytes.Buffer
	cmd := exec.Command(binaryPath, args...)
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	cmd.Stdin = stdin
	cmd.Env = os.Environ()
	if runErr := cmd.Run(); runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			return outBuf.String(), errBuf.String(), exitErr.ExitCode(), nil
		}
		return outBuf.String(), errBuf.String(), -1, runErr
	}
	return outBuf.String(), errBuf.String(), 0, nil
}

// runWithStdin is run() with arbitrary stdin bytes attached.
func runWithStdin(t *testing.T, stdin io.Reader, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	stdout, stderr, exitCode, err := runArgs(stdin, args...)
	if err != nil {
		t.Fatalf("failed to run command %v: %v", args, err)
	}
	return stdout, stderr, exitCode
}

// runOK runs a command and fails the test on non-zero exit.
func runOK(t *testing.T, args ...string) string {
	t.Helper()
	stdout, stderr, code := run(t, args...)
	if code != 0 {
		t.Fatalf("command %v failed (exit %d):\nstdout: %s\nstderr: %s",
			args, code, truncateOutput(stdout), truncateOutput(stderr))
	}
	return stdout
}

// runOKStderr runs a command and returns both stdout + stderr on success.
func runOKStderr(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()
	stdout, stderr, code := run(t, args...)
	if code != 0 {
		t.Fatalf("command %v failed (exit %d):\nstdout: %s\nstderr: %s",
			args, code, truncateOutput(stdout), truncateOutput(stderr))
	}
	return stdout, stderr
}

// runJSON runs with `--output json` and parses stdout as a JSON object.
func runJSON(t *testing.T, args ...string) map[string]interface{} {
	t.Helper()
	stdout := runOK(t, append(args, "--output", "json")...)
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse JSON object: %v\nraw: %s", err, truncateOutput(stdout))
	}
	return result
}

// runJSONArray runs with `--output json` and parses stdout as a JSON array.
func runJSONArray(t *testing.T, args ...string) []interface{} {
	t.Helper()
	stdout := runOK(t, append(args, "--output", "json")...)
	var result []interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse JSON array: %v\nraw: %s", err, truncateOutput(stdout))
	}
	return result
}

// ── Naming ──

// testID returns a unique prefix for artifacts. Also usable as part of a name.
func testID() string {
	return fmt.Sprintf("proton-cli-test-%d-%d", time.Now().UnixMilli(), rand.Intn(10000))
}

// ── Assertions ──

func assertContains(t *testing.T, stdout, substr string) {
	t.Helper()
	if !strings.Contains(stdout, substr) {
		t.Errorf("expected output to contain %q, got:\n%s", substr, truncateOutput(stdout))
	}
}

func assertNotContains(t *testing.T, stdout, substr string) {
	t.Helper()
	if strings.Contains(stdout, substr) {
		t.Errorf("expected output NOT to contain %q, got:\n%s", substr, truncateOutput(stdout))
	}
}

// assertField checks that "Key: Value" line exists.
func assertField(t *testing.T, stdout, field, expected string) {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, field) {
			value := strings.TrimSpace(strings.TrimPrefix(line, field))
			if value == expected {
				return
			}
			t.Errorf("field %s: got %q, want %q", field, value, expected)
			return
		}
	}
	t.Errorf("field %s not found in:\n%s", field, truncateOutput(stdout))
}

// ── Cleanup ──

// cleanup registers a cleanup fn that logs loudly on failure.
func cleanup(t *testing.T, description string, fn func() error) {
	t.Helper()
	t.Cleanup(func() {
		if err := fn(); err != nil {
			t.Logf("\n"+
				"╔══════════════════════════════════════════════════════════════╗\n"+
				"║  ⚠️  CLEANUP FAILED - MANUAL ACTION REQUIRED                ║\n"+
				"╠══════════════════════════════════════════════════════════════╣\n"+
				"║  %s\n"+
				"║  Error: %s\n"+
				"╚══════════════════════════════════════════════════════════════╝",
				description, err)
		}
	})
}

// cleanupRun registers a cleanup that invokes the CLI.
func cleanupRun(t *testing.T, description string, args ...string) {
	t.Helper()
	cleanup(t, description, func() error {
		_, stderr, code := run(t, args...)
		if code != 0 {
			return fmt.Errorf("exit %d: %s", code, strings.TrimSpace(stderr))
		}
		return nil
	})
}

// ── Convenience ──

func truncateOutput(s string) string {
	if len(s) > 500 {
		return s[:500] + "...(truncated)"
	}
	return s
}

// selfEmail returns PROTON_USER.
func selfEmail() string { return os.Getenv("PROTON_USER") }

// externalRecipient is the non-Proton (GMX) alt address, used by tests that
// must deliver to a real external mailbox (see tests/AGENTS.md "Test Alt
// Accounts"). Sending to a fake @example.com address instead bounces
// (nullMX), littering the inbox with MAILER-DAEMON returns.
const externalRecipient = "rl00@gmx.at"

// ── mail delivery + polling ──

// waitFor polls check every interval until it returns true or timeout elapses.
// It checks immediately (before the first sleep), so an already-true condition
// costs nothing. Returns whether check ultimately succeeded.
func waitFor(timeout, interval time.Duration, check func() bool) bool {
	deadline := time.Now().Add(timeout)
	for {
		if check() {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(interval)
	}
}

// messageIDInFolder returns the ID of the first message in folder whose subject
// matches exactly, or "" if none. t-free so fixtures/helpers can call it.
func messageIDInFolder(folder, subject string) string {
	stdout, _, code, err := runArgs(nil, "mail", "messages", "list",
		"--folder", folder, "--page-size", "20", "--output", "json")
	if err != nil || code != 0 {
		return ""
	}
	var data struct {
		Messages []struct {
			ID      string `json:"id"`
			Subject string `json:"subject"`
		} `json:"messages"`
	}
	if json.Unmarshal([]byte(stdout), &data) != nil {
		return ""
	}
	for _, m := range data.Messages {
		if m.Subject == subject {
			return m.ID
		}
	}
	return ""
}

// conversationIDOf returns a message's ConversationID, or "" on failure. t-free.
func conversationIDOf(msgID string) string {
	stdout, _, code, err := runArgs(nil, "api", "GET", "/mail/v4/messages/"+msgID)
	if err != nil || code != 0 {
		return ""
	}
	var v struct {
		Message struct{ ConversationID string }
	}
	if json.Unmarshal([]byte(stdout), &v) != nil {
		return ""
	}
	return v.Message.ConversationID
}

// sendSelfMail sends body to self with the given subject and waits for
// delivery, polling sent then inbox. Either returned ID may be "" if it never
// appeared. t-free: callers decide how to report failure and register cleanup.
func sendSelfMail(subject, body string) (sentID, inboxID string, err error) {
	if _, stderr, code, e := runArgs(nil, "mail", "messages", "send",
		"--to", selfEmail(), "--subject", subject, "--body", body); e != nil || code != 0 {
		return "", "", fmt.Errorf("send %q failed (exit %d): %v %s", subject, code, e, strings.TrimSpace(stderr))
	}
	waitFor(15*time.Second, 500*time.Millisecond, func() bool {
		sentID = messageIDInFolder("sent", subject)
		return sentID != ""
	})
	waitFor(25*time.Second, 750*time.Millisecond, func() bool {
		inboxID = messageIDInFolder("inbox", subject)
		return inboxID != ""
	})
	return sentID, inboxID, nil
}

// sendTestMail sends a mail to self, waits for delivery, registers per-test
// cleanup for the sent and inbox copies, and returns the inbox message ID
// (falling back to the sent ID if the inbox copy never appeared).
//
// Use it only when a test MUTATES its message (mark/star/move/trash) or exercises
// the send path itself. Read-only tests should use a shared fixture (plainMail,
// quotedMail, sharedAttachment) instead of sending their own.
func sendTestMail(t *testing.T, subject string) string {
	t.Helper()
	sentID, inboxID, err := sendSelfMail(subject, "Integration test body: "+subject)
	if err != nil {
		t.Fatal(err)
	}
	if sentID != "" {
		cleanupRun(t, "Delete sent mail: proton-cli mail messages delete -- "+sentID,
			"mail", "messages", "delete", "--", sentID)
	}
	if inboxID != "" {
		cleanupRun(t, "Delete inbox mail: proton-cli mail messages delete -- "+inboxID,
			"mail", "messages", "delete", "--", inboxID)
		return inboxID
	}
	if sentID == "" {
		t.Fatalf("mail %q was not delivered", subject)
	}
	return sentID
}

// ── suite-scoped cleanup ──
//
// Shared fixtures outlive any single test, so their teardown is registered here
// and flushed from TestMain after m.Run() (while binaryPath is still valid). A
// failed deletion prints the same loud, copy-pasteable box the per-test cleanup
// uses, so nothing silently leaks.

var (
	suiteCleanupMu  sync.Mutex
	suiteCleanupFns []func()
)

// registerSuiteCleanup queues a CLI command to run once, at suite teardown.
func registerSuiteCleanup(description string, args ...string) {
	suiteCleanupMu.Lock()
	defer suiteCleanupMu.Unlock()
	suiteCleanupFns = append(suiteCleanupFns, func() {
		if _, stderr, code, err := runArgs(nil, args...); err != nil || code != 0 {
			fmt.Fprintf(os.Stderr, "\n"+
				"╔══════════════════════════════════════════════════════════════╗\n"+
				"║  ⚠️  CLEANUP FAILED - MANUAL ACTION REQUIRED                ║\n"+
				"╠══════════════════════════════════════════════════════════════╣\n"+
				"║  %s\n"+
				"║  Error: exit %d: %v %s\n"+
				"╚══════════════════════════════════════════════════════════════╝\n",
				description, code, err, strings.TrimSpace(stderr))
		}
	})
}

// flushSuiteCleanup runs the queued teardowns in reverse registration order.
func flushSuiteCleanup() {
	suiteCleanupMu.Lock()
	defer suiteCleanupMu.Unlock()
	for i := len(suiteCleanupFns) - 1; i >= 0; i-- {
		suiteCleanupFns[i]()
	}
}

// ── shared mail fixtures ──
//
// Read-only mail tests share a handful of delivered messages created once per
// suite (guarded by sync.Once) instead of each sending and polling. This is the
// single biggest speedup lever: it collapses ~25 send+deliver waits into ~3.
// Mutating tests still send their own via sendTestMail.

type sharedMail struct {
	once    sync.Once
	msgID   string // inbox copy (falls back to sent)
	convID  string
	subject string
	err     error
}

func (f *sharedMail) ensure(bodyFor func(subject string) string) {
	f.once.Do(func() {
		f.subject = testID() + "-shared"
		sentID, inboxID, err := sendSelfMail(f.subject, bodyFor(f.subject))
		if err != nil {
			f.err = err
			return
		}
		f.msgID = inboxID
		if f.msgID == "" {
			f.msgID = sentID
		}
		if f.msgID == "" {
			f.err = fmt.Errorf("shared mail %q was not delivered", f.subject)
			return
		}
		if sentID != "" {
			registerSuiteCleanup("Delete shared sent mail: proton-cli mail messages delete "+sentID,
				"mail", "messages", "delete", "--", sentID)
		}
		if inboxID != "" && inboxID != sentID {
			registerSuiteCleanup("Delete shared inbox mail: proton-cli mail messages delete "+inboxID,
				"mail", "messages", "delete", "--", inboxID)
		}
		f.convID = conversationIDOf(f.msgID)
	})
}

var (
	plainMailFixture  sharedMail
	quotedMailFixture sharedMail
)

// plainMail returns a shared, delivered self-mail with a plain body (no quote
// markers, no attachments) plus its conversation ID and subject. Read-only.
func plainMail(t *testing.T) (msgID, convID, subject string) {
	t.Helper()
	plainMailFixture.ensure(func(s string) string { return "Integration test body: " + s })
	if plainMailFixture.err != nil {
		t.Fatalf("shared plain mail: %v", plainMailFixture.err)
	}
	return plainMailFixture.msgID, plainMailFixture.convID, plainMailFixture.subject
}

// quotedMail returns a shared, delivered self-mail whose body carries the
// canonical "On <date>, <name> <addr> wrote:" reply block. Read-only.
func quotedMail(t *testing.T) (msgID, subject string) {
	t.Helper()
	quotedMailFixture.ensure(func(s string) string {
		return "My new note for " + s + ".\n\nOn Tue, 24 Sep 2024, Sender <a@b.com> wrote:\n\n> ancient quoted text\n> that should disappear\n"
	})
	if quotedMailFixture.err != nil {
		t.Fatalf("shared quoted mail: %v", quotedMailFixture.err)
	}
	return quotedMailFixture.msgID, quotedMailFixture.subject
}

// ── shared attachment fixture ──

type sharedAttachMail struct {
	once    sync.Once
	msgID   string
	attID   string
	attName string
	err     error
}

var attachMailFixture sharedAttachMail

func (f *sharedAttachMail) ensure() {
	f.once.Do(func() {
		subject := testID() + "-shared-attach"
		dir, err := os.MkdirTemp("", "pcli-fixture-*")
		if err != nil {
			f.err = err
			return
		}
		defer func() { _ = os.RemoveAll(dir) }()
		path := filepath.Join(dir, "note.txt")
		if err := os.WriteFile(path, []byte("shared attachment body for "+subject), 0644); err != nil {
			f.err = err
			return
		}
		if _, stderr, code, e := runArgs(nil, "mail", "messages", "send",
			"--to", selfEmail(), "--subject", subject, "--body", "see attached",
			"--attach", path); e != nil || code != 0 {
			f.err = fmt.Errorf("send attachment mail failed (exit %d): %v %s", code, e, strings.TrimSpace(stderr))
			return
		}
		var inboxID string
		waitFor(25*time.Second, 750*time.Millisecond, func() bool {
			inboxID = messageIDInFolder("inbox", subject)
			return inboxID != ""
		})
		sentID := messageIDInFolder("sent", subject)
		if sentID != "" {
			registerSuiteCleanup("Delete shared attach sent mail: proton-cli mail messages delete "+sentID,
				"mail", "messages", "delete", "--", sentID)
		}
		if inboxID != "" && inboxID != sentID {
			registerSuiteCleanup("Delete shared attach inbox mail: proton-cli mail messages delete "+inboxID,
				"mail", "messages", "delete", "--", inboxID)
		}
		f.msgID = inboxID
		if f.msgID == "" {
			f.msgID = sentID
		}
		if f.msgID == "" {
			f.err = fmt.Errorf("shared attachment mail was not delivered")
			return
		}
		out, _, code, e := runArgs(nil, "mail", "attachments", "list", f.msgID, "--output", "json")
		if e != nil || code != 0 {
			f.err = fmt.Errorf("list shared attachment failed (exit %d): %v", code, e)
			return
		}
		var atts []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if json.Unmarshal([]byte(out), &atts) != nil || len(atts) == 0 {
			f.err = fmt.Errorf("shared attachment not found after delivery")
			return
		}
		f.attID = atts[0].ID
		f.attName = atts[0].Name
	})
}

// sharedAttachment returns a shared, delivered self-mail carrying one
// (non-inline) attachment, plus that attachment's ID and name. Read-only.
func sharedAttachment(t *testing.T) (msgID, attID, attName string) {
	t.Helper()
	attachMailFixture.ensure()
	if attachMailFixture.err != nil {
		t.Fatalf("shared attachment mail: %v", attachMailFixture.err)
	}
	return attachMailFixture.msgID, attachMailFixture.attID, attachMailFixture.attName
}

// ── shared mixed-disposition fixture ──
//
// A delivered self-mail carrying BOTH an inline image (embedded via Content-ID)
// and a regular attachment, so the inline-filter tests have a real message to
// assert against instead of skipping. Created via `mail messages send --html
// --attach ... --attach-inline ...`.

type sharedMixedMail struct {
	once  sync.Once
	msgID string
	err   error
}

var mixedMailFixture sharedMixedMail

func (f *sharedMixedMail) ensure() {
	f.once.Do(func() {
		subject := testID() + "-shared-mixed"
		dir, err := os.MkdirTemp("", "pcli-fixture-*")
		if err != nil {
			f.err = err
			return
		}
		defer func() { _ = os.RemoveAll(dir) }()
		reg := filepath.Join(dir, "note.txt")
		img := filepath.Join(dir, "pixel.png")
		if err := os.WriteFile(reg, []byte("regular attachment for "+subject), 0644); err != nil {
			f.err = err
			return
		}
		if err := os.WriteFile(img, tinyPNG(), 0644); err != nil {
			f.err = err
			return
		}
		if _, stderr, code, e := runArgs(nil, "mail", "messages", "send",
			"--to", selfEmail(), "--subject", subject, "--html", "--body", "<p>mixed</p>",
			"--attach", reg, "--attach-inline", img); e != nil || code != 0 {
			f.err = fmt.Errorf("send mixed mail failed (exit %d): %v %s", code, e, strings.TrimSpace(stderr))
			return
		}
		var inboxID string
		waitFor(25*time.Second, 750*time.Millisecond, func() bool {
			inboxID = messageIDInFolder("inbox", subject)
			return inboxID != ""
		})
		sentID := messageIDInFolder("sent", subject)
		if sentID != "" {
			registerSuiteCleanup("Delete shared mixed sent mail: proton-cli mail messages delete "+sentID,
				"mail", "messages", "delete", "--", sentID)
		}
		if inboxID != "" && inboxID != sentID {
			registerSuiteCleanup("Delete shared mixed inbox mail: proton-cli mail messages delete "+inboxID,
				"mail", "messages", "delete", "--", inboxID)
		}
		f.msgID = inboxID
		if f.msgID == "" {
			f.msgID = sentID
		}
		if f.msgID == "" {
			f.err = fmt.Errorf("shared mixed mail was not delivered")
		}
	})
}

// sharedMixedAttachment returns a shared, delivered self-mail with one inline
// and one regular attachment. Read-only.
func sharedMixedAttachment(t *testing.T) string {
	t.Helper()
	mixedMailFixture.ensure()
	if mixedMailFixture.err != nil {
		t.Fatalf("shared mixed-attachment mail: %v", mixedMailFixture.err)
	}
	return mixedMailFixture.msgID
}

// tinyPNG returns the bytes of a 1x1 PNG, used for inline-image fixtures.
func tinyPNG() []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1, 1)))
	return b.Bytes()
}
