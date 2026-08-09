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
	"slices"
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
	requireCredentials()

	dir, err := os.MkdirTemp("", "proton-cli-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	workDir = dir
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

	writePasswordFiles()
	seed()

	code := m.Run()

	// Shared fixtures outlive individual tests, so their teardown runs here
	// while binaryPath is still valid. os.Exit skips deferred funcs, so the
	// temp-dir removal is explicit too.
	flushSuiteCleanup()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// ── The two accounts ──

// The suite creates, mutates and deletes real data, so it runs on accounts kept
// for that and nothing else. Most tests act as `primary`; the handful that need
// two Proton users bring in `secondary`.
//
// These are the harness's own variables, not the CLI's: proton-cli takes an
// account from a signed-in profile, which signIn establishes below. The
// PROTON_CLI_TEST_ prefix keeps them clear of anything the binary reads.
const (
	primary   = "primary"
	secondary = "secondary"
)

// workDir is the per-run temp directory, where the password files live.
var workDir string

type testAccount struct {
	profile      string
	userVar      string
	passwordVar  string
	passwordFile string
}

var accounts = map[string]*testAccount{
	primary: {
		profile:     primary,
		userVar:     "PROTON_CLI_TEST_PRIMARY_USER",
		passwordVar: "PROTON_CLI_TEST_PRIMARY_PASSWORD",
	},
	secondary: {
		profile:     secondary,
		userVar:     "PROTON_CLI_TEST_SECONDARY_USER",
		passwordVar: "PROTON_CLI_TEST_SECONDARY_PASSWORD",
	},
}

// requireCredentials verifies both accounts are configured before any test runs,
// exiting instantly - ahead of the expensive binary build - if either is
// incomplete.
func requireCredentials() {
	var missing []string
	for _, name := range []string{primary, secondary} {
		a := accounts[name]
		for _, v := range []string{a.userVar, a.passwordVar} {
			if os.Getenv(v) == "" {
				missing = append(missing, v)
			}
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "integration tests require these env vars: %s\n", strings.Join(missing, ", "))
		os.Exit(1)
	}
}

// writePasswordFiles puts each account's password where runAs can hand it to
// the one command Proton guards behind an elevated session.
//
// A session cannot carry elevation: Proton re-authenticates over SRP, and the
// key blob sealed at login is a one-way derivation of the password rather than
// the password itself.
func writePasswordFiles() {
	for _, name := range []string{primary, secondary} {
		a := accounts[name]
		a.passwordFile = filepath.Join(workDir, a.profile+".password")
		if err := os.WriteFile(a.passwordFile, []byte(os.Getenv(a.passwordVar)), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write the %s password file: %v\n", a.profile, err)
			os.Exit(1)
		}
	}
}

// seed signs both accounts in and brings them to the state the fixture declares.
//
// Whole assertions here are guarded by a skip when a collection is empty - no
// contacts, no vaults, no calendars - which is what that data is for. Each datum
// is judged before it is touched, so an account already in shape costs reads.
func seed() {
	cmd := exec.Command("go", "run", "./scripts/seed")
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "PROTON_CLI="+binaryPath)
	out, err := cmd.CombinedOutput()
	fmt.Fprint(os.Stderr, string(out))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to seed the accounts: %v\n", err)
		os.Exit(1)
	}
}

// ── Running the binary ──

// run executes the CLI as the primary account and returns stdout, stderr, exit
// code.
func run(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	return runWithStdin(t, nil, args...)
}

// runArgs executes the CLI as the primary account without a *testing.T, so
// suite-level fixtures and cleanup (which run outside a test) can invoke it.
func runArgs(stdin io.Reader, args ...string) (stdout, stderr string, exitCode int, err error) {
	return runAs(primary, stdin, args...)
}

// runAs executes the CLI as one of the two accounts. It returns stdout, stderr,
// the exit code, and a non-nil error only when the process failed to start.
//
// The child environment is built from an allowlist rather than inherited. This
// is the one place a target account is chosen, so it is the one place the choice
// can be enforced: whatever a developer happens to have exported, the binary
// under test sees a stated environment and can act only as the profile named
// here.
//
// The arguments are otherwise passed through untouched. Consent is added by the
// helpers that demand success, never here: a runner that quietly agreed to
// everything would make `run` unable to observe a command being refused, which
// is the one thing the tests about refusal have to see.
func runAs(profile string, stdin io.Reader, args ...string) (stdout, stderr string, exitCode int, err error) {
	a, ok := accounts[profile]
	if !ok {
		return "", "", -1, fmt.Errorf("unknown test profile %q", profile)
	}
	args = withPassword(a, args)

	var outBuf, errBuf bytes.Buffer
	cmd := exec.Command(binaryPath, args...)
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	cmd.Stdin = stdin
	cmd.Env = childEnv(profile)
	if runErr := cmd.Run(); runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			return outBuf.String(), errBuf.String(), exitErr.ExitCode(), nil
		}
		return outBuf.String(), errBuf.String(), -1, runErr
	}
	return outBuf.String(), errBuf.String(), 0, nil
}

// reauthCommands are the commands Proton may ask to re-authenticate, and so the
// ones that carry the credential flags. It mirrors the set the CLI declares,
// which internal/cli/conformance_test.go pins.
var reauthCommands = [][]string{
	{"calendar", "settings", "calendars", "delete"},
	{"mail", "settings", "autoreply", "set"},
}

// withPassword hands such a command the profile's password file.
//
// It goes directly after the command's own words: the flag belongs to that
// command rather than to the root, so it is unknown before the subcommand, and
// anything after the `--` the binary inserts ahead of a leading-dash ID would be
// read as an argument.
func withPassword(a *testAccount, args []string) []string {
	if a.passwordFile == "" {
		return args
	}
	for _, cmd := range reauthCommands {
		at := indexOfRun(args, cmd...)
		if at < 0 {
			continue
		}
		out := make([]string, 0, len(args)+2)
		out = append(out, args[:at+len(cmd)]...)
		out = append(out, "--password-file", a.passwordFile)
		return append(out, args[at+len(cmd):]...)
	}
	return args
}

// indexOfRun reports where args holds the words in order and adjacent, or -1.
// That is how a command is recognised once the helpers have put their own flags
// in front of it.
func indexOfRun(args []string, run ...string) int {
	for i := 0; i+len(run) <= len(args); i++ {
		if slices.Equal(args[i:i+len(run)], run) {
			return i
		}
	}
	return -1
}

// childEnv is the whole environment the binary under test runs in: what it needs
// to find its toolchain, its home and its session store, plus the profile to act
// as. Nothing else is carried over.
func childEnv(profile string) []string {
	env := []string{
		"PROTON_PROFILE=" + profile,
		// There is no terminal here, so a missing credential should be an error
		// rather than a question asked of nobody.
		"PROTON_NO_INPUT=1",
	}
	for _, k := range []string{
		"PATH", "HOME", "TMPDIR", "USER", "LANG", "TZ",
		"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_RUNTIME_DIR",
		"DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY", "DBUS_SESSION_BUS_ADDRESS",
	} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// withEnv overrides entries in a child environment by name, so a caller never
// has to reason about which of two settings of the same variable wins.
func withEnv(env []string, overrides map[string]string) []string {
	out := make([]string, 0, len(env)+len(overrides))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if _, replaced := overrides[name]; !replaced {
			out = append(out, kv)
		}
	}
	for k, v := range overrides {
		out = append(out, k+"="+v)
	}
	return out
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

// runWithEnv runs the CLI with extra env vars layered on top of os.Environ().
// Returns stdout, stderr, exit code.
func runWithEnv(t *testing.T, env map[string]string, args ...string) (stdout, stderr string, exit int) {
	t.Helper()
	cmd := exec.Command(binaryPath, withPassword(accounts[primary], args)...)
	cmd.Env = withEnv(childEnv(primary), env)
	var outB, errB strings.Builder
	cmd.Stdout = &outB
	cmd.Stderr = &errB
	err := cmd.Run()
	exit = 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		} else {
			t.Fatalf("run %v: %v", args, err)
		}
	}
	return outB.String(), errB.String(), exit
}

// consenting puts --yes ahead of everything.
//
// The suite has no terminal, so anything that stops to ask before removing
// something would be refused here with nobody to answer. Every helper that
// demands success is doing setup or clearing up after itself, and means yes.
// `run` is deliberately left without it, so a test can still watch a command
// decline to act.
//
// The flag leads for the same reason --output json does: a Proton ID may begin
// with a dash, and anything after the `--` the binary then inserts would be read
// as an argument.
func consenting(args []string) []string {
	return append([]string{"--yes"}, args...)
}

// runOK runs a command and fails the test on non-zero exit.
func runOK(t *testing.T, args ...string) string {
	t.Helper()
	stdout, stderr, code := run(t, consenting(args)...)
	if code != 0 {
		t.Fatalf("command %v failed (exit %d):\nstdout: %s\nstderr: %s",
			args, code, truncateOutput(stdout), truncateOutput(stderr))
	}
	return stdout
}

// runOKStderr runs a command and returns both stdout + stderr on success.
func runOKStderr(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()
	stdout, stderr, code := run(t, consenting(args)...)
	if code != 0 {
		t.Fatalf("command %v failed (exit %d):\nstdout: %s\nstderr: %s",
			args, code, truncateOutput(stdout), truncateOutput(stderr))
	}
	return stdout, stderr
}

// asJSON puts `--output json` ahead of everything.
//
// It has to lead rather than trail: a Proton ID may begin with '-', which the
// binary auto-protects with a `--`, and any flag after that becomes a positional
// argument. Appending the flag would therefore break roughly one call in sixty,
// depending on which ID the account happened to hand out.
func asJSON(args []string) []string {
	return append([]string{"--output", "json"}, args...)
}

// runJSON runs with `--output json` and parses stdout as a JSON object.
func runJSON(t *testing.T, args ...string) map[string]interface{} {
	t.Helper()
	return parseJSONObject(t, runOK(t, asJSON(args)...))
}

func parseJSONObject(t *testing.T, stdout string) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse JSON object: %v\nraw: %s", err, truncateOutput(stdout))
	}
	return result
}

// runJSONArray runs with `--output json` and parses stdout as a JSON array.
// runJSONArray returns the rows of a collection.
//
// Every list is an envelope keyed by its plural noun - {"messages": [...],
// "count": 3} - so this unwraps whichever array it finds rather than making every
// caller know the noun. The count is checked against it, since the two disagreeing
// would be a bug in the envelope itself.
func runJSONArray(t *testing.T, args ...string) []interface{} {
	t.Helper()
	return parseJSONArray(t, runOK(t, asJSON(args)...))
}

func parseJSONArray(t *testing.T, stdout string) []interface{} {
	t.Helper()
	var env map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("collection output is not an envelope: %v\nraw: %s", err, truncateOutput(stdout))
	}
	var rows []interface{}
	found := ""
	for key, value := range env {
		if arr, ok := value.([]interface{}); ok {
			if found != "" {
				t.Fatalf("envelope has two arrays (%q and %q): %s", found, key, truncateOutput(stdout))
			}
			rows, found = arr, key
		}
	}
	if found == "" {
		t.Fatalf("envelope has no array of rows: %s", truncateOutput(stdout))
	}
	if count, ok := env["count"].(float64); !ok {
		t.Errorf("envelope has no count: %s", truncateOutput(stdout))
	} else if int(count) != len(rows) {
		t.Errorf("count is %d but %q has %d rows", int(count), found, len(rows))
	}
	return rows
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
// A cleanup's job is that nothing is left behind, so finding the thing already
// gone - exit 3 - is the job done. A test whose subject is deletion would
// otherwise raise the alarm every time it worked.
func cleanupRun(t *testing.T, description string, args ...string) {
	t.Helper()
	cleanup(t, description, func() error {
		_, stderr, code := run(t, consenting(args)...)
		if code != 0 && code != 3 {
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

// selfEmail returns the primary account's address.
func selfEmail() string { return os.Getenv(accounts[primary].userVar) }

// secondaryEmail returns the second account's address.
func secondaryEmail() string { return os.Getenv(accounts[secondary].userVar) }

// The secondary-account runners. A scenario needs one whenever it genuinely
// takes two Proton users: accepting a share invitation, receiving mail, or
// organizing an invite the primary RSVPs to.
//
// Run order matters - the primary invites or sends, the secondary accepts or
// receives - and a mutation made as one account registers its cleanup as the
// same one.

func runSecondary(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	stdout, stderr, exitCode, err := runAs(secondary, nil, args...)
	if err != nil {
		t.Fatalf("failed to run command %v as the secondary account: %v", args, err)
	}
	return stdout, stderr, exitCode
}

func runOKSecondary(t *testing.T, args ...string) string {
	t.Helper()
	stdout, stderr, code := runSecondary(t, consenting(args)...)
	if code != 0 {
		t.Fatalf("command %v failed as the secondary account (exit %d):\nstdout: %s\nstderr: %s",
			args, code, truncateOutput(stdout), truncateOutput(stderr))
	}
	return stdout
}

func runJSONSecondary(t *testing.T, args ...string) map[string]interface{} {
	t.Helper()
	return parseJSONObject(t, runOKSecondary(t, asJSON(args)...))
}

func runJSONArraySecondary(t *testing.T, args ...string) []interface{} {
	t.Helper()
	return parseJSONArray(t, runOKSecondary(t, asJSON(args)...))
}

// cleanupRunSecondary is cleanupRun for something the secondary account owns.
func cleanupRunSecondary(t *testing.T, description string, args ...string) {
	t.Helper()
	cleanup(t, description, func() error {
		_, stderr, code := runSecondary(t, consenting(args)...)
		if code != 0 && code != 3 {
			return fmt.Errorf("exit %d: %s", code, strings.TrimSpace(stderr))
		}
		return nil
	})
}

// externalRecipient is a non-Proton mailbox, for tests that must deliver outside
// Proton. Sending to a fake @example.com address instead bounces (nullMX),
// littering the inbox with MAILER-DAEMON returns, so a test that needs one skips
// when none is configured.
func externalRecipient(t *testing.T) string {
	t.Helper()
	v := os.Getenv("PROTON_CLI_TEST_EXTERNAL_RECIPIENT")
	if v == "" {
		t.Skip("PROTON_CLI_TEST_EXTERNAL_RECIPIENT is not set")
	}
	return v
}

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
		if _, stderr, code, err := runArgs(nil, consenting(args)...); err != nil || code != 0 {
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
		out, _, code, e := runArgs(nil, "mail", "messages", "attachments", "list", f.msgID, "--output", "json")
		if e != nil || code != 0 {
			f.err = fmt.Errorf("list shared attachment failed (exit %d): %v", code, e)
			return
		}
		var env struct {
			Attachments []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"attachments"`
		}
		if json.Unmarshal([]byte(out), &env) != nil || len(env.Attachments) == 0 {
			f.err = fmt.Errorf("shared attachment not found after delivery")
			return
		}
		f.attID = env.Attachments[0].ID
		f.attName = env.Attachments[0].Name
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

// ── the runner is the only way in ──

// TestEveryInvocationGoesThroughTheRunner keeps the binary from being spawned
// anywhere but here.
//
// runAs is the one place that chooses which account a command acts as and builds
// the environment it runs in. A test that starts the process itself inherits
// whatever the developer has exported and acts as whatever profile that names -
// which is how a stdin upload once landed in a personal Drive instead of the
// primary account's, and reported an empty folder rather than a failure.
func TestEveryInvocationGoesThroughTheRunner(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the test directory: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") || e.Name() == "integration_test.go" {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if strings.Contains(string(src), "exec.Command(binaryPath") {
			t.Errorf("%s starts the binary itself; go through run, runOK or runAs so the account is chosen in one place", e.Name())
		}
	}
}
