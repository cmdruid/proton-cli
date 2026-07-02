package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runWithEnv runs the CLI with extra env vars layered on top of os.Environ().
// Returns stdout, stderr, exit code.
func runWithEnv(t *testing.T, env map[string]string, args ...string) (stdout, stderr string, exit int) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
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

// firstIDLineCell extracts the first cell of the first non-header table row.
// Returns "" when no row is found.
func firstIDLineCell(stdout string) string {
	lines := strings.Split(stdout, "\n")
	// Skip header and separator lines.
	for i := 2; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 {
			return fields[0]
		}
	}
	return ""
}

func TestShortIDDisplayInTTY(t *testing.T) {
	skipIfNoCredentials(t)
	stdout, _, code := runWithEnv(t,
		map[string]string{"PROTON_CLI_FORCE_TTY": "1"},
		"mail", "messages", "list", "--page-size", "3")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	id := firstIDLineCell(stdout)
	if len(id) != 8 {
		t.Errorf("expected 8-char ID under PROTON_CLI_FORCE_TTY, got %q (len %d)", id, len(id))
	}
}

func TestShortIDPipeFullIDs(t *testing.T) {
	skipIfNoCredentials(t)
	stdout := runOK(t, "mail", "messages", "list", "--page-size", "3")
	id := firstIDLineCell(stdout)
	if len(id) <= 8 {
		t.Errorf("expected full ID when piped, got %q (len %d)", id, len(id))
	}
	if !strings.HasSuffix(id, "==") {
		t.Errorf("piped ID should end ==, got %q", id)
	}
}

func TestShortIDFullIDsFlagOverrides(t *testing.T) {
	skipIfNoCredentials(t)
	stdout, _, code := runWithEnv(t,
		map[string]string{"PROTON_CLI_FORCE_TTY": "1"},
		"--full-ids", "mail", "messages", "list", "--page-size", "3")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	id := firstIDLineCell(stdout)
	if len(id) <= 8 {
		t.Errorf("--full-ids should keep full ID even on TTY; got %q (len %d)", id, len(id))
	}
}

func TestShortIDJSONAlwaysFull(t *testing.T) {
	skipIfNoCredentials(t)
	// Even with TTY forced.
	stdout, _, code := runWithEnv(t,
		map[string]string{"PROTON_CLI_FORCE_TTY": "1"},
		"mail", "messages", "list", "--page-size", "1", "--output", "json")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &data); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	msgs := data["messages"].([]interface{})
	if len(msgs) == 0 {
		t.Skip("inbox empty")
	}
	id := msgs[0].(map[string]interface{})["id"].(string)
	if len(id) <= 8 {
		t.Errorf("JSON should always carry full ID, got %q", id)
	}
}

// idcachePathForDefault returns the production cache file path for the
// default profile. Tests inspect this file directly to verify cache
// population and to set up ambiguous-prefix scenarios.
func idcachePathForDefault(t *testing.T) string {
	t.Helper()
	cd, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	return filepath.Join(cd, "proton-cli", "idcache", "default.json")
}

func TestShortIDCacheFilePopulated(t *testing.T) {
	skipIfNoCredentials(t)
	// Run any list command to populate the cache.
	runOK(t, "mail", "messages", "list", "--page-size", "1")

	path := idcachePathForDefault(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		t.Fatalf("cache file is not a JSON array: %v\n%s", err, data)
	}
	if len(ids) == 0 {
		t.Errorf("cache should be non-empty after list command")
	}
	for _, id := range ids {
		if !strings.HasSuffix(id, "==") {
			t.Errorf("cached ID should be a full Proton ID, got %q", id)
		}
	}
}

func TestShortIDRoundTripMail(t *testing.T) {
	skipIfNoCredentials(t)
	msgID, _, subject := plainMail(t)

	// Run a list command so the cache learns the ID.
	runOK(t, "mail", "messages", "list", "--page-size", "20")

	prefix := msgID[:8]
	stdout, stderr, code := run(t, "mail", "messages", "read", prefix)
	if code != 0 {
		t.Fatalf("read by short prefix exit %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, subject) {
		t.Errorf("read by short prefix should resolve to message containing %q; stdout:\n%s",
			subject, truncateOutput(stdout))
	}
}

func TestShortIDPrefixCacheMissOK(t *testing.T) {
	skipIfNoCredentials(t)
	// On cache miss, ResolvePrefix passes the input through unchanged
	// (so commands like `pass items list --vault Personal` work even
	// when "Personal" looks short-ID-shaped). The downstream service
	// layer's keyword-search runs against the API; if nothing matches,
	// we still get a clean exit 3, but with the service's own "no X
	// matching Y" message rather than a cache-specific hint.
	prefix := "ZZZZ____" // 4 Z + 4 underscores; very unlikely to match
	_, stderr, code := run(t, "mail", "messages", "read", prefix)
	if code == 0 {
		t.Errorf("expected non-zero exit on no-match prefix, got 0")
	}
	// The error should NOT come from ResolvePrefix (cache-specific hint)
	// any more - it should be the downstream search's not-found error.
	if strings.Contains(stderr, "run a list command") {
		t.Errorf("unexpected cache-hint error; ResolvePrefix should fall through to search:\n%s", stderr)
	}
	if !strings.Contains(stderr, "matching") && !strings.Contains(stderr, prefix) {
		t.Errorf("expected downstream not-found error mentioning the input, got: %s", stderr)
	}
}

func TestShortIDAmbiguousErrors(t *testing.T) {
	skipIfNoCredentials(t)
	// Hand-craft a cache file with two IDs that share an 8-char prefix.
	path := idcachePathForDefault(t)
	backup := path + ".bak-" + testID()
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, backup); err != nil {
			t.Fatalf("backup cache: %v", err)
		}
		t.Cleanup(func() { _ = os.Rename(backup, path) })
	} else {
		t.Cleanup(func() { _ = os.Remove(path) })
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	// 88-char IDs sharing the first 8 chars.
	pad := strings.Repeat("A", 78)
	idA := "abcd1234" + "FIRSTabc" + pad + "=="
	idB := "abcd1234" + "SECONDab" + pad + "=="
	body, _ := json.Marshal([]string{idA, idB})
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	_, stderr, code := run(t, "mail", "messages", "read", "abcd1234")
	if code != 4 {
		t.Errorf("expected exit 4 on ambiguous prefix, got %d", code)
	}
	if !strings.Contains(stderr, "ambiguous") {
		t.Errorf("expected stderr to mention 'ambiguous', got: %s", stderr)
	}
	if !strings.Contains(stderr, idA) || !strings.Contains(stderr, idB) {
		t.Errorf("expected both candidate IDs in stderr, got: %s", stderr)
	}
}

func TestShortIDRoundTripContacts(t *testing.T) {
	skipIfNoCredentials(t)
	name := testID() + "-shortid-contact"
	stdout := runOK(t, "contacts", "create",
		"--name", name, "--email", "t+"+name+"@x.invalid")
	id := strings.TrimSpace(stdout)
	cleanupRun(t, "Delete contact: proton-cli contacts delete -- "+id,
		"contacts", "delete", "--", id)

	// Populate cache.
	runOK(t, "contacts", "list")

	// `--` guards against the ~1.5% of IDs whose 8-char prefix starts with '-'
	// (which cobra would otherwise read as a flag).
	prefix := id[:8]
	got := runOK(t, "contacts", "get", "--", prefix)
	if !strings.Contains(got, name) {
		t.Errorf("contacts get by short prefix should resolve; stdout:\n%s", got)
	}
}

func TestShortIDRoundTripPass(t *testing.T) {
	skipIfNoCredentials(t)
	name := testID() + "-shortid-pass"
	stdout := runOK(t, "pass", "items", "create",
		"--type", "note", "--name", name, "--note", "x")
	itemID := strings.TrimSpace(stdout)
	cleanupRun(t, "Delete pass item: proton-cli pass items delete "+name,
		"pass", "items", "delete", name)

	// Populate cache.
	runOK(t, "pass", "items", "list")

	// Get a vault SHARE_ID for the 2-arg path.
	vaults := runJSONArray(t, "pass", "vaults", "list")
	if len(vaults) == 0 {
		t.Skip("no vaults")
	}
	shareID := vaults[0].(map[string]interface{})["share_id"].(string)

	got := runOK(t, "pass", "items", "get", shareID[:8], itemID[:8])
	if !strings.Contains(got, name) {
		t.Errorf("pass items get by short prefixes should resolve; stdout:\n%s", got)
	}
}
